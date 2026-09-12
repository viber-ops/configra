package binding_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/viber-ops/configra/kubernetes/internal/binding"
	"github.com/viber-ops/configra/kubernetes/internal/source"
)

type fakeSource struct {
	calls     int
	sensitive bool
	contents  string
	version   string
	err       error
}

func (upstream *fakeSource) Read(context.Context, []source.Object, map[string][]byte) ([]source.Material, error) {
	upstream.calls++
	return []source.Material{{Path: "app.yaml", Version: upstream.version, Bytes: []byte(upstream.contents), Sensitive: upstream.sensitive, Format: "yaml"}}, upstream.err
}

func fixture(t *testing.T, kind, mode string, extra ...client.Object) (*binding.Reconciler, client.Client, *unstructured.Unstructured, *fakeSource) {
	t.Helper()
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	scheme.AddKnownTypeWithName(binding.GVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(binding.GVK.GroupVersion().WithKind("ConfigraBindingList"), &unstructured.UnstructuredList{})
	object := binding.NewObject()
	object.SetName("application")
	object.SetNamespace("apps")
	object.SetUID(types.UID("binding-owner"))
	object.SetGeneration(1)
	object.Object["spec"] = map[string]any{
		"credentialsSecretRef": map[string]any{"name": "reader-credentials"},
		"target":               map[string]any{"name": "application-config", "kind": kind, "mode": mode},
		"objects":              []any{map[string]any{"type": "config", "environment": "production", "config": "app", "path": "app.yaml"}},
	}
	credential := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "reader-credentials", Namespace: "apps"}, Data: map[string][]byte{"token": []byte("test-token")}}
	objects := append([]client.Object{object, credential}, extra...)
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(object).WithObjects(objects...).Build()
	upstream := &fakeSource{contents: "PORT: 8080\nENABLED: true\n", version: "revision-1"}
	return &binding.Reconciler{Client: kube, Source: upstream, Namespace: "apps"}, kube, object, upstream
}

func reconcile(t *testing.T, reconciler *binding.Reconciler) {
	t.Helper()
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "apps", Name: "application"}}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeSyncIsOwnedIdempotentAndRetainsLastGoodOnFailure(t *testing.T) {
	reconciler, kube, owner, upstream := fixture(t, "Secret", "files")
	reconcile(t, reconciler)
	var target corev1.Secret
	key := types.NamespacedName{Namespace: "apps", Name: "application-config"}
	if err := kube.Get(context.Background(), key, &target); err != nil || string(target.Data["app.yaml"]) != upstream.contents || !metav1.IsControlledBy(&target, owner) {
		t.Fatal("target was not atomically created with the binding owner")
	}
	version := target.ResourceVersion
	reconcile(t, reconciler)
	kube.Get(context.Background(), key, &target)
	if target.ResourceVersion != version {
		t.Fatal("unchanged content caused another target write")
	}
	upstream.err = errors.New("sensitive-value-sentinel")
	reconcile(t, reconciler)
	kube.Get(context.Background(), key, &target)
	if string(target.Data["app.yaml"]) != "PORT: 8080\nENABLED: true\n" {
		t.Fatal("failure replaced last-known-good data")
	}
	latest := binding.NewObject()
	kube.Get(context.Background(), types.NamespacedName{Namespace: "apps", Name: "application"}, latest)
	status, _ := json.Marshal(latest.Object["status"])
	if strings.Contains(string(status), "sensitive-value-sentinel") || !strings.Contains(string(status), "FetchFailed") {
		t.Fatal("binding status exposed private errors or missed failure")
	}
}

func TestNativeSyncRejectsUnownedTargetsAndCrossNamespaceRequests(t *testing.T) {
	existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "application-config", Namespace: "apps"}, Data: map[string][]byte{"owned-by-user": []byte("preserve")}}
	reconciler, kube, _, upstream := fixture(t, "Secret", "files", existing)
	reconcile(t, reconciler)
	var target corev1.Secret
	kube.Get(context.Background(), types.NamespacedName{Namespace: "apps", Name: "application-config"}, &target)
	if string(target.Data["owned-by-user"]) != "preserve" {
		t.Fatal("overwrote an unrelated target")
	}
	before := upstream.calls
	reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "another-namespace", Name: "application"}})
	if upstream.calls != before {
		t.Fatal("cross-namespace request reached Configra")
	}
}

func TestConfigMapEnvironmentModeUsesTextDataAndRequiresVaultOptIn(t *testing.T) {
	reconciler, kube, object, upstream := fixture(t, "ConfigMap", "env")
	upstream.sensitive = true
	reconcile(t, reconciler)
	var target corev1.ConfigMap
	key := types.NamespacedName{Namespace: "apps", Name: "application-config"}
	if err := kube.Get(context.Background(), key, &target); err == nil {
		t.Fatal("Vault-derived content was put in a ConfigMap without opt-in")
	}
	kube.Get(context.Background(), types.NamespacedName{Namespace: "apps", Name: "application"}, object)
	unstructured.SetNestedField(object.Object, true, "spec", "allowSensitiveConfigMap")
	object.SetGeneration(2)
	if err := kube.Update(context.Background(), object); err != nil {
		t.Fatal(err)
	}
	reconcile(t, reconciler)
	if err := kube.Get(context.Background(), key, &target); err != nil || target.Data["PORT"] != "8080" || target.Data["ENABLED"] != "true" || len(target.BinaryData) != 0 {
		t.Fatal("envFrom cannot consume the generated ConfigMap")
	}
	version := target.ResourceVersion
	reconcile(t, reconciler)
	kube.Get(context.Background(), key, &target)
	if target.ResourceVersion != version {
		t.Fatal("empty binaryData normalization caused an update loop")
	}
}

func TestEnvironmentProjectionRejectsNestedOrAmbiguousValues(t *testing.T) {
	for _, content := range []string{"PORT: 80\nPORT: 90\n", "BAD-NAME: value\n", "NESTED: {x: y}\n", "VALUE: null\n", "A: one\n---\nB: two\n", "- not-a-mapping\n"} {
		if _, err := binding.EnvironmentValues([]byte(content)); err == nil {
			t.Fatalf("accepted invalid env mapping %q", content)
		}
	}
}
