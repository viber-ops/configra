# Documentation verification — 2026-09-12

This is a record of executed documentation checks, not completion of the full
[production acceptance contract](production-readiness.md).

## Inputs

- Fresh clones of server `v0.1.0-rc.1` (`99e471d`) and SDK `v0.1.0-rc.1`
  (`5bb89dc`) into an empty temporary parent directory.
- Docker on an arm64 macOS host: 18 CPUs and 33,598,365,696 bytes assigned to the
  Docker VM. MySQL runs the exact `mysql:8.0.22` amd64 image.
- One explicitly disposable Compose project named `configra-doc-check-20260912`.
  No production database/cluster or unrelated containers were changed.
- Real MySQL, NATS, ClickHouse and Casdoor 3.158.0, not mocked login responses.
- The server-side fix under test adds the development-only `compose.local.yaml`
  overlay and separates its Compose project from the acceptance fixture.

## Observed results

| Check | Result |
| --- | --- |
| Published tag's unmodified `make local-run` | Failed during MySQL initialization: `io_setup()` returned `EAGAIN` |
| Host AIO counters at failure | 64,976 of 65,536 contexts were already in use |
| Development-only AIO overlay | MySQL, NATS, ClickHouse and Casdoor became healthy; the production/load fixture was not changed |
| Live server query | `SELECT VERSION()` returned `8.0.22`; development `innodb_use_native_aio` was `OFF` |
| Development startup | Installed/built the web UI, prepared development TLS and started Management |
| Management HTTPS readiness | `204`, verified with the generated server certificate rather than disabling TLS validation |
| Anonymous Management query | `401` |
| Real Casdoor Admin login | Authorization code, original state/nonce, PKCE S256 and Configra session callback succeeded |
| Real Casdoor Viewer mutation | Creating an Environment returned `403` |
| Quickstart data | Created `development`, `platform.database` with Text/Secret fields, and `payment` with four-part Vault references |
| Managed credentials | Created a CA, received its first export, issued a client certificate and created an mTLS-required Environment Token |
| Machine API startup/readiness | Independent API process became ready with the same database/Master Key and managed client CA trust; HTTPS readiness `204` |
| Published SDK example | `go run ./examples/basic` from the fresh SDK tag reported `Loaded Config revision 1 (yaml)` |
| Resolved machine read | mTLS read returned Config revision 1, Vault revision 1 and the expected resolved value; the value was compared without printing it |
| ETag | A repeat request with the returned ETag produced `304` |
| Missing client identity | The same Token without its client certificate returned `401`; a subsequent authenticated read still returned `200` |

The protocol checks are implemented in [scripts/docs-smoke.mjs](../scripts/docs-smoke.mjs).
The script requires an explicitly named disposable fixture, refuses a persistent
MySQL data fixture and checks an empty workspace before creating sample data.
It does not automate the browser UI, skip HTTPS verification, or print credential
exports/resolved values. Private test artifacts have owner-only permissions and
must not be committed.

## Documentation corrections made

1. The development databases are **tmpfs**, not persistent volumes. Stopping the
   containers discards data even without `down -v`. The earlier preservation
   wording was incorrect and is corrected in both website languages.
2. The backup script requires an existing output directory. Both language guides
   now include directory creation, the working directory, credential-file shape,
   required access and the expected backup filenames.
3. Homepage/product copy now describes products and operations instead of
   abstract design slogans. The three criticized homepage statements were removed.
4. Every public guide has a Chinese and English route. Type checking and 32 static
   tests cover page/asset links, anchors, matching-page language switches,
   locale-correct navigation and delivered license/notice text.

The AIO workaround is a **new development-source change**, not a property of the
already-published rc.1 tag. Existing tags are not rewritten. The default
production/load configuration still uses its original server command and version.

## Not yet verified by this run

- Browser interaction/visual acceptance of the full bilingual site or all
  Management dialogs.
- A clean, complete production Kubernetes install following the written guide.
- Execution of every revised backup/restore command with the guide's permissions
  and recovery targets. Previous integration evidence is not substituted for it.
- MySQL 8.4 runtime behavior, upgrade or restore compatibility.
- The ten-minute 1000 QPS gate, complete gateway Audit coverage, key-rotation
  recovery, bounded inventories and final release-distribution compliance.

These remain open in [production-work.md](production-work.md).

The live Management log also emitted one `Audit Event delivery did not complete`
warning during credential setup. This run did not inspect the final outbox and
ClickHouse delivery state, so it provides no evidence that every setup audit was
delivered. Reproduce this with the same smoke sequence and inspect durable
delivery/retry outcomes before closing the Audit requirement.

Follow-up: the warning was reproduced and traced to a 64-character certificate
fingerprint being rejected by a 63-character metadata limit. The original event
was recovered and queried successfully after the fix; see
[the audit verification record](audit-hardening-2026-09-12.md) for the executed
tests and remaining acceptance scope.
