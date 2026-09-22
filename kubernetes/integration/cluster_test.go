//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestNativeSynchronizationAndCSIRotationWithMTLS(t *testing.T) {
	configPath, image, hostIP := os.Getenv("CONFIGRA_K8S_TEST_KUBECONFIG"), os.Getenv("CONFIGRA_K8S_TEST_IMAGE"), os.Getenv("CONFIGRA_K8S_TEST_HOST_IP")
	if configPath == "" || image == "" || hostIP == "" {
		t.Skip("explicit disposable kind kubeconfig, image and host IP are required")
	}
	configuration, err := clientcmd.LoadFromFile(configPath)
	if err != nil || !strings.HasPrefix(configuration.CurrentContext, "kind-configra-review") {
		t.Fatal("test requires an explicitly named disposable configra-review kind cluster")
	}
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		t.Fatal(err)
	}
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	command := func(arguments ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", configPath}, arguments...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("kubectl %v: %v\n%s", arguments, err, output)
		}
		return output
	}
	apply := func(objects ...any) {
		t.Helper()
		var data bytes.Buffer
		encoder := json.NewEncoder(&data)
		for _, object := range objects {
			if err := encoder.Encode(object); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", configPath, "apply", "--server-side", "-f", "-")
		cmd.Stdin = &data
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("apply test objects: %v\n%s", err, output)
		}
	}
	namespace := fmt.Sprintf("configra-test-%d", time.Now().UnixNano()%1_000_000_000)
	providerNamespace := namespace + "-provider"
	for _, name := range []string{namespace, providerNamespace} {
		if _, err := kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			kube.CoreV1().Namespaces().Delete(cleanup, name, metav1.DeleteOptions{})
		})
	}
	serverPair, serverPEM, _ := certificate(t, true, nil, nil)
	_, clientCAPEM, ca := certificate(t, true, nil, nil)
	clientPair, clientPEM, _ := certificate(t, false, ca.certificate, ca.key)
	clientRoots := x509.NewCertPool()
	clientRoots.AppendCertsFromPEM(clientCAPEM)
	secret := make([]byte, 32)
	rand.Read(secret)
	token := "cfg_testnode_" + base64.RawURLEncoding.EncodeToString(secret)
	var activeToken, activeCertificate atomic.Value
	activeToken.Store(token)
	activeCertificate.Store(sha256.Sum256(clientPair.Certificate[0]))
	var revision atomic.Int64
	revision.Store(1)
	var reads atomic.Int64
	var failedCSIReads atomic.Int64
	var unavailable atomic.Bool
	const privateFailureMarker = "configra-private-upstream-failure"
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+activeToken.Load().(string) || request.TLS == nil || len(request.TLS.VerifiedChains) == 0 ||
			sha256.Sum256(request.TLS.PeerCertificates[0].Raw) != activeCertificate.Load().([32]byte) {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		reads.Add(1)
		if unavailable.Load() {
			http.Error(response, privateFailureMarker, http.StatusServiceUnavailable)
			if request.URL.Path == "/v1/environments/production/configs/app" {
				failedCSIReads.Add(1)
			}
			return
		}
		value := revision.Load()
		content := fmt.Sprintf("VERSION: %d\nPORT: %d\n", value, 8080+value)
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("ETag", fmt.Sprintf(`"revision-%d"`, value))
		json.NewEncoder(response).Encode(map[string]any{"format": "yaml", "content": content, "config_revision": value, "vault_revisions": map[string]uint64{}})
	}))
	upstream.Listener.Close()
	upstream.Listener, err = net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	upstream.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverPair}, ClientCAs: clientRoots, ClientAuth: tls.RequireAndVerifyClientCert}
	upstream.StartTLS()
	defer upstream.Close()
	_, port, _ := net.SplitHostPort(upstream.Listener.Addr().String())
	origin := "https://host.docker.internal:" + port
	keyDER, _ := x509.MarshalPKCS8PrivateKey(clientPair.PrivateKey)
	defer clear(keyDER)
	for _, name := range []string{namespace, providerNamespace} {
		if _, err := kube.CoreV1().Secrets(name).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "configra-server-trust"}, Data: map[string][]byte{"ca.crt": serverPEM}}, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := kube.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "configra-credentials"}, Data: map[string][]byte{
		"token": []byte(token), "tls.crt": clientPEM, "tls.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	command("apply", "-f", "../deploy/crd.yaml")
	render := func(directory, originalNamespace, replacementNamespace string) {
		t.Helper()
		output := command("kustomize", directory)
		output = bytes.ReplaceAll(output, []byte(originalNamespace), []byte(replacementNamespace))
		output = bytes.ReplaceAll(output, []byte("registry.example.com/configra-kubernetes:v0.1.0"), []byte(image))
		output = bytes.ReplaceAll(output, []byte("https://configra-api.configra.svc"), []byte(origin))
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(output), 4096)
		for {
			var object unstructured.Unstructured
			if err := decoder.Decode(&object); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			if object.Object == nil {
				continue
			}
			if object.GetKind() == "Deployment" || object.GetKind() == "DaemonSet" {
				unstructured.SetNestedSlice(object.Object, []any{map[string]any{"ip": hostIP, "hostnames": []any{"host.docker.internal"}}}, "spec", "template", "spec", "hostAliases")
			}
			apply(object.Object)
		}
	}
	render("../deploy/sync", "configra-app", namespace)
	render("../deploy/provider", "configra-provider", providerNamespace)
	command("-n", namespace, "rollout", "status", "deployment/configra-sync", "--timeout=150s")
	command("-n", providerNamespace, "rollout", "status", "daemonset/"+providerNamespace, "--timeout=150s")
	var adapters []corev1.Pod
	for _, name := range []string{namespace, providerNamespace} {
		pods, err := kube.CoreV1().Pods(name).List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		adapters = append(adapters, pods.Items...)
	}
	binding := func(name, kind, mode string) map[string]any {
		return map[string]any{
			"apiVersion": "configra.viber-ops.github.io/v1alpha1", "kind": "ConfigraBinding", "metadata": map[string]any{"name": name, "namespace": namespace},
			"spec": map[string]any{"credentialsSecretRef": map[string]any{"name": "configra-credentials"}, "target": map[string]any{"name": name, "kind": kind, "mode": mode}, "refreshInterval": "5s", "objects": []any{map[string]any{"type": "config", "environment": "production", "config": "native_app", "path": "app.yaml"}}},
		}
	}
	apply(binding("native-file", "Secret", "files"), binding("native-env", "ConfigMap", "env"))
	waitFor(t, ctx, func() bool {
		file, err := kube.CoreV1().Secrets(namespace).Get(ctx, "native-file", metav1.GetOptions{})
		environment, other := kube.CoreV1().ConfigMaps(namespace).Get(ctx, "native-env", metav1.GetOptions{})
		return err == nil && other == nil && strings.Contains(string(file.Data["app.yaml"]), "VERSION: 1") && environment.Data["PORT"] == "8081"
	}, "native Secret and ConfigMap synchronization")
	apply(map[string]any{"apiVersion": "secrets-store.csi.x-k8s.io/v1", "kind": "SecretProviderClass", "metadata": map[string]any{"name": "configra", "namespace": namespace},
		"spec": map[string]any{"provider": "configra", "parameters": map[string]any{"fileMode": "0444", "objects": "- type: config\n  environment: production\n  config: app\n  path: app.yaml\n"}}})
	consumer := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "consumer", "namespace": namespace}, "spec": map[string]any{
		"automountServiceAccountToken": false, "securityContext": map[string]any{"runAsUser": int64(1000), "runAsGroup": int64(1000), "fsGroup": int64(1000), "runAsNonRoot": true, "seccompProfile": map[string]any{"type": "RuntimeDefault"}},
		"containers": []any{map[string]any{"name": "app", "image": "busybox:1.37.0", "command": []any{"sh", "-c", "sleep 3600"},
			"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []any{"ALL"}}},
			"envFrom":         []any{map[string]any{"configMapRef": map[string]any{"name": "native-env"}}}, "volumeMounts": []any{map[string]any{"name": "config", "mountPath": "/etc/application", "readOnly": true}}}},
		"volumes": []any{map[string]any{"name": "config", "csi": map[string]any{"driver": "secrets-store.csi.k8s.io", "readOnly": true, "volumeAttributes": map[string]any{"secretProviderClass": "configra"}, "nodePublishSecretRef": map[string]any{"name": "configra-credentials"}}}},
	}}
	apply(consumer)
	command("-n", namespace, "wait", "pod/consumer", "--for=condition=Ready", "--timeout=150s")
	if value := command("-n", namespace, "exec", "consumer", "--", "cat", "/etc/application/app.yaml"); !bytes.Contains(value, []byte("VERSION: 1")) {
		t.Fatal("CSI did not mount the initial content")
	}
	if value := command("-n", namespace, "exec", "consumer", "--", "printenv", "PORT"); strings.TrimSpace(string(value)) != "8081" {
		t.Fatal("envFrom did not receive the generated ConfigMap")
	}
	revision.Store(2)
	waitFor(t, ctx, func() bool {
		environment, err := kube.CoreV1().ConfigMaps(namespace).Get(ctx, "native-env", metav1.GetOptions{})
		return err == nil && environment.Data["PORT"] == "8082"
	}, "native refresh")
	waitFor(t, ctx, func() bool {
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", configPath, "-n", namespace, "exec", "consumer", "--", "cat", "/etc/application/app.yaml")
		output, err := cmd.CombinedOutput()
		return err == nil && bytes.Contains(output, []byte("VERSION: 2"))
	}, "CSI file rotation")

	// Observe failures in both adapters before checking their last delivered data.
	unavailable.Store(true)
	revision.Store(3)
	command("-n", namespace, "wait", "configrabinding/native-file", "--for=condition=Ready=false", "--timeout=150s")
	// Native bindings use native_app so only CSI contributes to this counter.
	// Kubelet republish failures do not necessarily emit Pod Warning events.
	waitFor(t, ctx, func() bool { return failedCSIReads.Load() > 0 }, "CSI refresh failure during the upstream outage")
	if value := command("-n", namespace, "get", "configrabinding/native-file", "-o", "json"); bytes.Contains(value, []byte(privateFailureMarker)) {
		t.Fatal("binding status disclosed an upstream response body")
	}
	file, err := kube.CoreV1().Secrets(namespace).Get(ctx, "native-file", metav1.GetOptions{})
	if err != nil || !bytes.Contains(file.Data["app.yaml"], []byte("VERSION: 2")) {
		t.Fatal("upstream failure replaced the last native target")
	}
	if value := command("-n", namespace, "exec", "consumer", "--", "cat", "/etc/application/app.yaml"); !bytes.Contains(value, []byte("VERSION: 2")) {
		t.Fatal("upstream failure replaced the last CSI content")
	}

	// A new Token and client certificate must work without restarting either adapter.
	rotatedPair, rotatedPEM, _ := certificate(t, false, ca.certificate, ca.key)
	rotatedKey, err := x509.MarshalPKCS8PrivateKey(rotatedPair.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(rotatedKey)
	rand.Read(secret)
	token = "cfg_testnode_" + base64.RawURLEncoding.EncodeToString(secret)
	activeToken.Store(token)
	activeCertificate.Store(sha256.Sum256(rotatedPair.Certificate[0]))
	credential, err := kube.CoreV1().Secrets(namespace).Get(ctx, "configra-credentials", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	credential.Data = map[string][]byte{"token": []byte(token), "tls.crt": rotatedPEM, "tls.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rotatedKey})}
	if _, err := kube.CoreV1().Secrets(namespace).Update(ctx, credential, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	unavailable.Store(false)
	waitFor(t, ctx, func() bool {
		file, err := kube.CoreV1().Secrets(namespace).Get(ctx, "native-file", metav1.GetOptions{})
		environment, other := kube.CoreV1().ConfigMaps(namespace).Get(ctx, "native-env", metav1.GetOptions{})
		return err == nil && other == nil && bytes.Contains(file.Data["app.yaml"], []byte("VERSION: 3")) && environment.Data["PORT"] == "8083"
	}, "native recovery with rotated credentials")
	waitFor(t, ctx, func() bool {
		return bytes.Contains(command("-n", namespace, "exec", "consumer", "--", "cat", "/etc/application/app.yaml"), []byte("VERSION: 3"))
	}, "CSI recovery with rotated credentials")
	command("-n", namespace, "wait", "configrabinding/native-file", "--for=condition=Ready", "--timeout=150s")
	for _, before := range adapters {
		pod, err := kube.CoreV1().Pods(before.Namespace).Get(ctx, before.Name, metav1.GetOptions{})
		if err != nil || pod.UID != before.UID {
			t.Fatal("adapter was replaced during credential rotation")
		}
		for _, container := range pod.Status.ContainerStatuses {
			if container.RestartCount != 0 {
				t.Fatal("adapter restarted during credential rotation")
			}
		}
	}

	lease, err := kube.CoordinationV1().Leases(namespace).Get(ctx, "configra-binding-controller", metav1.GetOptions{})
	if err != nil || lease.Spec.HolderIdentity == nil {
		t.Fatal("sync controller has no elected leader")
	}
	previousLeader := *lease.Spec.HolderIdentity
	leaderName, _, _ := strings.Cut(previousLeader, "_")
	leader, err := kube.CoreV1().Pods(namespace).Get(ctx, leaderName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := kube.CoreV1().Pods(namespace).Delete(ctx, leader.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &leader.UID}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ctx, func() bool {
		lease, err := kube.CoordinationV1().Leases(namespace).Get(ctx, "configra-binding-controller", metav1.GetOptions{})
		return err == nil && lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != previousLeader
	}, "sync controller leader replacement")
	revision.Store(4)
	waitFor(t, ctx, func() bool {
		file, err := kube.CoreV1().Secrets(namespace).Get(ctx, "native-file", metav1.GetOptions{})
		environment, other := kube.CoreV1().ConfigMaps(namespace).Get(ctx, "native-env", metav1.GetOptions{})
		return err == nil && other == nil && bytes.Contains(file.Data["app.yaml"], []byte("VERSION: 4")) && environment.Data["PORT"] == "8084"
	}, "native synchronization after leader replacement")
	pod, err := kube.CoreV1().Pods(namespace).Get(ctx, "consumer", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, container := range pod.Status.ContainerStatuses {
		if container.RestartCount != 0 {
			t.Fatal("consumer restarted during the rotation test")
		}
	}
	if value := command("-n", namespace, "exec", "consumer", "--", "printenv", "PORT"); strings.TrimSpace(string(value)) != "8081" {
		t.Fatal("unexpected mutation of a running process environment")
	}
	if reads.Load() < 4 {
		t.Fatal("expected authenticated reads for synchronization and rotation")
	}
	t.Logf("Native/CSI updates, outage retention, credential rotation and leader replacement passed with %d authenticated mTLS reads", reads.Load())
}

type authority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
}

func certificate(t *testing.T, isCA bool, parent *x509.Certificate, signer *ecdsa.PrivateKey) (tls.Certificate, []byte, authority) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Configra integration"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		BasicConstraintsValid: true, IsCA: isCA, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, DNSNames: []string{"host.docker.internal"}}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	if parent == nil {
		parent, signer = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: parsed}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), authority{parsed, key}
}

func waitFor(t *testing.T, ctx context.Context, condition func() bool, label string) {
	t.Helper()
	deadline := time.NewTimer(150 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("test context expired: " + label)
		case <-deadline.C:
			t.Fatal("timed out: " + label)
		case <-ticker.C:
		}
	}
}
