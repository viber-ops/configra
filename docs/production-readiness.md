# Production readiness contract

This document is the completion contract for Configra V1. A capability is complete only when its evidence below exists and passes from a clean checkout; implementation progress or a narrower test is not substitute evidence.

## Public test seams

Tests exercise behavior only through these already-decided interfaces:

1. **Management HTTP** — OIDC-authenticated human queries and Mutations used by the Management UI.
2. **Machine Read HTTP** — the two exact V1 endpoints fixed by ADR-0018, including Token, mTLS, ETag, limits, and stable error envelopes.
3. **Go Client and Viper Handler** — exported `configra-go` interfaces for resolved Config, File bytes, Load, Reload, Current, Watch, and Change Callback.
4. **Process and container** — startup validation, readiness, shutdown, migration, Crypto Sentinel verification, and dependency failure behavior.

Domain behavior may have fast package tests, but acceptance tests do not query implementation tables or mock Configra's own modules. MySQL, ClickHouse, NATS, TLS, and OIDC test adapters sit only at real external seams.
The release image contains no embedded Identity Provider, local users, test identities, or authentication bypass. Production acceptance authenticates through the configured external OIDC Provider and returns through the fixed Configra callback.

## Verification order

The MySQL compatibility baseline is the production version `8.0.22`; container integration and release gates must run against that exact image rather than a newer MySQL release.

1. `make tla` model-checks concurrency, authorization, and client reload invariants.
2. `go test -race ./...` and `go vet ./...` pass in both repositories.
3. `govulncheck` reports no reachable vulnerability in either repository.
4. Container integration tests run against real MySQL, ClickHouse, NATS, and TLS endpoints.
5. End-to-end tests exercise Management HTTP, Machine Read HTTP, and the real SDK without database-side assertions.
6. A release image passes startup, migration, readiness, graceful-shutdown, backup-verification, and dependency-recovery drills.
7. Load and leakage gates pass on a recorded machine and configuration.

Both modules require Go `1.25.13` or newer, the first patched Go 1.25 release
covering the reachable standard-library findings present in 1.25.8. The
production image is built by the digest-pinned Go `1.26.7` builder. On
2026-08-27, `govulncheck v1.7.0` reported zero reachable vulnerabilities in
both repositories.

Current image/deployment evidence is runnable with `make image-test` and
`go test ./internal/app`. The former builds the actual scratch image and verifies
startup, readiness, non-root/read-only execution, graceful shutdown, wrong-key
failure, and log redaction against exact MySQL `8.0.22`; the latter also renders
and checks the separated Kubernetes base. Backup/recovery and load evidence remain
separate gates and are not implied by these checks. `make backup-test` verifies
MySQL 8.0.22 service-state and ClickHouse log backup/restore, corrupt and missing
artifacts, overwrite refusal, plaintext exclusion, and Crypto Sentinel failure.
An operator-selected RPO/RTO, durable encrypted storage, retention policy, and
scheduled off-site drill are still deployment-specific release evidence.

`make web-test` currently builds the embedded production bundle and passes 36
Playwright flows covering both locales, Viewer/Admin boundaries, lifecycle operations,
Environment detail and exact inventory filters, arbitrary historical Revision inspection,
Config validate/format and same- or cross-Environment Revision compare/restore/clone,
dirty-draft and Conflict handling, value-free current Vault usages, destructive-edit impact
confirmation, explicit value reveal, copyable Vault references, credentials, Notifications,
Access/Audit, and deployment status.

## Behavior matrix

