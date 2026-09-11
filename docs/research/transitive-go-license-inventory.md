# Transitive Go license inventory for review.2 binaries

Reviewed 2026-09-12. This report inventories **93 distinct application-module/version pairs** in the eight `v0.1.0-rc.2-review.2` executables under `.cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/`. It covers the server and Kubernetes command on Darwin/Linux and amd64/arm64. The Go runtime, UI/npm dependencies, and container CA bundle have separate scope; see [distribution requirements](distribution-license-requirements.md).

## Result and evidence boundary

The module/version sets obtained from target-matched source package loading exactly match the embedded dependency records of all eight binaries. Server binaries contain 31 dependency modules each; Kubernetes contains 67 on Darwin and 68 on Linux. The Linux-only difference is `github.com/prometheus/procfs`. All eight binaries report Go 1.26.7 and `CGO_ENABLED=0`. Their hashes are recorded in the JSON below.

The installed host-native `go-licenses v2.0.1` was verified through its build metadata and run in CSV mode for all eight GOOS/GOARCH combinations, with Go 1.26.7, `CGO_ENABLED=0`, `GOWORK=off`, read-only module mode, and offline module resolution. It reported only the already-known Segmentio MIT-0 classification gap. **CSV returned exit code 0 while emitting Unknown rows/errors**, so success status alone is not an acceptance gate.

The review also compared **128 applicable standalone license/NOTICE/AUTHORS/PATENTS files** with pinned upstream files or exact canonical module-archive entries. Every comparison matched. Source selection covered 4,646 application files; legal notices and mixed-license declarations in source headers, READMEs, and the warned assembly paths were reviewed. Supplemental source/assembly and copied-code records bring the machine-readable collection to **169 files**, all with verified source bytes and SHA-256. This is a package/file-scope inventory, not a proof of every surviving linker symbol or the historical authorship of every copied algorithm.

Thirteen older modules have no VCS Origin in the available canonical Go metadata. Their records use verified canonical archive URLs, archive hashes, and Go module sums; a full Git commit is explicitly unknown. Other origins use the exact available VCS hash. Classifier-generated URLs were not trusted: for example, inherited submodule licenses may live at a repository root.

## Per-module/version inventory

`S` means all four server targets; `K` means all four Kubernetes targets; `S+K` means all eight. Licenses joined with `+` describe a component/file combination, **not a blanket OR relicensing choice**. The qualification and file lists in the JSON are authoritative for the reviewed scope. In particular, the full klauspost root text contains Apache terms for an unselected subtree; retaining that text does not establish that subtree is linked.

Every evidence link is pinned. Where it points to a module ZIP, the table filename is relative to the ZIP's `<module>@<version>/` root; the exact inner path is also in the JSON. The main Configra modules are not dependency rows: their first-party LICENSE/NOTICE are handled separately.

