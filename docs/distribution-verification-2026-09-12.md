# Distribution verification — 2026-09-12

Project-license delivery, a reviewed Go runtime notice collection, and the public
SDK pin are implemented and verified. This does **not** complete the full
distribution audit or the [production acceptance contract](production-readiness.md).

## Published SDK

[SDK v0.1.0-rc.2](https://github.com/viber-ops/configra-go/releases/tag/v0.1.0-rc.2)
points to `bddf8e91cb73ff6fddafbf11b890745a637ea014`. Runtime source is unchanged
from rc.1; the release adds Apache-2.0 LICENSE/NOTICE, contribution/security
policies and corrected English documentation links. Existing tags were not changed.

The [public Go proxy ZIP](https://proxy.golang.org/github.com/viber-ops/configra-go/@v/v0.1.0-rc.2.zip)
was fetched with the public proxy/checksum database explicitly enabled and private
module exclusions cleared. Its LICENSE and NOTICE matched reviewed source byte
for byte. The downloaded origin was the commit above; its module sum was
`h1:r2ObWg0WK5yhO03gvDypfReW4uIAJYMIgtEUq4ILnwQ=`.

Kubernetes now uses that version in both `go.mod` and its actual image binary.
Its Docker build uses the Configra checkout as its context, without an adjacent
SDK, local `replace`, or GitHub credentials.

## Candidate artifacts

Source: `29b52e5f5838dffbcee3f88da3dd6ff0650b2cfa`, built in a clean detached
worktree. Bundle name: `v0.1.0-rc.2-review.2`. These server bundles are **local
validation artifacts, not published releases**. The host was macOS arm64 with
Docker Desktop's Linux arm64 runtime. Go 1.26.7 built the binaries with cgo disabled.

```sh
# From a clean checkout of that source commit, with a fresh output directory:
node scripts/build-release.mjs v0.1.0-rc.2-review.2
node scripts/verify-release.mjs dist/v0.1.0-rc.2-review.2
make image-license-test \
  IMAGE=configra:go-notices-check-20260912 \
  KUBERNETES_IMAGE=configra-kubernetes:go-notices-check-20260912
```

| Check | Result |
| --- | --- |
| Four tar archives | Passed checksums, project/Go notice presence, binary target/toolchain, disabled cgo, no local dependency replacements, and actual SDK pin. The verifier reads archives, not just staging files. |
| Eight binary build records | Both commands, macOS/Linux × amd64/arm64, matched BUILD.json targets and the pinned SDK where applicable. |
| Runtime `--version` | Both commands passed on native macOS arm64, Linux arm64 and emulated Linux amd64. Output matched the candidate version and complete commit. macOS amd64 was inspected, not executed. |
| Both scratch images | Copied out actual files and compared project LICENSE/NOTICE plus all 40 Go notice inputs byte for byte. CLI passed as UID 65532 with read-only filesystem, no network and no capabilities. |
| Go notice collection | Includes the source-embedded terms in the [reviewed manifest](research/distribution-license-requirements.md). Complete source files retain their bytes, with `.txt` appended to source filenames. |
| Unsupported profiles | Go 1.25.13, cgo enabled, `boringcrypto`, and Linux/riscv64 each failed before creating output. These are distribution-profile guards, not removal of source compatibility. |
| Source checks | Go 1.26.7 race tests and vet passed for service, SDK and Kubernetes. Service race tests also passed in the clean worktree after populating `dist/`. |

Verified local image IDs:

- Configra: `sha256:9703a8273c0973f7f97c9d1a37eac2c7d4947ecd43c11484c9c09ca48e0e314b`
- Kubernetes: `sha256:6de76f094dd0ea8ef90900f73b01458c38494133edf86e1f712eb8e6ef317b97`

Archive SHA-256 values (not a reproducible-build promise):

```text
bfae32ceaeee259179275a0d60c5d1329d543c0efd81e1fd234230e849fc34c1  configra_0.1.0-rc.2-review.2_darwin_amd64.tar.gz
d83cadd79f3bfde0ff6b188299eedfab218a7ecce3ec378247b55e89750c4325  configra_0.1.0-rc.2-review.2_darwin_arm64.tar.gz
6abb3929a62e0ae881d1b89137cee1c69520af98494ba89e59a82fb8a4e782a8  configra_0.1.0-rc.2-review.2_linux_amd64.tar.gz
19ef35ee15a528cbd83cc2fda3162cff3d40e2e7d9a7c4352c4a2890ebb82482  configra_0.1.0-rc.2-review.2_linux_arm64.tar.gz
```

## Failures reproduced

- The rc.1 archive failed with `missing LICENSE`. The first project-license-only
  candidate subsequently failed the Go notice check. The final local candidate
  passed both.
- The earlier `configra:review-20260911` image lacked
  `/licenses/configra/LICENSE`; the actual-image verifier rejected it.
- Raw `.go` notice files under `dist/` made `go list ./...` fail with disallowed
  standard-library internal imports. The process-level regression reproduced
  that problem, then passed after adding `.txt` suffixes without changing contents.

## Vulnerability scan scope

`govulncheck v1.7.0` with Go 1.26.7 reported zero reachable-symbol and zero imported-
package vulnerabilities. Service and Kubernetes were additionally scanned for
Linux arm64 with cgo disabled, using a host-native scanner executable with target-
specific analysis settings. The SDK also had no module-only findings. The other
two scans retained these module-only findings:

| Module | Findings outside the imported package graph | Follow-up |
| --- | --- | --- |
| Service `golang.org/x/crypto v0.54.0` | SSH [GO-2026-6355](https://pkg.go.dev/vuln/GO-2026-6355), [GO-2026-6354](https://pkg.go.dev/vuln/GO-2026-6354), [GO-2026-6303](https://pkg.go.dev/vuln/GO-2026-6303) | Review a compatible update; fixes are in v0.56.0 for the first two and v0.55.0 for the third. |
| Service `golang.org/x/crypto v0.54.0` | OpenPGP [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) | No fixed version is listed. Do not introduce this unmaintained package into the import graph. |
| Kubernetes `golang.org/x/net v0.55.0` | DNS parser [GO-2026-5942](https://pkg.go.dev/vuln/GO-2026-5942) | Review a compatible v0.56.0 update. |

These scans do not establish reachable Configra vulnerabilities, nor do they
establish that every required module is vulnerability-free. Recheck target-
specific reachability and dependency changes in release CI.

## Remaining work

- Application-module and bundled UI transitive notices, including file-level
  exceptions and supplemental AUTHORS/PATENTS material.
- MySQL-driver covered-source access and the container CA bundle's license,
  source and provenance materials.
- SBOM, final artifact reconciliation, and needed supplements for already-
  published rc.1 artifacts without rewriting tags or existing artifact bytes.
- Final process/migration/recovery and Kubernetes acceptance, the unchanged
  ten-minute capacity/leakage gate, and the rest of the production matrix.

No database or Kubernetes cluster was started or changed. Inspection and binary-
smoke containers were removed on success and failure. Local images, build cache
and candidates remain for follow-up. MySQL 8.0.22 remains the required baseline.
This run is not stable-release or legal-compliance certification.
