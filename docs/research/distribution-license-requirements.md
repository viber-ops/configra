# Distribution license materials for Configra

Research dates: 2026-09-11–12. Scope: materials accompanying the compiled Configra
and Kubernetes binaries, embedded web UI and scratch-image CA bundle. This is an
engineering inclusion manifest, not legal certification. The
[Go inventory](transitive-go-license-inventory.md) records the reviewed direct and
transitive modules; [UI metadata](../../web/scripts/reviewed-licenses.json) records
the collected npm materials. Later [artifact checks](../verification/2026-09-12.md#distribution)
passed for local candidates. Final published-candidate checks and prior-release
supplements remain open in [production readiness](../production-readiness.md).

## Observed distribution inputs

The eight existing `v0.1.0-rc.1` binaries under `dist/v0.1.0-rc.1/configra_0.1.0-rc.1_{darwin,linux}_{amd64,arm64}/` were inspected with `go version -m`, without executing them. All report Go `1.26.7` and `CGO_ENABLED=0`. Each server binary lists 31 dependency modules; Kubernetes lists 67 on Darwin and 68 on Linux. The server includes `go-sql-driver/mysql v1.10.0`; Kubernetes includes several YAML modules and the older SDK pseudo-version recorded in the earlier inventory. These observations are not a claim about subsequently rebuilt binaries.

At the pre-`9670636` inspection snapshot, the server Dockerfile built and embedded
the Vite output. Both final scratch stages copied the executable and CA bundle,
without explicit license-material copies. The three then-current `web/dist`
JS/CSS assets had no MIT permission/disclaimer text, React/CodeMirror copyright
strings or accompanying LICENSE/NOTICE asset. The Kubernetes build used a sibling
SDK replacement, so its provenance differed from the version named by `go.mod`.
These are historical findings, not descriptions of the current Dockerfiles.

Commit `9670636` added first-party LICENSE/NOTICE packaging, pinned SDK rc.2 and
removed the sibling replacement. Subsequent collectors added Go/runtime/UI/CA
materials and covered source. Their local archive/image evidence is recorded
separately; it does not certify every published artifact. Current implementation:
[archive packaging](../../scripts/build-release.mjs), [server image](../../Dockerfile),
[Kubernetes image](../../kubernetes/Dockerfile).

| Shipped component | Materials to include with that artifact |
| --- | --- |
| Either Go executable | Project LICENSE/NOTICE, applicable dependency licenses and attributions, required covered-source access information, Go runtime/standard-library notices, and build/dependency provenance. |
| Configra server's embedded UI | Complete applicable npm copyright/license texts, including bundled transitives, in distributed UI assets or accompanying accessible materials; verify the embedded build includes them. |
| Scratch image containing the copied CA bundle | The CA copyright/license information and source/provenance record described below, in addition to the executable's materials. |
| Source/module archives | Project and applicable dependency source notices, modification notices where required, and license-bearing first-party module versions. Adding a local LICENSE does not alter an older downloaded module zip. |

Filenames and directories such as `licenses/`, `THIRD_PARTY_NOTICES`, or `SOURCE_ACCESS` are implementation choices; satisfying the applicable terms is the requirement.

## Earlier direct-dependency review and SDK archive gaps

The September 11 review covered every non-`indirect` Go requirement and all ten
UI runtime npm dependencies, plus `ch-go`. All 27 cited third-party Go files
matched pinned upstream bytes; all ten npm archives matched lockfile integrity
and inspected license texts. The larger inventories linked above replace those
duplicated package tables. The original dated report in the [review archive](https://github.com/viber-ops/configra/releases/download/v0.1.0-rc.2/review-records-2026-09-12.tar.gz)
retains its exact inputs and citations.

The review distinguished manifest directness from runtime use: `go-jose` has
direct test imports but also ships through OIDC, and `client-go` ships through
controller-runtime. SCS's MySQL submodule inherits the repository-root MIT text;
Viper v1.21.0 is MIT, whereas Cobra v1.10.2 is Apache-2.0. Preserve package-specific
texts and provenance, not a license inferred from a related package.

The canonical SDK rc.1 ZIP contained 17 files, with no LICENSE, NOTICE or COPYING
under `github.com/viber-ops/configra-go@v0.1.0-rc.1/`. The older pseudo-version ZIP
contained nine files with the same absence. This was checked in downloaded
archives, not inferred from local source. [rc.1 ZIP](https://proxy.golang.org/github.com/viber-ops/configra-go/@v/v0.1.0-rc.1.zip),
[pseudo-version ZIP](https://proxy.golang.org/github.com/viber-ops/configra-go/@v/v0.0.0-20260911101812-8e4339f8884e.zip).
The later [rc.2 ZIP verification](../verification/2026-09-12.md#sdk-release)
confirmed LICENSE/NOTICE against commit `bddf8e91cb73ff6fddafbf11b890745a637ea014`.
It does not change the older archives. Other historical module ZIPs were outside
the direct review's scope.

## Application dependency materials

**Apache-2.0.** Supply the license, preserve relevant upstream NOTICE attributions, preserve applicable notices in distributed source, and identify modifications to covered files. A general statement that dependencies retain their licenses does not reproduce those materials. The known direct notices include CoreOS go-oidc's NOTICE, gRPC's NOTICE.txt, and YAML's NOTICE; the transitive inventory can add others. [Apache-2.0 section 4](https://www.apache.org/licenses/LICENSE-2.0), [pinned CoreOS NOTICE](https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/NOTICE), [pinned gRPC NOTICE](https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/NOTICE.txt)

**MySQL driver, MPL-2.0.** When distributing a binary containing the driver, make its covered source available under MPL-2.0 and tell recipients how to obtain that source. Preserve its license/copyright notices and any modifications to covered files. A practical package can include the exact driver source tree with LICENSE/AUTHORS, plus a short source-access notice identifying its version and source location. Alternatively, provide a maintained, version-specific source-access route; do not merely link a moving default branch. Separately authored Configra files may retain Apache-2.0. [MPL sections 3.1–3.4](https://www.mozilla.org/en-US/MPL/2.0/), [Mozilla FAQ Q8 and Q11–Q13](https://www.mozilla.org/en-US/MPL/2.0/FAQ/), [driver source at the v1.10.0 commit](https://github.com/go-sql-driver/mysql/tree/a065b60ab6d0c8e15468e7709c7f76acf4431647)

**Mixed YAML.** Preserve the actual LICENSE and NOTICE, not a single Apache label: `go.yaml.in/yaml/v3 v3.0.4` assigns MIT to eight libyaml-derived files and Apache-2.0 to the rest. Kubernetes also reports `go.yaml.in/yaml/v2`, `gopkg.in/yaml.v3`, and `sigs.k8s.io/yaml`; classify each pinned artifact separately rather than transferring the v3 result to all of them. [Exact v3 file allocation](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE), [v3 NOTICE](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/NOTICE)

**MIT/BSD npm and Go dependencies.** Keep the copyright, permission/conditions, and disclaimer text required by each license. All ten direct npm runtime packages were verified as MIT in the earlier inventory, but this does not classify their transitives. Shipping license files only in development `node_modules` does not deliver them with the served, minified UI. Include the selected texts in the release assets and verify their availability from the embedded UI distribution. [React 19.2.8 archive, `package/LICENSE`](https://registry.npmjs.org/react/-/react-19.2.8.tgz), [CodeMirror 6.0.2 archive, `package/LICENSE`](https://registry.npmjs.org/codemirror/-/codemirror-6.0.2.tgz), [OAuth2 BSD license](https://go.googlesource.com/oauth2/+/4d954e69a88d9e1ccb8439f8d5b6cbef230c4ef9/LICENSE)

## Go 1.26.7 inclusion manifest

Go's root BSD-3-Clause license requires its copyright, conditions, and disclaimer in the documentation/materials accompanying binary distributions. The root license is not a statement that every bundled source file uses the same terms: Go's README expressly recognizes exceptions. Include the root LICENSE; retaining PATENTS alongside it preserves Google's separate patent-grant text as a packaging recommendation. [Go 1.26.7 LICENSE](https://github.com/golang/go/blob/go1.26.7/LICENSE), [PATENTS](https://github.com/golang/go/blob/go1.26.7/PATENTS), [README](https://github.com/golang/go/blob/go1.26.7/README.md)

To identify relevant material, `go list -deps -json` was run with Go 1.26.7, `CGO_ENABLED=0`, `GOWORK=off`, read-only module mode, and offline dependency resolution for both commands on Linux/Darwin and amd64/arm64. The source-selection union contained 1,534 standard-package Go/assembly/header files. Standard package counts were 207–208 for the server and 227–228 for Kubernetes. Legal/README files in their directory ancestry and source-embedded notices were inspected. **Package-selected source is broader than the set of functions surviving the linker.** The following is a conservative inclusion set, not a symbol-level proof that every named implementation is present in every binary.

Preserve these paths from the exact toolchain tree, retaining directory structure under a clearly identified `go1.26.7` notices directory. Copying the whole listed source files is a straightforward way to retain complete embedded headers; a reviewed notice extractor may retain the full applicable blocks instead.

```text
LICENSE
PATENTS
README.md
src/README.vendor
src/vendor/golang.org/x/crypto/LICENSE
src/vendor/golang.org/x/crypto/PATENTS
src/vendor/golang.org/x/net/LICENSE
src/vendor/golang.org/x/net/PATENTS
src/vendor/golang.org/x/sys/LICENSE
src/vendor/golang.org/x/sys/PATENTS
src/vendor/golang.org/x/text/LICENSE
src/vendor/golang.org/x/text/PATENTS
src/crypto/internal/boring/LICENSE
src/crypto/internal/boring/README.md
src/crypto/internal/fips140/nistec/fiat/README
src/crypto/internal/fips140/edwards25519/scalar.go
src/crypto/internal/fips140/aes/aes_generic.go
src/runtime/memmove_amd64.s
src/internal/profile/graph.go
src/math/acosh.go
src/math/asinh.go
src/math/atan.go
src/math/atanh.go
src/math/cbrt.go
src/math/erf.go
src/math/exp.go
src/math/exp_amd64.s
src/math/expm1.go
src/math/gamma.go
src/math/j0.go
src/math/j1.go
src/math/jn.go
src/math/lgamma.go
src/math/log.go
src/math/log1p.go
src/math/remainder.go
src/math/sin.go
src/math/sqrt.go
src/math/tan.go
src/math/tanh.go
```

| Material | Why it is included / qualification |
| --- | --- |
| `src/vendor/golang.org/x/{crypto,net,text,sys}/LICENSE` and PATENTS | Go's own vendored runtime dependencies are separate from similarly named application modules. The license texts are BSD-3-Clause. The `x/sys` ancestry appeared on amd64 in this query; keeping the full union simplifies a shared notice bundle. [Example pinned vendored license](https://github.com/golang/go/blob/go1.26.7/src/vendor/golang.org/x/net/LICENSE) |
| `nistec/fiat/README` and `edwards25519/scalar.go` | Both contain fiat-crypto's own BSD-1-Clause-form terms and attribution for v0.0.9-generated code. The explicit condition concerns source redistribution; retain these blocks when supplying source/materials, and preserve them in the notice superset. The Go root LICENSE does not replace them. [README](https://github.com/golang/go/blob/go1.26.7/src/crypto/internal/fips140/nistec/fiat/README), [inline terms](https://github.com/golang/go/blob/go1.26.7/src/crypto/internal/fips140/edwards25519/scalar.go#L29) |
| `runtime/memmove_amd64.s` | Its Lucent/Vita Nuova MIT terms require copyright and permission notices with copies/substantial portions. This is selected for amd64 and is missed by legal-filename-only collection. [Complete header](https://github.com/golang/go/blob/go1.26.7/src/runtime/memmove_amd64.s#L1) |
| Sun-derived math files | `acosh`, `asinh`, `atanh`, `cbrt`, `erf`, `exp`, `expm1`, `j0`, `j1`, `jn`, `lgamma`, `log`, `log1p`, `remainder`, and `sqrt` retain Sun permission notices requiring notice preservation. Keep each original header, including its year. [Example](https://github.com/golang/go/blob/go1.26.7/src/math/acosh.go#L7) |
| Cephes-derived math files | `atan`, `gamma`, `sin`, `tan`, and `tanh` retain Stephen L. Moshier/Cephes attribution and permission/disclaimer context. Preserve those original blocks without collapsing them into a generic Go copyright. [Example](https://github.com/golang/go/blob/go1.26.7/src/math/atan.go#L11) |
| `internal/profile/graph.go` | Contains a Google 2014 Apache-2.0 header. Its selected-source presence is another reason not to classify the whole Go source tree solely from root LICENSE. [Header](https://github.com/golang/go/blob/go1.26.7/src/internal/profile/graph.go#L1) |
| AES generic and amd64 math exponential | Retain the source's Rijndael and SLEEF public-domain/provenance notices in the conservative superset. No new attribution condition is inferred from the public-domain declarations themselves. [AES header](https://github.com/golang/go/blob/go1.26.7/src/crypto/internal/fips140/aes/aes_generic.go#L5), [exponential header](https://github.com/golang/go/blob/go1.26.7/src/math/exp_amd64.s#L7) |
| `crypto/internal/boring` LICENSE/README | Directory ancestry includes this package's non-Boring fallback code. This is conservative retention, **not a finding that BoringSSL is linked**: the Boring implementation requires `boringcrypto` and cgo, unlike the observed builds. Re-evaluate if build experiments change. [Build constraints](https://github.com/golang/go/blob/go1.26.7/src/crypto/internal/boring/boring.go#L5) |

Preserving all legal-named files and README files under the Go source tree is a reasonable broader collection policy, but it still needs the source-embedded notice files listed above. Label that collection as a superset so it does not assert that every compiler, test, or optional crypto component is shipped. A source archive reference supplements—not substitutes for—required notices accompanying binaries.

The official [Go 1.26.7 source archive](https://go.dev/dl/go1.26.7.src.tar.gz) was downloaded in memory and hash-verified against [Go's release metadata](https://go.dev/dl/?mode=json&include=all). Its SHA-256 is `0ed24eac755105085b89fe9cabc2742b91a0ad7b94b59d3ad364918ebc8956ad`; the listed tree paths occur beneath the archive's `go/` root. The 25 additional source-notice files above were also fetched from the pinned GitHub tag and matched the installed source byte for byte.

## Version-bound Segmentio MIT-0 classification

The packaging tool reported unknown licenses for `github.com/segmentio/asm v1.2.1` packages `bswap`, `cpu`, `cpu/arm`, `cpu/arm64`, `cpu/cpuid`, and `cpu/x86`. The actual root license is **MIT No Attribution**, SPDX **MIT-0**, with Copyright 2023 Segment. The standard MIT attribution paragraph is absent; that is the named license's definition, not missing permission. [Pinned upstream LICENSE](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/LICENSE), [SPDX MIT-0](https://spdx.org/licenses/MIT-0.html)

| Reviewed classifier input | Exact value |
| --- | --- |
| Module/version | `github.com/segmentio/asm v1.2.1` |
| Origin commit | `1cfacc81a878d4a07b13f51f2368cd86893d23fa` |
| License file | `LICENSE` |
| License SHA-256 | `cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443` |
| Reviewed identifier | `MIT-0` |

The pinned upstream license was byte-identical to the module-cache copy. A module-wide license/copyright search found one LICENSE and no additional legal files or per-file license exceptions. The reported packages' Go/assembly headers were reviewed: amd64 uses the generated `bswap/swap64_amd64.s` under `!purego`; arm64 selects the Go fallback. No additional assembly license notice was found. CPU feature packages import `golang.org/x/sys/cpu`, which retains its own separate dependency entry. [Pinned assembly](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/bswap/swap64_amd64.s), [fallback](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/bswap/swap64_default.go), [CPU imports](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/cpu/arm64/arm64.go)

A narrowly recorded manual classification may use these version/commit/license-hash inputs and retain the original LICENSE as evidence. Reopen review if any bound input changes; do not map all unknown results to an allowed license or remove the module from the inventory. MIT-0's lack of attribution requirement does not prevent retaining its text voluntarily.

The inspected `go-licenses v2.0.1` save implementation copies the whole license directory when source sharing is required. Its notice-only path copies the chosen license plus files matching `NOTICE`, `NOTICE.txt`, or `NOTICE.md`; it is not a general AUTHORS/PATENTS/embedded-header collector. Supplement its output accordingly, and verify the final result instead of treating a successful classifier run as complete legal-material collection. [Pinned save implementation](https://github.com/google/go-licenses/blob/3e084b0caf710f7bfead967567539214f598c0a2/save.go#L100)

## Exact scratch-image CA input

The locally available **linux/arm64** variant of the Dockerfiles' pinned Go builder was inspected using a temporary read-only, network-disabled container. It contained:

| Item | Observed identity |
| --- | --- |
| Builder reference | `golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514` |
| Debian package | `ca-certificates 20250419~deb12u1` |
| Bundle | `/etc/ssl/certs/ca-certificates.crt` |
| Bundle SHA-256 | `714d457d580922dbf1d0be8bd35ba236a842b50b0072ae791582a19adef772a5` |
| Copyright file | `/usr/share/doc/ca-certificates/copyright` |
| Copyright SHA-256 | `e85e1bcad3a915dc7e6f41412bc5bdeba275cadd817896ea0451f2140a93967c` |
| Selection configuration SHA-256 | `/etc/ca-certificates.conf`: `ab7339d40969fb1084cb23011fbdae7cc8edb46ac7e410897bb6b7016f07ed7f` |

No additional files were present in `/usr/local/share/ca-certificates`. This
research inspection covered arm64 only. Later [candidate checks](../verification/2026-09-12.md#distribution)
verified both amd64 and arm64; future builds still need their actual bundle
identities checked rather than assuming this hash applies universally.

Debian's exact source package assigns MPL-2.0 to `mozilla/certdata.txt` and `mozilla/nssckbi.h`, with Mozilla Contributors attribution. Its package/conversion/update scripts are GPL-2-or-later. `certdata2pem.py` emits the PEM certificate bytes; `update-ca-certificates` concatenates selected certificate files. The current scratch Dockerfiles copy the resulting data, not those GPL scripts. Do not infer that Configra's executable becomes GPL-licensed merely from the package-level script label. [Exact Debian source archive](https://security.debian.org/debian-security/pool/updates/main/c/ca-certificates/ca-certificates_20250419~deb12u1.tar.xz), inner files `ca-certificates/debian/copyright`, `ca-certificates/mozilla/certdata2pem.py`, and `ca-certificates/sbin/update-ca-certificates`.

For this copied Mozilla-derived data, a conservative distribution treatment is to include the complete Debian copyright file, MPL-2.0 text, and a source-access/provenance notice naming the package, source archive, bundle hash, and selection configuration. Retain the exact source inputs so the notice remains useful. This is a documented packaging treatment of the generated data; the copyrightability of individual certificates and the precise preferred form of modification are not adjudicated by this research. If update/conversion scripts are themselves redistributed, include and evaluate their GPL materials too; those obligations should not be silently assigned to or omitted from unrelated components.

The verified source archive is `ca-certificates_20250419~deb12u1.tar.xz`, SHA-256 `b2a431cbab9a0ece921cffacbe238dc27a3e382ad4a1806dc8968c5eff30471d`, matching Debian's [source control file](https://security.debian.org/debian-security/pool/updates/main/c/ca-certificates/ca-certificates_20250419~deb12u1.dsc). The corresponding MPL input paths are `ca-certificates/mozilla/certdata.txt` and `ca-certificates/mozilla/nssckbi.h`. Copying Debian's copyright text alone leaves its reference to `/usr/share/common-licenses/GPL-2` unresolved in scratch; include that text when distributing the GPL scripts, or retain it as a clearly scoped documentary supplement if keeping the full package copyright record.

## Artifact acceptance checks

1. Bind each artifact to its source commit, actual Go version, GOOS/GOARCH/build flags, dependency versions/replacements, UI lockfile/build output, and—where present—CA source/hash. Reconcile the new binary's metadata; do not reuse rc.1 counts as an acceptance condition for rc.2.
2. Ensure archives and each scratch image contain the intended license/notice collection and a readable index connecting texts to components. Include the Go source-embedded notices above; retain relevant AUTHORS/PATENTS or other supplemental materials the collector does not copy.
3. Verify the MySQL driver's source-access path and any shipped covered source/patches. Verify actual first-party module archives contain their licensing materials; an intended future SDK version is not evidence of a published artifact.
4. Verify UI notices survive the actual Vite build and embedding and are delivered to UI recipients. Inventory the bundled runtime transitives; inspect build-generated code/assets before excluding development tools categorically.
5. Keep unknown classifications fail-visible until individually resolved. Record the Segmentio MIT-0 review narrowly and test changed-version/hash rejection. An SBOM and a passing scanner are useful evidence, not substitutes for the license texts, notices, source access, or unresolved file-level review.

The research itself inspected existing binaries, source and builder metadata;
it did not publish or change release refs. Temporary inspection containers were
removed automatically. Transitive classification and artifact inclusion have
separate evidence linked above; neither those checks nor this report constitutes
legal certification.
