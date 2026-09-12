# CA and complete-payload verification — 2026-09-12

Verified source: `7dbdfcd79a37455494e1f64cc30dae4f7285caa3`, in a clean detached
worktree. Local candidate: `v0.1.0-rc.2-review.4`. These server artifacts were not
published as a release. This completes the current candidate's CA-material and
payload-composition checks, not the [production acceptance contract](production-readiness.md).

## System trust material

Both amd64 and arm64 variants of the pinned Go builder were inspected. Each used
Debian `ca-certificates 20250419~deb12u1` with identical certificate bytes,
selection configuration, copyright and complete MPL/GPL license texts.

The images now retain the exact Debian source archive, with SHA-256
`b2a431cbab9a0ece921cffacbe238dc27a3e382ad4a1806dc8968c5eff30471d`, downloaded through
Docker's checksum-checked ADD. Collection independently verifies that archive and
the runtime bundle. The latter's SHA-256 remains
`714d457d580922dbf1d0be8bd35ba236a842b50b0072ae791582a19adef772a5`.

The delivered collection contains the source archive, copyright record, complete
MPL-2.0/GPL-2.0 texts, CA selection configuration, source-access instructions and
hash metadata. Mozilla certificate inputs and Debian script sources keep their
separate terms. The archived scripts are not installed as runtime commands, and
Configra's original code remains Apache-2.0. This does not touch Configra-managed
CAs, workload certificates or private keys. Source/term details are in the
[primary-source review](research/distribution-license-requirements.md).

## Aggregate and file-inventory results

`SBOM.cdx.json` combines the original per-module/UI evidence, executable identities,
Go runtime, and image CA data/source components. Identical component identities
are merged only when their source checksums and licensing evidence agree.

`INVENTORY.json` records relative file paths, sizes, modes and SHA-256 values,
including the aggregate SBOM. It excludes only itself to avoid a self-hash cycle.
The SBOM's payload-file digest is explicitly **not an OCI manifest digest or an
independent signature**.

| Delivered root | Components | Inventory file entries, excluding inventory itself |
| --- | ---: | ---: |
| macOS amd64/arm64 release bundle | 121 | 270 |
| Linux amd64/arm64 release bundle | 122 | 272 |
| Configra image, either architecture | 61 | 136 |
| Kubernetes image, either architecture | 72 | 169 |

All four release archives were extracted and checked against their inventories.
All four images were exported and checked against actual filesystem content;
the verifier rejects missing, altered, extra, linked or special payload files.
Docker-generated mount helpers are accounted for separately, including the exact
`etc/mtab -> /proc/mounts` alias, which is checked but never extracted.
The aggregate also preserves source component identities and dependency edges.
All eight aggregate SBOMs passed the official CycloneDX 1.6 JSON schema.

Native binary bundles do not claim to include the host OS's trust store.
Databases, OIDC, NATS, ClickHouse and Kubernetes/CSI infrastructure remain
operator-supplied external systems, not bundled software.

## Commands exercised

From the clean source checkout, with a new candidate output directory:

```sh
node scripts/build-release.mjs v0.1.0-rc.2-review.4
make image-license-test \
  IMAGE=configra:composition-check-20260912 \
  KUBERNETES_IMAGE=configra-kubernetes:composition-check-20260912
```

Both images were also built with `--platform linux/amd64` and verified with
`scripts/verify-image.mjs`. The local host/runtime was macOS arm64 and Docker
Desktop Linux arm64; amd64 CLI execution used emulation. Probes ran non-root,
read-only, without network or capabilities. Full runtime startup/recovery was
not implied by `--version`.

| Image | Verified local ID |
| --- | --- |
| Configra arm64 | `sha256:ca5d2d481ae998d50cca2de9caa60c334a4376815febf04a55d3d94f757c819c` |
| Kubernetes arm64 | `sha256:e04e13bfb9675a9d79035f704a5a23ac43862b04c0b0b58e3595002030323e83` |
| Configra amd64 | `sha256:2ea2833b6b043228ddd5d262b6c0404c08ca208e13d00a5cbbccf552d66b4615` |
| Kubernetes amd64 | `sha256:ff83aeb87b44bd1a4521934615122c38314635be60085600b26c7a756531ae8c` |

The process regressions cover changed/unreviewed CA source, existing-output
preservation, extra/changed delivered files and conflicting same-version module
checksums. The checksum-conflict test initially failed and passed after the merge
guard was corrected. Old images/bundles lacking CA or aggregate metadata were
rejected. Server race tests and vet passed; race tests also passed after the clean
worktree's complete release build populated `dist/`.

## Archive hashes

```text
bfb1d1d9ab361c54b60349d5c0d3d5b9a6de29f13d39acd20c80c8e496c64c30  configra_0.1.0-rc.2-review.4_darwin_amd64.tar.gz
3a963e23d234b354233b08d4daaf59e6a00dc2a683d44a59660497de87593a88  configra_0.1.0-rc.2-review.4_darwin_arm64.tar.gz
7cfcf80a50f4ee6ab66fc2c5917ee37aa5eaf63ac2e1ee3b62a7dc10e5c6ef09  configra_0.1.0-rc.2-review.4_linux_amd64.tar.gz
194f0b5bfaeda8704b15f0cf74a6f9b54d251b3239b679012351b5b03a1e63e2  configra_0.1.0-rc.2-review.4_linux_arm64.tar.gz
```

These identify this run, not a reproducible-build guarantee. Inspection containers
were removed; local candidate files, images and build cache remain for follow-up.
No database, production service or Kubernetes cluster was changed.

## Remaining release work

Already-published rc.1 artifacts still need appropriate supplements without
rewriting tags or existing bytes. The final published candidate must repeat these
checks, along with continuous release CI and the full operational/security matrix.
Inventory bounding, key lifecycle/recovery, executable documentation walkthroughs,
and the unchanged ten-minute capacity/leakage gate remain open. MySQL 8.0.22 remains
the required compatibility baseline. No stable server release is approved here.
