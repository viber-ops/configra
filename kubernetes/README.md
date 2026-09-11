# Kubernetes integration

Configra itself runs as separate Management and API Deployments; see
[service deployment](../deploy/kubernetes/README.md). This module adds two ways
for application Pods to consume its configuration.

| Mode | Consumption | Persistence | Update behavior |
| --- | --- | --- | --- |
| CSI provider | Files in an ephemeral CSI volume | No native configuration object unless CSI secret sync is separately enabled | Driver rotation replaces files; applications must reload them |
| Binding controller | Native Secret or ConfigMap | Kubernetes API/etcd | Native volume refresh follows Kubernetes; `envFrom` requires Pod restart |

The native controller defaults to Secrets. ConfigMaps are not a confidential
store. Vault-derived content requires `allowSensitiveConfigMap: true` before the
controller will put it in a ConfigMap. Plain Config content may also be sensitive;
the operator must choose the appropriate target. These lifecycle limitations come
from [Kubernetes](https://kubernetes.io/docs/concepts/configuration/configmap/) and
the [CSI rotation contract](https://secrets-store-csi-driver.sigs.k8s.io/topics/secret-auto-rotation.html).

## Build

From the Configra repository (no adjacent SDK checkout is required):

```sh
GOWORK=off go -C kubernetes test -race ./...
docker build -f kubernetes/Dockerfile -t registry.example.com/configra-kubernetes:YOUR_VERSION .
```

The build downloads the public SDK version pinned in `kubernetes/go.mod` and
verifies module checksums. It does not use a sibling checkout, a local replacement,
or GitHub credentials. Its dedicated ignore file limits the repository build
context to the required source and license files. Replace `YOUR_VERSION` with
your chosen image tag before pushing it to your registry.

## Workload credentials and server trust

1. Create a CA in Administration and save its initial private-key export securely.
2. Issue a client certificate for the workload. Save `client.crt` and `client.key`
   from the one-time ZIP export.
3. Create an API Token granting only the required Environments.
4. Create the workload credential Secret in the application's namespace:

```sh
kubectl -n configra-app create secret generic configra-credentials \
  --from-file=token=/secure/configra-token \
  --from-file=tls.crt=/secure/client.crt \
  --from-file=tls.key=/secure/client.key
```

The SDK's existing explicitly allowed Token-only mode can omit `tls.crt` and
`tls.key`; HTTPS server verification remains mandatory. No provider-wide Configra
Token is configured. A workload cannot substitute another API origin in its
SecretProviderClass or binding.

The generated client CA is **not** the CA of the API server's HTTPS certificate.
The deployment examples use `configra-server-trust` containing `ca.crt` for the
server's actual issuing CA. Create that Secret in each controller/provider
namespace. If the server uses public Web PKI, remove `--server-ca-file` and its
mount to use the image's system trust roots. Set `--configra-url` to the hostname
covered by the API server certificate; the service's standard HTTPS port is 443.

Machine authorization is currently Environment-wide. Separate workload Tokens
provide independent rotation/revocation, but Tokens granting the same Environment
can read the same Configs and Vault values. Kubernetes namespaces do not narrow
that server-side grant. Use separate trust domains/deployments or appropriately
separated Environments when workloads are not mutually trusted.

## CSI provider

Install a current patched Secrets Store CSI Driver (the cluster validation uses
v1.6.1), then deploy the provider:

```sh
kubectl kustomize kubernetes/deploy/provider
kubectl apply -k kubernetes/deploy/provider
kubectl apply -f kubernetes/examples/provider.yaml
```

Replace the example image and API URL in an overlay before applying. Set the
driver's `enableSecretRotation=true` and an appropriate `rotationPollInterval`.
Kubelet republish cadence determines actual refresh timing. The provider defaults
to `fileMode: "0440"`; allowed alternatives are `"0400"` and `"0444"` (all
read-only). The non-root example explicitly uses `"0444"`, making files readable
by all UIDs inside the selected Pod mount. Use `0400`/`0440` when the consumer's
ownership/group setup supports it; do not assume `fsGroup` alone will repair
ownership after every CSI rotation. Do not use `subPath` for files needing rotation.

The provider socket is `/var/run/secrets-store-csi-providers/configra.sock`, shared
with the driver. The DaemonSet is a trusted node extension: it runs as root to own
the socket, uses a hostPath, drops all capabilities, and has no Kubernetes API
token. Install it only in a namespace permitted to run CSI node extensions. Its
protocol follows the upstream [provider interface](https://secrets-store-csi-driver.sigs.k8s.io/providers.html).

`parameters.objects` is a YAML or JSON list. Each entry has a unique relative
`path`; absolute paths, backslashes, traversal, and `..`-prefixed elements are
rejected. Example File field:

```yaml
- type: file
  environment: production
  namespace: platform
  item: database
  field: tls_cert
  path: database/client.pem
```

Mounts contain at most 32 objects and 3 MiB in total. Fetch failure returns no
partial file set. ETags populate `SecretProviderClassPodStatus` versions. Each
Config read is consistent internally; multiple objects do not imply one shared
database snapshot. Existing mounted values can remain usable after credentials
are revoked; revocation prevents further authorized reads, not use of bytes
already delivered.

## Native Secret / ConfigMap synchronization

Install the CRD once, then a controller in each application namespace:

```sh
kubectl apply -f kubernetes/deploy/crd.yaml
kubectl kustomize kubernetes/deploy/sync
kubectl apply -k kubernetes/deploy/sync
kubectl apply -f kubernetes/examples/binding.yaml
kubectl -n configra-app get configrabindings
```

Use an overlay to select the application namespace, image, and API URL. The
controller's Role is namespaced; it needs read access to credential Secrets and
write access to its native targets. Keep it out of the namespace containing
Configra's bootstrap Master Key. Two replicas use leader election. A binding
cannot reference credentials or targets in another namespace.

The controller creates a target with a controller owner reference and refuses
to overwrite an unrelated object, even if that object has a similar label. It
updates the complete target atomically, retains previous content on read errors,
and removes an old owned target after a successful target-name/kind change.
Deleting the binding lets Kubernetes garbage collection remove its owned targets.
Unchanged data does not create a write loop. Status exposes synchronization
conditions and revision digest, never content or private upstream errors.

`target.mode: files` uses each object's flat filename as a key. `target.mode: env`
projects Config documents into environment entries:

```yaml
target:
  name: application-environment
  kind: ConfigMap
  mode: env
objects:
  - type: config
    environment: production
    config: application_env
    path: environment.yaml
```

The Config document should be a flat mapping such as `PORT: 8080` and
`LOG_LEVEL: info`. Portable environment names are required; nested values, nulls,
duplicate keys, and collisions across objects are rejected. Env documents are
limited to 128 KiB each. Native targets are limited to 900 KiB in total, leaving
room below Kubernetes' object limit. File fields use `files` mode. A running
process does not acquire new environment variables until its Pod is replaced.

## Verification

Unit tests exercise real provider gRPC calls, credential/path validation, complete
responses, owner protection, namespace restriction, native text/binary mapping,
idempotency, and retention on failure. The opt-in cluster suite also exercises a
real mTLS endpoint, native synchronization, non-root CSI reads, rotation, and the
unchanged environment of an already-running container:

```sh
CONFIGRA_K8S_TEST_KUBECONFIG=/path/to/disposable-kind.kubeconfig \
CONFIGRA_K8S_TEST_IMAGE=configra-kubernetes:test \
CONFIGRA_K8S_TEST_HOST_IP=host-address-reachable-from-kind \
GOWORK=off go -C kubernetes test -tags=integration -count=1 -timeout=10m ./integration
```

The test kubeconfig context must start with `kind-configra-review`. Load the image
and BusyBox 1.37.0 into that cluster first, install the CSI driver with rotation
enabled and `fsGroupPolicy: File`, and use a reachable host IP for the temporary
HTTPS fixture. The suite creates and removes its own namespaces; it is not a
production-cluster test.
