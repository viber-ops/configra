# Kubernetes deployment

`base/` runs one Management replica and one independently scalable API replica.
Render it before applying:

```sh
kubectl kustomize deploy/kubernetes/base
```

Use an overlay to replace the example image tag, public names, OIDC settings, and
NATS endpoints. Create these Secrets out of band; do not commit their values:

| Secret | Keys |
|---|---|
| `configra-runtime` | `mysql-dsn`, `clickhouse-dsn`, `oidc-client-secret` |
| `configra-management-tls` | `tls.crt`, `tls.key` |
| `configra-api-tls` | `tls.crt`, `tls.key` |
| `configra-master-key` | `master-key` containing one base64-encoded 32-byte key |
| `configra-nats` | `nats.creds`, `ca.pem` |

Expose Management through the normal HTTPS ingress. Expose the Machine API with
TCP/TLS pass-through so TLS and the optional Client Certificate reach Configra,
which performs its own certificate validation.

Create client CAs and issue client certificates in Administration after the
Management deployment is ready. CA signing keys are encrypted in MySQL with the
shared Master Key; private export material is returned only by the original
creation request. API replicas refresh the public trust bundle every five seconds,
and per-request authorization checks issuer revocation even on existing TLS
connections. An optional `tls.client_ca_file` and volume can be added in an overlay
to retain external client CA trust.

The Management/API server HTTPS certificates are separate from these client CAs.
Their keys, the Master Key, OIDC credentials, and database connection information
remain external bootstrap Secrets. Configra does not bootstrap itself through its
own provider. All replicas must use the same Master Key and transactional database.
Deploy Management first when upgrading schema v1 to v2; it verifies the Crypto
Sentinel before applying the additive migration under a database lock, then deploy
the API image. The base retains a coordinated Management replacement; scale or
change its rollout strategy only with an asset/version rollout plan.

For workload configuration consumption, see [Kubernetes integration](../../kubernetes/README.md):
the CSI provider mounts files directly, and the namespace-scoped controller manages
native ConfigMaps/Secrets. Install sync controllers in application namespaces,
separate from the namespace containing Configra's Master Key.

Before publishing an image, run `make image-test`. It starts the production image
with its hardened runtime flags and verifies the database is exactly MySQL
`8.0.22`, matching production.
