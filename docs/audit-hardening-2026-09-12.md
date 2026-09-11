# Audit verification — 2026-09-12

Scope: repair the observed Audit delivery failure and cover authenticated HTTP
rejections. This is not a completion claim for the full production contract.

## Reproduced defect

The original worker marked `client_certificate.issued` as `dead` with
`invalid_audit_payload`. The payload used a valid 64-character SHA-256 fingerprint,
while the decoder applied the 63-character Resource Key limit. Other setup events
were delivered. The HTTP regression in `scripts/docs-smoke.mjs audit` failed
because the issued certificate never appeared in Management's Audit query.

The decoder now recognizes typed credential identifiers. After restarting the
worker against the same database, that original event was requeued, appeared in
Management Audit with its full fingerprint, and completed on delivery attempt 2.
No SQL rewrite or deletion of the event was used for recovery.

## Request rejection handling

The Management handler records authenticated mutation rejections before sending
their error response. It stores a fresh Request ID, a controlled error code,
authenticated actor and validated route metadata. It does not store request
bodies, query strings, raw URLs or the caller's unaccepted operation key.

The rejection does not reserve an OperationID. Storage errors marked after a
committed audit transaction retain their original logical Operation audit instead
of adding another gateway event. Invalid resource identifiers are omitted from
validation-failure audit metadata, including when decoding older failed records.

If the receipt cannot be stored, the error is replaced with one value-free
`503 audit_unavailable` envelope and the same Request ID. This is fail-closed
handling, not a promise of durable records while the database itself is unavailable.
Unauthenticated requests and non-mutating preview/validation routes are not
reclassified as audited Mutations.

## Evidence

| Check | Result |
| --- | --- |
| Original issued-certificate Audit query | Failed before fix; passed after worker restart and recovery |
| Viewer denial | `403 forbidden`; searchable by returned Request ID |
| Malformed JSON / unknown JSON field | `400 invalid_request`; value-free receipt found |
| Missing / invalid OperationID | `422 validation_failed`; receipt uses Request ID without reserving an OperationID |
| CSRF rejection | `403 csrf_rejected`; receipt found |
| Unsupported content type | `415 unsupported_media_type`; receipt found |
| Invalid certificate identifier | One logical validation audit; rejected identifier omitted |
| Receipt store unavailable | One `503 audit_unavailable` JSON response; no original error/value concatenation |
| Reuse after a rejected request | Admin can subsequently use the same OperationID successfully |
| Replay of accepted Operation | Original logical audit retained, no new business audit |
| Browser Audit page | Shows Request ID and error reason, supports searching by Request ID |
| Go race suite and vet | Passed |
| Full real-dependency integration and E2E suites | Passed on MySQL 8.0.22, ClickHouse and NATS |
| Browser regressions | 42 passed |

The first combined integration run was invalidated by fixture interference:
the live demonstration worker consumed a test NATS event, and the backup test was
not given the fixture's Compose project name. The demonstration process was
stopped and the correct project identity was supplied; both complete suites then
passed without changing assertions.

## Rollout and remaining checks

See [ADR-0031](adr/0031-correlate-rejected-requests-without-reserving-operations.md).
Deploy Management workers together for the new audit payload, not as a prolonged
mixed-version worker set. ClickHouse request/error columns are added idempotently;
older records keep empty values. Recovery reads bounded pages, retains event IDs
and attempt history, and only requeues records accepted by the strict decoder.

At-least-once delivery can still produce repeated physical ClickHouse rows for
one event ID; the logical identity is the event ID. The healthy-path tests are not
proof of exactly-once physical storage. Final release-image acceptance, large
dead-letter recovery/fault-injection drills, capacity and the other production
requirements remain open.
