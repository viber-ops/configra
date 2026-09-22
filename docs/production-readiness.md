# Production readiness

This document is the completion contract for Configra V1. A capability is complete only when its evidence below exists and passes from a clean checkout; implementation progress or a narrower test is not substitute evidence.

## Current status — 2026-09-22

The release target is **v1.0.1** for the server and **v1.0.0** for the Go SDK. This page records
release acceptance, not a blanket approval of every production deployment.
The final runtime passes native Linux integration, capacity and recovery checks,
three-node Kubernetes acceptance on native Linux and the Mac, and the complete
[hosted CI run](https://github.com/viber-ops/configra/actions/runs/35694430155).
Dated results and failed attempts remain in the
[verification record](verification/2026-09-22.md). Tagged archive/download checks
are completed before publishing the [release](https://github.com/viber-ops/configra/releases/tag/v1.0.1);
their post-tag evidence is recorded there.

Production ingress, capacity on your hardware, independent-host/zone failures,
backup retention and maintenance-access controls must be checked in your own
deployment. The recorded test results are not an SLA. The published rc.2 tag
and checksums remain unchanged.

| Requirement | Required evidence | Current state |
| --- | --- | --- |
| Apache-2.0 distribution | License/notice in source, nested module, binaries and images; third-party license inventory | Reviewed Go inventory and runtime/module/UI notices, source access and SBOM checks pass for source-built bundles and both images. Tagged downloadable archives are checked separately before publication |
| Open-source maintenance | Contribution/security policies, working private disclosure, continuous checks and dependency updates | Policies/private reporting, pinned CI actions and weekly dependency checks are live. SDK Linux/macOS CI and all four server Go-version/module jobs, 51 UI tests and integration pass |
| MySQL 8.0.22 support | Exact-version integration, migration and restore tests; unchanged capacity gate | Final native Linux integration, ten-minute load, service backup/restore and offline rotation pass on 8.0.22. No schema change or 8.4-only requirement |
| MySQL 8.4 comparison | Official compatibility/upgrade sources, separately identified runtime evidence | [Primary-source comparison](research/mysql-8.0.22-and-8.4-compatibility.md) completed; runtime evidence absent |
| Complete authenticated mutation audit | Authenticated rejections have durable, value-free receipts; logical Operation identities survive replay; final image/failure drills pass | Real-MySQL replay/rejection tests and final native-image OIDC rejection receipts, ClickHouse outage recovery and post-rotation Audit delivery pass |
| Bounded management inventories | Server-side limits/pagination, consumer behavior and scale/query evidence | All seven inventories, Config/Vault histories, notification subscriptions and Vault usages are paged. Clean-checkout SQL/HTTP/UI tests pass, including cross-page grants and the actual 79-Config seed rerun. Counts/search can still be linear |
| Key recovery and lifecycle | Correct/wrong-key drills, usable rotation procedure, CA/client lifecycle checks | Final native-image backup/restore, SELECT-only doctor, offline rotation, restarted SDK reads and restored CA issuance/revocation pass. Partial rollback, concurrent writers and lost COMMIT responses have real-MySQL regressions |
| Production process and Kubernetes | Exact runtime images, readiness/shutdown, TLS pass-through, rollout, CSI/sync and dependency failure tests | Native Linux, Mac and hosted three-node checks pass for both providers, credential/API/controller/worker faults, rolling updates, offline rotation and fresh delivery. Native Linux verifies 111 no-retry reads during three complete API rollouts. Production ingress and independent-host/zone checks remain deployment-specific |
| 1000 QPS for ten minutes | Production image, real encrypted Vault reference, mTLS, 2 CPU/512 MiB limit, unchanged gate, leakage checks and recorded host | [Final runtime passes](verification/2026-09-22.md#final-runtime-native-linux-acceptance) on four-vCPU Linux: 600,000 HTTP 200, 999.997 completed/s, p99 31.935 ms, all Access and leakage checks pass. API limit stays 2 CPU/512 MiB. Earlier Mac/two-vCPU/hosted-runner failures remain recorded |
| Usable instructions, not slogans | Clean-environment walkthroughs of quickstart, SDK, installation, Kubernetes and backup/restore; failures fixed | Real local startup/seed, OIDC browser operations, SDK example and native-image backup/restore/rotation pass. Kubernetes checks exercise the deployment base and CSI example. Exact downloaded-binary installation is checked before publication |
| Chinese and English website | Same-page language switching, corresponding guides, locale-correct links/metadata, build and content checks | Both languages distinguish server v1.0.1 and SDK v1.0.0. Type check, 28-page build, 36 tests and Chromium checks of 26 routes at desktop/mobile widths are repeated after version updates. Published-site checking follows release publication |
| Release integrity | Clean commit/tag mapping, tests, license/notices/SBOM and downloadable multi-platform artifacts | SDK v1.0.0 is published and verified against four Linux/macOS CI jobs and its module checksum. Server archives are built from the tag, held as a draft and independently downloaded/verified before publication |

The full sequence of fixes, failing reproductions and earlier image/host runs is
kept in the [dated verification record](verification/2026-09-22.md), rather than
duplicated here. Six TLA+ models and Go 1.25.13/1.26.7 race/vet checks pass;
current source scanning finds no reachable vulnerability.

## Confirmed review scope

Short maintenance windows are accepted for Master Key rotation. This review uses
the offline transaction and recovery procedure; online rotation and a multi-key
keyring are not required. A lost COMMIT response must be resolved with both keys
while services remain stopped, not treated as proof of rollback.

The user supplied temporary independent Linux machines on September 22 after
the Mac runs. The initial two-vCPU/3,622-MiB VM saturated under the complete
stack; the user released it and supplied an eight-vCPU/15,319-MiB replacement,
also without swap. API, MySQL, NATS, ClickHouse and the load generator share
that host. The API remains limited to two CPUs and 512 MiB; native MySQL 8.0.22,
query defaults and capacity thresholds are unchanged. Mac fixtures remain useful for
the three-node Kubernetes checks; do not stop unrelated projects or equate
arm64-host/emulated-MySQL measurements with native Linux results. Preserve the
remote evidence and remove owned fixtures before the user releases the machine.
Those hosts were subsequently released. Final runtime acceptance uses a new
four-vCPU/7,521-MiB native Linux host: the complete stack passes the unchanged
gate there, with the API still capped at two CPUs and 512 MiB. Its exact result,
image and distinction from the failed hosted-runner warmup are recorded above.

The production Kubernetes version and ingress product are not selected. The
tested review baseline is Kubernetes 1.35.0 with native Services and TLS-preserving
NodePort routing. No specific ingress controller or cloud load balancer is a
prerequisite for this codebase review. Machine API deployment requires end-to-end
TLS/client-certificate delivery; the chosen external TCP/TLS pass-through path and
independent-host failure behavior still need deployment-specific verification.

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
production image is built by the digest-pinned Go `1.26.7` builder. The current
core, SDK and Kubernetes suites passed race checks on both supported toolchains.
`govulncheck v1.7.0` found no reachable vulnerabilities after the gRPC update;
unimported advisory-bearing modules in the core graph remain identified separately.

Current image/deployment evidence is runnable with `make image-test` and
`go test ./internal/app`. The former builds the actual scratch image and verifies
startup, readiness, non-root/read-only execution, graceful shutdown, wrong-key
failure, and log redaction against exact MySQL `8.0.22`; the latter also renders
and checks the separated Kubernetes base. Backup/recovery and load evidence remain
separate gates and are not implied by these checks. `make backup-test` verifies
MySQL 8.0.22 service-state and ClickHouse log backup/restore, corrupt and missing
artifacts, overwrite refusal, plaintext exclusion (including hex-encoded dump
values), and Crypto Sentinel failure. It now also builds the real
`doctor --verify-vault` CLI, runs it with a SELECT-only account, checks all stored
encrypted record kinds after restore, issues a client certificate with the
restored CA, and rejects empty databases, wrong keys, timeouts, malformed DSNs and
historical corruption. The same drill performs offline rotation with the
documented per-table permissions, verifies it with the new key and rechecks
Config/File reads and CA issuance. These commands are not in rc.2; see the
[source runbook](../deploy/backup/README.md).
An operator-selected RPO/RTO, durable encrypted storage, retention policy, and
scheduled off-site drill are still deployment-specific release evidence.

`make service-test` adds the full local Management/API image chain: real HTTPS
Casdoor login, authorization/rejection audit, mTLS Config/File reads, dependency
stalls, durable Audit recovery, image-driven doctor/rotation, restored CA
issuance/revocation and the adjacent SDK example. It reuses the disposable
development Compose fixture, retains pre-recreation logs for leakage checks,
and removes its uniquely named project afterward. See
[contributor prerequisites](../CONTRIBUTING.md#work-locally) and the
[scope/evidence](verification/2026-09-22.md#actual-service-images-and-dependency-faults).
It is not a Kubernetes deployment, a capacity test or a published-release result.

`make kubernetes-service-test` reuses those external dependencies and runs the
actual service/adapter images in an owned three-node kind cluster, using the
deployment base and example CSI Pod. It adds native/CSI delivery on both workers,
credential and API failure recovery, controller failover and repeated no-retry
mTLS reads during rolling replacement. Pausing the sync leader's worker checks
endpoint removal, cross-node leadership, updates and a new mount on the surviving
worker, followed by convergence after recovery. It also stops all service Pods, runs image-driven offline key
rotation, verifies both keys, updates the fixture Secret and checks fresh
SDK/CA/Audit/native/CSI behavior. The [cluster evidence](verification/2026-09-22.md#actual-configra-services-in-kubernetes)
records the fixed termination race; the [three-node follow-up](verification/2026-09-22.md#three-node-provider-and-service-failover)
records the current image and fault scope. NodePort preserves TLS here, and all
nodes share one Docker host. This does not prove uninterrupted reads during node
detection, production ingress or independent-host/zone resilience.

The latest recorded build/browser run passed
[51 Playwright flows](verification/2026-09-22.md#review-regression-fixes-and-stable-release-preparation), covering both locales,
Viewer/Admin boundaries, lifecycle operations,
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
| Credentials | Admin creates one-time Token, replaces or incrementally updates Environment grants, imports multiple CA-signed public certificates, and permanently revokes either credential | Viewer denial, Token replay without plaintext, cross-page grant retention, invalid/expired/untrusted/private-key certificate input, missing/archived Environment grant, malformed fingerprint, and no secret/digest/DER in lists |
| Machine reads | Resolved Config with composite ETag and `<namespace>.<item>` Revision evidence; `304`; exact namespaced File bytes and metadata | Missing/archived resource, non-File content endpoint, unresolved or legacy reference, cross-Namespace substitution, crypto-integrity failure, timeout, unavailable dependency; never return partial content |
| SDK/Viper | Initial Load, explicit Reload, jittered Watch, atomic changed Snapshot, serialized callback, runtime certificate rotation | Cold-start fetch failure, network/5xx/parse failure retaining LKG, unchanged response, callback failure without rollback, cancellation, interval below minimum, concurrent readers |
| Audit/Access | Mutation Audit reaches ClickHouse through MySQL Outbox; content read may emit Access Event through NATS | ClickHouse outage retains Audit Outbox; NATS/Management outage never blocks Machine read; records and errors contain metadata only |
| Notifications | Generic Webhook/Feishu delivery, signing, durable retry, Test, history, manual redelivery | SSRF/special-use targets, DNS rebinding, proxy environment, all redirects, TLS failure, timeout, malformed/oversize provider response, `Retry-After`, 429/5xx retry, terminal 4xx, attempt limit, corrupt credentials, duplicate/out-of-order delivery, secret-bearing URL redaction |
| Management | English/Chinese light/dark UI; Environment detail and linked filtering; Config and Vault complete History; Config validate/format and Revision compare/restore/clone; value-free Vault usages; dirty-draft protection; credential and Notification lifecycle; read-only deployment status | Viewer mutation/Secret/File/resolved-preview denial, malformed Config/Snapshot input, unauthenticated access, CSRF/session expiry, conflicts with retained input and copyable Request ID, no-op/double submits, archived and revoked lifecycle states |

## 1000 QPS gate

The release candidate must sustain at least 1000 completed resolved-Config reads per second for ten minutes through HTTPS using the production image and a representative encrypted Vault reference. `make load-smoke` checks the complete fixture for 15 seconds; `make load-test` runs the release gate. Both refuse any MySQL server version other than the production baseline `8.0.22`, use mTLS plus an Environment-scoped Token, limit the API container to 2 CPUs and 512 MiB, keep the NATS-to-ClickHouse Access pipeline active, and record CPU, RSS, PIDs, MySQL connected/running threads, latency percentiles, response classes, and exact image/host metadata in `.cache/load/latest.json`. The gate fails on a non-200 response, throughput below 980 completed reads/s, authentication bypass, panic, resource ceiling, or sustained RSS growth after warm-up. Heap or goroutine profiling is added only when the process-level trend grows or is too noisy to diagnose.

The report now records `passed`, the last `stage`, warmup results and measured
samples on failure too. Failed runs also retain API logs and body-free Vegeta
results in `.cache/load/failed-*`, with owner-only directory/file permissions.
Do not publish those raw files: a failing leakage check may mean they contain
test secrets. The Token-bearing target file and generated private-key files
are not copied. These diagnostics do not change the acceptance thresholds.

The early [September 22 runs](verification/2026-09-22.md#capacity-and-leak-detection)
used native MySQL AIO on the shared Mac/Docker host and included both passes and
failures. The final [four-vCPU Linux result](verification/2026-09-22.md#final-runtime-native-linux-acceptance)
passes the release runtime with default queries and a two-CPU API limit.
It does not validate every deployment. The older result below remains historical,
not a result for the current source.

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

## Working rules

- Keep MySQL 8.0.22 in scope. A newer compatibility target adds evidence; it does
  not erase the requested legacy-version support.
- Use disposable fixtures, never the user's production database or cluster.
- A successful build, narrow smoke test or old benchmark is not evidence that
  all acceptance requirements pass for the current candidate.
- Do not lower resource, throughput, security or leakage thresholds to obtain a
  green result. Record failures and investigate them.
- Documentation commands must identify their working directory, prerequisites,
  placeholders and expected outcome. Mark anything not yet executed as such.
- Publish new tags for changed releases; do not silently rewrite published
  versions. The explicit server rc.1 retirement is documented in its successor's
  [release notes](releases/v0.1.0-rc.2.md); SDK tags
  remain unchanged.
- The goal stays open until each requirement has direct, current evidence.

## First-party licensing versus distribution

The [distribution requirements](research/distribution-license-requirements.md)
and [Go dependency inventory](research/transitive-go-license-inventory.md) record
MPL-2.0 for the MySQL driver, file-specific MIT/Apache licensing for YAML and
required upstream notices. Old SDK module archives lack LICENSE/NOTICE files;
the SDK v1.0.0 ZIP contains them and is pinned by Kubernetes.

The September 12 four-platform bundles and both image architectures passed checks for
project/runtime, Go-module and UI notice delivery, covered MySQL source, system
CA materials, aggregate SBOMs and complete file inventories. Before distribution
approval of the new fixes, rebuild complete bundles and verify the final published files
recipients actually download. A top-level Apache file or a successful scanner
does not complete that audit.
