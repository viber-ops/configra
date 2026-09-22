# Security and architecture review — 2026-09-11

Follow-up on 2026-09-12: authenticated request-rejection capture and an observed
credential Audit decoding failure are addressed in
[the audit verification record](verification/2026-09-12.md#audit). The findings
below retain their original review date; see [current status](production-readiness.md)
for release acceptance and deployment-specific work.

## Follow-up — 2026-09-22

This pass concentrates on the API process, Kubernetes adapters and Go SDK. Changes
are included in v1.0.0; the published rc.2 artifacts are unchanged.
The [verification record](verification/2026-09-22.md) distinguishes unit,
real-dependency, container and cluster checks.

| Priority | Confirmed problem | Change |
| --- | --- | --- |
| Availability / error contract | The Machine File read treated a database failure while reading field metadata as a missing File and returned HTTP 404. Clients could mistake an outage for a deleted resource. | Propagate the storage error at the shared File read; the HTTP layer returns the existing value-free 503 envelope. A real-MySQL table-unavailability regression fails on the original source and passes after the fix, including recovery and genuine 404 cases. [Evidence](verification/2026-09-22.md#file-metadata-failure-classification). |
| Medium, availability | JSON/YAML checked the 5 MiB result limit only after full encoding. Small templates repeating large Vault values could allocate much more than the limit before rejection; JSON formatting also amplified indentation. | Bound substituted text early, preflight JSON's formatted size and bound YAML writes in the shared canonicalize/resolve/Merge paths. The original-source regression fails; current dual-toolchain checks, exact boundaries, fuzzing and real-MySQL HTTP rejection/recovery pass. [Allocation evidence](verification/2026-09-22.md#document-encoding-limits) is separate from earlier image capacity results. |
| Compatibility / availability | Enabling the MySQL driver's parameter interpolation made Management mutations and rejected-request Audit receipts fail: JSON columns received binary literals. A read-only capacity test did not detect it. | Bind JSON as text at the three shared SQL write sites. Prepared/interpolated and default/NO_BACKSLASH_ESCAPES regressions preserve mutation replay and quoted/Unicode Audit metadata; the interpolation-profile storage and SDK E2E suites pass. [Evidence](verification/2026-09-22.md#mysql-json-parameter-types); defaults remain unchanged. |
| Medium, data integrity | SDK `Snapshot.Unmarshal` could expose shared nested slices/maps through `any` values. Mutating one result changed another reader's result and the supposedly immutable snapshot. | Copy parsed mutable values for each decode. The public Load/Unmarshal regression failed before the fix and passes afterward. No extra parser or dependency. |
| Medium, data loss | Native target cleanup checked ownership and then issued an unconditional Delete. A replacement or ownership change between those requests could remove another writer's object. | Require the observed UID and resource version on Delete. Both Secret and ConfigMap regressions reproduce the race and retain the unrelated object on retry. |
| Medium, data integrity | An older in-flight native fetch could load a newer target's resource version after the fetch, then overwrite that newer delivery. Leader election alone does not fence every overlapping request. | Observe the target before fetching and retain that version for Create/Update. Secret/ConfigMap regressions fail on the old source for both missing and existing targets and pass after the fix; no extra lock or dependency. |
| Medium, availability | Kubernetes credential/spec requests had no per-reconciliation deadline; the 30-second limit covered only Configra reads. | Bound the complete reconcile to 45 seconds, honoring shorter caller deadlines and retaining the 30-second fetch limit. The deadline regression, dual-toolchain race/vet and rebuilt-adapter three-node acceptance pass. |
| Medium, availability | The HTTP socket write timeout did not bound database work. SCS's MySQL adapter used non-context SQL, including cleanup that could delay shutdown. | Give requests a 20-second context deadline; implement SCS's existing context-store interface with five-second SQL bounds and cancellable, batched cleanup. Keep the same MySQL 8.0.22 table and session security settings. |
| Medium, availability | Every Machine Config/File request loaded and sorted all of its Token's Environment grants, including conditional reads. Cost grew with the grant count before checking the secret. | Use one indexed existence check for the requested Environment in the existing Token query. Keep digest, expiry, revocation and certificate checks; add no permission cache. Real MySQL tests compare one and 1,005 grants; SDK tests cover grant removal/restoration, archive/unarchive and Token revocation with unchanged ETags. |
| Medium, error-path disclosure | Session failure logging included the raw request path before routing/validation. A real MySQL-stall drill placed a private sentinel in a malformed resource path and found it in the Management image's log. | Remove the unvalidated path field at the shared session error callback. Preserve the generic error and HTTP method. The image regression keeps logs from both sides of container recreation and checks that the sentinel is absent after the fix. |
| Medium, rollout availability | A real Kubernetes API rollout failed a machine read when an old Pod stopped. The base had no endpoint-drain delay; `maxUnavailable: 0` did not prevent listener shutdown racing Service routing updates. | Add kubelet's native five-second `preStop.sleep` to both service Deployments, within the existing termination budget. Three complete local API rollouts pass 97 sampled mTLS reads without retries. Kubernetes version requirements and ingress-specific limits are documented; this is not a general zero-downtime guarantee. |
| Medium, availability | Config/Vault history reads returned every revision. Long-lived resources caused ever-growing SQL results, JSON responses and browser state. Later MySQL 8.0.22 runs also exposed an index-intersection/filesort plan in the first paged Config query. | Query by the existing resource/revision primary key, with an exclusive revision cursor and at most 101 rows for a 100-item page. Keep Config history on that primary key with a query-local index hint. Update history, comparison and transfer consumers; never fetch all pages automatically. Real HTTPS/MySQL tests cover concurrent writes, isolation, archive visibility and value-free responses; expanded storage counters guard against the observed scan. |
| Medium, availability | Environment, Config, Vault and Token inventories returned all parents and associations; client-side pages did not bound SQL results or browser memory. | Page parents in SQL, cap association previews at three and expose complete counts/scoped Environment pages. Search, overview, detail lookup, selectors and resource rails use the new contract. MySQL 8.0.22 tests exercise 1,005 of each resource and a 1,005-environment association set. Counts/search remain potentially linear, not constant-time queries. |
| Pagination interaction risk | Feeding one page of checkboxes or a three-item summary into the existing Token grant replacement would remove unseen grants. | Add an Admin-only, atomic, audited grant PATCH; preserve legacy PUT operation digests. The UI tracks explicit additions/removals across pages. HTTP/storage/browser tests retain untouched grants and reject revoked Tokens, archived additions and overlapping edits. |
| Correctness | CSI accepted both `database` and `database/client.pem` in one mount, although one path cannot be both file and directory. | Reject prefix collisions in the shared validator, in either order, before contacting Configra. |
| Input validation | SDK initialization accepted `https://:443`, which has a port but no hostname. | Validate the parsed hostname in the common constructor. A regression first reproduced the acceptance; environment/file helpers share the corrected check. |
| Recovery correctness | The ADR named a `doctor --verify-vault` command that did not exist. Readiness and the Sentinel did not detect corrupt historical Vault revisions, CA keys or inactive notification credentials. | Add an actual read-only CLI using a consistent snapshot and shared crypto/schema checks. The real MySQL 8.0.22 restore drill runs it with SELECT-only permissions, proves failure without initialization on an empty target, detects historical corruption, and issues a client with the restored CA. Doctor does not rotate keys or prove backup completeness; offline rotation is a separate command. |
| Error disclosure / stored-input validation | Bootstrap YAML decoder errors could echo malformed values. Notification credential decoding was duplicated and accepted a trailing JSON document. | Bound YAML reads and omit decoder values from errors; share the credential decoder across mutations, delivery and recovery, require EOF, and validate the stored URL/secret limits. Add malformed-input and value-exclusion regressions. |
| Key lifecycle / data integrity | Replacing the bootstrap key stranded existing data, and already-running writers could create records with the old key. A lost COMMIT reply could be mistaken for rollback. | Add an offline, audited InnoDB rewrap transaction and database-name/offline confirmations. Current-code encrypted writers verify and lock the Sentinel; readiness rejects a stale key. Real MySQL tests cover history/content retention, partial-write rollback, queued writers, nontransactional storage and a dropped COMMIT acknowledgment. The local image/cluster drills below now pass too; this is not online rotation or final-release acceptance. |
| Dependency security | Newly published gRPC advisories affect the pinned version. | Upgrade to 1.83.2 and review the five changed shipped module versions. Current Kubernetes source scanning reports no findings. |

The gRPC [HTTP/2 fragmentation advisory](https://pkg.go.dev/vuln/GO-2026-6348)
applies to a transport used by the provider. Its listener is a root-owned `0600`
Unix socket for the trusted CSI driver, not a public TCP endpoint. The separate
[missing-authority panic advisory](https://pkg.go.dev/vuln/GO-2026-6443) requires
xDS routing, which this program does not enable. A scanner call path alone does
not prove that panic is exploitable here. Version 1.83.2 covers both advisories.

The Snapshot mutation finding is distinct from September 11's non-reproducing
decoder-error disclosure suspicion; it is not a claim that error messages leaked
configuration. SDK certificate-rotation guidance also now distinguishes closing
idle connections from replacing an identity on active HTTP/2 connections.

CI and weekly dependency-update configurations were added for service and SDK.
They use pinned actions, read-only checkout credentials, race/vet checks and
vulnerability scanning; service CI also includes real dependency and UI tests.
SDK CI has passed on Linux and macOS. The server's manual CI run also exercises
the ten-minute capacity gate and three-node Kubernetes recovery drill. Passing
ordinary unit/UI jobs alone does not replace those acceptance checks.
The integration recipe also omitted NATS/ClickHouse settings from its E2E process,
silently skipping the durable rejection/replay Audit test. It now passes all
three dependencies; a Make dry-run regression checks this, and the actual Audit
test has passed with the disposable services.

Credential/notification/usage pagination and their cross-page safety checks now
pass locally; see the [September 22 evidence](verification/2026-09-22.md#security-inventories-and-vault-impact).
The [read-only recovery check](verification/2026-09-22.md#encrypted-state-recovery)
and [offline key rotation](verification/2026-09-22.md#offline-master-key-rotation)
also pass locally. [Actual service-image checks](verification/2026-09-22.md#actual-service-images-and-dependency-faults)
now cover real HTTPS OIDC, dependency stalls, Audit recovery, encrypted restore,
rotation and post-recovery SDK/CA behavior. [Real Configra Pods](verification/2026-09-22.md#actual-configra-services-in-kubernetes)
also pass both adapters, credential revocation/rotation, API outage retention,
controller failover and three API rollouts. A subsequent
[offline cluster rotation drill](verification/2026-09-22.md#offline-key-rotation-in-kubernetes)
also passes: complete service shutdown, backup, same-image maintenance Pods,
old-key rejection/new-key verification, bootstrap Secret replacement and fresh
SDK/CA/Audit/native/CSI reads. The later
[final-runtime native Linux acceptance](verification/2026-09-22.md#final-runtime-native-linux-acceptance)
passes capacity, service faults, backup recovery and offline rotation from the
clean release runtime. Production ingress/independent-host failures and
deployment-specific maintenance controls still require the operator's own drill.
The current server image also passes a [three-node worker-loss drill](verification/2026-09-22.md#three-node-provider-and-service-failover):
native/CSI reads on both workers, cross-node sync leadership, existing and fresh
mounts on the survivor, recovery and three API rollouts. Requests are checked
after failed-node endpoint removal; this does not claim uninterrupted reads
during failure detection. All test nodes share one Docker host.
The later [reconciliation ordering and deadline fixes](verification/2026-09-22.md#native-reconciliation-ordering-and-request-bounds)
have separate failing/passing regressions and a rebuilt-adapter cluster replay,
including 79 no-retry rollout reads and fresh native/CSI delivery after rotation.
The [new capacity record](verification/2026-09-22.md#capacity-and-leak-detection)
includes both a passing baseline and subsequent failing runs. It also corrects
a test-only leakage scan that put fixture secrets into ClickHouse query logs,
adds direct NATS payload checks and retains private diagnostics on failure.
These changes do not establish production capacity or a production data leak.
The subsequent [API query review](verification/2026-09-22.md#api-authorization-and-bounded-history-queries)
records the per-request grant fix and the reproduced MySQL history-plan failure,
including the unsuccessful first fix. Both query regressions, full core/E2E
integration, the rebuilt image's service/recovery drill and its short mTLS/leak
smoke now pass. The prior long-run capacity failures remain open.
The following September 11 review
is historical evidence, not a statement that those requirements have passed.

Scope: service and Go SDK, management interactions, certificate lifecycle, and the
new Kubernetes adapters. Evidence combines source inspection, failing/passing
regression tests, race-enabled tests, dependency analysis, and container/cluster
checks. This is a code and design review, not a certification of an unspecified
production deployment.

## Findings addressed

| Priority | Finding | Resolution and evidence |
| --- | --- | --- |
| High | Vault detail state could survive navigation to another item with the same revision/variant identity. Previously fetched values could then appear under the new item without a new reveal. | Full resource identity now keys the detail component. `web/tests/security.spec.js` reproduced the missing reveal gate and now verifies that each item requires explicit access. |
| High | Notification delivery claimed and incremented attempts for up to 1000 targets before sending them, inside a ten-second work context. Cancellation could consume attempts for targets never sent and omit the attempted delivery record. | One outbox event is claimed at a time, targets are claimed immediately before sending, work ends before its lease expires, and attempted outcomes get a bounded recording context. `TestNotificationCancellationRecordsAttemptWithoutClaimingUnsentTargets` failed with two claims and zero records before the fix, then passed with one claim and one record. |
| Medium | The token UI silently selected non-expiring tokens despite the server's 90-day default. | Non-expiring credentials now require explicit selection. Browser tests verify both default expiry and the intentional exception. |
| Medium | Replacing `http.DefaultTransport` with an application tracing/wrapping transport could panic the SDK constructor. | The SDK owns its transport. A public-constructor regression reproduced the panic and now passes. Configurable lower response limits also bound constrained consumers. |
| Medium | The new Kubernetes dependency graph initially included reachable gRPC and HTTP/2/IDNA vulnerabilities. | gRPC was updated to v1.82.1 and `x/net` to v0.55.0. The resulting source scan reports no vulnerable symbols or imported packages. |
| Correctness | Global search/inventory could remain stale after resource mutations; long Vault field inventories produced excessive rendering and scrolling. | Successful resource mutations refresh shared inventory; details have contextual navigation, field search, 25-field pages, and an explicit conceal action. |

The dependency findings were [GO-2026-6061](https://pkg.go.dev/vuln/GO-2026-6061),
[GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026), and
[GO-2026-4918](https://pkg.go.dev/vuln/GO-2026-4918). A suspected SDK decoder-value
leak did **not** reproduce; its confidentiality regression passed without changing
decoder behavior and is not reported as a fixed vulnerability.

## Security properties checked

- Machine reads still require an active Environment-scoped Token and, by default,
  a registered client certificate. Presented certificates are never ignored by
  Token-only credentials. Constant-time token-digest comparison and expiry checks
  remain in place.
- OIDC sessions retain secure, HttpOnly, host-scoped cookies, nonce/state/PKCE
  validation, and standard cross-origin protection. Management credential
  operations require Admin authorization. The existing CSP uses style nonces;
  the editor extraction preserves that mechanism.
- Vault snapshots and CA signing keys use the existing authenticated encryption
  module. CA key encryption additionally binds its authority identity and public
  certificate. Client private keys and credential exports are excluded from
  durable replay/audit data.
- Concurrent CA creation through separate database connections produces exactly
  one private export. Revocation is checked on each authorized read, including
  an already-established mTLS connection. Issuer expiry is also enforced.
- Kubernetes callers cannot override the Configra origin or server trust through
  bindings/SPC parameters. Paths are bounded and traversal-safe, failures return
  no partial CSI file set, and native writes require matching owner references.
  Credential references and native targets remain in the watched namespace.

## Architecture assessment

The separation between human management, machine reads, transactional MySQL
state, and asynchronous log delivery is appropriate for this workload. MySQL
transactions provide consistent resolution and mutation/outbox atomicity. NATS
Access delivery stays best effort, while Audit uses durable outbox delivery.
Management's NATS consumer uses a queue group, and operation locks plus shared
session storage support multiple backend replicas.

Managed PKI remains behind a small authority interface and uses shared database
state. API replicas refresh immutable public trust snapshots independently of
handshakes, avoiding a new database query for every untrusted TLS connection.
Revocation authorization does not wait for that cache refresh. Bootstrap secrets
stay external, so running Configra in Kubernetes does not create a self-startup
dependency on its own provider.

Kubernetes code has its own Go module and image, isolating the substantial
controller/CSI dependency graph from the core API process. Both adapters reuse the
SDK. Native reconciliation checks current spec generation, writes complete
objects, protects unrelated ownership, retains previous data on failure, and
avoids writes when content/metadata are unchanged.

Verification also caught an HTTP/2 negotiation regression in the new dynamic
TLS configuration. Per-handshake configurations now carry the HTTP server's ALPN
protocols, with a regression test asserting HTTP/2 rather than a silent HTTP/1.1
downgrade.

The management UI now separates navigation, resource selection, and detail work.
Its initial JavaScript fell from approximately 782 kB to 353 kB (about 99 kB gzip)
by loading the 447 kB editor chunk only where needed. This improves initial-load
cost; it is not a substitute for production latency measurement.

## Design limits and follow-up

1. **Environment-wide authorization is not workload isolation.** Tokens granting
   the same Environment can read the same Configs and Vault values, irrespective
   of Kubernetes namespace or client certificate identity. Human Admin/Viewer
   roles are also workspace-wide. This is suitable for a mutually trusted team;
   untrusted applications/tenants need narrower resource grants or separate
   Configra trust domains. The Kubernetes adapter does not pretend to narrow an
   existing server grant.
2. **Authenticated rejection coverage was incomplete at review time.** Malformed
   JSON, missing/invalid operation IDs and Viewer-forbidden mutations could be
   rejected before storage wrote an audit. The September 12 follow-up added
   value-free gateway receipts keyed by Request ID without duplicating completed
   operation audits. See [executed checks and remaining recovery drills](verification/2026-09-12.md#audit).
3. **Bounded responses do not imply constant-time queries.** Config/Vault history
   and all seven management resource inventories now have SQL/UI paging, as do
   notification subscriptions and current Vault usages. CA trust loading is a
   separate complete read; Vault edit impact uses a complete count, not a visible
   page. Resource counts, substring searches and deep offsets can still scan many
   rows. The thousand-row checks are not a capacity result. Avoid shared plaintext
   caches to hide query cost. Vault impact is a live UI warning, not a write-side
   lock against concurrent Config reference changes or a requirement on direct API writes.
4. **Management's repository facade and main UI file remain broad.** Authority
   handling and editors have been separated, but Config/Vault/Token workflows
   still share substantial orchestration. Further extraction should follow real
   domain interfaces rather than adding pass-through layers.
5. **CA custody has the same trust assumptions as the existing Master Key.** A
   database-only disclosure does not reveal signing keys; compromise of both the
   database and Master Key does. Use separate bootstrap access, tested recovery,
   and strong administrator authentication. Automated Master Key rotation/HSM
   custody remains outside this implementation.
6. **Deployments require operational configuration.** Management's embedded
   versioned assets need coordinated rollout; the base keeps its replacement
   strategy. The CSI provider is a trusted node extension with a hostPath socket.
   Sync controllers need namespace Secret permissions and should not run where
   the service Master Key is stored. Network policy, backup retention, HA database
   operation, and real OIDC configuration are deployment-specific.

Source scans can still report advisory-bearing modules for packages not imported
by these programs (for example the unmaintained OpenPGP package in `x/crypto`).
Stripped ELF scans conservatively report whole-module findings; they must not be
misrepresented as proven reachable calls. See the earlier
[migration review](verification/2026-09-11-migration.md) for that scanner limitation.

The historical ten-minute load evidence retains its original date. A new full
production certification, external penetration test, and infrastructure recovery
drill are not implied by these changes.

## Validation record for this change

- Core service and sibling SDK race suites, service/SDK integration tests with
  MySQL 8.0.22, NATS and ClickHouse, schema migration, backup/restore coverage,
  authority concurrency, imported issuer association, certificate tamper rejection,
  and existing-connection revocation checks passed.
- The production scratch image built; readiness, shutdown and wrong-Master-Key
  rejection tests passed. Both service and Kubernetes images use the pinned
  Go 1.26.7 builder.
- The Kubernetes module's tests, real gRPC wire checks and source vulnerability
  analysis passed with zero vulnerable symbols/imported packages. Its disposable
  Kubernetes 1.35.0 cluster test used CSI Driver 1.6.1 and completed 36 authenticated
  mTLS reads, verifying native Secret/ConfigMap updates, non-root CSI reads,
  file rotation, no consumer restart, and unchanged existing process environment.
- Browser regression coverage includes 40 functional cases and a workspace
  keyboard/filter/responsive case. Light, dark and mobile screenshots were
  inspected; the obsolete mobile layout rules and editor-loading placeholder
  behavior were corrected during that review.
- Gitleaks v8.30.1 scanned the publication candidates with redaction enabled and
  reported no leaks. All 41 browser tests and npm dependency audit passed; Go
  formatting, module verification, vet, and staged whitespace checks passed.

**The 1000 QPS gate did not pass in this environment.** A 3-second warmup returned
all requests successfully but did not meet rate/throughput requirements. A longer
15-second warmup of the unchanged gate completed 13,918 requests at 927.81 offered
requests/s and 883.68 successful responses/s, with two five-second client timeouts.
The measured phase therefore did not run. These runs used the shared Docker host
and MySQL with native AIO disabled; they establish neither a production capacity
claim nor an isolated comparison against the previous version. Keep this as a
release blocker for any promised 1000 QPS deployment and profile it on a controlled
host before promotion. No throughput threshold was lowered to obtain a pass.
