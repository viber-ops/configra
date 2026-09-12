# Migration review — 2026-09-11

Historical record: the dependency versions, README instructions, browser counts
and bundle sizes below describe the migration snapshot, not the current release.
See [current status](../production-readiness.md) for subsequent work.

This review covers the service and SDK snapshots prepared for the `viber-ops`
repositories. Each repository starts with its own initial commit. The source
repositories were left unchanged, and their Git history, local caches, generated
certificates, private keys, and development runtime files were not copied.

## Module paths

| Component | Module |
| --- | --- |
| Service | `github.com/viber-ops/configra` |
| SDK | `github.com/viber-ops/configra-go` |
| Integration tests | `github.com/viber-ops/configra/e2e` |

Imports, module requirements, local replacements, and SDK installation instructions
use the new paths. The integration module retains its relative replacements for
the adjacent service and SDK checkouts. The README now documents that layout and
uses `@latest` instead of assuming that a release tag already exists. Private SDK
installation requires GitHub authentication and an appropriate `GOPRIVATE` pattern.

## Review fixes

- YAML alias targets were traversed repeatedly. An escaped Vault reference could
  become an active reference on a later visit, and a Secret whose text resembled a
  reference could trigger another lookup. Two regression cases reproduced these
  failures before the fix. Validation and resolution now remember completed nodes,
  preserving literal values and avoiding exponential traversal of shared alias
  graphs. Tests also cover a 48-level shared graph and rejection of alias cycles.
- The SDK now uses `golang.org/x/sys v0.47.0` and `golang.org/x/text v0.40.0`, matching
  versions already used by the service/integration dependency graph. These updates
  remove [GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024) and
  [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970) from its dependencies while
  retaining the Go 1.25.13 minimum.
- SDK ignore rules exclude local caches, test binaries, and coverage artifacts.
- The Docker compile step now mounts the same Go module cache as the dependency
  download step. Previously the downloaded modules were unavailable in the later
  step, so the build fetched every dependency again.

Manual review focused on module boundaries, SDK reload concurrency, HTTPS/mTLS,
machine authorization, OIDC sessions and CSRF, Vault encryption and resolution,
transaction/outbox handling, outbound notification restrictions, and repository
contents intended for publication.

## Verification

- Service and SDK unit tests passed with `-race -count=1`; `go vet` passed.
- Service and SDK integration tests passed with `-tags=integration -race -count=1`
  against MySQL 8.0.22, NATS, and ClickHouse. This includes the backup/restore tests.
  Integration/load-tagged test code also passed `go vet`.
- The shared-alias regression cases failed before the implementation change and
  passed afterward. The complete integration suite was rerun after the fix.
- The frontend production build and all 36 Playwright tests passed; `npm audit`
  reported no vulnerabilities. The rebuilt embedded bundle matches the source
  snapshot's bundle.
- The SDK cross-compiled for Windows/amd64 after the dependency updates.
- The Linux/arm64 scratch image built with Go 1.26.7. Its readiness, clean shutdown,
  and wrong-Master-Key rejection tests passed with the race-enabled test harness.
- All three modules passed `go mod verify` and `go mod tidy -diff`; Go formatting
  and staged whitespace checks passed.
- Gitleaks v8.30.1 scanned copies of the staged service and SDK files with redaction
  enabled and reported no leaks. Searches found no previous private module host or
  workstation paths in the publication candidates.
- Govulncheck v1.7.0 found no reachable vulnerabilities in either module, no
  vulnerable imported packages in the service, and no remaining SDK advisories.

The MySQL test container initially failed to allocate native AIO (`io_setup:
EAGAIN`). Tests used a separate Compose project with native AIO disabled in an
ignored local override. This changes the test host's I/O mechanism, not the MySQL
version or the committed deployment configuration.

## Remaining observations

The service dependency `golang.org/x/crypto v0.54.0` has four module-level advisories
in packages the service does not import: SSH advisories
[GO-2026-6355](https://pkg.go.dev/vuln/GO-2026-6355),
[GO-2026-6354](https://pkg.go.dev/vuln/GO-2026-6354), and
[GO-2026-6303](https://pkg.go.dev/vuln/GO-2026-6303), plus the unmaintained OpenPGP
package [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932). The SSH fixes are in
v0.56.0, which requires Go 1.26; OpenPGP has no fixed version. Updating the service's
minimum Go version is a separate compatibility decision.

The production image uses `-s -w` to strip symbols. Its binary scan conservatively
reported the same four advisories because govulncheck falls back to module-level
precision for stripped ELF binaries; see its
[binary analysis implementation](https://github.com/golang/vuln/blob/v1.7.0/internal/vulncheck/binary.go).
The Go 1.26.7 Linux/arm64 dependency graph excludes SSH and OpenPGP. A matching
Linux/arm64 build retaining symbols was scanned separately and reported zero
vulnerable symbols or imported packages, corroborating the source analysis.

Vite reports a 781.52 kB minified JavaScript chunk (239.11 kB gzip). This is an
existing performance improvement opportunity; no frontend behavior was changed.

The formal model suite and ten-minute production load certification were not
rerun for this migration. Historical evidence in [production readiness](../production-readiness.md) retains
its original date and is not presented as a new release certification.
