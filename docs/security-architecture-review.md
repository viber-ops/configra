# Security and architecture review — 2026-09-11

Follow-up on 2026-09-12: authenticated request-rejection capture and an observed
credential Audit decoding failure are addressed in
[the audit verification record](verification/2026-09-12.md#audit). The findings
below retain their original review date; see [current status](production-readiness.md)
for remaining work. The full production acceptance is still open.

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
3. **Inventory APIs still return full collections.** Client-side filtering and
   pagination improve rendering, but large installations need server-side bounded
   lists/search and measured database query budgets. Avoid adding shared plaintext
   caches merely to hide this cost.
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
