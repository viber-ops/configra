# Direct dependency licenses and redistribution evidence

Research date: 2026-09-11. This is an engineering inventory of first-party license texts and release-packaging evidence, not legal certification or a complete software bill of materials.

## Finding and scope

The reviewed direct third-party dependencies use Apache-2.0, MIT, BSD-3-Clause, MPL-2.0, or a file-specific MIT/Apache-2.0 combination. **The MySQL Go driver is MPL-2.0** and is the identified direct dependency with copyleft/source-availability requirements. This does not require relicensing Configra's separately authored files away from Apache-2.0, but a project-owned Apache license does not replace dependency licenses. [Driver license](https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/LICENSE), [Mozilla FAQ Q8, Q11, Q13](https://www.mozilla.org/en-US/MPL/2.0/FAQ/)

Inputs were the current [server manifest](../../go.mod), [SDK manifest](https://github.com/viber-ops/configra-go/blob/5bb89dcbb4574981bcc34f4366cccb5bafba6168/go.mod), [Kubernetes manifest](../../kubernetes/go.mod), and the embedded [UI manifest](../../web/package.json). The tables include every non-`indirect` Go requirement and every npm `dependencies` entry. Go runtime imports were checked to distinguish manifest directness from package use. The separate E2E module, npm development dependencies, website dependencies, and the complete transitive graphs are outside this inventory. `ch-go`, an indirect dependency, was additionally checked because the ClickHouse drivers were explicitly in scope.

Evidence was read from the installed artifacts and is cited through portable, version-specific sources. Go upstream repository URLs and exact commits were resolved from `go mod download -json` metadata (`Origin.URL`, `Origin.Hash`, and `Origin.Subdir`); all 27 cited third-party Go license/source files were fetched and matched the inspected copies byte for byte. The SCS MySQL submodule inherits its license from the repository root, which is the source linked below. npm citations point to the exact registry archives recorded in the [UI lockfile](../../web/package-lock.json); `package/LICENSE` names the file inside each archive. All ten archives matched the lockfile integrity hashes and contained license texts identical to the inspected copies. Standard Apache clauses 1–9 were compared after whitespace normalization; copyright/appendix differences and positive root NOTICE files were separately inspected.

## Configra server Go dependencies

| Module | Pinned version | License from installed text | Primary artifact evidence |
| --- | --- | --- | --- |
| `github.com/ClickHouse/clickhouse-go/v2` | `v2.48.0` | Apache-2.0 | [LICENSE](https://github.com/ClickHouse/clickhouse-go/blob/69b5195a9b2e04a999f9c130a7a9dc1248288689/LICENSE) |
| `github.com/alexedwards/scs/mysqlstore` | `v0.0.0-20251002162104-209de6e426de` | MIT | [LICENSE](https://github.com/alexedwards/scs/blob/209de6e426de9259665975ce16b91331d228f052/LICENSE) |
| `github.com/alexedwards/scs/v2` | `v2.9.0` | MIT | [LICENSE](https://github.com/alexedwards/scs/blob/ab20b3feb5e9981c1f79cee8a97a289810134163/LICENSE) |
| `github.com/coreos/go-oidc/v3` | `v3.20.0` | Apache-2.0 | [LICENSE](https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/LICENSE), [NOTICE](https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/NOTICE) |
| `github.com/go-jose/go-jose/v4` | `v4.1.4` | Apache-2.0 | [LICENSE](https://github.com/go-jose/go-jose/blob/0e59876635f3dbf46d7b5e97b52bb75a3f96e7d9/LICENSE) |
| `github.com/go-sql-driver/mysql` | `v1.10.0` | MPL-2.0 | [LICENSE](https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/LICENSE), [AUTHORS](https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/AUTHORS), [source notice](https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/driver.go#L1) |
| `github.com/nats-io/nats.go` | `v1.53.1` | Apache-2.0 | [LICENSE](https://github.com/nats-io/nats.go/blob/db1375fcffae2eb0b4ced1b7bad4d47c4447e4ac/LICENSE) |
| `github.com/spf13/cobra` | `v1.10.2` | Apache-2.0 | [LICENSE.txt](https://github.com/spf13/cobra/blob/88b30ab89da2d0d0abb153818746c5a2d30eccec/LICENSE.txt) |
| `go.uber.org/zap` | `v1.28.0` | MIT | [LICENSE](https://github.com/uber-go/zap/blob/5b81b37b81b8e2ed447a6f57991e372ee4fa5c8f/LICENSE) |
| `go.yaml.in/yaml/v3` | `v3.0.4` | MIT and Apache-2.0, assigned by file | [LICENSE](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE), [NOTICE](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/NOTICE) |
| `golang.org/x/oauth2` | `v0.36.0` | BSD-3-Clause | [LICENSE](https://go.googlesource.com/oauth2/+/4d954e69a88d9e1ccb8439f8d5b6cbef230c4ef9/LICENSE) |

`go-jose` is imported directly by Configra's OIDC tests, and also used at runtime by the pinned OIDC library, so it should not be discarded as test-only. [Configra OIDC test](../../internal/humanauth/oidc_test.go), [OIDC runtime import](https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/oidc/verify.go#L12)

The additional ClickHouse spot-check found `github.com/ClickHouse/ch-go v0.74.0` under Apache-2.0, with ClickHouse and Go Faster authors identified in its root AUTHORS file. This says nothing yet about every compression, hashing, geometry, or telemetry dependency of either driver. [LICENSE](https://github.com/ClickHouse/ch-go/blob/373f5030e95e0b271822e4c2be33d639c526f196/LICENSE), [AUTHORS](https://github.com/ClickHouse/ch-go/blob/373f5030e95e0b271822e4c2be33d639c526f196/AUTHORS)

## SDK and Kubernetes direct Go dependencies

The SDK directly requires `github.com/spf13/viper v1.21.0` under **MIT**, plus `go.yaml.in/yaml/v3 v3.0.4` under the mixed license above. Viper and Cobra have different licenses in these versions. [Viper LICENSE](https://github.com/spf13/viper/blob/394040caccbdf5821fa6839386a35f0fb1b1ee9e/LICENSE), [SDK runtime imports](https://github.com/viber-ops/configra-go/blob/5bb89dcbb4574981bcc34f4366cccb5bafba6168/handler.go)

| Kubernetes module | Pinned version | License/evidence |
| --- | --- | --- |
| `github.com/viber-ops/configra-go` | `v0.0.0-20260911101812-8e4339f8884e` | First-party SDK. The then-unpublished local SDK `LICENSE` and `NOTICE` declared Apache-2.0 for original code. This local evidence is not attributed to the older module version; that archive lacks these files—see below. |
| `go.yaml.in/yaml/v3` | `v3.0.4` | Same file-specific MIT/Apache-2.0 [LICENSE](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE) and [NOTICE](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/NOTICE). |
| `google.golang.org/grpc` | `v1.82.1` | Apache-2.0: [LICENSE](https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/LICENSE), [NOTICE.txt](https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/NOTICE.txt). |
| `k8s.io/api` | `v0.35.0` | Apache-2.0: [LICENSE](https://github.com/kubernetes/api/blob/9afe7de0a56c582b06ca094f7309015bc84657a7/LICENSE). |
| `k8s.io/apimachinery` | `v0.35.0` | Apache-2.0: [LICENSE](https://github.com/kubernetes/apimachinery/blob/72d71eac265e06713c6d83d7034aac609450243f/LICENSE). |
| `k8s.io/client-go` | `v0.35.0` | Apache-2.0: [LICENSE](https://github.com/kubernetes/client-go/blob/9bcb69436287b966d0c5c195efef00aed921fb1b/LICENSE). |
| `sigs.k8s.io/controller-runtime` | `v0.23.1` | Apache-2.0: [LICENSE](https://github.com/kubernetes-sigs/controller-runtime/blob/f52bbb8bb1a2275cbe90dec8d6c12d5cacb1a7de/LICENSE). |
| `sigs.k8s.io/secrets-store-csi-driver` | `v1.6.0` | Apache-2.0: [LICENSE](https://github.com/kubernetes-sigs/secrets-store-csi-driver/blob/4960f9637c51ad9cb9ef264481bc6d9a98bf1f1c/LICENSE). |

Configra directly imports the CSI provider API and gRPC in its provider implementation. `client-go` has direct test imports but is also imported by controller-runtime's production client implementation. The table is about Go code included through these modules; it is not a license inventory for an entire Kubernetes cluster or separately installed CSI container. [Provider source](../../kubernetes/internal/provider/server.go), [controller-runtime client imports](https://github.com/kubernetes-sigs/controller-runtime/blob/f52bbb8bb1a2275cbe90dec8d6c12d5cacb1a7de/pkg/client/client.go#L31)

## Embedded web UI direct runtime npm packages

Each package below contains an MIT license, including its own copyright attribution. The installed text—not only `package.json`'s `license` field—was checked.

| Package | Version | Registry archive; inspect the named inner file |
| --- | --- | --- |
| `@codemirror/lang-json` | `6.0.2` | [package/LICENSE](https://registry.npmjs.org/@codemirror/lang-json/-/lang-json-6.0.2.tgz) |
| `@codemirror/lang-yaml` | `6.1.3` | [package/LICENSE](https://registry.npmjs.org/@codemirror/lang-yaml/-/lang-yaml-6.1.3.tgz) |
| `@codemirror/language` | `6.12.4` | [package/LICENSE](https://registry.npmjs.org/@codemirror/language/-/language-6.12.4.tgz) |
| `@codemirror/merge` | `6.12.2` | [package/LICENSE](https://registry.npmjs.org/@codemirror/merge/-/merge-6.12.2.tgz) |
| `@codemirror/state` | `6.7.1` | [package/LICENSE](https://registry.npmjs.org/@codemirror/state/-/state-6.7.1.tgz) |
| `@codemirror/view` | `6.43.9` | [package/LICENSE](https://registry.npmjs.org/@codemirror/view/-/view-6.43.9.tgz) |
| `@lezer/highlight` | `1.2.3` | [package/LICENSE](https://registry.npmjs.org/@lezer/highlight/-/highlight-1.2.3.tgz) |
| `codemirror` | `6.0.2` | [package/LICENSE](https://registry.npmjs.org/codemirror/-/codemirror-6.0.2.tgz) |
| `react` | `19.2.8` | [package/LICENSE](https://registry.npmjs.org/react/-/react-19.2.8.tgz) |
| `react-dom` | `19.2.8` | [package/LICENSE](https://registry.npmjs.org/react-dom/-/react-dom-19.2.8.tgz) |

The CodeMirror/Lezer texts identify Marijn Haverbeke and others; React and React DOM identify Meta Platforms and affiliates. Preserve the actual texts, including the package-specific copyright lines. Configra embeds `web/dist`, so merely leaving licenses in development `node_modules` does not establish that recipients receive them. An implementation should retain the required notices in the distributed UI or accompanying accessible license materials and verify the built artifact. [React package/LICENSE](https://registry.npmjs.org/react/-/react-19.2.8.tgz), [CodeMirror package/LICENSE](https://registry.npmjs.org/codemirror/-/codemirror-6.0.2.tgz), [embed declaration](../../web/embed.go#L13)

## Redistribution requirements indicated by the texts

### MySQL driver: MPL-2.0

Distribution of a binary containing the driver requires making the covered source available under MPL-2.0 and telling binary recipients how to obtain it. Preserve its license/copyright notices; modifications to covered files must remain available under MPL-2.0. Use a source-access notice naming the exact driver version and a stable source location, and retain the corresponding source/patches as part of release preparation. A generic Apache NOTICE or a list of package names does not fulfill these source-access requirements. [MPL sections 3.1–3.4](https://www.mozilla.org/en-US/MPL/2.0/)

Mozilla explicitly allows static linking into a larger work, including combinations with Apache-licensed code. Separately authored files containing no MPL code need not become MPL-licensed. Thus the direct driver finding supports keeping Configra's original code under Apache-2.0 while honoring the driver's separate terms; it is not a reason by itself to replace the driver. [Mozilla FAQ Q8, Q11–Q13](https://www.mozilla.org/en-US/MPL/2.0/FAQ/)

### Apache-2.0, MIT, BSD, and mixed YAML

Apache-2.0 section 4 requires providing the license, marking modified files, retaining applicable source notices, and reproducing relevant upstream NOTICE attributions when present. The inspected positive root notices include CoreOS go-oidc's NOTICE, gRPC's NOTICE.txt, and YAML's NOTICE. A suitable release process can collect these in accompanying license/notice materials; project-owned attribution alone is insufficient. Apache's trademark permissions are limited, so identifying a dependency is not permission to suggest endorsement. [Apache-2.0 sections 4 and 6](https://www.apache.org/licenses/LICENSE-2.0), [CoreOS NOTICE](https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/NOTICE)

The inspected MIT texts require preserving copyright and permission notices in copies or substantial portions. The OAuth2 BSD-3-Clause text requires the copyright, conditions, and disclaimer in source distributions and in documentation/materials accompanying binaries, and prohibits using the named parties to endorse a derived product without permission. Neither text imposes the MPL-style covered-source availability condition. [Viper MIT text](https://github.com/spf13/viper/blob/394040caccbdf5821fa6839386a35f0fb1b1ee9e/LICENSE), [OAuth2 BSD text](https://go.googlesource.com/oauth2/+/4d954e69a88d9e1ccb8439f8d5b6cbef230c4ef9/LICENSE)

YAML's license assigns MIT to eight libyaml-derived Go files and Apache-2.0 to the remaining project files. This is a **file-specific combination**, not a blanket `MIT OR Apache-2.0` choice. Keep both applicable license texts and the YAML NOTICE. [YAML license and file allocation](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE)

## Observed release gaps

At inspection time, the project-owned [server NOTICE](../../NOTICE), [Kubernetes NOTICE](../../kubernetes/NOTICE), and the then-unpublished local SDK `NOTICE` stated that dependencies retain their own terms. Those files did not themselves contain the complete third-party notices, license texts, or MySQL driver's source-access information. The SDK statement is a working-tree observation, not a claim about licensing files in an earlier published tag.

The **SDK `v0.1.0-rc.1` module zip contains 17 files and no LICENSE, NOTICE, or COPYING file** under its inner root `github.com/viber-ops/configra-go@v0.1.0-rc.1/`. The pinned pseudo-version archive contains nine files and no such licensing files under `github.com/viber-ops/configra-go@v0.0.0-20260911101812-8e4339f8884e/`. The canonical module archives were fetched and their entries checked against the cached evidence; the absence is not inferred from the working tree. These findings remain separate from the newly added local SDK license files. Validate a subsequent release's actual module zip before describing that artifact as carrying the new licensing materials. Other historical rc.1 module zips and release archives were not inspected here. [SDK rc.1 module archive](https://proxy.golang.org/github.com/viber-ops/configra-go/@v/v0.1.0-rc.1.zip), [pinned SDK module archive](https://proxy.golang.org/github.com/viber-ops/configra-go/@v/v0.0.0-20260911101812-8e4339f8884e.zip)

At inspection time, both Dockerfiles end in `FROM scratch` and explicitly copy only the executable and CA bundle into the final image. They contain no explicit copy of project or third-party license materials. The Kubernetes build also replaces its SDK dependency with the local sibling source tree, so its built provenance must be distinguished from the SDK version named by `go.mod`. This research did not inspect built image contents or determine whether embedded output already contains any notices. [Server Dockerfile](../../Dockerfile), [Kubernetes Dockerfile](../../kubernetes/Dockerfile)

## Work still needed for a complete release inventory

1. Inventory the resolved, actually linked Go dependencies for each released binary/platform/build configuration, including indirect modules, the Go runtime/standard library, bundled source, and per-file exceptions. Reconcile manifest requirements with the built binary's dependency/provenance information; this direct-module review is not that proof.
2. Inventory npm runtime transitives and what actually enters the production JS/CSS bundle: further CodeMirror/Lezer packages, React's supporting packages, and any build-generated helpers/assets. The current ten-package MIT result must not be extended to unreviewed transitive packages. Review development/build tools separately if their code or assets are redistributed.
3. Check copied CA material and any other final-image contents. A scratch image removes the base OS filesystem from the final stage but does not remove the executable's linked dependencies or the copied CA bundle. Separately distributed MySQL/ClickHouse/NATS/Casdoor/CSI images require their own scope assessment.
4. Assemble versioned third-party license/notice materials and the MPL source-access information, then verify the actual archives, module zips, container images, and served UI. Preserve source changes and release provenance; a top-level Apache file, dependency-name list, or SBOM alone is not evidence that all distribution terms have been met.

Recommendation: retain Apache-2.0 for project-owned source, retain the reviewed dependencies, and make artifact-level license/notice delivery plus MySQL-driver source access part of release preparation. Complete the transitive inventory before claiming that an entire production artifact has been reviewed. No dependency replacement, source/build edit, installation, commit, or publication was performed for this report.
