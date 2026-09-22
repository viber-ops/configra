# Kubernetes deployment

`base/` runs one Management replica and one independently scalable API replica.
Render it before applying:

```sh
kubectl kustomize deploy/kubernetes/base
```

The base uses kubelet's native `preStop.sleep` hook. It is enabled by default in
Kubernetes 1.30–1.33 and stable in 1.34+; check that `PodLifecycleSleepAction` is
enabled if using an older cluster. The scratch image has no shell or `sleep`
binary. See [Kubernetes lifecycle hooks](https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/)
and [feature versions](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/#feature-gates-for-alpha-or-beta-features).

During termination, the five-second hook lets Service endpoint removal propagate
while Configra still accepts requests. The server then has up to 15 seconds to
finish requests, within the Pod's 30-second grace period. Merely setting
`maxUnavailable: 0` does not close this routing/shutdown race. Five seconds is a
starting value, not a guarantee for every ingress or load balancer: measure your
own endpoint propagation and increase both the hook and termination budget if
needed. Management still uses `Recreate`, so its upgrade includes downtime.

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

When adding a CA while API replicas are already running, allow a refresh interval
before using certificates from that CA. During propagation, a TLS handshake can
report `unknown certificate authority` on a replica with the previous bundle.
Do not disable certificate verification to work around it. If rejection continues,
check the CA/certificate status and API trust-refresh warnings. Issuing another
certificate from an already trusted CA does not require a new CA bundle.

The Management/API server HTTPS certificates are separate from these client CAs.
Their keys, the Master Key, OIDC credentials, and database connection information
remain external bootstrap Secrets. Configra does not bootstrap itself through its
own provider. All replicas must use the same Master Key and transactional database.
Deploy Management first when upgrading schema v1 to v2; it verifies the Crypto
Sentinel before applying the additive migration under a database lock, then deploy
the API image. The base retains a coordinated Management replacement; scale or
change its rollout strategy only with an asset/version rollout plan.

Master Key rotation requires a maintenance window; see the
[offline rotation runbook](../backup/README.md#offline-master-key-rotation).
Suspend autoscaling and automated rollouts, record replica counts, and stop both
`configra-management` and `configra-api` before running the maintenance binary
with separately mounted old/new keys. It does not update Kubernetes Secrets for
you. Verify with the new key, update the external Secret, then restart every
replica. v1.0.0 readiness also verifies the Crypto Sentinel, so a replica
holding the old key becomes unready; liveness remains separate. This guard and
the rotation command require v1.0.0 or later.

Run the maintenance container with `restartPolicy: Never`, no HTTP probes and
labels that do not match either Service. It needs MySQL and separately mounted
key/config files, not OIDC, NATS, ClickHouse or server TLS. Retain the service
image's non-root/read-only restrictions and disable ServiceAccount-token mounting.
If using a Job, also set `backoffLimit: 0`: `Never` alone does not stop the Job
controller from creating a replacement Pod after failure. See the
[Kubernetes Job retry behavior](https://kubernetes.io/docs/concepts/workloads/controllers/job/#pod-backoff-failure-policy).
A lost connection or terminated runner can leave the database commit outcome
unknown; check both keys with doctor before deciding what to do next.

`make kubernetes-service-test` rehearses shutdown, backup, maintenance Pods,
key checks, Secret replacement and fresh SDK/CA/native/CSI reads in a disposable
cluster. This is source-candidate evidence; repeat it with the final image and
your deployment controls before a production maintenance window.

Both HTTP servers give request work a 20-second context deadline, bounded by any
earlier caller deadline. Session database operations have a five-second limit and
observe request cancellation, including while waiting for a pooled connection.
The 30-second socket write timeout is separate; it is not a database-query timeout.
Keep ingress and client timeouts consistent with these bounds. A failed session
read does not bypass authentication or fall back to an in-memory session.

For workload configuration consumption, see [Kubernetes integration](../../kubernetes/README.md):
the CSI provider mounts files directly, and the namespace-scoped controller manages
native ConfigMaps/Secrets. Install sync controllers in application namespaces,
separate from the namespace containing Configra's Master Key.

Before publishing an image, run `make image-test`. It starts the production image
with its hardened runtime flags and verifies the database is exactly MySQL
`8.0.22`, matching production.

`make kubernetes-service-test` also exercises this base in a new kind cluster,
with one control plane, two workers, two replicas of each service, OIDC, SDK
reads and both Kubernetes adapters. It pauses the worker holding the sync leader
and checks service endpoint removal, leadership transfer, updates and fresh CSI
mounts on the survivor, then resumes the worker and checks convergence. The base's
default replica counts are unchanged. This uses TLS-preserving NodePort routing
on one Docker host, not your production ingress or independent physical nodes. See
[test prerequisites](../../CONTRIBUTING.md#work-locally) and
[current evidence](../../docs/verification/2026-09-22.md#three-node-provider-and-service-failover).
