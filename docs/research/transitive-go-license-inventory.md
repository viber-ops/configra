# Transitive Go license inventory for review.2 binaries

Reviewed 2026-09-12. This report inventories **93 distinct application-module/version pairs** in the eight `v0.1.0-rc.2-review.2` executables under `.cache/release-license-check-20260912/dist/v0.1.0-rc.2-review.2/`. It covers the server and Kubernetes command on Darwin/Linux and amd64/arm64. The Go runtime, UI/npm dependencies, and container CA bundle have separate scope; see [distribution requirements](distribution-license-requirements.md).

## Result and evidence boundary

The module/version sets obtained from target-matched source package loading exactly match the embedded dependency records of all eight binaries. Server binaries contain 31 dependency modules each; Kubernetes contains 67 on Darwin and 68 on Linux. The Linux-only difference is `github.com/prometheus/procfs`. All eight binaries report Go 1.26.7 and `CGO_ENABLED=0`. Their hashes are recorded under [machine-readable evidence](#machine-readable-evidence).

The installed host-native `go-licenses v2.0.1` was verified through its build metadata and run in CSV mode for all eight GOOS/GOARCH combinations, with Go 1.26.7, `CGO_ENABLED=0`, `GOWORK=off`, read-only module mode, and offline module resolution. It reported only the already-known Segmentio MIT-0 classification gap. **CSV returned exit code 0 while emitting Unknown rows/errors**, so success status alone is not an acceptance gate.

The review also compared **128 applicable standalone license/NOTICE/AUTHORS/PATENTS files** with pinned upstream files or exact canonical module-archive entries. Every comparison matched. Source selection covered 4,646 application files; legal notices and mixed-license declarations in source headers, READMEs, and the warned assembly paths were reviewed. Supplemental source/assembly and copied-code records bring the machine-readable collection to **169 files**, all with verified source bytes and SHA-256. This is a package/file-scope inventory, not a proof of every surviving linker symbol or the historical authorship of every copied algorithm.

Thirteen older modules have no VCS Origin in the available canonical Go metadata. Their records use verified canonical archive URLs, archive hashes, and Go module sums; a full Git commit is explicitly unknown. Other origins use the exact available VCS hash. Classifier-generated URLs were not trusted: for example, inherited submodule licenses may live at a repository root.

## Per-module/version inventory

`S` means all four server targets; `K` means all four Kubernetes targets; `S+K` means all eight. Licenses joined with `+` describe a component/file combination, **not a blanket OR relicensing choice**. The qualification and file lists in the linked manifest are authoritative for the reviewed scope. In particular, the full klauspost root text contains Apache terms for an unselected subtree; retaining that text does not establish that subtree is linked.

Every evidence link is pinned. Where it points to a module ZIP, the table filename is relative to the ZIP's `<module>@<version>/` root; the exact inner path is also in the linked manifest. The main Configra modules are not dependency rows: their first-party LICENSE/NOTICE are handled separately.

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

3. **shopspring/decimal's Go-source text gap.** Its root [LICENSE](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/LICENSE) contains two MIT notices, for Spring and Oguz Bilgic. However, [decimal-go.go](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/decimal-go.go) and [rounding.go](https://github.com/shopspring/decimal/blob/a2e78c6cff3451d68a784428ce443e5a9021a89f/rounding.go) retain 2009 Go copyright/BSD-style headers. **Yes: preserving those original headers plus the complete Go BSD-3-Clause license addresses the observed notice-text omission**, alongside both MIT notices. It does not turn the Go-derived files into MIT-only code. The manifest supplies a hash-bound supplemental [Go BSD text](https://github.com/golang/go/blob/go1.26.7/LICENSE); its Go 1.26.7 reference supplies stable terms and is not asserted to identify the historical port commit. This remedies the recorded text gap, not all possible historical provenance questions.

4. **klauspost/compress is a file-specific collection.** Preserve the complete root [LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/LICENSE), selected [internal/snapref/LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/internal/snapref/LICENSE), and [zstd/internal/xxhash/LICENSE.txt](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/internal/xxhash/LICENSE.txt). Root Apache terms are scoped to `gzhttp/*`, which this package graph does not select. The selected amd64 [zstd/matchlen_amd64.s](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/zstd/matchlen_amd64.s) explicitly says it was copied from S2, so retain [s2/LICENSE](https://github.com/klauspost/compress/blob/2602f4afea09fe72f2b58d4ed04d43a6047a0131/s2/LICENSE) and the related Snappy authorship record even though the `s2` package itself is not imported. Preserve relevant source credits to Klaus Post, the Go/Snappy-Go authors, Yann Collet, and Caleb Spare.

5. **Nested Go forks.** Selected BSD scopes include Brotli's `flate/LICENSE`, go-jose's `json/LICENSE`, apimachinery's `third_party/forked/golang/LICENSE` and PATENTS, utils' `internal/third_party/forked/golang/LICENSE` and PATENTS, kube-openapi's nested go-json-experiment LICENSE/AUTHORS, and Prometheus's gddo LICENSE. All exact paths/hashes are in the linked manifest. Discovered legal files in unused test/tool/other package subtrees are listed separately as excluded; they must be reconsidered if those source trees themselves are redistributed.

6. **All YAML identities stay separate.** `go.yaml.in/yaml/v2 v2.4.3` needs both LICENSE and LICENSE.libyaml, plus NOTICE. Both `go.yaml.in/yaml/v3 v3.0.4` and `gopkg.in/yaml.v3 v3.0.1` assign MIT to eight libyaml-derived files and Apache-2.0 to the rest. Their classifier MIT-only result is incomplete. `sigs.k8s.io/yaml` preserves its own composite MIT/Go-BSD/Go-YAML terms; `sigs.k8s.io/json` explicitly assigns BSD to `internal/golang/*` and Apache elsewhere. [LICENSE.libyaml](https://github.com/yaml/go-yaml/blob/3b57511c5e469cd030f5df7705d1f4208aa8b339/LICENSE.libyaml), [yaml v3 allocation](https://github.com/yaml/go-yaml/blob/c3552c15f996075a7634df5159d9161c67bf3d76/LICENSE), [sigs JSON allocation](https://github.com/kubernetes-sigs/json/blob/2d320260d730f3842fef7b08d9a807bdbc617824/LICENSE)

7. **Supplement NOTICE/AUTHORS/PATENTS.** Retain the applicable root NOTICE files for go-oidc, SDK, Prometheus client_golang/client_model/common/procfs, randfill, and YAML, plus gRPC NOTICE.txt. Retain listed AUTHORS and PATENTS as identified source/rights materials; not every such file creates a separate binary-attribution requirement, but notice-only collection must not silently drop applicable material. OpenTelemetry otel and trace each include appended Go BSD text in their root license, which must not be truncated to the Apache portion.

## Classifier gap and assembly review

The only Unknown classification was `github.com/segmentio/asm v1.2.1` for `bswap`, `cpu`, `cpu/arm`, `cpu/arm64`, `cpu/cpuid`, and `cpu/x86`. Its primary license is MIT No Attribution, SPDX [MIT-0](https://spdx.org/licenses/MIT-0.html). The manual resolution remains bound to Origin commit `1cfacc81a878d4a07b13f51f2368cd86893d23fa`, its recorded module sum, and LICENSE SHA-256 `cca993712df289a5958bdef69031a5dac0f951ac15afeb313f9eeea55ed59443`. Preserve the text and fail review on changed inputs; do not add a blanket Unknown exception. [LICENSE](https://github.com/segmentio/asm/blob/1cfacc81a878d4a07b13f51f2368cd86893d23fa/LICENSE)

The tool warned about assembly in Edwards25519, xxhash, klauspost/compress, lz4, Segmentio asm, x/crypto, x/sys, and reflect2. Their indicated files were inspected and byte-matched to their sources; the JSON includes the reviewed paths. The reflect2 assembly stubs are empty files. The notable cross-directory provenance is the S2-derived zstd match-length routine described above. The remaining reviewed files carry applicable enclosing terms, Go BSD headers, or generated-code markers; no additional license identifier was found in those assembly notices. This does not make the classifier capable of discovering arbitrary native-code dependencies or prove every historical code origin.

## Packaging disposition

- Preserve all listed applicable license and notice files. Source-embedded notice and assembly evidence can be copied under non-buildable `.txt` names; manifest hashes apply to the original bytes, so changing contents requires a separate transformed-artifact hash.
- The MySQL driver is the identified MPL-2.0 application dependency. Distribute its **exact verified module ZIP** as covered source and provide a readable source-access notice; do not extract a buildable vendor tree under `dist/`. The canonical ZIP SHA-256 is `dc93f5770556406e82bf750a980d2316f882a19d883a3689eadb820708c2b651`, byte-identical to the cache, and its module sum matches the binary. [Exact source ZIP](https://proxy.golang.org/github.com/go-sql-driver/mysql/@v/v1.10.0.zip), [MPL requirements](https://www.mozilla.org/en-US/MPL/2.0/)
- Reconcile actual collection output against this manifest, not only classifier counts. Keep separate identities for the two pflag versions and two zap versions. Repeat the inventory when dependencies, target platforms, tags, replacements, or build flags change.
- The first-party SDK rc.2 license-bearing ZIP and first-party archive/image LICENSE/NOTICE copies remain useful completed checks; this report does not turn them into clearance of all third-party/runtime/UI/CA contents. Runtime and CA requirements remain in the separate report, and final UI/package inclusion remains outside this task.

## Machine-readable evidence

The collector's [reviewed manifest](../../scripts/go-licenses/reviewed.json) is the
single maintained JSON copy. Its schema, toolchain, package profiles, target bits
and all 93 module records were compared with this report's original JSON before
consolidation and matched exactly. Use the [pinned reviewed manifest](https://github.com/viber-ops/configra/blob/v0.1.0-rc.2/scripts/go-licenses/reviewed.json)
when reproducing this dated review; use the current manifest for current builds.
The [collector instructions](../../scripts/go-licenses/README.md) explain its guards.

Each package profile hashes UTF-8 bytewise-sorted import paths from
`go list -deps -f '{{.ImportPath}}'`, joined with LF and one final LF, including
stdlib and main. Baseline source: `29b52e5f5838dffbcee3f88da3dd6ff0650b2cfa`;
toolchain: Go 1.26.7; `CGO_ENABLED=0`, `GOWORK=off`, `GOFLAGS=-mod=readonly`,
`GOTOOLCHAIN=local`, empty `GOEXPERIMENT`. All eight working-root profiles matched
on 2026-09-12 at `7f1a28f7a86ae635c343de664aab361c0d4722b2`. Package guards
complement per-file hashes; unchanged import paths alone cannot prove unchanged code.

`target_mask` uses the manifest's `target_bits`. File paths are module-relative
unless marked `external_to_module`. Assembly/authorship entries retain reviewed
evidence without inventing extra obligations. The supplemental Go BSD notice is
not claimed to exist inside the shopspring ZIP. Explicit excluded legal paths
must be reconsidered when shipped source or package selection changes.

### Inspected review.2 executables

| Target | Dependency modules | Executable SHA-256 |
| --- | ---: | --- |
| `configra/darwin/amd64` | 31 | `faec69b25b7fa7b7dbd9b93376b598c58f94e2ddd22ade14dac3bef087ad06af` |
| `configra-kubernetes/darwin/amd64` | 67 | `01cdcb2e1a1f92b04b140e785abfec1ec0f8fa372b0cecbe162edd9756d2de14` |
| `configra/darwin/arm64` | 31 | `d358d99c4623f3941bd16adfe35a5afd2332a0381217997e90c60e803e761e4f` |
| `configra-kubernetes/darwin/arm64` | 67 | `cafd7f096e8141acac199a4b58a121b825218f8ca58a48a641aa94ff82f2db07` |
| `configra/linux/amd64` | 31 | `de7925769e0713774cfdd8d5f06263b32303a0e65b51ce247248f3332870dc1f` |
| `configra-kubernetes/linux/amd64` | 68 | `29bc0d2fa1392d0d46dce0793554eeff9c2c031bd13d63c18157fb3298475883` |
| `configra/linux/arm64` | 31 | `684bafc8002b9bd4747f94c2a704d8169827a6017c71167895feaaa9dd1724ed` |
| `configra-kubernetes/linux/arm64` | 68 | `fd41b62df1526605ad95ed93e42da253c988786034e92fded09d33ea4b36ea0e` |

The [review archive](https://github.com/viber-ops/configra/releases/download/v0.1.0-rc.2/review-records-2026-09-12.tar.gz) retains the original report with its full historical JSON,
artifact paths, source-verification details and collection notes. Later archive
and image inclusion checks are in the [verification record](../verification/2026-09-12.md#distribution).

## Limits and completion record

This completes the declared-license/material inventory for the 93 module/version pairs in these exact eight binary inputs, including the documented source and assembly exceptions. It is not legal certification, a full historical authorship audit, or a guarantee that undeclared third-party material can never exist. The reviewed source-package sets match the binaries' dependency metadata; symbol-level dead stripping, non-Go data provenance beyond the inspected notices, and packaging transformations require their own evidence.

The 13 missing VCS-Origin records remain explicitly archive-bound. Final collectors must also verify hashes and contents of produced notice bundles, preserve source-access availability, and prove the intended files reach every release archive, image, and served UI. No other source, dependency, package, reference, or release was modified for this research.
