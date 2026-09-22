package binding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/viber-ops/configra/kubernetes/internal/source"
)

var GVK = schema.GroupVersionKind{Group: "configra.viber-ops.github.io", Version: "v1alpha1", Kind: "ConfigraBinding"}

const maxNativeBytes = 900 << 10

type Spec struct {
	CredentialsSecretRef struct {
		Name string `json:"name"`
	} `json:"credentialsSecretRef"`
	Target struct {
		Name string `json:"name"`
		Kind string `json:"kind,omitempty"`
		Mode string `json:"mode,omitempty"`
	} `json:"target"`
	Objects                 []source.Object `json:"objects"`
	RefreshInterval         string          `json:"refreshInterval,omitempty"`
	AllowSensitiveConfigMap bool            `json:"allowSensitiveConfigMap,omitempty"`
}

type Reconciler struct {
	Client    client.Client
	APIReader client.Reader
	Source    source.Fetcher
	Namespace string
}

func NewObject() *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(GVK)
	return object
}

func ParseSpec(object *unstructured.Unstructured) (Spec, time.Duration, error) {
	var spec Spec
	encoded, err := json.Marshal(object.Object["spec"])
	if err != nil || len(encoded) > 64<<10 {
		return spec, 0, errors.New("invalid binding spec")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&spec) != nil {
		return spec, 0, errors.New("invalid binding spec")
	}
	if spec.Target.Kind == "" {
		spec.Target.Kind = "Secret"
	}
	if spec.Target.Mode == "" {
		spec.Target.Mode = "files"
	}
	interval := time.Minute
	if spec.RefreshInterval != "" {
		interval, err = time.ParseDuration(spec.RefreshInterval)
	}
	if err != nil || interval < 5*time.Second || interval > 24*time.Hour {
		return spec, 0, errors.New("refresh interval must be between 5s and 24h")
	}
	if len(validation.IsDNS1123Subdomain(spec.CredentialsSecretRef.Name)) != 0 || spec.CredentialsSecretRef.Name == "" ||
		len(validation.IsDNS1123Subdomain(spec.Target.Name)) != 0 || spec.Target.Name == "" ||
		(spec.Target.Kind != "Secret" && spec.Target.Kind != "ConfigMap") ||
		(spec.Target.Mode != "files" && spec.Target.Mode != "env") ||
		(spec.Target.Kind == "Secret" && spec.Target.Name == spec.CredentialsSecretRef.Name) {
		return spec, 0, errors.New("invalid target or credential reference")
	}
	if err := source.ValidateObjects(spec.Objects); err != nil {
		return spec, 0, err
	}
	for _, object := range spec.Objects {
		if strings.Contains(object.Path, "/") || (spec.Target.Mode == "env" && object.Type == "file") {
			return spec, 0, errors.New("native bindings require flat filenames; env mode accepts Configs only")
		}
	}
	return spec, interval, nil
}

