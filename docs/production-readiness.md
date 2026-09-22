# Production readiness

This document is the completion contract for Configra V1. A capability is complete only when its evidence below exists and passes from a clean checkout; implementation progress or a narrower test is not substitute evidence.

## Current status — 2026-09-22

The objective is a production-usable, Apache-2.0 open-source release. It is still
open. This is the single current work record; the acceptance requirements below
are unchanged. Dated results live in [verification/](verification/).

The requested release target is now **v1.0.0**, conditional on clean-checkout
CI, the unchanged capacity/leakage gate, service/Kubernetes recovery checks and
complete release-bundle verification. Deployment-specific ingress, capacity and
independent-host/zone checks remain the deployer's responsibility; they are not
an implied SLA. The [rc.2 release](https://github.com/viber-ops/configra/releases/tag/v0.1.0-rc.2)
and its checksums remain unchanged. See the [current verification record](verification/2026-09-22.md).

| Requirement | Required evidence | Current state |
| --- | --- | --- |
| Apache-2.0 distribution | License/notice in source, nested module, binaries and images; third-party license inventory | Updated 92-record Go inventory, eight binary collectors and both arm64 image inventories [pass locally](verification/2026-09-22.md#distribution); full new release bundles/download checks remain |
| Open-source maintenance | Contribution/security policies, working private disclosure, continuous checks and dependency updates | Existing policies/private reporting retained; SDK CI passes on GitHub, server CI and manual release gates are prepared; pinned actions and weekly dependency checks retained |
| MySQL 8.0.22 support | Exact-version integration, migration and restore tests; unchanged capacity gate | Current core/E2E/backup tests passed on 8.0.22. A later full run exposed an index-intersection scan in Config history; the query-local fix and expanded read-budget tests now [pass on both Go versions](verification/2026-09-22.md#api-authorization-and-bounded-history-queries). No schema change or 8.4-only requirement |
| MySQL 8.4 comparison | Official compatibility/upgrade sources, separately identified runtime evidence | [Primary-source comparison](research/mysql-8.0.22-and-8.4-compatibility.md) completed; runtime evidence absent |
| Complete authenticated mutation audit | Authenticated rejections have durable, value-free receipts; logical Operation identities survive replay; final image/failure drills pass | Gateway capture and credential-identifier repair implemented; local source image now passes real OIDC rejection receipts, ClickHouse outage/recovery and post-rotation Audit delivery. [Image evidence](verification/2026-09-22.md#actual-service-images-and-dependency-faults); final-release/stress replay remains |
| Bounded management inventories | Server-side limits/pagination, consumer behavior and scale/query evidence | Config/Vault histories, all seven resource inventories, notification subscriptions and Vault usages have SQL/UI paging. Complete impact counts and separate CA trust reads preserve safety across pages. [Earlier four-inventory checks](verification/2026-09-22.md#resource-inventories) and [security inventory checks](verification/2026-09-22.md#security-inventories-and-vault-impact) are local evidence; clean-checkout CI and final-image acceptance remain |
| Key recovery and lifecycle | Correct/wrong-key drills, usable rotation procedure, CA/client lifecycle checks | MySQL 8.0.22 storage/restore/fault checks and [offline rotation in real Kubernetes Pods](verification/2026-09-22.md#offline-key-rotation-in-kubernetes) pass locally: all service Pods stop, both keys are checked before Secret replacement, then SDK/Config/File/ETag, CA issuance/revocation, Audit and fresh native/CSI delivery pass. Final-release and production maintenance controls still need replay |
| Production process and Kubernetes | Exact release images, readiness/shutdown, TLS pass-through, rollout, CSI/sync and dependency failure tests | The [latest server/adapter replay](verification/2026-09-22.md#latest-image-kubernetes-attempts) passes two-worker native/CSI delivery, abrupt worker loss, cross-node leadership, fresh mounts, recovery, offline key rotation and three complete no-retry mTLS rollouts (78 reads). Failed setup/host-resource attempts are retained. Production ingress, independent-host/zone failure and final-release acceptance remain |
| 1000 QPS for ten minutes | Production image, real encrypted Vault reference, mTLS, 2 CPU/512 MiB limit, unchanged gate, leakage checks and recorded host | The [current amd64 source image passes](verification/2026-09-22.md#native-linux-default-profile-ten-minute-gate) on the eight-vCPU Linux host with default query parameters: 600,000 HTTP 200, 999.998 completed/s, p99 2.559 ms, all Access events and leakage checks passing. API limit remains 2 CPU/512 MiB. Failed Mac/two-vCPU runs are retained; final clean-release replay and deployment-specific sizing remain |
| Usable instructions, not slogans | Clean-environment walkthroughs of quickstart, SDK, installation, Kubernetes and backup/restore; failures fixed | [Local protocol and SDK walkthrough](verification/2026-09-12.md#documentation) executed; startup failure fixed in development source, remaining guides still need full execution |
| Chinese and English website | Same-page language switching, corresponding guides, locale-correct links/metadata, build and content checks | Paired deployment/credential/CSI and offline-maintenance guidance updated; 28 pages built, type check and 36 rebuilt-site tests passed. [Local Chromium walkthrough](verification/2026-09-22.md#website-browser-walkthrough) passed 26 pages at desktop/mobile widths, language switching and clipboard checks; deployed-site verification remains |
| Release integrity | Clean commit/tag mapping, tests, license/notices/SBOM and downloadable multi-platform artifacts | SDK v1.0.0 passed four Linux/macOS CI jobs. Server v1.0.0 preparation includes the SDK snapshot fix in the Kubernetes dependency pin; final server gates and downloadable artifacts remain pending |

The latest [API follow-up](verification/2026-09-22.md#api-authorization-and-bounded-history-queries)
also replaces the per-request full Token-grant list with a requested-Environment
lookup, preserves revocation/ETag behavior and repairs a silently skipped Audit
E2E test. Full local core/E2E integration, rebuilt-image startup/distribution,
real OIDC/dependency/recovery checks and a 15-second mTLS/leak smoke pass.
The later native Linux run below passes the capacity gate on its recorded
hardware; earlier shared-host failures remain in the evidence. This is not final
release or deployment sign-off.

A later [native-controller follow-up](verification/2026-09-22.md#native-reconciliation-ordering-and-request-bounds)
reproduced and fixed an overlapping-fetch overwrite and unbounded Kubernetes
request contexts. Dual-toolchain race/vet and vulnerability checks pass. The
rebuilt adapter `776c7ff8073c…` also passes image-distribution checks and the
complete three-node service/failure/rotation drill; this is distinct from the
older adapter's results. Production ingress and independent-host/zone failure
verification remain.

The [document-size follow-up](verification/2026-09-22.md#document-encoding-limits)
then moved output limits ahead of unbounded JSON/YAML allocation in the shared
formatting paths. Its regressions, dual-toolchain checks and real-MySQL tests
pass locally. A separate [JSON parameter repair](verification/2026-09-22.md#mysql-json-parameter-types)
fixes three shared writes that MySQL rejected when parameter interpolation was
enabled. Both query modes pass mutation/replay and Audit regressions, including
quotes, backslashes, Unicode and `NO_BACKSLASH_ESCAPES`. The new server image
`42a1b5120ddd…` contains both fixes and passes distribution/startup checks. Its
explicit interpolation profile subsequently passed the ten-minute gate on the
Mac, without resolving the default-profile failures.

A later [File error-classification fix](verification/2026-09-22.md#file-metadata-failure-classification)
preserves HTTP 503 for database metadata failures instead of reporting a missing
File. Full source integration and the arm64 image `977d21382ba5…` pass startup,
real service/SDK, dependency recovery, backup/restore, offline rotation and the
complete latest-image Kubernetes drill. The two-vCPU Linux host failed both load
profiles. On its eight-vCPU replacement, amd64 image `d61de319c15f…` passes
distribution/startup, full core/E2E/SDK checks, the default-profile ten-minute
capacity/leakage gate and the full native Linux service/fault/recovery drill.
Native Linux also exposed two fixture permission problems, now fixed without
changing production privileges. Remote reports and test backups were retrieved;
the owned fixtures and temporary workspace were removed afterward.

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
[50 Playwright flows](verification/2026-09-22.md#review-regression-fixes-and-stable-release-preparation), covering both locales,
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
failures. The later [eight-vCPU Linux result](verification/2026-09-22.md#native-linux-default-profile-ten-minute-gate)
passes the current source image with default queries and a two-CPU API limit.
Neither validates every deployment. The older result below remains historical,
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
  versions. The explicit server rc.1 retirement is documented above; SDK tags
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
