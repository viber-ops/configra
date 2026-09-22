package binding_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/viber-ops/configra/kubernetes/internal/binding"
	"github.com/viber-ops/configra/kubernetes/internal/source"
)

type fakeSource struct {
	calls        int
	sensitive    bool
	contents     string
	version      string
	err          error
	beforeReturn func()
}

func (upstream *fakeSource) Read(context.Context, []source.Object, map[string][]byte) ([]source.Material, error) {
	upstream.calls++
	materials := []source.Material{{Path: "app.yaml", Version: upstream.version, Bytes: []byte(upstream.contents), Sensitive: upstream.sensitive, Format: "yaml"}}
	if upstream.beforeReturn != nil {
		upstream.beforeReturn()
	}
	return materials, upstream.err
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

func TestDocumentedYAMLAndJSONProjectTheSameEnvironment(t *testing.T) {
	document, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{"PORT": []byte("8080"), "LOG_LEVEL": []byte("info"), "ENABLE_METRICS": []byte("true")}
	checked := 0
	for _, block := range regexp.MustCompile("(?ms)^```(yaml|json)\\n(.*?)^```").FindAllStringSubmatch(string(document), -1) {
		if !strings.Contains(block[2], "ENABLE_METRICS") {
			continue
		}
		t.Run(block[1], func(t *testing.T) {
			reconciler, kube, _, upstream := fixture(t, "Secret", "env")
			upstream.contents = block[2]
			reconcile(t, reconciler)
			var target corev1.Secret
			if err := kube.Get(context.Background(), types.NamespacedName{Namespace: "apps", Name: "application-config"}, &target); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(target.Data, want) {
				t.Fatal("documented Config must produce exactly the three promised environment strings")
			}
		})
		checked++
	}
	if checked != 2 {
		t.Fatalf("expected YAML and JSON examples, checked %d", checked)
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

func TestOlderFetchCannotOverwriteAConcurrentDelivery(t *testing.T) {
	for _, kind := range []string{"Secret", "ConfigMap"} {
		for _, state := range []string{"missing", "existing"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				reconciler, kube, _, upstream := fixture(t, kind, "files")
				if state == "existing" {
					reconcile(t, reconciler)
				}
				upstream.contents, upstream.version = "PORT: 8081\n", "revision-2"
				newer := *reconciler
				newer.Source = &fakeSource{contents: "PORT: 9090\n", version: "revision-3"}
				// The replacement finishes while the original read is still in flight.
				upstream.beforeReturn = func() { reconcile(t, &newer) }
				_, _ = reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "apps", Name: "application"}})
				key := types.NamespacedName{Namespace: "apps", Name: "application-config"}
				var content string
				if kind == "Secret" {
					var target corev1.Secret
					if err := kube.Get(context.Background(), key, &target); err != nil {
						t.Fatal(err)
					}
					content = string(target.Data["app.yaml"])
				} else {
					var target corev1.ConfigMap
					if err := kube.Get(context.Background(), key, &target); err != nil {
						t.Fatal(err)
					}
					content = target.Data["app.yaml"]
				}
				if content != "PORT: 9090\n" {
					t.Fatal("an older in-flight fetch overwrote the newer target")
				}
			})
		}
	}
}

func TestReconcileBoundsKubernetesWorkBeforeFetchingConfigra(t *testing.T) {
	reconciler, kube, _, _ := fixture(t, "Secret", "files")
	checked := false
	reconciler.APIReader = interceptor.NewClient(kube.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, real client.WithWatch, key client.ObjectKey, object client.Object, options ...client.GetOption) error {
			deadline, bounded := ctx.Deadline()
			if !bounded || time.Until(deadline) > 45*time.Second {
				t.Fatal("Kubernetes reads must share the reconciliation deadline, not just Configra reads")
			}
			checked = true
			return real.Get(ctx, key, object, options...)
		},
	})
	reconcile(t, reconciler)
	if !checked {
		t.Fatal("the Kubernetes API reader was not exercised")
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

func TestTargetCleanupDoesNotDeleteAnObjectWhoseOwnershipChanged(t *testing.T) {
	for _, kind := range []string{"Secret", "ConfigMap"} {
		t.Run(kind, func(t *testing.T) {
			reconciler, kube, object, _ := fixture(t, kind, "files")
			reconcile(t, reconciler)
			ctx := context.Background()
			if err := kube.Get(ctx, client.ObjectKeyFromObject(object), object); err != nil {
				t.Fatal(err)
			}
			if err := unstructured.SetNestedField(object.Object, "new-target", "spec", "target", "name"); err != nil {
				t.Fatal(err)
			}
			object.SetGeneration(2)
			if err := kube.Update(ctx, object); err != nil {
				t.Fatal(err)
			}
			var target client.Object = &corev1.Secret{}
			if kind == "ConfigMap" {
				target = &corev1.ConfigMap{}
			}
			key := types.NamespacedName{Namespace: "apps", Name: "application-config"}
			deleted := false
			reconciler.Client = interceptor.NewClient(kube.(client.WithWatch), interceptor.Funcs{
				Delete: func(ctx context.Context, real client.WithWatch, stale client.Object, options ...client.DeleteOption) error {
					deleted = true
					preconditions := (&client.DeleteOptions{}).ApplyOptions(options).Preconditions
					if preconditions == nil || preconditions.UID == nil || *preconditions.UID != stale.GetUID() ||
						preconditions.ResourceVersion == nil || *preconditions.ResourceVersion != stale.GetResourceVersion() {
						t.Fatal("cleanup must condition deletion on the observed identity and version")
					}
					if err := real.Get(ctx, key, target); err != nil {
						t.Fatal(err)
					}
					// Another writer takes ownership after cleanup's Get, before its Delete.
					target.SetOwnerReferences(nil)
					if err := real.Update(ctx, target); err != nil {
						t.Fatal(err)
					}
					return real.Delete(ctx, stale, options...)
				},
			})
			reconcile(t, reconciler)
			if !deleted {
				t.Fatal("cleanup did not exercise the concurrent delete")
			}
			if err := kube.Get(ctx, key, target); err != nil {
				t.Fatalf("cleanup deleted a target now owned by another writer: %v", err)
			}
			reconciler.Client = kube
			reconcile(t, reconciler)
			if err := kube.Get(ctx, key, target); err != nil {
				t.Fatalf("cleanup retry deleted the unrelated target: %v", err)
			}
		})
	}
}
