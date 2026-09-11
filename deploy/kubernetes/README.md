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
| `configra-client-ca` | `ca.pem` |
| `configra-master-key` | `master-key` containing one base64-encoded 32-byte key |
| `configra-nats` | `nats.creds`, `ca.pem` |

Expose Management through the normal HTTPS ingress. Expose the Machine API with
TCP/TLS pass-through so TLS and the optional Client Certificate reach Configra,
which performs its own certificate validation.

Before publishing an image, run `make image-test`. It starts the production image
with its hardened runtime flags and verifies the database is exactly MySQL
`8.0.22`, matching production.