| Module | Version | Used by | License identifiers / qualified scope | Primary root text |
| --- | --- | --- | --- | --- |
| `filippo.io/edwards25519` | `v1.2.0` | S | BSD-3-Clause + BSD-1-Clause | [LICENSE](https://github.com/FiloSottile/edwards25519/blob/b182a6575cfd9f4fbb1d1d4e487a6b00a3ec06f7/LICENSE) |
| `github.com/alexedwards/scs/mysqlstore` | `v0.0.0-20251002162104-209de6e426de` | S | MIT | [LICENSE](https://github.com/alexedwards/scs/blob/209de6e426de9259665975ce16b91331d228f052/LICENSE) |
| `github.com/alexedwards/scs/v2` | `v2.9.0` | S | MIT | [LICENSE](https://github.com/alexedwards/scs/blob/ab20b3feb5e9981c1f79cee8a97a289810134163/LICENSE) |
| `github.com/andybalholm/brotli` | `v1.2.2` | S | BSD-3-Clause + MIT | [LICENSE](https://github.com/andybalholm/brotli/blob/785aba538b2118979d2c573eccb287c4da157faf/LICENSE) |
| `github.com/beorn7/perks` | `v1.0.1` | K | MIT | [LICENSE](https://proxy.golang.org/github.com/beorn7/perks/@v/v1.0.1.zip) |
| `github.com/cespare/xxhash/v2` | `v2.3.0` | S+K | MIT | [LICENSE.txt](https://github.com/cespare/xxhash/blob/998dce232f17418a7a5721ecf87ca714025a3243/LICENSE.txt) |
| `github.com/ClickHouse/ch-go` | `v0.74.0` | S | Apache-2.0 | [LICENSE](https://github.com/ClickHouse/ch-go/blob/373f5030e95e0b271822e4c2be33d639c526f196/LICENSE) |
| `github.com/ClickHouse/clickhouse-go/v2` | `v2.48.0` | S | Apache-2.0 | [LICENSE](https://github.com/ClickHouse/clickhouse-go/blob/69b5195a9b2e04a999f9c130a7a9dc1248288689/LICENSE) |
| `github.com/coreos/go-oidc/v3` | `v3.20.0` | S | Apache-2.0 | [LICENSE](https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/LICENSE) |
| `github.com/davecgh/go-spew` | `v1.1.1` | K | ISC | [LICENSE](https://proxy.golang.org/github.com/davecgh/go-spew/@v/v1.1.1.zip) |
| `github.com/emicklei/go-restful/v3` | `v3.12.2` | K | MIT | [LICENSE](https://github.com/emicklei/go-restful/blob/d59fac5bd1b1c244342c44e3e41699b8c03a14c1/LICENSE) |
| `github.com/evanphx/json-patch/v5` | `v5.9.11` | K | BSD-3-Clause | [LICENSE](https://github.com/evanphx/json-patch/blob/84a4bb100ade42a86fce2647c95a7dbcbf569cb2/LICENSE) |
| `github.com/fsnotify/fsnotify` | `v1.9.0` | K | BSD-3-Clause | [LICENSE](https://github.com/fsnotify/fsnotify/blob/ae0e7923765f64fb8061396db7edebb558cf6093/LICENSE) |
| `github.com/fxamacker/cbor/v2` | `v2.9.0` | K | MIT | [LICENSE](https://github.com/fxamacker/cbor/blob/d29ad7351b55b1844387cf9306c4101658cc5256/LICENSE) |
| `github.com/go-faster/city` | `v1.0.1` | S | MIT | [LICENSE](https://proxy.golang.org/github.com/go-faster/city/@v/v1.0.1.zip) |
| `github.com/go-faster/errors` | `v0.7.1` | S | BSD-3-Clause | [LICENSE](https://github.com/go-faster/errors/blob/a3f192e8d7ff4970217f4934b6a418c828ad8774/LICENSE) |
| `github.com/go-jose/go-jose/v4` | `v4.1.4` | S | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/go-jose/go-jose/blob/0e59876635f3dbf46d7b5e97b52bb75a3f96e7d9/LICENSE) |
| `github.com/go-logr/logr` | `v1.4.3` | K | Apache-2.0 | [LICENSE](https://github.com/go-logr/logr/blob/38a1c47ef633fa6b2eee6b8f2e1371ba8626e557/LICENSE) |
| `github.com/go-logr/zapr` | `v1.3.0` | K | Apache-2.0 | [LICENSE](https://github.com/go-logr/zapr/blob/78b8af5329abd1ba8695aad821f95fb2e7f4e651/LICENSE) |
| `github.com/go-openapi/jsonpointer` | `v0.21.0` | K | Apache-2.0 | [LICENSE](https://github.com/go-openapi/jsonpointer/blob/8b546b950409bd7b131488a88613339cd8937b7f/LICENSE) |
| `github.com/go-openapi/jsonreference` | `v0.20.2` | K | Apache-2.0 | [LICENSE](https://github.com/go-openapi/jsonreference/blob/1f158e563669961b8e54817e3ea57978d439ffff/LICENSE) |
| `github.com/go-openapi/swag` | `v0.23.0` | K | Apache-2.0 | [LICENSE](https://github.com/go-openapi/swag/blob/53e32e82f758c8e884819330a87aef294ff10c1f/LICENSE) |
| `github.com/go-sql-driver/mysql` | `v1.10.0` | S | MPL-2.0 | [LICENSE](https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/LICENSE) |
| `github.com/go-viper/mapstructure/v2` | `v2.4.0` | K | MIT | [LICENSE](https://github.com/go-viper/mapstructure/blob/b9794a5f0e73d425210d6614ed833067029155f5/LICENSE) |
| `github.com/google/btree` | `v1.1.3` | K | Apache-2.0 | [LICENSE](https://github.com/google/btree/blob/aeba20f7a1e1315badec4eca4fdc9f754f5f880a/LICENSE) |
| `github.com/google/gnostic-models` | `v0.7.0` | K | Apache-2.0 | [LICENSE](https://github.com/google/gnostic-models/blob/82b4ba06c153dcd30e1dbcf93601b3bee5cb3792/LICENSE) |
| `github.com/google/uuid` | `v1.6.0` | S+K | BSD-3-Clause | [LICENSE](https://github.com/google/uuid/blob/0f11ee6918f41a04c201eceeadf612a377bc7fbc/LICENSE) |
| `github.com/josharian/intern` | `v1.0.0` | K | MIT | [license.md](https://proxy.golang.org/github.com/josharian/intern/@v/v1.0.0.zip) |
| `github.com/json-iterator/go` | `v1.1.12` | K | MIT | [LICENSE](https://proxy.golang.org/github.com/json-iterator/go/@v/v1.1.12.zip) |
| `github.com/klauspost/compress` | `v1.19.1` | S | BSD-3-Clause + MIT + Apache-2.0 | [LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/LICENSE) |
| `github.com/mailru/easyjson` | `v0.7.7` | K | MIT | [LICENSE](https://proxy.golang.org/github.com/mailru/easyjson/@v/v0.7.7.zip) |
| `github.com/modern-go/concurrent` | `v0.0.0-20180306012644-bacd9c7ef1dd` | K | Apache-2.0 | [LICENSE](https://proxy.golang.org/github.com/modern-go/concurrent/@v/v0.0.0-20180306012644-bacd9c7ef1dd.zip) |
| `github.com/modern-go/reflect2` | `v1.0.3-0.20250322232337-35a7c28c31ee` | K | Apache-2.0 | [LICENSE](https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/LICENSE) |
| `github.com/munnerz/goautoneg` | `v0.0.0-20191010083416-a7dc8b61c822` | K | BSD-3-Clause | [LICENSE](https://proxy.golang.org/github.com/munnerz/goautoneg/@v/v0.0.0-20191010083416-a7dc8b61c822.zip) |
| `github.com/nats-io/nats.go` | `v1.53.1` | S | Apache-2.0 | [LICENSE](https://github.com/nats-io/nats.go/blob/db1375fcffae2eb0b4ced1b7bad4d47c4447e4ac/LICENSE) |
| `github.com/nats-io/nkeys` | `v0.4.15` | S | Apache-2.0 | [LICENSE](https://github.com/nats-io/nkeys/blob/0f430772b63004155287d5f3c061d41995f74b15/LICENSE) |
| `github.com/nats-io/nuid` | `v1.0.1` | S | Apache-2.0 | [LICENSE](https://proxy.golang.org/github.com/nats-io/nuid/@v/v1.0.1.zip) |
| `github.com/paulmach/orb` | `v0.13.0` | S | MIT | [LICENSE.md](https://github.com/paulmach/orb/blob/a12a48ea0c2bcfdea706cc1ab274c735cbc68939/LICENSE.md) |
| `github.com/pelletier/go-toml/v2` | `v2.2.4` | K | MIT | [LICENSE](https://github.com/pelletier/go-toml/blob/ee07c9203b72060f12e31c04ace80e8a187d5a67/LICENSE) |
| `github.com/pierrec/lz4/v4` | `v4.1.27` | S | BSD-3-Clause | [LICENSE](https://github.com/pierrec/lz4/blob/a296161b6e73e177e902d9cdafbd3e8b9ffaa68f/LICENSE) |
| `github.com/pmezard/go-difflib` | `v1.0.0` | K | BSD-3-Clause | [LICENSE](https://proxy.golang.org/github.com/pmezard/go-difflib/@v/v1.0.0.zip) |
| `github.com/prometheus/client_golang` | `v1.23.2` | K | Apache-2.0 + BSD-3-Clause + MIT | [LICENSE](https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/LICENSE) |
| `github.com/prometheus/client_model` | `v0.6.2` | K | Apache-2.0 | [LICENSE](https://github.com/prometheus/client_model/blob/eb136e513d419e0c31ad750922f0a6f7675c2dee/LICENSE) |
| `github.com/prometheus/common` | `v0.66.1` | K | Apache-2.0 | [LICENSE](https://github.com/prometheus/common/blob/8975dde6db7208309e9872891f24c7301aa77dfb/LICENSE) |
| `github.com/prometheus/procfs` | `v0.17.0` | K Linux | Apache-2.0 | [LICENSE](https://github.com/prometheus/procfs/blob/61fe41207276bc95c4c391762e9ef137385e8a5d/LICENSE) |
| `github.com/sagikazarmark/locafero` | `v0.11.0` | K | MIT | [LICENSE](https://github.com/sagikazarmark/locafero/blob/d051b254767db64f7c9b392375a86f2385879606/LICENSE) |
| `github.com/segmentio/asm` | `v1.2.1` | S | MIT-0 | [LICENSE](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/LICENSE) |
| `github.com/shopspring/decimal` | `v1.4.0` | S | MIT + BSD-3-Clause | [LICENSE](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/LICENSE) |
| `github.com/sourcegraph/conc` | `v0.3.1-0.20240121214520-5f936abd7ae8` | K | MIT | [LICENSE](https://github.com/sourcegraph/conc/blob/5f936abd7ae87036af1f75c95fb9d0daaf00116b/LICENSE) |
| `github.com/spf13/afero` | `v1.15.0` | K | Apache-2.0 | [LICENSE.txt](https://github.com/spf13/afero/blob/399bb34ad9fd8a252ad1d8bfaef96279b66dc774/LICENSE.txt) |
| `github.com/spf13/cast` | `v1.10.0` | K | MIT | [LICENSE](https://github.com/spf13/cast/blob/fc73346bfc4e6597bc520fb6eea04360299e77d2/LICENSE) |
| `github.com/spf13/cobra` | `v1.10.2` | S | Apache-2.0 | [LICENSE.txt](https://github.com/spf13/cobra/blob/88b30ab89da2d0d0abb153818746c5a2d30eccec/LICENSE.txt) |
| `github.com/spf13/pflag` | `v1.0.10` | K | BSD-3-Clause | [LICENSE](https://github.com/spf13/pflag/blob/0491e5702ad2bb108bc519a5221bcc0f52aa9564/LICENSE) |
| `github.com/spf13/pflag` | `v1.0.9` | S | BSD-3-Clause | [LICENSE](https://github.com/spf13/pflag/blob/10438578954bba2527fe5cae3684d4532b064bbe/LICENSE) |
| `github.com/spf13/viper` | `v1.21.0` | K | MIT | [LICENSE](https://github.com/spf13/viper/blob/394040caccbdf5821fa6839386a35f0fb1b1ee9e/LICENSE) |
| `github.com/subosito/gotenv` | `v1.6.0` | K | MIT | [LICENSE](https://github.com/subosito/gotenv/blob/14a05352a5cf0f66fd7cbce114374f56065891f0/LICENSE) |
| `github.com/viber-ops/configra-go` | `v0.1.0-rc.2` | K | Apache-2.0 | [LICENSE](https://github.com/viber-ops/configra-go/blob/bddf8e91cb73ff6fddafbf11b890745a637ea014/LICENSE) |
| `github.com/x448/float16` | `v0.8.4` | K | MIT | [LICENSE](https://proxy.golang.org/github.com/x448/float16/@v/v0.8.4.zip) |
| `go.opentelemetry.io/otel` | `v1.44.0` | S | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/open-telemetry/opentelemetry-go/blob/b62d92831b2dd142f5a0cc89c828270274196877/LICENSE) |
| `go.opentelemetry.io/otel/trace` | `v1.44.0` | S | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/open-telemetry/opentelemetry-go/blob/b62d92831b2dd142f5a0cc89c828270274196877/LICENSE) |
| `go.uber.org/multierr` | `v1.11.0` | S+K | MIT | [LICENSE.txt](https://github.com/uber-go/multierr/blob/de75ae527b39a27afcb50a84427ec7b84021d5f4/LICENSE.txt) |
| `go.uber.org/zap` | `v1.27.0` | K | MIT | [LICENSE](https://github.com/uber-go/zap/blob/fcf8ee58669e358bbd6460bef5c2ee7a53c0803a/LICENSE) |
| `go.uber.org/zap` | `v1.28.0` | S | MIT | [LICENSE](https://github.com/uber-go/zap/blob/5b81b37b81b8e2ed447a6f57991e372ee4fa5c8f/LICENSE) |
| `go.yaml.in/yaml/v2` | `v2.4.3` | K | Apache-2.0 + MIT | [LICENSE](https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/LICENSE), [LICENSE.libyaml](https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/LICENSE.libyaml) |
| `go.yaml.in/yaml/v3` | `v3.0.4` | S+K | Apache-2.0 + MIT | [LICENSE](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE) |
| `golang.org/x/crypto` | `v0.54.0` | S | BSD-3-Clause | [LICENSE](https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/LICENSE) |
| `golang.org/x/net` | `v0.55.0` | K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/net/+/7770ec48d03fec35e378665337b4faca93c38423/LICENSE) |
| `golang.org/x/oauth2` | `v0.36.0` | S+K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/oauth2/+/4d954e69a88d9e1ccb8439f8d5b6cbef230c4ef9/LICENSE) |
| `golang.org/x/sync` | `v0.22.0` | K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/sync/+/1eb64d4bc0cde6da1bb8ebc7f178bb577508e5d0/LICENSE) |
| `golang.org/x/sys` | `v0.47.0` | S+K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/LICENSE) |
| `golang.org/x/term` | `v0.43.0` | K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/term/+/3c3e4855f7d2eb06c3e48933554add9ec6b599b5/LICENSE) |
| `golang.org/x/text` | `v0.40.0` | K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/text/+/724af9c35838492dcaacc1ac51a8a0187c994c54/LICENSE) |
| `golang.org/x/time` | `v0.9.0` | K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/time/+/1ce61fe87e0e5dd90752d2b6c5972f9b6918e77c/LICENSE) |
| `gomodules.xyz/jsonpatch/v2` | `v2.4.0` | K | Apache-2.0 | [LICENSE](https://github.com/gomodules/jsonpatch/blob/17d7994fea15a033020ac51ee05b7dda0afda0fe/LICENSE) |
| `google.golang.org/genproto/googleapis/rpc` | `v0.0.0-20260414002931-afd174a4e478` | K | Apache-2.0 | [LICENSE](https://github.com/googleapis/go-genproto/blob/afd174a4e4785681a98d8dac6439fd597d488b20/LICENSE) |
| `google.golang.org/grpc` | `v1.82.1` | K | Apache-2.0 | [LICENSE](https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/LICENSE) |
| `google.golang.org/protobuf` | `v1.36.11` | K | BSD-3-Clause | [LICENSE](https://go.googlesource.com/protobuf/+/96a179180f0ad6bba9b1e7b6e38d0affb0168e9a/LICENSE) |
| `gopkg.in/evanphx/json-patch.v4` | `v4.13.0` | K | BSD-3-Clause | [LICENSE](https://proxy.golang.org/gopkg.in/evanphx/json-patch.v4/@v/v4.13.0.zip) |
| `gopkg.in/inf.v0` | `v0.9.1` | K | BSD-3-Clause | [LICENSE](https://proxy.golang.org/gopkg.in/inf.v0/@v/v0.9.1.zip) |
| `gopkg.in/yaml.v3` | `v3.0.1` | K | Apache-2.0 + MIT | [LICENSE](https://proxy.golang.org/gopkg.in/yaml.v3/@v/v3.0.1.zip) |
| `k8s.io/api` | `v0.35.0` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes/api/blob/9afe7de0a56c582b06ca094f7309015bc84657a7/LICENSE) |
| `k8s.io/apiextensions-apiserver` | `v0.35.0` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes/apiextensions-apiserver/blob/a8d2a03a6b798832f3b9a63638d404ef89fe5b69/LICENSE) |
| `k8s.io/apimachinery` | `v0.35.0` | K | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/kubernetes/apimachinery/blob/72d71eac265e06713c6d83d7034aac609450243f/LICENSE) |
| `k8s.io/client-go` | `v0.35.0` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes/client-go/blob/9bcb69436287b966d0c5c195efef00aed921fb1b/LICENSE) |
| `k8s.io/klog/v2` | `v2.130.1` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes/klog/blob/75663bb798999a49e3e4c0f2375ed5cca8164194/LICENSE) |
| `k8s.io/kube-openapi` | `v0.0.0-20250910181357-589584f1c912` | K | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/kubernetes/kube-openapi/blob/589584f1c912f4367fe8954f649a59a98b912da5/LICENSE) |
| `k8s.io/utils` | `v0.0.0-20251002143259-bc988d571ff4` | K | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/kubernetes/utils/blob/bc988d571ff40eb17793769e9c1b71ecf8ee9c0f/LICENSE) |
| `sigs.k8s.io/controller-runtime` | `v0.23.1` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes-sigs/controller-runtime/blob/f52bbb8bb1a2275cbe90dec8d6c12d5cacb1a7de/LICENSE) |
| `sigs.k8s.io/json` | `v0.0.0-20250730193827-2d320260d730` | K | Apache-2.0 + BSD-3-Clause | [LICENSE](https://github.com/kubernetes-sigs/json/blob/2d320260d730f3842fef7b08d9a807bdbc617824/LICENSE) |
| `sigs.k8s.io/randfill` | `v1.0.0` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes-sigs/randfill/blob/1b6128de8ceabf6d20c4d81d770bf439c1494960/LICENSE) |
| `sigs.k8s.io/secrets-store-csi-driver` | `v1.6.0` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes-sigs/secrets-store-csi-driver/blob/4960f9637c51ad9cb9ef264481bc6d9a98bf1f1c/LICENSE) |
| `sigs.k8s.io/structured-merge-diff/v6` | `v6.3.2-0.20260122202528-d9cc6641c482` | K | Apache-2.0 | [LICENSE](https://github.com/kubernetes-sigs/structured-merge-diff/blob/d9cc6641c48292946e838bfc3b9bf7c297526757/LICENSE) |
| `sigs.k8s.io/yaml` | `v1.6.0` | K | MIT + BSD-3-Clause + Apache-2.0 | [LICENSE](https://github.com/kubernetes-sigs/yaml/blob/048d724aca2d37ddb5b03c90b5b4550a3a48766d/LICENSE) |

## Required supplemental collection and scope qualifications

1. **Fiat Cryptography inside the standalone Edwards25519 module.** Preserve [scalar.go](https://github.com/FiloSottile/edwards25519/blob/b182a6575cfd9f4fbb1d1d4e487a6b00a3ec06f7/scalar.go) as a non-buildable notice/source-text copy in addition to the module's BSD-3-Clause LICENSE. It carries separate BSD-1-Clause terms for the generated scalar implementation. Its source-retention condition must not be confused with a requirement to publish all Configra code.

2. **Prometheus's embedded MIT helper.** [prometheus/internal/almost_equal.go](https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/prometheus/internal/almost_equal.go) includes Björn Rabenstein's complete MIT notice. Root Apache LICENSE and NOTICE do not reproduce that full block. Also retain the selected [internal/github.com/golang/gddo/LICENSE](https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/internal/github.com/golang/gddo/LICENSE) and root NOTICE.

3. **shopspring/decimal's Go-source text gap.** Its root [LICENSE](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/LICENSE) contains two MIT notices, for Spring and Oguz Bilgic. However, [decimal-go.go](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/decimal-go.go) and [rounding.go](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/rounding.go) retain 2009 Go copyright/BSD-style headers. **Yes: preserving those original headers plus the complete Go BSD-3-Clause license addresses the observed notice-text omission**, alongside both MIT notices. It does not turn the Go-derived files into MIT-only code. The JSON supplies a hash-bound supplemental [Go BSD text](https://github.com/golang/go/blob/go1.26.7/LICENSE); its Go 1.26.7 reference supplies stable terms and is not asserted to identify the historical port commit. This remedies the recorded text gap, not all possible historical provenance questions.

4. **klauspost/compress is a file-specific collection.** Preserve the complete root [LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/LICENSE), selected [internal/snapref/LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/internal/snapref/LICENSE), and [zstd/internal/xxhash/LICENSE.txt](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/internal/xxhash/LICENSE.txt). Root Apache terms are scoped to `gzhttp/*`, which this package graph does not select. The selected amd64 [zstd/matchlen_amd64.s](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/matchlen_amd64.s) explicitly says it was copied from S2, so retain [s2/LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/s2/LICENSE) and the related Snappy authorship record even though the `s2` package itself is not imported. Preserve relevant source credits to Klaus Post, the Go/Snappy-Go authors, Yann Collet, and Caleb Spare.

5. **Nested Go forks.** Selected BSD scopes include Brotli's `flate/LICENSE`, go-jose's `json/LICENSE`, apimachinery's `third_party/forked/golang/LICENSE` and PATENTS, utils' `internal/third_party/forked/golang/LICENSE` and PATENTS, kube-openapi's nested go-json-experiment LICENSE/AUTHORS, and Prometheus's gddo LICENSE. All exact paths/hashes are in the JSON. Discovered legal files in unused test/tool/other package subtrees are listed separately as excluded; they must be reconsidered if those source trees themselves are redistributed.

6. **All YAML identities stay separate.** `go.yaml.in/yaml/v2 v2.4.3` needs both LICENSE and LICENSE.libyaml, plus NOTICE. Both `go.yaml.in/yaml/v3 v3.0.4` and `gopkg.in/yaml.v3 v3.0.1` assign MIT to eight libyaml-derived files and Apache-2.0 to the rest. Their classifier MIT-only result is incomplete. `sigs.k8s.io/yaml` preserves its own composite MIT/Go-BSD/Go-YAML terms; `sigs.k8s.io/json` explicitly assigns BSD to `internal/golang/*` and Apache elsewhere. [LICENSE.libyaml](https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/LICENSE.libyaml), [yaml v3 allocation](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE), [sigs JSON allocation](https://github.com/kubernetes-sigs/json/blob/2d320260d730f3842fef7b08d9a807bdbc617824/LICENSE)

7. **Supplement NOTICE/AUTHORS/PATENTS.** Retain the applicable root NOTICE files for go-oidc, SDK, Prometheus client_golang/client_model/common/procfs, randfill, and YAML, plus gRPC NOTICE.txt. Retain listed AUTHORS and PATENTS as identified source/rights materials; not every such file creates a separate binary-attribution requirement, but notice-only collection must not silently drop applicable material. OpenTelemetry otel and trace each include appended Go BSD text in their root license, which must not be truncated to the Apache portion.

## Classifier gap and assembly review

The only Unknown classification was `github.com/segmentio/asm v1.2.1` for `bswap`, `cpu`, `cpu/arm`, `cpu/arm64`, `cpu/cpuid`, and `cpu/x86`. Its primary license is MIT No Attribution, SPDX [MIT-0](https://spdx.org/licenses/MIT-0.html). The manual resolution remains bound to Origin commit `1cfacc81a878d4a07b13f51f2368cd86893d23fa`, its recorded module sum, and LICENSE SHA-256 `cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443`. Preserve the text and fail review on changed inputs; do not add a blanket Unknown exception. [LICENSE](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/LICENSE)

The tool warned about assembly in Edwards25519, xxhash, klauspost/compress, lz4, Segmentio asm, x/crypto, x/sys, and reflect2. Their indicated files were inspected and byte-matched to their sources; the JSON includes the reviewed paths. The reflect2 assembly stubs are empty files. The notable cross-directory provenance is the S2-derived zstd match-length routine described above. The remaining reviewed files carry applicable enclosing terms, Go BSD headers, or generated-code markers; no additional license identifier was found in those assembly notices. This does not make the classifier capable of discovering arbitrary native-code dependencies or prove every historical code origin.

## Packaging disposition

- Preserve all listed applicable license and notice files. Source-embedded notice and assembly evidence can be copied under non-buildable `.txt` names; hashes below apply to the original bytes, so changing contents requires a separate transformed-artifact hash.
- The MySQL driver is the identified MPL-2.0 application dependency. Distribute its **exact verified module ZIP** as covered source and provide a readable source-access notice; do not extract a buildable vendor tree under `dist/`. The canonical ZIP SHA-256 is `dc93f5770556406e82bf750a980d2316f882a19d883a3689eadb820708c2b651`, byte-identical to the cache, and its module sum matches the binary. [Exact source ZIP](https://proxy.golang.org/github.com/go-sql-driver/mysql/@v/v1.10.0.zip), [MPL requirements](https://www.mozilla.org/en-US/MPL/2.0/)
- Reconcile actual collection output against this manifest, not only classifier counts. Keep separate identities for the two pflag versions and two zap versions. Repeat the inventory when dependencies, target platforms, tags, replacements, or build flags change.
- The first-party SDK rc.2 license-bearing ZIP and first-party archive/image LICENSE/NOTICE copies remain useful completed checks; this report does not turn them into clearance of all third-party/runtime/UI/CA contents. Runtime and CA requirements remain in the separate report, and final UI/package inclusion remains outside this task.

## Machine-readable evidence

The JSON also records per-target package-selection guards from the clean review.2 source worktree at `29b52e5f5838dffbcee3f88da3dd6ff0650b2cfa`. Each guard hashes the UTF-8 bytewise-sorted output of `go list -deps -f '{{.ImportPath}}'` for its command, joined with LF and one final LF, including the standard library, project packages, and main package. All eight current working-root production graphs matched those baselines on 2026-09-12. Enforce the recorded toolchain/build profile as well as count/hash; package-set guards complement the per-file hashes and cannot detect changes that keep the same import paths.

Exactly one JSON block follows. `target_mask` uses `target_bits`; each set bit indicates a binary input that includes the module. `files[].path` is module-relative, except entries explicitly marked `external_to_module`. `assembly_review` and `related_authorship` are conservative evidence-retention records, not claims of extra obligations merely because a file exists. The supplemental Go BSD file is a notice-copy name, not a file claimed to exist inside shopspring's module ZIP. `excluded_discovered_legal_paths` records scoped exclusions rather than silently losing them.

```json
{
  "schema_version": 1,
  "review_date": "2026-09-12",
  "artifact_set": "v0.1.0-rc.2-review.2",
  "scope": "Application Go modules selected for the eight recorded binaries; excludes Go toolchain/runtime, UI npm packages, CA bundle, container OS packages.",
  "toolchain": "go1.26.7",
  "build_flags": {
    "CGO_ENABLED": "0",
    "GOWORK": "off",
    "GOFLAGS": "-mod=readonly"
  },
  "package_profile_spec": {
    "source_commit": "29b52e5f5838dffbcee3f88da3dd6ff0650b2cfa",
    "toolchain": "go1.26.7",
    "command": [
      "go",
      "list",
      "-deps",
      "-f",
      "{{.ImportPath}}",
      "<command_package>"
    ],
    "environment": {
      "CGO_ENABLED": "0",
      "GOWORK": "off",
      "GOFLAGS": "-mod=readonly",
      "GOTOOLCHAIN": "local",
      "GOEXPERIMENT": ""
    },
    "includes_stdlib_and_main": true,
    "hash_algorithm": "sha256",
    "encoding": "UTF-8",
    "sort_order": "bytewise-ascending",
    "line_separator": "\n",
    "final_newline": true,
    "baseline_clean_before_and_after": true,
    "current_root_verification": {
      "date": "2026-09-12",
      "head": "7f1a28f7a86ae635c343de664aab361c0d4722b2",
      "all_profiles_match": true
    }
  },
  "package_profiles": [
    {
      "target": "configra/darwin/amd64",
      "module_directory": ".",
      "command_package": "./cmd/configra",
      "goos": "darwin",
      "goarch": "amd64",
      "goamd64": "v1",
      "package_count": 313,
      "package_paths_sha256": "59a0cde75bbd9a01f9231cec33655c14450a5883b608b51d5d52884100d3f88d"
    },
    {
      "target": "configra-kubernetes/darwin/amd64",
      "module_directory": "kubernetes",
      "command_package": "./cmd/configra-kubernetes",
      "goos": "darwin",
      "goarch": "amd64",
      "goamd64": "v1",
      "package_count": 917,
      "package_paths_sha256": "6bd25d13c8bf2ad1e84ec405992e3d638fe3d3b09afcb7f86d50cc852497f666"
    },
    {
      "target": "configra/darwin/arm64",
      "module_directory": ".",
      "command_package": "./cmd/configra",
      "goos": "darwin",
      "goarch": "arm64",
      "goarm64": "v8.0",
      "package_count": 311,
      "package_paths_sha256": "866d19077e71f3ecda5d7e5e6aa9025138e292779d82ceee3cdf87522a3674ba"
    },
    {
      "target": "configra-kubernetes/darwin/arm64",
      "module_directory": "kubernetes",
      "command_package": "./cmd/configra-kubernetes",
      "goos": "darwin",
      "goarch": "arm64",
      "goarm64": "v8.0",
      "package_count": 916,
      "package_paths_sha256": "105d15b82efbc574a3b70b37eea2393eeabc459a8f0317f0727c991e7d5cdd99"
    },
    {
      "target": "configra/linux/amd64",
      "module_directory": ".",
      "command_package": "./cmd/configra",
      "goos": "linux",
      "goarch": "amd64",
      "goamd64": "v1",
      "package_count": 313,
      "package_paths_sha256": "feb1289f1861d00995bb7124afdc7dfa74f5a526f66388c2fb74c68e515804b9"
    },
    {
      "target": "configra-kubernetes/linux/amd64",
      "module_directory": "kubernetes",
      "command_package": "./cmd/configra-kubernetes",
      "goos": "linux",
      "goarch": "amd64",
      "goamd64": "v1",
      "package_count": 920,
      "package_paths_sha256": "b096bfcf46d058eb01aeda80e4768af83cc64239b988f9bed81042fa7fdc2374"
    },
    {
      "target": "configra/linux/arm64",
      "module_directory": ".",
      "command_package": "./cmd/configra",
      "goos": "linux",
      "goarch": "arm64",
      "goarm64": "v8.0",
      "package_count": 311,
      "package_paths_sha256": "c51b3c6a6b996fb0ad55f0158c4a8460bfbbbea5cdd30b0ca033d01da2bff3b1"
    },
    {
      "target": "configra-kubernetes/linux/arm64",
      "module_directory": "kubernetes",
      "command_package": "./cmd/configra-kubernetes",
      "goos": "linux",
      "goarch": "arm64",
      "goarm64": "v8.0",
      "package_count": 919,
      "package_paths_sha256": "234633c96840c8d26f744cea5590738ae99a1e67fc94fc7d8ecb257ebb0e460a"
    }
  ],
  "target_bits": {
    "0": "configra/darwin/amd64",
    "1": "configra-kubernetes/darwin/amd64",
    "2": "configra/darwin/arm64",
    "3": "configra-kubernetes/darwin/arm64",
    "4": "configra/linux/amd64",
    "5": "configra-kubernetes/linux/amd64",
    "6": "configra/linux/arm64",
    "7": "configra-kubernetes/linux/arm64"
  },
  "artifacts": [
    {
      "target": "configra/darwin/amd64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_darwin_amd64/configra",
      "sha256": "faec69b25b7fa7b7dbd9b93376b598c58f94e2ddd22ade14dac3bef087ad06af",
      "dependency_module_count": 31
    },
    {
      "target": "configra-kubernetes/darwin/amd64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_darwin_amd64/configra-kubernetes",
      "sha256": "01cdcb2e1a1f92b04b140e785abfec1ec0f8fa372b0cecbe162edd9756d2de14",
      "dependency_module_count": 67
    },
    {
      "target": "configra/darwin/arm64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_darwin_arm64/configra",
      "sha256": "d358d99c4623f3941bd16adfe35a5afd2332a0381217997e90c60e803e761e4f",
      "dependency_module_count": 31
    },
    {
      "target": "configra-kubernetes/darwin/arm64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_darwin_arm64/configra-kubernetes",
      "sha256": "cafd7f096e8141acac199a4b58a121b825218f8ca58a48a641aa94ff82f2db07",
      "dependency_module_count": 67
    },
    {
      "target": "configra/linux/amd64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_linux_amd64/configra",
      "sha256": "de7925769e0713774cfdd8d5f06263b32303a0e65b51ce247248f3332870dc1f",
      "dependency_module_count": 31
    },
    {
      "target": "configra-kubernetes/linux/amd64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_linux_amd64/configra-kubernetes",
      "sha256": "29bc0d2fa1392d0d46dce0793554eeff9c2c031bd13d63c18157fb3298475883",
      "dependency_module_count": 68
    },
    {
      "target": "configra/linux/arm64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_linux_arm64/configra",
      "sha256": "684bafc8002b9bd4747f94c2a704d8169827a6017c71167895feaaa9dd1724ed",
      "dependency_module_count": 31
    },
    {
      "target": "configra-kubernetes/linux/arm64",
      "path": ".cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/configra_0.1.0-rc.2-review.2_linux_arm64/configra-kubernetes",
      "sha256": "fd41b62df1526605ad95ed93e42da253c988786034e92fded09d33ea4b36ea0e",
      "dependency_module_count": 68
    }
  ],
  "verification": {
    "module_version_pairs": 93,
    "source_module_sets_match_binary_sets": true,
    "selected_application_files": 4646,
    "primary_legal_files_verified": 128,
    "total_file_records": 169,
    "all_recorded_files_byte_verified": true,
    "classifier": "github.com/google/go-licenses/v2 v2.0.1",
    "classifier_unknown_module": "github.com/segmentio/asm@v1.2.1",
    "additional_unknown_classifications": [],
    "vcs_origin_missing_count": 13
  },
  "collection_notes": [
    "source URLs are provenance citations: retrieve raw file bytes for GitHub/googlesource views, and use archive_inner_path for module ZIP links. The recorded SHA-256 is for the original file contents, not an HTML source-view page.",
    "File paths are module-relative except external_to_module=true supplemental licenses. SHA-256 describes the original source bytes; renaming a source-notice file to .txt preserves that hash if contents are unchanged.",
    "Retain source_notice, assembly_provenance, assembly_review and related_authorship records as non-buildable .txt evidence if collected; these records do not themselves assert a new license obligation beyond their qualified scope.",
    "License arrays represent the file-specific/component combination recorded by the source texts, not blanket OR relicensing choices.",
    "Excluded discovered legal paths were outside selected package ancestry; source archives or copied code may require a broader scope. S2 copied-assembly provenance is explicitly retained despite not importing the s2 package.",
    "Distribute the MPL driver's exact verified module ZIP as covered source, not a buildable extracted vendor tree under dist.",
    "Manual MIT-0 classification must remain bound to module version, Origin hash, module sum and LICENSE SHA-256. Unknowns outside that record remain unresolved.",
    "Retaining shopspring's original Go BSD-style source headers plus the complete Go BSD-3-Clause license text addresses the observed licensing-text omission; it does not relicense those files as MIT or identify the historical port commit."
  ],
  "modules": [
    {
      "module": "filippo.io/edwards25519",
      "version": "v1.2.0",
      "module_sum": "h1:crnVqOiS4jqYleHd9vaKZ+HKtHfllngJIiOpNpoJsjo=",
      "primary_license_identifiers": [
        "BSD-3-Clause",
        "BSD-1-Clause"
      ],
      "qualification": "Root BSD-3-Clause; scalar.go contains separate fiat-crypto BSD-1-Clause terms for generated scalar code.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/FiloSottile/edwards25519",
        "hash": "b182a6575cfd9f4fbb1d1d4e487a6b00a3ec06f7"
      },
      "files": [
        {
          "path": "field/fe_amd64.s",
          "kind": "assembly_review",
          "sha256": "affcfc732685da135acea57e6c0a09d893ad21547128b67c2e9583c23654f7c7",
          "source": "https://github.com/FiloSottile/edwards25519/blob/b182a6575cfd9f4fbb1d1d4e487a6b00a3ec06f7/field/fe_amd64.s",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067",
          "source": "https://github.com/FiloSottile/edwards25519/blob/b182a6575cfd9f4fbb1d1d4e487a6b00a3ec06f7/LICENSE",
          "verified": true
        },
        {
          "path": "scalar.go",
          "kind": "source_notice",
          "sha256": "e4aebdd46ca174b66c9bb782aae062020f6984a20434c9f119ce271a6f30abce",
          "source": "https://github.com/FiloSottile/edwards25519/blob/b182a6575cfd9f4fbb1d1d4e487a6b00a3ec06f7/scalar.go",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/alexedwards/scs/mysqlstore",
      "version": "v0.0.0-20251002162104-209de6e426de",
      "module_sum": "h1:/Y/iIFgV1Ofvk4Euv5gUQ74vgqFZOQ1wlJQ3yz/zYGs=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/alexedwards/scs",
        "hash": "209de6e426de9259665975ce16b91331d228f052",
        "subdir": "mysqlstore"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "60aa83c124df5a317fe85b1ad9c4984ad056e0221d572d195b00d5b45b8f3bb1",
          "source": "https://github.com/alexedwards/scs/blob/209de6e426de9259665975ce16b91331d228f052/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/alexedwards/scs/v2",
      "version": "v2.9.0",
      "module_sum": "h1:xa05mVpwTBm1iLeTMNFfAWpKUm4fXAW7CeAViqBVS90=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/alexedwards/scs",
        "hash": "ab20b3feb5e9981c1f79cee8a97a289810134163"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "60aa83c124df5a317fe85b1ad9c4984ad056e0221d572d195b00d5b45b8f3bb1",
          "source": "https://github.com/alexedwards/scs/blob/ab20b3feb5e9981c1f79cee8a97a289810134163/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/andybalholm/brotli",
      "version": "v1.2.2",
      "module_sum": "h1:HzTuoo2ErYQqf5qvcJInB8uvqSVxRttzkFexPWtnceM=",
      "primary_license_identifiers": [
        "BSD-3-Clause",
        "MIT"
      ],
      "qualification": "File-specific combination across the selected package/license scopes; not an OR choice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/andybalholm/brotli",
        "hash": "785aba538b2118979d2c573eccb287c4da157faf"
      },
      "files": [
        {
          "path": "flate/LICENSE",
          "kind": "license",
          "sha256": "2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067",
          "source": "https://github.com/andybalholm/brotli/blob/785aba538b2118979d2c573eccb287c4da157faf/flate/LICENSE",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "3d180008e36922a4e8daec11c34c7af264fed5962d07924aea928c38e8663c94",
          "source": "https://github.com/andybalholm/brotli/blob/785aba538b2118979d2c573eccb287c4da157faf/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/beorn7/perks",
      "version": "v1.0.1",
      "module_sum": "h1:VlbKKnNfV8bJzeqoa4cOKqO6bYr3WgKZxO8Z16+hsOM=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/beorn7/perks/@v/v1.0.1.zip",
        "sha256": "25bd9e2d94aca770e6dbc1f53725f84f6af4432f631d35dd2c46f96ef0512f1a",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "0db7c9ebb3717e526f34f87dd1ee8bc77d36846e29cb0cec9246f7138fbe962b",
          "source": "https://proxy.golang.org/github.com/beorn7/perks/@v/v1.0.1.zip",
          "archive_inner_path": "github.com/beorn7/perks@v1.0.1/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/cespare/xxhash/v2",
      "version": "v2.3.0",
      "module_sum": "h1:UL815xU9SqsFlibzuggzjXhog7bL6oX9BbNZnL2UFvs=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/cespare/xxhash",
        "hash": "998dce232f17418a7a5721ecf87ca714025a3243"
      },
      "files": [
        {
          "path": "LICENSE.txt",
          "kind": "license",
          "sha256": "f566a9f97bacdaf00d9f21dd991e81dc11201c4e016c86b470799429a1c9a79c",
          "source": "https://github.com/cespare/xxhash/blob/998dce232f17418a7a5721ecf87ca714025a3243/LICENSE.txt",
          "verified": true
        },
        {
          "path": "xxhash_amd64.s",
          "kind": "assembly_review",
          "sha256": "580c39fa974ecc91035f33cc258a4141cf52bc767d2f5283fb5b8609e4a856db",
          "source": "https://github.com/cespare/xxhash/blob/998dce232f17418a7a5721ecf87ca714025a3243/xxhash_amd64.s",
          "verified": true
        },
        {
          "path": "xxhash_arm64.s",
          "kind": "assembly_review",
          "sha256": "f878f122d4af5bf05d12d5cffb9ab841a42aebba32ef551afe153d9b3c2c3ad0",
          "source": "https://github.com/cespare/xxhash/blob/998dce232f17418a7a5721ecf87ca714025a3243/xxhash_arm64.s",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 255
    },
    {
      "module": "github.com/ClickHouse/ch-go",
      "version": "v0.74.0",
      "module_sum": "h1:uYs2m4wIt0ZHSM1E72rg0maCfzhR2V3xWb/vZEgpeWE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/ClickHouse/ch-go",
        "hash": "373f5030e95e0b271822e4c2be33d639c526f196"
      },
      "files": [
        {
          "path": "AUTHORS",
          "kind": "authors",
          "sha256": "f5a4b14af9e8afa4bd3dfbdabd3df297a01f6c3f92bcb1f59c112891c89dd213",
          "source": "https://github.com/ClickHouse/ch-go/blob/373f5030e95e0b271822e4c2be33d639c526f196/AUTHORS",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "22e7fd421de32ec18e6f9e4f62655696386d9d5dc661722df0b6324351e8cb08",
          "source": "https://github.com/ClickHouse/ch-go/blob/373f5030e95e0b271822e4c2be33d639c526f196/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/ClickHouse/clickhouse-go/v2",
      "version": "v2.48.0",
      "module_sum": "h1:auzd4VkapQYhQF8F2Gog7s3x78Bi1JZmByxGbrw3C+4=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/ClickHouse/clickhouse-go",
        "hash": "69b5195a9b2e04a999f9c130a7a9dc1248288689"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "27099b82691d1d17cb18138319e2a4f9980ef59c1186c9b02a3273c757c0f91b",
          "source": "https://github.com/ClickHouse/clickhouse-go/blob/69b5195a9b2e04a999f9c130a7a9dc1248288689/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/coreos/go-oidc/v3",
      "version": "v3.20.0",
      "module_sum": "h1:EtE0WIBHk03N+DqGkY4+UONzzZHk7amKt6IyNd7OsZE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/coreos/go-oidc",
        "hash": "75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cb5e8e7e5f4a3988e1063c142c60dc2df75605f4c46515e776e3aca6df976e14",
          "source": "https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "dccd26c6fd9c296daf44d0bc56bb4efc566edd4880381b3331c9a63e6e471338",
          "source": "https://github.com/coreos/go-oidc/blob/75dfa5c0626c48e0ad8b761fdd9e1dc51cb8498a/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/davecgh/go-spew",
      "version": "v1.1.1",
      "module_sum": "h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=",
      "primary_license_identifiers": [
        "ISC"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/davecgh/go-spew/@v/v1.1.1.zip",
        "sha256": "6b44a843951f371b7010c754ecc3cabefe815d5ced1c5b9409fb2d697e8a890d",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "1b93a317849ee09d3d7e4f1d20c2b78ddb230b4becb12d7c224c927b9d470251",
          "source": "https://proxy.golang.org/github.com/davecgh/go-spew/@v/v1.1.1.zip",
          "archive_inner_path": "github.com/davecgh/go-spew@v1.1.1/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/emicklei/go-restful/v3",
      "version": "v3.12.2",
      "module_sum": "h1:DhwDP0vY3k8ZzE0RunuJy8GhNpPL6zqLkDf9B/a0/xU=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/emicklei/go-restful",
        "hash": "d59fac5bd1b1c244342c44e3e41699b8c03a14c1"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "bf4c2b4f64abc0b760f62c766f0dd5e735a8f1335d5cf2f83db5db65e01399ab",
          "source": "https://github.com/emicklei/go-restful/blob/d59fac5bd1b1c244342c44e3e41699b8c03a14c1/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/evanphx/json-patch/v5",
      "version": "v5.9.11",
      "module_sum": "h1:/8HVnzMq13/3x9TPvjG08wUGqBTmZBsCWzjTM0wiaDU=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/evanphx/json-patch",
        "hash": "84a4bb100ade42a86fce2647c95a7dbcbf569cb2"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "9cb8ce7ebecf456a156e04c999c32b2eeafb3f7d7e05b9435ac8829d6e0bf734",
          "source": "https://github.com/evanphx/json-patch/blob/84a4bb100ade42a86fce2647c95a7dbcbf569cb2/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/fsnotify/fsnotify",
      "version": "v1.9.0",
      "module_sum": "h1:2Ml+OJNzbYCTzsxtv8vKSFD9PbJjmhYF14k/jKC7S9k=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/fsnotify/fsnotify",
        "hash": "ae0e7923765f64fb8061396db7edebb558cf6093"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "2dfbe6d2b5eb18b55148f3805d88648a09e0d17e38428123259afb08c8996e37",
          "source": "https://github.com/fsnotify/fsnotify/blob/ae0e7923765f64fb8061396db7edebb558cf6093/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/fxamacker/cbor/v2",
      "version": "v2.9.0",
      "module_sum": "h1:NpKPmjDBgUfBms6tr6JZkTHtfFGcMKsw3eGcmD/sapM=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/fxamacker/cbor",
        "hash": "d29ad7351b55b1844387cf9306c4101658cc5256"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "78cad457d5ea7318230f3d969d4cdf29cef45524a1fc8ca3a97646da1ad7a841",
          "source": "https://github.com/fxamacker/cbor/blob/d29ad7351b55b1844387cf9306c4101658cc5256/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/go-faster/city",
      "version": "v1.0.1",
      "module_sum": "h1:4WAxSZ3V2Ws4QRDrscLEDcibJY8uf41H6AhXDrNDcGw=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/go-faster/city/@v/v1.0.1.zip",
        "sha256": "14383d1599340763c14141cc91feaf9789167118f342bed49704c65c18a99817",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "ee60dfaa1cb16b6606355feddfcec03bd7305f335d067e261d36ff20716425fd",
          "source": "https://proxy.golang.org/github.com/go-faster/city/@v/v1.0.1.zip",
          "archive_inner_path": "github.com/go-faster/city@v1.0.1/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/go-faster/errors",
      "version": "v0.7.1",
      "module_sum": "h1:MkJTnDoEdi9pDabt1dpWf7AA8/BaSYZqibYyhZ20AYg=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-faster/errors",
        "hash": "a3f192e8d7ff4970217f4934b6a418c828ad8774"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067",
          "source": "https://github.com/go-faster/errors/blob/a3f192e8d7ff4970217f4934b6a418c828ad8774/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/go-jose/go-jose/v4",
      "version": "v4.1.4",
      "module_sum": "h1:moDMcTHmvE6Groj34emNPLs/qtYXRVcd6S7NHbHz3kA=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "File-specific combination across the selected package/license scopes; not an OR choice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-jose/go-jose",
        "hash": "0e59876635f3dbf46d7b5e97b52bb75a3f96e7d9"
      },
      "files": [
        {
          "path": "json/LICENSE",
          "kind": "license",
          "sha256": "dd26a7abddd02e2d0aba97805b31f248ef7835d9e10da289b22e3b8ab78b324d",
          "source": "https://github.com/go-jose/go-jose/blob/0e59876635f3dbf46d7b5e97b52bb75a3f96e7d9/json/LICENSE",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/go-jose/go-jose/blob/0e59876635f3dbf46d7b5e97b52bb75a3f96e7d9/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/go-logr/logr",
      "version": "v1.4.3",
      "module_sum": "h1:CjnDlHq8ikf6E492q6eKboGOC0T8CDaOvkHCIg8idEI=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-logr/logr",
        "hash": "38a1c47ef633fa6b2eee6b8f2e1371ba8626e557"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1",
          "source": "https://github.com/go-logr/logr/blob/38a1c47ef633fa6b2eee6b8f2e1371ba8626e557/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/go-logr/zapr",
      "version": "v1.3.0",
      "module_sum": "h1:XGdV8XW8zdwFiwOA2Dryh1gj2KRQyOOoNmBy4EplIcQ=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-logr/zapr",
        "hash": "78b8af5329abd1ba8695aad821f95fb2e7f4e651"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1",
          "source": "https://github.com/go-logr/zapr/blob/78b8af5329abd1ba8695aad821f95fb2e7f4e651/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/go-openapi/jsonpointer",
      "version": "v0.21.0",
      "module_sum": "h1:YgdVicSA9vH5RiHs9TZW5oyafXZFc6+2Vc1rr/O9oNQ=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-openapi/jsonpointer",
        "hash": "8b546b950409bd7b131488a88613339cd8937b7f"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/go-openapi/jsonpointer/blob/8b546b950409bd7b131488a88613339cd8937b7f/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/go-openapi/jsonreference",
      "version": "v0.20.2",
      "module_sum": "h1:3sVjiK66+uXK/6oQ8xgcRKcFgQ5KXa2KvnJRumpMGbE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-openapi/jsonreference",
        "hash": "1f158e563669961b8e54817e3ea57978d439ffff"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/go-openapi/jsonreference/blob/1f158e563669961b8e54817e3ea57978d439ffff/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/go-openapi/swag",
      "version": "v0.23.0",
      "module_sum": "h1:vsEVJDUo2hPJ2tu0/Xc+4noaxyEffXNIs3cOULZ+GrE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-openapi/swag",
        "hash": "53e32e82f758c8e884819330a87aef294ff10c1f"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/go-openapi/swag/blob/53e32e82f758c8e884819330a87aef294ff10c1f/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/go-sql-driver/mysql",
      "version": "v1.10.0",
      "module_sum": "h1:Q+1LV8DkHJvSYAdR83XzuhDaTykuDx0l6fkXxoWCWfw=",
      "primary_license_identifiers": [
        "MPL-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-sql-driver/mysql",
        "hash": "a065b60ab6d0c8e15468e7709c7f76acf4431647"
      },
      "files": [
        {
          "path": "AUTHORS",
          "kind": "authors",
          "sha256": "d926803a640d9c701aab3b54e70828953ce45e3e25b5078b88dfeae93b2f110a",
          "source": "https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/AUTHORS",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "fab3dd6bdab226f1c08630b1dd917e11fcb4ec5e1e020e2c16f83a0a13863e85",
          "source": "https://github.com/go-sql-driver/mysql/blob/a065b60ab6d0c8e15468e7709c7f76acf4431647/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "covered_source_archive": {
        "url": "https://proxy.golang.org/github.com/go-sql-driver/mysql/@v/v1.10.0.zip",
        "sha256": "dc93f5770556406e82bf750a980d2316f882a19d883a3689eadb820708c2b651",
        "module_sum": "h1:Q+1LV8DkHJvSYAdR83XzuhDaTykuDx0l6fkXxoWCWfw=",
        "inner_root": "github.com/go-sql-driver/mysql@v1.10.0/",
        "verified": true
      },
      "target_mask": 85
    },
    {
      "module": "github.com/go-viper/mapstructure/v2",
      "version": "v2.4.0",
      "module_sum": "h1:EBsztssimR/CONLSZZ04E8qAkxNYq4Qp9LvH92wZUgs=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/go-viper/mapstructure",
        "hash": "b9794a5f0e73d425210d6614ed833067029155f5"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "22adc4abdece712a737573672f082fd61ac2b21df878efb87ffcff4354a07f26",
          "source": "https://github.com/go-viper/mapstructure/blob/b9794a5f0e73d425210d6614ed833067029155f5/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/google/btree",
      "version": "v1.1.3",
      "module_sum": "h1:CVpQJjYgC4VbzxeGVHfvZrv1ctoYCAI8vbl07Fcxlyg=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/google/btree",
        "hash": "aeba20f7a1e1315badec4eca4fdc9f754f5f880a"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/google/btree/blob/aeba20f7a1e1315badec4eca4fdc9f754f5f880a/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/google/gnostic-models",
      "version": "v0.7.0",
      "module_sum": "h1:qwTtogB15McXDaNqTZdzPJRHvaVJlAl+HVQnLmJEJxo=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/google/gnostic-models",
        "hash": "82b4ba06c153dcd30e1dbcf93601b3bee5cb3792"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "8c6db340475136df3c1201d458fa5755698eace76e510471ecc9d857d6083dac",
          "source": "https://github.com/google/gnostic-models/blob/82b4ba06c153dcd30e1dbcf93601b3bee5cb3792/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/google/uuid",
      "version": "v1.6.0",
      "module_sum": "h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/google/uuid",
        "hash": "0f11ee6918f41a04c201eceeadf612a377bc7fbc"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "0a8d61ed3cbfd5312326e8126c31ce9c627a283adc99131b56896d29ada04b2d",
          "source": "https://github.com/google/uuid/blob/0f11ee6918f41a04c201eceeadf612a377bc7fbc/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 255
    },
    {
      "module": "github.com/josharian/intern",
      "version": "v1.0.0",
      "module_sum": "h1:vlS4z54oSdjm0bgjRigI+G1HpF+tI+9rE5LLzOg8HmY=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/josharian/intern/@v/v1.0.0.zip",
        "sha256": "5679bfd11c14adccdb45bd1a0f9cf4b445b95caeed6fb507ba96ecced11c248d",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "license.md",
          "kind": "license",
          "sha256": "ad754e4f7d16f789dd0694828254cb4011928025b0d500339a5c4eded5a0c346",
          "source": "https://proxy.golang.org/github.com/josharian/intern/@v/v1.0.0.zip",
          "archive_inner_path": "github.com/josharian/intern@v1.0.0/license.md",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/json-iterator/go",
      "version": "v1.1.12",
      "module_sum": "h1:PV8peI4a0ysnczrg+LtxykD8LfKY9ML6u2jnxaEnrnM=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/json-iterator/go/@v/v1.1.12.zip",
        "sha256": "d001ea57081afd0e378467c8f4a9b6a51259996bb8bb763f78107eaf12f99501",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "3247931083f058b00760a3c32a9ca0962c05e4d562ad2ffcc1753451fa8d4486",
          "source": "https://proxy.golang.org/github.com/json-iterator/go/@v/v1.1.12.zip",
          "archive_inner_path": "github.com/json-iterator/go@v1.1.12/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/klauspost/compress",
      "version": "v1.19.1",
      "module_sum": "h1:VsB4HPswih7mmZ8WleSFQ75c/Ui1M4trX5oAsJnhSlk=",
      "primary_license_identifiers": [
        "BSD-3-Clause",
        "MIT",
        "Apache-2.0"
      ],
      "qualification": "Preserve full composite root text. Selected code uses BSD-3-Clause and nested MIT xxhash. Root Apache text is scoped to unselected gzhttp; it is retained documentary scope, not an assertion gzhttp is linked. S2 copied assembly provenance requires S2 BSD notice retention.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/klauspost/compress",
        "hash": "2602f4afea09fe72f2b58d4ed04d43a6047a0131"
      },
      "files": [
        {
          "path": "huff0/decompress_amd64.s",
          "kind": "assembly_review",
          "sha256": "1be4b028f6a98b957cf7557629bbc3aaead44fccf0c60f796fdee608b8d7ee9e",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/huff0/decompress_amd64.s",
          "verified": true
        },
        {
          "path": "internal/cpuinfo/cpuinfo_amd64.s",
          "kind": "assembly_review",
          "sha256": "9ea28b6d2e9e2210f12ef7d44af4716a3dc148c4e214f23f001271fd100547b9",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/internal/cpuinfo/cpuinfo_amd64.s",
          "verified": true
        },
        {
          "path": "internal/snapref/LICENSE",
          "kind": "license",
          "sha256": "f69f157b0be75da373605dbc8bbf142e8924ee82d8f44f11bcaf351335bf98cf",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/internal/snapref/LICENSE",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "0d9e582ee4bff57bf1189c9e514e6da7ce277f9cd3bc2d488b22fbb39a6d87cf",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/LICENSE",
          "verified": true
        },
        {
          "path": "s2/LICENSE",
          "kind": "copied_code_license",
          "sha256": "08683b14bda8ae3538abf19e1879e853a39e3b8276a8d673363b529d15a61c1a",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/s2/LICENSE",
          "verified": true
        },
        {
          "path": "snappy/AUTHORS",
          "kind": "related_authorship",
          "sha256": "f6f3821e64460346c1ed1a15fb32b94ad241617276b60726c1350ca315309463",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/snappy/AUTHORS",
          "verified": true
        },
        {
          "path": "zstd/fse_decoder_amd64.s",
          "kind": "assembly_review",
          "sha256": "d25ab2895d0e28f4fa5dc1312e496b050b401ec1bb270ec450fc8ca217806dea",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/fse_decoder_amd64.s",
          "verified": true
        },
        {
          "path": "zstd/fse_decoder_arm64.s",
          "kind": "assembly_review",
          "sha256": "b24881b593aa817f236a1ddee57874a6e61141b57a251675a7ff71d72d3c64b8",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/fse_decoder_arm64.s",
          "verified": true
        },
        {
          "path": "zstd/internal/xxhash/LICENSE.txt",
          "kind": "license",
          "sha256": "f566a9f97bacdaf00d9f21dd991e81dc11201c4e016c86b470799429a1c9a79c",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/internal/xxhash/LICENSE.txt",
          "verified": true
        },
        {
          "path": "zstd/internal/xxhash/xxhash_amd64.s",
          "kind": "assembly_review",
          "sha256": "3796a9c399d49392c3ad83af1452099d24c1e6fb993941d8bd1735879f5edbc8",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/internal/xxhash/xxhash_amd64.s",
          "verified": true
        },
        {
          "path": "zstd/internal/xxhash/xxhash_arm64.s",
          "kind": "assembly_review",
          "sha256": "0e2b30d48c0ab8035e201d06c5b74813e39da76c7dc7e3239f4dd4acba7fbb64",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/internal/xxhash/xxhash_arm64.s",
          "verified": true
        },
        {
          "path": "zstd/matchlen_amd64.s",
          "kind": "assembly_provenance",
          "sha256": "f6983c33f36ef09e255c1d029158fa31adfaf6cbc56d264d3ff6431097cfbc0f",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/matchlen_amd64.s",
          "verified": true
        },
        {
          "path": "zstd/seqdec_amd64.s",
          "kind": "assembly_review",
          "sha256": "6eb5daf9c91f2717e3a1920e97bb82546f47d7392958885133f0e4e877e70b23",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/seqdec_amd64.s",
          "verified": true
        },
        {
          "path": "zstd/seqdec_arm64.s",
          "kind": "assembly_review",
          "sha256": "a1fda36bec7c6dae411006cce54a2b5910f90de1be4c9cba91c4c6a778eaf0ee",
          "source": "https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/seqdec_arm64.s",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [
        "gzhttp/LICENSE",
        "internal/lz4ref/LICENSE",
        "s2/cmd/internal/filepathx/LICENSE",
        "s2/cmd/internal/readahead/LICENSE",
        "snappy/LICENSE",
        "snappy/xerial/LICENSE"
      ],
      "target_mask": 85
    },
    {
      "module": "github.com/mailru/easyjson",
      "version": "v0.7.7",
      "module_sum": "h1:UGYAvKxe3sBsEDzO8ZeWOSlIQfWFlxbzLZe7hwFURr0=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/mailru/easyjson/@v/v0.7.7.zip",
        "sha256": "139387981a220d499c9f47cece42a2002f105e4ee3ab9c74188a7fb8a9be711e",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "87baa92c3100cf37441761262d622db99492ee386a063460d4676143db49af57",
          "source": "https://proxy.golang.org/github.com/mailru/easyjson/@v/v0.7.7.zip",
          "archive_inner_path": "github.com/mailru/easyjson@v0.7.7/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/modern-go/concurrent",
      "version": "v0.0.0-20180306012644-bacd9c7ef1dd",
      "module_sum": "h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/modern-go/concurrent/@v/v0.0.0-20180306012644-bacd9c7ef1dd.zip",
        "sha256": "91ef49599bec459869d94ff3dec128871ab66bd2dfa61041f1e1169f9b4a8073",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://proxy.golang.org/github.com/modern-go/concurrent/@v/v0.0.0-20180306012644-bacd9c7ef1dd.zip",
          "archive_inner_path": "github.com/modern-go/concurrent@v0.0.0-20180306012644-bacd9c7ef1dd/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/modern-go/reflect2",
      "version": "v1.0.3-0.20250322232337-35a7c28c31ee",
      "module_sum": "h1:W5t00kpgFdJifH4BDsTlE89Zl93FEloxaWZfGcifgq8=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/modern-go/reflect2",
        "hash": "35a7c28c31ee079903db043180532306a621943a"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/LICENSE",
          "verified": true
        },
        {
          "path": "reflect2_amd64.s",
          "kind": "assembly_review",
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
          "source": "https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/reflect2_amd64.s",
          "verified": true
        },
        {
          "path": "relfect2_arm64.s",
          "kind": "assembly_review",
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
          "source": "https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/relfect2_arm64.s",
          "verified": true
        },
        {
          "path": "relfect2_mips64x.s",
          "kind": "assembly_review",
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
          "source": "https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/relfect2_mips64x.s",
          "verified": true
        },
        {
          "path": "relfect2_mipsx.s",
          "kind": "assembly_review",
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
          "source": "https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/relfect2_mipsx.s",
          "verified": true
        },
        {
          "path": "relfect2_ppc64x.s",
          "kind": "assembly_review",
          "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
          "source": "https://github.com/modern-go/reflect2/blob/35a7c28c31ee079903db043180532306a621943a/relfect2_ppc64x.s",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/munnerz/goautoneg",
      "version": "v0.0.0-20191010083416-a7dc8b61c822",
      "module_sum": "h1:C3w9PqII01/Oq1c1nUAm88MOHcQC9l5mIlSMApZMrHA=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/munnerz/goautoneg/@v/v0.0.0-20191010083416-a7dc8b61c822.zip",
        "sha256": "3d7ce17916779890be02ea6b3dd6345c3c30c1df502ad9d8b5b9b310e636afd9",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "aa1376b9bc5dea6f30cdefde40c176a254f247d2814d0a9929395138631b2ae0",
          "source": "https://proxy.golang.org/github.com/munnerz/goautoneg/@v/v0.0.0-20191010083416-a7dc8b61c822.zip",
          "archive_inner_path": "github.com/munnerz/goautoneg@v0.0.0-20191010083416-a7dc8b61c822/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/nats-io/nats.go",
      "version": "v1.53.1",
      "module_sum": "h1:Otsq3uLc/kLdjmkNHkXH0jBqwUquwdKFoe3fq6/3/Xo=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/nats-io/nats.go",
        "hash": "db1375fcffae2eb0b4ced1b7bad4d47c4447e4ac"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/nats-io/nats.go/blob/db1375fcffae2eb0b4ced1b7bad4d47c4447e4ac/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/nats-io/nkeys",
      "version": "v0.4.15",
      "module_sum": "h1:JACV5jRVO9V856KOapQ7x+EY8Jo3qw1vJt/9Jpwzkk4=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/nats-io/nkeys",
        "hash": "0f430772b63004155287d5f3c061d41995f74b15"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/nats-io/nkeys/blob/0f430772b63004155287d5f3c061d41995f74b15/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/nats-io/nuid",
      "version": "v1.0.1",
      "module_sum": "h1:5iA8DT8V7q8WK2EScv2padNa/rTESc1KdnPw4TC2paw=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/nats-io/nuid/@v/v1.0.1.zip",
        "sha256": "809d144fbd16f91651a433e28d2008d339e19dafc450c5995e2ed92f1c17c1f3",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://proxy.golang.org/github.com/nats-io/nuid/@v/v1.0.1.zip",
          "archive_inner_path": "github.com/nats-io/nuid@v1.0.1/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/paulmach/orb",
      "version": "v0.13.0",
      "module_sum": "h1:r7n7mQGGF+cj/CbcivEj9J3HGK+XR+yXnvzRdq9saIw=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/paulmach/orb",
        "hash": "a12a48ea0c2bcfdea706cc1ab274c735cbc68939"
      },
      "files": [
        {
          "path": "LICENSE.md",
          "kind": "license",
          "sha256": "91d97518f7a7bd54f8f5fa763a1ae268d23477f37317d787e348d66f56ff7b42",
          "source": "https://github.com/paulmach/orb/blob/a12a48ea0c2bcfdea706cc1ab274c735cbc68939/LICENSE.md",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/pelletier/go-toml/v2",
      "version": "v2.2.4",
      "module_sum": "h1:mye9XuhQ6gvn5h28+VilKrrPoQVanw5PMw/TB0t5Ec4=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/pelletier/go-toml",
        "hash": "ee07c9203b72060f12e31c04ace80e8a187d5a67"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "26844e4b53c5adec04e557fd7dfef281cc0205a7d355626b1c68b778b99e9e7b",
          "source": "https://github.com/pelletier/go-toml/blob/ee07c9203b72060f12e31c04ace80e8a187d5a67/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/pierrec/lz4/v4",
      "version": "v4.1.27",
      "module_sum": "h1:+PhzhWDrjRj89TH2sw43nE3+4+W8lSxIuQadEHZyjUk=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/pierrec/lz4",
        "hash": "a296161b6e73e177e902d9cdafbd3e8b9ffaa68f"
      },
      "files": [
        {
          "path": "internal/lz4block/decode_amd64.s",
          "kind": "assembly_review",
          "sha256": "0b9fe3077ea0341f79af9330383be895897852a23663d4531f10eb48f003da50",
          "source": "https://github.com/pierrec/lz4/blob/a296161b6e73e177e902d9cdafbd3e8b9ffaa68f/internal/lz4block/decode_amd64.s",
          "verified": true
        },
        {
          "path": "internal/lz4block/decode_arm64.s",
          "kind": "assembly_review",
          "sha256": "ebdc73c673f2813fe06d082909d7ea40fdbdf11eafac619add7ddb0464028777",
          "source": "https://github.com/pierrec/lz4/blob/a296161b6e73e177e902d9cdafbd3e8b9ffaa68f/internal/lz4block/decode_arm64.s",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "6a358d2540ca14048f02d366f23787c0a480157e58f058113f0e27168dd4e447",
          "source": "https://github.com/pierrec/lz4/blob/a296161b6e73e177e902d9cdafbd3e8b9ffaa68f/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/pmezard/go-difflib",
      "version": "v1.0.0",
      "module_sum": "h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/pmezard/go-difflib/@v/v1.0.0.zip",
        "sha256": "de04cecc1a4b8d53e4357051026794bcbc54f2e6a260cfac508ce69d5d6457a0",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "2eb550be6801c1ea434feba53bf6d12e7c71c90253e0a9de4a4f46cf88b56477",
          "source": "https://proxy.golang.org/github.com/pmezard/go-difflib/@v/v1.0.0.zip",
          "archive_inner_path": "github.com/pmezard/go-difflib@v1.0.0/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/prometheus/client_golang",
      "version": "v1.23.2",
      "module_sum": "h1:Je96obch5RDVy3FDMndoUsjAhG5Edi49h0RJWRi/o0o=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause",
        "MIT"
      ],
      "qualification": "Apache root/NOTICE; nested gddo BSD license; prometheus/internal/almost_equal.go embeds Björn Rabenstein's MIT notice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/prometheus/client_golang",
        "hash": "8179a560819f2c64ef6ade70e6ae4c73aecaca3c"
      },
      "files": [
        {
          "path": "internal/github.com/golang/gddo/LICENSE",
          "kind": "license",
          "sha256": "dfffb9ec737eaba985243e95a549bd8c3a4d462032cfe603e2a6704c346427b6",
          "source": "https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/internal/github.com/golang/gddo/LICENSE",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "8548bfb2bc1913396df38c548a6ec60b8dbcac3b0800485a40569c8bdd184471",
          "source": "https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/NOTICE",
          "verified": true
        },
        {
          "path": "prometheus/internal/almost_equal.go",
          "kind": "source_notice",
          "sha256": "e22e24dab3b51d50a9f036ad4ee2154bba4e4a5cee61553535423c0d9c340f2a",
          "source": "https://github.com/prometheus/client_golang/blob/8179a560819f2c64ef6ade70e6ae4c73aecaca3c/prometheus/internal/almost_equal.go",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/prometheus/client_model",
      "version": "v0.6.2",
      "module_sum": "h1:oBsgwpGs7iVziMvrGhE53c/GrLUsZdHnqNwqPLxwZyk=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/prometheus/client_model",
        "hash": "eb136e513d419e0c31ad750922f0a6f7675c2dee"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/prometheus/client_model/blob/eb136e513d419e0c31ad750922f0a6f7675c2dee/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "6c79faa15168885fb88a316ae0df18f486deafddefdc16826cdc56dfbd421e24",
          "source": "https://github.com/prometheus/client_model/blob/eb136e513d419e0c31ad750922f0a6f7675c2dee/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/prometheus/common",
      "version": "v0.66.1",
      "module_sum": "h1:h5E0h5/Y8niHc5DlaLlWLArTQI7tMrsfQjHV+d9ZoGs=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/prometheus/common",
        "hash": "8975dde6db7208309e9872891f24c7301aa77dfb"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/prometheus/common/blob/8975dde6db7208309e9872891f24c7301aa77dfb/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "600244c8052c8c1d307043fafacbc83b429c171dd8e08f5a537c0a54014585ee",
          "source": "https://github.com/prometheus/common/blob/8975dde6db7208309e9872891f24c7301aa77dfb/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/prometheus/procfs",
      "version": "v0.17.0",
      "module_sum": "h1:FuLQ+05u4ZI+SS/w9+BWEM2TXiHKsUQ9TADiRH7DuK0=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/prometheus/procfs",
        "hash": "61fe41207276bc95c4c391762e9ef137385e8a5d"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4",
          "source": "https://github.com/prometheus/procfs/blob/61fe41207276bc95c4c391762e9ef137385e8a5d/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "497fcdb9fae55924c12f55f05cbe83a096662e5c591d79b74e4a8f23bab5ffee",
          "source": "https://github.com/prometheus/procfs/blob/61fe41207276bc95c4c391762e9ef137385e8a5d/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 160
    },
    {
      "module": "github.com/sagikazarmark/locafero",
      "version": "v0.11.0",
      "module_sum": "h1:1iurJgmM9G3PA/I+wWYIOw/5SyBtxapeHDcg+AAIFXc=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/sagikazarmark/locafero",
        "hash": "d051b254767db64f7c9b392375a86f2385879606"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c4129f2c2a5eb20e4ac7b2535dab899fde485865106a15189726e2db68ff3483",
          "source": "https://github.com/sagikazarmark/locafero/blob/d051b254767db64f7c9b392375a86f2385879606/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/segmentio/asm",
      "version": "v1.2.1",
      "module_sum": "h1:DTNbBqs57ioxAD4PrArqftgypG4/qNpXoJx8TVXxPR0=",
      "primary_license_identifiers": [
        "MIT-0"
      ],
      "qualification": "Manual MIT-0 classification bound to v1.2.1, Origin commit, module sum and LICENSE SHA-256; no blanket Unknown bypass.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/segmentio/asm",
        "hash": "1cfacc81a878d4a07b13f51f2368cd86893d23fa"
      },
      "files": [
        {
          "path": "bswap/swap64_amd64.s",
          "kind": "assembly_review",
          "sha256": "c707389e61e17869586d366369bb2c7c0348d3576ed8ce88dc28889b6b9beae6",
          "source": "https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/bswap/swap64_amd64.s",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443",
          "source": "https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/shopspring/decimal",
      "version": "v1.4.0",
      "module_sum": "h1:bxl37RwXBklmTi0C79JfXCEBD1cqqHt0bbgBAGFp81k=",
      "primary_license_identifiers": [
        "MIT",
        "BSD-3-Clause"
      ],
      "qualification": "Root LICENSE contains Spring and Oguz Bilgic MIT notices. decimal-go.go and rounding.go retain Go BSD-style headers; the complete Go BSD-3-Clause terms and both original headers address the observed text omission. Supplement is not a claim about the historical Go port version.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/shopspring/decimal",
        "hash": "a2e78c6cff3451d68a784428ce443e5a9021a89f"
      },
      "files": [
        {
          "path": "decimal-go.go",
          "kind": "source_notice",
          "sha256": "e3da06b81105dd26de72443f5980242cd821458d65d5558b44891d1a108cd97f",
          "source": "https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/decimal-go.go",
          "verified": true
        },
        {
          "path": "Go-BSD-3-Clause.LICENSE",
          "kind": "supplemental_upstream_license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://github.com/golang/go/blob/go1.26.7/LICENSE",
          "verified": true,
          "qualification": "Full Go BSD-3-Clause terms supplement the original Go copyright headers retained by decimal-go.go and rounding.go; this toolchain version is not asserted to be their historical port origin.",
          "external_to_module": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b92ba0f6ee02f2309628bfdadb123668a17c016e475ba477b857d33470d9d625",
          "source": "https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/LICENSE",
          "verified": true
        },
        {
          "path": "rounding.go",
          "kind": "source_notice",
          "sha256": "1db1de0fd77a66d2ed5282ef4c95fcb42967c1426dd7cc53ce0c9159978b6d1a",
          "source": "https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/rounding.go",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/sourcegraph/conc",
      "version": "v0.3.1-0.20240121214520-5f936abd7ae8",
      "module_sum": "h1:+jumHNA0Wrelhe64i8F6HNlS8pkoyMv5sreGx2Ry5Rw=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/sourcegraph/conc",
        "hash": "5f936abd7ae87036af1f75c95fb9d0daaf00116b"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "e3dfc7ac698a5b64eefc720531595b9d6e3f6c5da7f741ef15e453709e173ee2",
          "source": "https://github.com/sourcegraph/conc/blob/5f936abd7ae87036af1f75c95fb9d0daaf00116b/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/spf13/afero",
      "version": "v1.15.0",
      "module_sum": "h1:b/YBCLWAJdFWJTN9cLhiXXcD7mzKn9Dm86dNnfyQw1I=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/spf13/afero",
        "hash": "399bb34ad9fd8a252ad1d8bfaef96279b66dc774"
      },
      "files": [
        {
          "path": "LICENSE.txt",
          "kind": "license",
          "sha256": "5e3400b93bbb099e83e52bab885e7441750673c21f97988ca3f1240639b63283",
          "source": "https://github.com/spf13/afero/blob/399bb34ad9fd8a252ad1d8bfaef96279b66dc774/LICENSE.txt",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/spf13/cast",
      "version": "v1.10.0",
      "module_sum": "h1:h2x0u2shc1QuLHfxi+cTJvs30+ZAHOGRic8uyGTDWxY=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/spf13/cast",
        "hash": "fc73346bfc4e6597bc520fb6eea04360299e77d2"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "feb6d17a0e7a64e5ab0f7e2b0e0ee3e69c1a6396626fd554dc0cddaa06851b44",
          "source": "https://github.com/spf13/cast/blob/fc73346bfc4e6597bc520fb6eea04360299e77d2/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/spf13/cobra",
      "version": "v1.10.2",
      "module_sum": "h1:DMTTonx5m65Ic0GOoRY2c16WCbHxOOw6xxezuLaBpcU=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/spf13/cobra",
        "hash": "88b30ab89da2d0d0abb153818746c5a2d30eccec"
      },
      "files": [
        {
          "path": "LICENSE.txt",
          "kind": "license",
          "sha256": "5e3400b93bbb099e83e52bab885e7441750673c21f97988ca3f1240639b63283",
          "source": "https://github.com/spf13/cobra/blob/88b30ab89da2d0d0abb153818746c5a2d30eccec/LICENSE.txt",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/spf13/pflag",
      "version": "v1.0.10",
      "module_sum": "h1:4EBh2KAYBwaONj6b2Ye1GiHfwjqyROoF4RwYO+vPwFk=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/spf13/pflag",
        "hash": "0491e5702ad2bb108bc519a5221bcc0f52aa9564"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b8514c577c1c4b46cee454d5a882b15fa411e72c5bd7f801f241591789fce61a",
          "source": "https://github.com/spf13/pflag/blob/0491e5702ad2bb108bc519a5221bcc0f52aa9564/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/spf13/pflag",
      "version": "v1.0.9",
      "module_sum": "h1:9exaQaMOCwffKiiiYk6/BndUBv+iRViNW+4lEMi0PvY=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/spf13/pflag",
        "hash": "10438578954bba2527fe5cae3684d4532b064bbe"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b8514c577c1c4b46cee454d5a882b15fa411e72c5bd7f801f241591789fce61a",
          "source": "https://github.com/spf13/pflag/blob/10438578954bba2527fe5cae3684d4532b064bbe/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "github.com/spf13/viper",
      "version": "v1.21.0",
      "module_sum": "h1:x5S+0EU27Lbphp4UKm1C+1oQO+rKx36vfCoaVebLFSU=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/spf13/viper",
        "hash": "394040caccbdf5821fa6839386a35f0fb1b1ee9e"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "feb6d17a0e7a64e5ab0f7e2b0e0ee3e69c1a6396626fd554dc0cddaa06851b44",
          "source": "https://github.com/spf13/viper/blob/394040caccbdf5821fa6839386a35f0fb1b1ee9e/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/subosito/gotenv",
      "version": "v1.6.0",
      "module_sum": "h1:9NlTDc1FTs4qu0DDq7AEtTPNw6SVm7uBMsUCUjABIf8=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/subosito/gotenv",
        "hash": "14a05352a5cf0f66fd7cbce114374f56065891f0"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "1fd29c692f599736d92f7c02b7826836c0d2ccc838689b21822f4ce0253c39df",
          "source": "https://github.com/subosito/gotenv/blob/14a05352a5cf0f66fd7cbce114374f56065891f0/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/viber-ops/configra-go",
      "version": "v0.1.0-rc.2",
      "module_sum": "h1:r2ObWg0WK5yhO03gvDypfReW4uIAJYMIgtEUq4ILnwQ=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/viber-ops/configra-go",
        "hash": "bddf8e91cb73ff6fddafbf11b890745a637ea014"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/viber-ops/configra-go/blob/bddf8e91cb73ff6fddafbf11b890745a637ea014/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "2f02607c1c0a7ad8a8cd5c89bb620d68e3b2ad2d438c5878a604773999aeffab",
          "source": "https://github.com/viber-ops/configra-go/blob/bddf8e91cb73ff6fddafbf11b890745a637ea014/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "github.com/x448/float16",
      "version": "v0.8.4",
      "module_sum": "h1:qLwI1I70+NjRFUR3zs1JPUCgaCXSh3SW62uAKT1mSBM=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/github.com/x448/float16/@v/v0.8.4.zip",
        "sha256": "73b24a41037ea999ab66851e3798a0973dbb1f214925915b01f0820f7b2f1500",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "a555f1194fdac34da70fb416968f7e2217b02352c26c1eac2fa45fcb4290ae8d",
          "source": "https://proxy.golang.org/github.com/x448/float16/@v/v0.8.4.zip",
          "archive_inner_path": "github.com/x448/float16@v0.8.4/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "go.opentelemetry.io/otel",
      "version": "v1.44.0",
      "module_sum": "h1:JjwHmHpA4iZ3wBxluu2fbbE7j4kqlE8jXyAyPXH7HqU=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "Root LICENSE includes Apache and appended Go BSD-3-Clause text; preserve the complete file.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/open-telemetry/opentelemetry-go",
        "hash": "b62d92831b2dd142f5a0cc89c828270274196877"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "1ae07514be1d7bb33f0698f8d91fb51b8b9fe1463157ec1c72081a49b9bc6f40",
          "source": "https://github.com/open-telemetry/opentelemetry-go/blob/b62d92831b2dd142f5a0cc89c828270274196877/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "go.opentelemetry.io/otel/trace",
      "version": "v1.44.0",
      "module_sum": "h1:jxF5CsGYCe74MCRx2X4g7WsY/VBKRqqpNvXlX/6gtIk=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "Submodule license inherits the complete repository-root Apache plus Go BSD text.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/open-telemetry/opentelemetry-go",
        "hash": "b62d92831b2dd142f5a0cc89c828270274196877",
        "subdir": "trace"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "1ae07514be1d7bb33f0698f8d91fb51b8b9fe1463157ec1c72081a49b9bc6f40",
          "source": "https://github.com/open-telemetry/opentelemetry-go/blob/b62d92831b2dd142f5a0cc89c828270274196877/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "go.uber.org/multierr",
      "version": "v1.11.0",
      "module_sum": "h1:blXXJkSxSSfBVBlC76pxqeO+LN3aDfLQo+309xJstO0=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/uber-go/multierr",
        "hash": "de75ae527b39a27afcb50a84427ec7b84021d5f4"
      },
      "files": [
        {
          "path": "LICENSE.txt",
          "kind": "license",
          "sha256": "dcdabe03bef2382a130640d1c3a4cd5ec42aba1035095c38272fde694eb72405",
          "source": "https://github.com/uber-go/multierr/blob/de75ae527b39a27afcb50a84427ec7b84021d5f4/LICENSE.txt",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 255
    },
    {
      "module": "go.uber.org/zap",
      "version": "v1.27.0",
      "module_sum": "h1:aJMhYGrd5QSmlpLMr2MftRKl7t8J8PTZPA732ud/XR8=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/uber-go/zap",
        "hash": "fcf8ee58669e358bbd6460bef5c2ee7a53c0803a"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "7de716e70addb64f9305298ef32a9dd68e40d5b3095a5d868ba4461404dbfbcf",
          "source": "https://github.com/uber-go/zap/blob/fcf8ee58669e358bbd6460bef5c2ee7a53c0803a/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "go.uber.org/zap",
      "version": "v1.28.0",
      "module_sum": "h1:IZzaP1Fv73/T/pBMLk4VutPl36uNC+OSUh3JLG3FIjo=",
      "primary_license_identifiers": [
        "MIT"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/uber-go/zap",
        "hash": "5b81b37b81b8e2ed447a6f57991e372ee4fa5c8f"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c2b97b3281be272711909076c8402499b4b5a3216196af47a25b1d3674d86152",
          "source": "https://github.com/uber-go/zap/blob/5b81b37b81b8e2ed447a6f57991e372ee4fa5c8f/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "go.yaml.in/yaml/v2",
      "version": "v2.4.3",
      "module_sum": "h1:6gvOSjQoTB3vt1l+CU+tSyi/HOjfOjRLJ4YwYZGwRO0=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "MIT"
      ],
      "qualification": "File-specific: Apache root plus MIT LICENSE.libyaml for eight libyaml-derived files; not an OR choice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/yaml/go-yaml",
        "hash": "3b57511c5e469cd030f5df7705d1f4208aa8b339"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1",
          "source": "https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/LICENSE",
          "verified": true
        },
        {
          "path": "LICENSE.libyaml",
          "kind": "license",
          "sha256": "a94710b55e03b5285f77d048c5ba61bb9d6ee04a06c0eb90e68821e11b0c707a",
          "source": "https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/LICENSE.libyaml",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "f6c2dd3a67b576eafb89b80200b8b1627230bf3821a0c14cb99a22ac19107d00",
          "source": "https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "go.yaml.in/yaml/v3",
      "version": "v3.0.4",
      "module_sum": "h1:tfq32ie2Jv2UxXFdLJdh3jXuOzWiL1fo0bu/FbuKpbc=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "MIT"
      ],
      "qualification": "File-specific MIT for eight libyaml-derived files and Apache for the rest; retain LICENSE and NOTICE.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/yaml/go-yaml",
        "hash": "c3552c15f996075a7634df5159d9161c67bf3d76"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "d18f6323b71b0b768bb5e9616e36da390fbd39369a81807cca352de4e4e6aa0b",
          "source": "https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "f6c2dd3a67b576eafb89b80200b8b1627230bf3821a0c14cb99a22ac19107d00",
          "source": "https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 255
    },
    {
      "module": "golang.org/x/crypto",
      "version": "v0.54.0",
      "module_sum": "h1:YLIA59K4fiNzHzjnZt2tUJQjQtUWfWbeHBqKtk3eScw=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/crypto",
        "hash": "cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62"
      },
      "files": [
        {
          "path": "blake2b/blake2b_amd64.s",
          "kind": "assembly_review",
          "sha256": "f499772b4f15bc364b1c3d862a3c604c1b95a6ca9a11c131f080f31cc6f373bf",
          "source": "https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/blake2b/blake2b_amd64.s",
          "verified": true
        },
        {
          "path": "blake2b/blake2bAVX2_amd64.s",
          "kind": "assembly_review",
          "sha256": "5c29c80d50428e8541c97c6f31846c8d38b646f0f70bfc6325cd7b620b402651",
          "source": "https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/blake2b/blake2bAVX2_amd64.s",
          "verified": true
        },
        {
          "path": "internal/poly1305/sum_amd64.s",
          "kind": "assembly_review",
          "sha256": "f8959555c2e70f460ba88bca1f37705d6c570c0f99f37650a907e9391a960446",
          "source": "https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/internal/poly1305/sum_amd64.s",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/PATENTS",
          "verified": true
        },
        {
          "path": "salsa20/salsa/salsa20_amd64.s",
          "kind": "assembly_review",
          "sha256": "76483bbb543d9e30081bd9981dbcd2679adfc2a9b8978c8f39bd85916e2890f2",
          "source": "https://go.googlesource.com/crypto/+/cdce021fa6c7d9c7eb2743bfbe551f0a98fd5d62/salsa20/salsa/salsa20_amd64.s",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 85
    },
    {
      "module": "golang.org/x/net",
      "version": "v0.55.0",
      "module_sum": "h1:bcvxaJn3e1U6InsFWt1JUq1aSjnRxLzT2rtD2KfkDF8=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/net",
        "hash": "7770ec48d03fec35e378665337b4faca93c38423"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/net/+/7770ec48d03fec35e378665337b4faca93c38423/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/net/+/7770ec48d03fec35e378665337b4faca93c38423/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "golang.org/x/oauth2",
      "version": "v0.36.0",
      "module_sum": "h1:peZ/1z27fi9hUOFCAZaHyrpWG5lwe0RJEEEeH0ThlIs=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/oauth2",
        "hash": "4d954e69a88d9e1ccb8439f8d5b6cbef230c4ef9"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/oauth2/+/4d954e69a88d9e1ccb8439f8d5b6cbef230c4ef9/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 255
    },
    {
      "module": "golang.org/x/sync",
      "version": "v0.22.0",
      "module_sum": "h1:SZjpbeLmrCk4xhRSZFNZW5gFUeCeFgjekvI/+gfScek=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/sync",
        "hash": "1eb64d4bc0cde6da1bb8ebc7f178bb577508e5d0"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/sync/+/1eb64d4bc0cde6da1bb8ebc7f178bb577508e5d0/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/sync/+/1eb64d4bc0cde6da1bb8ebc7f178bb577508e5d0/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "golang.org/x/sys",
      "version": "v0.47.0",
      "module_sum": "h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/sys",
        "hash": "9e7e939dcafac07e8ab4cffa6e5fc74908413f00"
      },
      "files": [
        {
          "path": "cpu/asm_darwin_arm64_gc.s",
          "kind": "assembly_review",
          "sha256": "6f6f35fbf284f205f49db2410b6554bd72aa1fc8ccf35f30a619482277e31ccd",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/cpu/asm_darwin_arm64_gc.s",
          "verified": true
        },
        {
          "path": "cpu/asm_darwin_x86_gc.s",
          "kind": "assembly_review",
          "sha256": "21022bf7461861b3744775075f4a4ef960c036cc469914684f87a4da9fc8b3a8",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/cpu/asm_darwin_x86_gc.s",
          "verified": true
        },
        {
          "path": "cpu/cpu_arm64.s",
          "kind": "assembly_review",
          "sha256": "704264fabff92f961c0bc68f0cb3cc49a21f62141e44ba709caf0ee2eccb0e5d",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/cpu/cpu_arm64.s",
          "verified": true
        },
        {
          "path": "cpu/cpu_gc_x86.s",
          "kind": "assembly_review",
          "sha256": "74ac7fc7ef9c56c3306238cf031ea8ef7c0312a7116cb9fba6d07f0b1382df80",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/cpu/cpu_gc_x86.s",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/PATENTS",
          "verified": true
        },
        {
          "path": "unix/asm_bsd_amd64.s",
          "kind": "assembly_review",
          "sha256": "1fecd01932d872c0d4ec06178a1860ae12bfab8490056dcf0a9d7a16cd455531",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/unix/asm_bsd_amd64.s",
          "verified": true
        },
        {
          "path": "unix/asm_bsd_arm64.s",
          "kind": "assembly_review",
          "sha256": "f7740a9d925eccd280e54e7971a36508a7d2856d9ef996a394ad5cfd80bec8c3",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/unix/asm_bsd_arm64.s",
          "verified": true
        },
        {
          "path": "unix/asm_linux_amd64.s",
          "kind": "assembly_review",
          "sha256": "14c826e5d2db337e49c32e0b5a66317b58da198874a0eb950c33aac571e9573c",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/unix/asm_linux_amd64.s",
          "verified": true
        },
        {
          "path": "unix/asm_linux_arm64.s",
          "kind": "assembly_review",
          "sha256": "9d1514c08da093cd38b77f03ff5fe265c16500a42295b32c66509dc828a34045",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/unix/asm_linux_arm64.s",
          "verified": true
        },
        {
          "path": "unix/zsyscall_darwin_amd64.s",
          "kind": "assembly_review",
          "sha256": "08e931fd2055f452d04b450a839ce2d87423f28ebc2aa014e391a7a764612b13",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/unix/zsyscall_darwin_amd64.s",
          "verified": true
        },
        {
          "path": "unix/zsyscall_darwin_arm64.s",
          "kind": "assembly_review",
          "sha256": "5daa70eefd10942e6ba8da79d69152b330da1981874d6726d1d09cf8d8a0d30e",
          "source": "https://go.googlesource.com/sys/+/9e7e939dcafac07e8ab4cffa6e5fc74908413f00/unix/zsyscall_darwin_arm64.s",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 255
    },
    {
      "module": "golang.org/x/term",
      "version": "v0.43.0",
      "module_sum": "h1:S4RLU2sB31O/NCl+zFN9Aru9A/Cq2aqKpTZJ6B+DwT4=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/term",
        "hash": "3c3e4855f7d2eb06c3e48933554add9ec6b599b5"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/term/+/3c3e4855f7d2eb06c3e48933554add9ec6b599b5/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/term/+/3c3e4855f7d2eb06c3e48933554add9ec6b599b5/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "golang.org/x/text",
      "version": "v0.40.0",
      "module_sum": "h1:Ub2Z6/xjgF1WrYQz2nuITOEegKFtiIy+rieRJ5lHZKs=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/text",
        "hash": "724af9c35838492dcaacc1ac51a8a0187c994c54"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/text/+/724af9c35838492dcaacc1ac51a8a0187c994c54/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/text/+/724af9c35838492dcaacc1ac51a8a0187c994c54/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "golang.org/x/time",
      "version": "v0.9.0",
      "module_sum": "h1:EsRrnYcQiGH+5FfbgvV4AP7qEZstoyrHB0DzarOQ4ZY=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/time",
        "hash": "1ce61fe87e0e5dd90752d2b6c5972f9b6918e77c"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad",
          "source": "https://go.googlesource.com/time/+/1ce61fe87e0e5dd90752d2b6c5972f9b6918e77c/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/time/+/1ce61fe87e0e5dd90752d2b6c5972f9b6918e77c/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "gomodules.xyz/jsonpatch/v2",
      "version": "v2.4.0",
      "module_sum": "h1:Ci3iUJyx9UeRx7CeFN8ARgGbkESwJK+KB9lLcWxY/Zw=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/gomodules/jsonpatch",
        "hash": "17d7994fea15a033020ac51ee05b7dda0afda0fe"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "c6596eb7be8581c18be736c846fb9173b69eccf6ef94c5135893ec56bd92ba08",
          "source": "https://github.com/gomodules/jsonpatch/blob/17d7994fea15a033020ac51ee05b7dda0afda0fe/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "google.golang.org/genproto/googleapis/rpc",
      "version": "v0.0.0-20260414002931-afd174a4e478",
      "module_sum": "h1:RmoJA1ujG+/lRGNfUnOMfhCy5EipVMyvUE+KNbPbTlw=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/googleapis/go-genproto",
        "hash": "afd174a4e4785681a98d8dac6439fd597d488b20",
        "subdir": "googleapis/rpc"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/googleapis/go-genproto/blob/afd174a4e4785681a98d8dac6439fd597d488b20/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "google.golang.org/grpc",
      "version": "v1.82.1",
      "module_sum": "h1:NnAxzGRA0677vCa4BUkOAnO5+FfQqVl9iUXeD0IqcGE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/grpc/grpc-go",
        "hash": "ebd8f06a09426fbece97157c95c3917abff28f4e"
      },
      "files": [
        {
          "path": "AUTHORS",
          "kind": "authors",
          "sha256": "627c695f80fdac6412d66e6ad4160a4acb41923ca7ee3fd7e112e105f0ff46b9",
          "source": "https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/AUTHORS",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE.txt",
          "kind": "notice",
          "sha256": "693ff28ec216d5112ac1bbfe64ef539005867d1c7bd427b57d579683293b947f",
          "source": "https://github.com/grpc/grpc-go/blob/ebd8f06a09426fbece97157c95c3917abff28f4e/NOTICE.txt",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "google.golang.org/protobuf",
      "version": "v1.36.11",
      "module_sum": "h1:fV6ZwhNocDyBLK0dj+fg8ektcVegBBuEolpbTQyBNVE=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://go.googlesource.com/protobuf",
        "hash": "96a179180f0ad6bba9b1e7b6e38d0affb0168e9a"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "4835612df0098ca95f8e7d9e3bffcb02358d435dbb38057c844c99d7f725eb20",
          "source": "https://go.googlesource.com/protobuf/+/96a179180f0ad6bba9b1e7b6e38d0affb0168e9a/LICENSE",
          "verified": true
        },
        {
          "path": "PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://go.googlesource.com/protobuf/+/96a179180f0ad6bba9b1e7b6e38d0affb0168e9a/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "gopkg.in/evanphx/json-patch.v4",
      "version": "v4.13.0",
      "module_sum": "h1:czT3CmqEaQ1aanPc5SdlgQrrEIb8w/wwCvWWnfEbYzo=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://gopkg.in/evanphx/json-patch.v4",
        "hash": "84a4bb100ade42a86fce2647c95a7dbcbf569cb2"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "9cb8ce7ebecf456a156e04c999c32b2eeafb3f7d7e05b9435ac8829d6e0bf734",
          "source": "https://proxy.golang.org/gopkg.in/evanphx/json-patch.v4/@v/v4.13.0.zip",
          "archive_inner_path": "gopkg.in/evanphx/json-patch.v4@v4.13.0/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "gopkg.in/inf.v0",
      "version": "v0.9.1",
      "module_sum": "h1:73M5CoZyi3ZLMOyDlQh031Cx6N9NDJ2Vvfl76EDAgDc=",
      "primary_license_identifiers": [
        "BSD-3-Clause"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/gopkg.in/inf.v0/@v/v0.9.1.zip",
        "sha256": "08abac18c95cc43b725d4925f63309398d618beab68b4669659b61255e5374a0",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "050855d9ceedf916a9e9f30d20c6f61a484448d2c2ed5810934bc8aef43861b4",
          "source": "https://proxy.golang.org/gopkg.in/inf.v0/@v/v0.9.1.zip",
          "archive_inner_path": "gopkg.in/inf.v0@v0.9.1/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "gopkg.in/yaml.v3",
      "version": "v3.0.1",
      "module_sum": "h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "MIT"
      ],
      "qualification": "File-specific MIT for eight libyaml-derived files and Apache for the rest; retain LICENSE and NOTICE.",
      "origin": {
        "kind": "module_archive",
        "url": "https://proxy.golang.org/gopkg.in/yaml.v3/@v/v3.0.1.zip",
        "sha256": "aab8fbc4e6300ea08e6afe1caea18a21c90c79f489f52c53e2f20431f1a9a015",
        "vcs_hash": null,
        "qualification": "The canonical Go module metadata lacks VCS Origin; verified archive identity is recorded instead of inventing a commit."
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "d18f6323b71b0b768bb5e9616e36da390fbd39369a81807cca352de4e4e6aa0b",
          "source": "https://proxy.golang.org/gopkg.in/yaml.v3/@v/v3.0.1.zip",
          "archive_inner_path": "gopkg.in/yaml.v3@v3.0.1/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "f6c2dd3a67b576eafb89b80200b8b1627230bf3821a0c14cb99a22ac19107d00",
          "source": "https://proxy.golang.org/gopkg.in/yaml.v3/@v/v3.0.1.zip",
          "archive_inner_path": "gopkg.in/yaml.v3@v3.0.1/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "k8s.io/api",
      "version": "v0.35.0",
      "module_sum": "h1:iBAU5LTyBI9vw3L5glmat1njFK34srdLmktWwLTprlY=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/api",
        "hash": "9afe7de0a56c582b06ca094f7309015bc84657a7"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/api/blob/9afe7de0a56c582b06ca094f7309015bc84657a7/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "k8s.io/apiextensions-apiserver",
      "version": "v0.35.0",
      "module_sum": "h1:3xHk2rTOdWXXJM+RDQZJvdx0yEOgC0FgQ1PlJatA5T4=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/apiextensions-apiserver",
        "hash": "a8d2a03a6b798832f3b9a63638d404ef89fe5b69"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/apiextensions-apiserver/blob/a8d2a03a6b798832f3b9a63638d404ef89fe5b69/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "k8s.io/apimachinery",
      "version": "v0.35.0",
      "module_sum": "h1:Z2L3IHvPVv/MJ7xRxHEtk6GoJElaAqDCCU0S6ncYok8=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "File-specific combination across the selected package/license scopes; not an OR choice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/apimachinery",
        "hash": "72d71eac265e06713c6d83d7034aac609450243f"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/apimachinery/blob/72d71eac265e06713c6d83d7034aac609450243f/LICENSE",
          "verified": true
        },
        {
          "path": "third_party/forked/golang/LICENSE",
          "kind": "license",
          "sha256": "2d36597f7117c38b006835ae7f537487207d8ec407aa9d9980794b2030cbc067",
          "source": "https://github.com/kubernetes/apimachinery/blob/72d71eac265e06713c6d83d7034aac609450243f/third_party/forked/golang/LICENSE",
          "verified": true
        },
        {
          "path": "third_party/forked/golang/PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://github.com/kubernetes/apimachinery/blob/72d71eac265e06713c6d83d7034aac609450243f/third_party/forked/golang/PATENTS",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "k8s.io/client-go",
      "version": "v0.35.0",
      "module_sum": "h1:IAW0ifFbfQQwQmga0UdoH0yvdqrbwMdq9vIFEhRpxBE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/client-go",
        "hash": "9bcb69436287b966d0c5c195efef00aed921fb1b"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/client-go/blob/9bcb69436287b966d0c5c195efef00aed921fb1b/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [
        "third_party/forked/golang/LICENSE",
        "third_party/forked/golang/PATENTS"
      ],
      "target_mask": 170
    },
    {
      "module": "k8s.io/klog/v2",
      "version": "v2.130.1",
      "module_sum": "h1:n9Xl7H1Xvksem4KFG4PYbdQCQxqc/tTUyrgXaOhHSzk=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/klog",
        "hash": "75663bb798999a49e3e4c0f2375ed5cca8164194"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "73ba74dfaa520b49a401b5d21459a8523a146f3b7518a833eea5efa85130bf68",
          "source": "https://github.com/kubernetes/klog/blob/75663bb798999a49e3e4c0f2375ed5cca8164194/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "k8s.io/kube-openapi",
      "version": "v0.0.0-20250910181357-589584f1c912",
      "module_sum": "h1:Y3gxNAuB0OBLImH611+UDZcmKS3g6CthxToOb37KgwE=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "File-specific combination across the selected package/license scopes; not an OR choice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/kube-openapi",
        "hash": "589584f1c912f4367fe8954f649a59a98b912da5"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/kube-openapi/blob/589584f1c912f4367fe8954f649a59a98b912da5/LICENSE",
          "verified": true
        },
        {
          "path": "pkg/internal/third_party/go-json-experiment/json/AUTHORS",
          "kind": "authors",
          "sha256": "5aa3167c44245f0b12b27195d85c6a8c8e067cfdd511059daa4d7b3c5b232129",
          "source": "https://github.com/kubernetes/kube-openapi/blob/589584f1c912f4367fe8954f649a59a98b912da5/pkg/internal/third_party/go-json-experiment/json/AUTHORS",
          "verified": true
        },
        {
          "path": "pkg/internal/third_party/go-json-experiment/json/LICENSE",
          "kind": "license",
          "sha256": "14a34c4db2d21bf9cf80d028b802cd22fed9bf597a6c2db7ce30ee6ffd04967a",
          "source": "https://github.com/kubernetes/kube-openapi/blob/589584f1c912f4367fe8954f649a59a98b912da5/pkg/internal/third_party/go-json-experiment/json/LICENSE",
          "verified": true
        },
        {
          "path": "pkg/validation/spec/LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/kube-openapi/blob/589584f1c912f4367fe8954f649a59a98b912da5/pkg/validation/spec/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [
        "pkg/internal/third_party/govalidator/LICENSE",
        "pkg/validation/errors/LICENSE",
        "pkg/validation/strfmt/LICENSE",
        "pkg/validation/validate/LICENSE"
      ],
      "target_mask": 170
    },
    {
      "module": "k8s.io/utils",
      "version": "v0.0.0-20251002143259-bc988d571ff4",
      "module_sum": "h1:SjGebBtkBqHFOli+05xYbK8YF1Dzkbzn+gDM4X9T4Ck=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "File-specific combination across the selected package/license scopes; not an OR choice.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes/utils",
        "hash": "bc988d571ff40eb17793769e9c1b71ecf8ee9c0f"
      },
      "files": [
        {
          "path": "internal/third_party/forked/golang/LICENSE",
          "kind": "license",
          "sha256": "dd26a7abddd02e2d0aba97805b31f248ef7835d9e10da289b22e3b8ab78b324d",
          "source": "https://github.com/kubernetes/utils/blob/bc988d571ff40eb17793769e9c1b71ecf8ee9c0f/internal/third_party/forked/golang/LICENSE",
          "verified": true
        },
        {
          "path": "internal/third_party/forked/golang/PATENTS",
          "kind": "patent_notice",
          "sha256": "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc",
          "source": "https://github.com/kubernetes/utils/blob/bc988d571ff40eb17793769e9c1b71ecf8ee9c0f/internal/third_party/forked/golang/PATENTS",
          "verified": true
        },
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30",
          "source": "https://github.com/kubernetes/utils/blob/bc988d571ff40eb17793769e9c1b71ecf8ee9c0f/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [
        "inotify/LICENSE",
        "inotify/PATENTS",
        "third_party/forked/golang/LICENSE",
        "third_party/forked/golang/PATENTS"
      ],
      "target_mask": 170
    },
    {
      "module": "sigs.k8s.io/controller-runtime",
      "version": "v0.23.1",
      "module_sum": "h1:TjJSM80Nf43Mg21+RCy3J70aj/W6KyvDtOlpKf+PupE=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes-sigs/controller-runtime",
        "hash": "f52bbb8bb1a2275cbe90dec8d6c12d5cacb1a7de"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1",
          "source": "https://github.com/kubernetes-sigs/controller-runtime/blob/f52bbb8bb1a2275cbe90dec8d6c12d5cacb1a7de/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "sigs.k8s.io/json",
      "version": "v0.0.0-20250730193827-2d320260d730",
      "module_sum": "h1:IpInykpT6ceI+QxKBbEflcR5EXP7sU1kvOlxwZh5txg=",
      "primary_license_identifiers": [
        "Apache-2.0",
        "BSD-3-Clause"
      ],
      "qualification": "Root LICENSE explicitly assigns BSD to internal/golang/* and Apache to other files.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes-sigs/json",
        "hash": "2d320260d730f3842fef7b08d9a807bdbc617824"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "ef523ecf6292d4b80e02e6003dcbaf386e774efe70e8607ad762b1437d1f383b",
          "source": "https://github.com/kubernetes-sigs/json/blob/2d320260d730f3842fef7b08d9a807bdbc617824/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "sigs.k8s.io/randfill",
      "version": "v1.0.0",
      "module_sum": "h1:JfjMILfT8A6RbawdsK2JXGBR5AQVfd+9TbzrlneTyrU=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes-sigs/randfill",
        "hash": "1b6128de8ceabf6d20c4d81d770bf439c1494960"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "fafd604c0c068855ab2ec82d6d962d21fb0d08e13085f01ac4a8463053fa1138",
          "source": "https://github.com/kubernetes-sigs/randfill/blob/1b6128de8ceabf6d20c4d81d770bf439c1494960/LICENSE",
          "verified": true
        },
        {
          "path": "NOTICE",
          "kind": "notice",
          "sha256": "69b2d8ca988fc62b41cd6217562c7804aed88826c393f40512e22e4aaa5ea38f",
          "source": "https://github.com/kubernetes-sigs/randfill/blob/1b6128de8ceabf6d20c4d81d770bf439c1494960/NOTICE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "sigs.k8s.io/secrets-store-csi-driver",
      "version": "v1.6.0",
      "module_sum": "h1:YpKG/2hJkp3EkRGpH5SPxg1/5AkmeD5pwHNKIlE90FU=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes-sigs/secrets-store-csi-driver",
        "hash": "4960f9637c51ad9cb9ef264481bc6d9a98bf1f1c"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "43070e2d4e532684de521b885f385d0841030efa2b1a20bafb76133a5e1379c1",
          "source": "https://github.com/kubernetes-sigs/secrets-store-csi-driver/blob/4960f9637c51ad9cb9ef264481bc6d9a98bf1f1c/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [
        "third_party/japaric/trust/LICENSE"
      ],
      "target_mask": 170
    },
    {
      "module": "sigs.k8s.io/structured-merge-diff/v6",
      "version": "v6.3.2-0.20260122202528-d9cc6641c482",
      "module_sum": "h1:2WOzJpHUBVrrkDjU4KBT8n5LDcj824eX0I5UKcgeRUs=",
      "primary_license_identifiers": [
        "Apache-2.0"
      ],
      "qualification": "Root license for the selected package scopes.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes-sigs/structured-merge-diff",
        "hash": "d9cc6641c48292946e838bfc3b9bf7c297526757"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "b40930bbcf80744c86c46a12bc9da056641d722716c378f5659b9e555ef833e1",
          "source": "https://github.com/kubernetes-sigs/structured-merge-diff/blob/d9cc6641c48292946e838bfc3b9bf7c297526757/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    },
    {
      "module": "sigs.k8s.io/yaml",
      "version": "v1.6.0",
      "module_sum": "h1:G8fkbMSAFqgEFgh4b1wmtzDnioxFCUgTZhlbj5P9QYs=",
      "primary_license_identifiers": [
        "MIT",
        "BSD-3-Clause",
        "Apache-2.0"
      ],
      "qualification": "Composite root license retains original MIT, Go BSD, and Go-YAML MIT/Apache terms. Imported YAML modules remain separately inventoried.",
      "origin": {
        "kind": "vcs",
        "vcs": "git",
        "url": "https://github.com/kubernetes-sigs/yaml",
        "hash": "048d724aca2d37ddb5b03c90b5b4550a3a48766d"
      },
      "files": [
        {
          "path": "LICENSE",
          "kind": "license",
          "sha256": "5dc79da695aea39a47245008a168e3f7d461b686da8bec299fd55fee0393cbcc",
          "source": "https://github.com/kubernetes-sigs/yaml/blob/048d724aca2d37ddb5b03c90b5b4550a3a48766d/LICENSE",
          "verified": true
        }
      ],
      "excluded_discovered_legal_paths": [],
      "target_mask": 170
    }
  ]
}
```

## Limits and completion record

This completes the declared-license/material inventory for the 93 module/version pairs in these exact eight binary inputs, including the documented source and assembly exceptions. It is not legal certification, a full historical authorship audit, or a guarantee that undeclared third-party material can never exist. The reviewed source-package sets match the binaries' dependency metadata; symbol-level dead stripping, non-Go data provenance beyond the inspected notices, and packaging transformations require their own evidence.

The 13 missing VCS-Origin records remain explicitly archive-bound. Final collectors must also verify hashes and contents of produced notice bundles, preserve source-access availability, and prove the intended files reach every release archive, image, and served UI. No other source, dependency, package, reference, or release was modified for this research.