func (reconciler *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if request.Namespace != reconciler.Namespace {
		return ctrl.Result{}, nil
	}
	// Bound Kubernetes work too, leaving time to report a 30-second fetch failure.
	ctx, stop := context.WithTimeout(ctx, 45*time.Second)
	defer stop()
	object := NewObject()
	if err := reconciler.Client.Get(ctx, request.NamespacedName, object); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !object.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}
	spec, interval, err := ParseSpec(object)
	if err != nil {
		return reconciler.failure(ctx, object, "InvalidSpec", "Check the object mapping, target, and refresh interval.")
	}
	reader := reconciler.APIReader
	if reader == nil {
		reader = reconciler.Client
	}
	var credential corev1.Secret
	if err := reader.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: spec.CredentialsSecretRef.Name}, &credential); err != nil {
		return reconciler.failure(ctx, object, "CredentialsUnavailable", "The workload credential Secret could not be read.")
	}
	// Keep the pre-fetch version: a replaced leader's late response must not
	// overwrite a target delivered by another reconcile while this read ran.
	var target client.Object = &corev1.Secret{}
	if spec.Target.Kind == "ConfigMap" {
		target = &corev1.ConfigMap{}
	}
	err = reconciler.Client.Get(ctx, types.NamespacedName{Namespace: request.Namespace, Name: spec.Target.Name}, target)
	missing := apierrors.IsNotFound(err)
	if err != nil && !missing {
		return reconciler.failure(ctx, object, "TargetWriteFailed", "The target could not be read; the previous target is retained.")
	}
	readContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	materials, err := reconciler.Source.Read(readContext, spec.Objects, credential.Data)
	cancel()
	if err != nil {
		return reconciler.failure(ctx, object, "FetchFailed", "Configra objects could not be fetched; the previous target is retained.")
	}
	if len(materials) != len(spec.Objects) {
		return reconciler.failure(ctx, object, "FetchFailed", "Configra returned incomplete data.")
	}
	data := map[string][]byte{}
	versions := sha256.New()
	for index, material := range materials {
		if material.Path != spec.Objects[index].Path || material.Version == "" {
			return reconciler.failure(ctx, object, "FetchFailed", "Configra returned inconsistent object identities.")
		}
		if spec.Target.Kind == "ConfigMap" && material.Sensitive && !spec.AllowSensitiveConfigMap {
			return reconciler.failure(ctx, object, "SensitiveConfigMapDenied", "Use a Secret for Vault-resolved values, or explicitly allow a ConfigMap.")
		}
		_, _ = versions.Write([]byte(material.Path + "\x00" + material.Version + "\x00"))
		if spec.Target.Mode == "env" {
			values, err := EnvironmentValues(material.Bytes)
			if err != nil {
				return reconciler.failure(ctx, object, "InvalidEnvironmentMap", "Env mode requires a flat mapping of portable environment names to scalar values.")
			}
			for key, value := range values {
				if _, duplicate := data[key]; duplicate {
					return reconciler.failure(ctx, object, "DuplicateEnvironmentName", "Multiple objects define the same environment name.")
				}
				data[key] = value
			}
		} else {
			data[material.Path] = material.Bytes
		}
	}
	total := 0
	for key, value := range data {
		total += len(key) + len(value)
	}
	if total > maxNativeBytes {
		return reconciler.failure(ctx, object, "TargetTooLarge", "Native configuration objects are limited to 900 KiB by this controller.")
	}
	// Recheck uncached state so an in-flight read cannot publish a superseded spec.
	latest := NewObject()
	if err := reader.Get(ctx, request.NamespacedName, latest); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if latest.GetUID() != object.GetUID() || latest.GetGeneration() != object.GetGeneration() || !latest.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}
	digest := hex.EncodeToString(versions.Sum(nil))
	if err := reconciler.writeTarget(ctx, object, spec, target, missing, data, digest); err != nil {
		if errors.Is(err, errUnowned) {
			return reconciler.failure(ctx, object, "TargetCollision", "The target is owned by another resource or is not managed by this binding.")
		}
		return reconciler.failure(ctx, object, "TargetWriteFailed", "The target could not be updated; no partial data was published.")
	}
	if err := reconciler.cleanupPreviousTarget(ctx, object, spec); err != nil {
		return reconciler.failure(ctx, object, "PreviousTargetCleanupFailed", "The new target is ready, but a previous owned target could not be removed.")
	}
	if err := reconciler.setStatus(ctx, object, true, "Synchronized", "The target reflects the latest Configra reads.", digest, spec); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

var errUnowned = errors.New("target is not owned by this binding")

