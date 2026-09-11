# Third-party material delivery — 2026-09-12

Source candidate: `e0cb6df3ebb66301962eca2f1d92cc653e189824`.
Local bundle version: `v0.1.0-rc.2-review.3`. No server release/tag or public
container image was published in this run. This extends the earlier
[project/runtime verification](distribution-verification-2026-09-12.md), not the
completion criteria in [production readiness](production-readiness.md).

## Delivered and checked

| Material | Scope and evidence |
| --- | --- |
| Go dependency notices | [93 module/version pairs and 169 reviewed files](research/transitive-go-license-inventory.md), covering 31 server modules per target and 67/68 Kubernetes modules on macOS/Linux. Actual binary records, target-matched source loading, module sums, source archive identities and notice hashes are reconciled. |
| Scope-change guards | Eight reviewed package-selection digests include stdlib and the command package. Changed imports, target/CPU flags, toolchain, cgo, replacements or module versions cannot silently reuse the old review. A different application graph was rejected through the collector CLI. |
| Mixed and embedded notices | Retains the actual YAML/OpenTelemetry/compression texts, nested Go-fork licenses, supplemental AUTHORS/PATENTS and source-embedded terms. Source/assembly files use `.txt` suffixes without changing their bytes. Segmentio MIT-0 is hash-bound, not a blanket Unknown bypass. |
| MySQL covered source | Each server collection carries the exact unmodified `mysql-v1.10.0.zip`, its MPL-2.0 terms and readable source-access instructions. SHA-256: `dc93f5770556406e82bf750a980d2316f882a19d883a3689eadb820708c2b651`. |
| UI notices | 25 package versions, including transitives and emitted Vite/Rolldown helpers. All 26 legal files were compared with canonical npm archives whose integrity matched the lockfile. The built UI links to the original texts; both UI locales expose that link. |
| UI loading and consistency | Browser verification observed no background download of notices/SBOM on normal sign-in load. Legal metadata export rejects changed JS/CSS and stale file sets. Copied image materials and the same bytes embedded in the executable both passed inspection. |
| Scoped SBOMs | Go application-module SBOMs are bound to exact executable hashes; UI SBOMs bind the actual JS/CSS hashes. All 12 documents across four bundles passed the official CycloneDX 1.6 JSON schema. These are scoped inventories, not a complete container/OS/runtime composition claim. |

The UI's immutable assets are:

- `assets/third-party-notices-9dc7af0b03463eb5.txt`
- `assets/ui-sbom-07b813edc5328ddc.cdx.json`

The notice asset is about 150 KB uncompressed and is fetched only when requested.
Vite's license appendix and Rolldown's Rollup/esbuild attribution are retained as
supersets for generated helpers, not as a claim that the whole native build-tool
dependency tree runs in the browser.

## Commands exercised

From a clean source worktree, with a fresh output version directory:

```sh
node scripts/build-release.mjs v0.1.0-rc.2-review.3
node scripts/verify-release.mjs dist/v0.1.0-rc.2-review.3
```

The current verifier was additionally run against those actual archives after
strengthening its per-file checks. It rejects links/devices and unsafe archive
paths, then checks each extracted module notice against its recorded SHA-256.
The module list and sums match the binary's own build information. It also
checks UI bytes remain embedded and the covered MySQL ZIP remains exact.

```sh
docker build --progress=plain --tag configra:third-party-check-20260912 .
docker build --progress=plain -f kubernetes/Dockerfile \
  --tag configra-kubernetes:third-party-check-20260912 .
node scripts/verify-image.mjs configra:third-party-check-20260912 configra
node scripts/verify-image.mjs configra-kubernetes:third-party-check-20260912 configra-kubernetes
npm --prefix web run test:licenses
npm --prefix web test
```

Image inspections used Linux arm64 containers on Docker Desktop. Verified IDs:

- Configra: `sha256:84019084d3a9ffb5e79b6e12a0891d08a46bfa080b12566b4c45ac1cd75d986c`
- Kubernetes: `sha256:c5620ebf44f450a29d76231b65364e6ba9f4a7cf7534d097537c6036d5ecaacf`

Both CLI probes ran with a read-only filesystem, disabled network, no capabilities
and UID 65532. Inspection containers were removed automatically. No database or
Kubernetes cluster was started or modified. Images/cache/candidate artifacts
remain local for follow-up; other Docker services were not touched.

UI results: **43 Playwright tests passed**, including the new bilingual license
link; three production-build/export tests passed for normal delivery, unreviewed
dependency version/legal text, and altered output content. Go race tests passed
in the server, SDK and Kubernetes modules. Server race tests also passed in the
clean worktree after the complete release build populated `dist/`; vet passed.

Negative controls included the older archive missing module material, the old
Kubernetes binary's SDK mismatch, a different package graph and an existing
output directory. Failed collection did not replace existing material or publish
its temporary directory.

## Archive hashes

These identify the inspected local files; they are not a reproducible-build claim.

```text
23d12a1914b490822d08581b6cf70a596bc8bf36f0795eb5de2f08801033c872  configra_0.1.0-rc.2-review.3_darwin_amd64.tar.gz
8766054cc37ab6ad5b236679fec9b1e314920a830a5c7c997cfed6b46918c41c  configra_0.1.0-rc.2-review.3_darwin_arm64.tar.gz
c66469fbe566d5e7ba1754bcbcb9bbe91eda7128a39709b732f96afae508ec82  configra_0.1.0-rc.2-review.3_linux_amd64.tar.gz
39a35d7c4d21c707faaa22a492f218dbf4dbe656ac16681ae8483ab2d70c1a96  configra_0.1.0-rc.2-review.3_linux_arm64.tar.gz
```

## Still open

The copied system CA trust bundle needs its license/source/provenance material.
The per-module/UI SBOMs must be reconciled into the complete distributable scope,
including Go runtime and CA evidence. Already-published rc.1 artifacts still need
appropriate supplements without rewriting their tags or existing bytes.

This does not pass the remaining production startup/recovery/Kubernetes, key
lifecycle, inventory bounding, documentation walkthrough or ten-minute capacity
and leakage gates. MySQL 8.0.22 remains required. The earlier module-only
vulnerability findings and continuous release-CI work remain tracked separately.