| Area | Happy paths | Unhappy paths that must be observed |
|---|---|---|
| Bootstrap | New database initializes once; existing database verifies Crypto Sentinel; both server modes start from YAML | Missing/invalid Master Key, missing Sentinel in initialized DB, invalid YAML, unreachable MySQL, and invalid TLS material fail closed without values |
| Environment | Create, rename Display Name, archive, unarchive, dense inventory, detail with current Config and Vault bindings, exact linked filters | Duplicate/invalid Resource Key, archived reads and Mutations, stale operation, and unarchive permission restoration |
| Config | Create, canonical save, complete history inspection, diff, directional merge/replace, restore, clone, resolved preview, dirty-draft protection | Invalid YAML/JSON, duplicate keys, oversize source/resolution, no-change/double submit, stale Target Conflict with preserved draft and Request ID, reused OperationID with different request, unresolved Vault Reference |
| Vault | Composite Namespace/Item identity, same Item Key in different Namespaces, Item/Field/Variant lifecycle, current value-free Config usages, impact confirmation, encryption, restore, Text/Secret resolution, File bytes | Three-segment reference, cross-Namespace collision, overlapping Environment binding, incomplete Variant, immutable key/type changes, field/item limits, missing Variant, corrupt ciphertext/AAD, wrong Master Key, archived Field/Item |
| Machine auth | Active Token + accepted certificate; explicitly Token-only Token; authorized Environment; identical decision across Vault Namespaces | Missing/malformed/expired/revoked Token, disallowed Environment, absent required certificate, invalid/unregistered/revoked presented certificate, archived Environment; Namespace must never act as a grant |
| Credentials | Admin creates one-time Token, replaces Environment grants, imports multiple CA-signed public certificates, and permanently revokes either credential | Viewer denial, Token replay without plaintext, invalid/expired/untrusted/private-key certificate input, missing/archived Environment grant, malformed fingerprint, and no secret/digest/DER in lists |
| Machine reads | Resolved Config with composite ETag and `<namespace>.<item>` Revision evidence; `304`; exact namespaced File bytes and metadata | Missing/archived resource, non-File content endpoint, unresolved or legacy reference, cross-Namespace substitution, crypto-integrity failure, timeout, unavailable dependency; never return partial content |
| SDK/Viper | Initial Load, explicit Reload, jittered Watch, atomic changed Snapshot, serialized callback, runtime certificate rotation | Cold-start fetch failure, network/5xx/parse failure retaining LKG, unchanged response, callback failure without rollback, cancellation, interval below minimum, concurrent readers |
| Audit/Access | Mutation Audit reaches ClickHouse through MySQL Outbox; content read may emit Access Event through NATS | ClickHouse outage retains Audit Outbox; NATS/Management outage never blocks Machine read; records and errors contain metadata only |
| Notifications | Generic Webhook/Feishu delivery, signing, durable retry, Test, history, manual redelivery | SSRF/special-use targets, DNS rebinding, proxy environment, all redirects, TLS failure, timeout, malformed/oversize provider response, `Retry-After`, 429/5xx retry, terminal 4xx, attempt limit, corrupt credentials, duplicate/out-of-order delivery, secret-bearing URL redaction |
| Management | English/Chinese light/dark UI; Environment detail and linked filtering; Config and Vault complete History; Config validate/format and Revision compare/restore/clone; value-free Vault usages; dirty-draft protection; credential and Notification lifecycle; read-only deployment status | Viewer mutation/Secret/File/resolved-preview denial, malformed Config/Snapshot input, unauthenticated access, CSRF/session expiry, conflicts with retained input and copyable Request ID, no-op/double submits, archived and revoked lifecycle states |

## 1000 QPS gate

The release candidate must sustain at least 1000 completed resolved-Config reads per second for ten minutes through HTTPS using the production image and a representative encrypted Vault reference. `make load-smoke` checks the complete fixture for 15 seconds; `make load-test` runs the release gate. Both refuse any MySQL server version other than the production baseline `8.0.22`, use mTLS plus an Environment-scoped Token, limit the API container to 2 CPUs and 512 MiB, keep the NATS-to-ClickHouse Access pipeline active, and record CPU, RSS, PIDs, MySQL connected/running threads, latency percentiles, response classes, and exact image/host metadata in `.cache/load/latest.json`. The gate fails on a non-200 response, throughput below 980 completed reads/s, authentication bypass, panic, resource ceiling, or sustained RSS growth after warm-up. Heap or goroutine profiling is added only when the process-level trend grows or is too noisy to diagnose.

The 2026-08-27 V1 release-candidate gate served 600,000/600,000 successful reads
in 10 minutes at 999.997 completed reads/s. Mean/p50/p95/p99 latency was
4.93/3.58/9.08/31.34 ms; maximum latency was 490.91 ms. API CPU peaked at
140.37% of the 2-CPU allowance, RSS stayed between 14.77 and 23.51 MiB with
first/last window medians of 15.61/22.02 MiB, PIDs peaked at 9, MySQL
connected/running threads at 26/7, and 615,002/615,002 expected Access Events
reached ClickHouse. The gate used the 5,936,487-byte arm64 Linux scratch image
`sha256:313f3f008c916…`, the harness used Go 1.25.13, the image used the pinned
Go 1.26.7 builder, live MySQL reported exactly `8.0.22`, and the automated
sentinel leak scan passed.

## Leakage gate

Every end-to-end run uses unique sentinel values for API Token, Vault Secret, File bytes, Master Key, certificate private key, OIDC secret, and Notification URL credentials. After happy, unhappy, and load runs, automated scans must find none of those sentinels in application/container logs, ClickHouse Access/Audit records, captured NATS payloads, HTTP error bodies, traces, metrics labels, or generated diagnostics. The load gate additionally scans its bounded HTTP error and body-free Vegeta result, then compares process-level RSS windows and PID counts before accepting the soak; transient plaintext required to serve a request is not misrepresented as zero in-memory exposure.

## Completion evidence

The final audit links each row above to a runnable test, model-check output, container run, load report, or leakage report. Missing or indirect evidence keeps the production goal open.