func (reconciler *Reconciler) writeTarget(ctx context.Context, binding *unstructured.Unstructured, spec Spec, target client.Object, missing bool, data map[string][]byte, version string) error {
	if !missing && !metav1.IsControlledBy(target, binding) {
		return errUnowned
	}
	before := target.DeepCopyObject()
	target.SetNamespace(binding.GetNamespace())
	target.SetName(spec.Target.Name)
	controller := true
	if missing {
		target.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: GVK.GroupVersion().String(), Kind: GVK.Kind, Name: binding.GetName(), UID: binding.GetUID(), Controller: &controller}})
	}
	labels := target.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["app.kubernetes.io/managed-by"] = "configra"
	target.SetLabels(labels)
	annotations := target.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["configra.viber-ops.github.io/version"] = version
	target.SetAnnotations(annotations)
	switch typed := target.(type) {
	case *corev1.Secret:
		typed.Type = corev1.SecretTypeOpaque
		typed.Data, typed.StringData = data, nil
		if len(typed.Data) == 0 {
			typed.Data = nil
		}
	case *corev1.ConfigMap:
		typed.Data, typed.BinaryData = map[string]string{}, map[string][]byte{}
		for name, value := range data {
			if utf8.Valid(value) {
				typed.Data[name] = string(value)
			} else {
				typed.BinaryData[name] = value
			}
		}
		if len(typed.Data) == 0 {
			typed.Data = nil
		}
		if len(typed.BinaryData) == 0 {
			typed.BinaryData = nil
		}
	}
	if missing {
		return reconciler.Client.Create(ctx, target)
	}
	if reflect.DeepEqual(before, target) {
		return nil
	}
	return reconciler.Client.Update(ctx, target)
}

func (reconciler *Reconciler) cleanupPreviousTarget(ctx context.Context, binding *unstructured.Unstructured, spec Spec) error {
	name, _, _ := unstructured.NestedString(binding.Object, "status", "target", "name")
	kind, _, _ := unstructured.NestedString(binding.Object, "status", "target", "kind")
	if name == "" || (name == spec.Target.Name && kind == spec.Target.Kind) {
		return nil
	}
	var target client.Object
	switch kind {
	case "ConfigMap":
		target = &corev1.ConfigMap{}
	case "Secret":
		target = &corev1.Secret{}
	default:
		return nil
	}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: binding.GetNamespace(), Name: name}, target); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !metav1.IsControlledBy(target, binding) {
		return nil
	}
	// Ownership can change, or the name can be reused, between Get and Delete.
	uid, version := target.GetUID(), target.GetResourceVersion()
	return client.IgnoreNotFound(reconciler.Client.Delete(ctx, target, client.Preconditions{UID: &uid, ResourceVersion: &version}))
}

func (reconciler *Reconciler) failure(ctx context.Context, object *unstructured.Unstructured, reason, message string) (ctrl.Result, error) {
	if err := reconciler.setStatus(ctx, object, false, reason, message, "", Spec{}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (reconciler *Reconciler) setStatus(ctx context.Context, object *unstructured.Unstructured, ready bool, reason, message, version string, spec Spec) error {
	updated := object.DeepCopy()
	state := "False"
	if ready {
		state = "True"
	}
	transition := time.Now().UTC().Format(time.RFC3339)
	previous, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	if len(previous) == 1 {
		if condition, ok := previous[0].(map[string]any); ok && condition["status"] == state && condition["reason"] == reason {
			transition, _ = condition["lastTransitionTime"].(string)
		}
	}
	status, _, _ := unstructured.NestedMap(updated.Object, "status")
	if status == nil {
		status = map[string]any{}
	}
	status["observedGeneration"] = object.GetGeneration()
	status["conditions"] = []any{map[string]any{"type": "Ready", "status": state, "reason": reason, "message": message, "lastTransitionTime": transition, "observedGeneration": object.GetGeneration()}}
	if ready {
		status["version"] = version
		status["lastSyncedAt"] = time.Now().UTC().Format(time.RFC3339)
		status["target"] = map[string]any{"kind": spec.Target.Kind, "name": spec.Target.Name}
	}
	updated.Object["status"] = status
	if reflect.DeepEqual(object.Object["status"], status) {
		return nil
	}
	return reconciler.Client.Status().Update(ctx, updated)
}

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
