# Configra Go 类库选型

调研日期：2026-08-26。范围仅包括 Config 文档处理、应用日志和进程 CLI。判断标准是：先复用现有依赖或标准库；只有成熟类库能准确覆盖产品语义时才新增依赖。

## 结论

| 能力 | V1 选择 | 结论 |
| --- | --- | --- |
| YAML 解析、AST、校验、格式化 | `go.yaml.in/yaml/v3` | 保留现有依赖；用其 `yaml.Node` 和 Encoder，不自研解析器或格式化器 |
| YAML 二路 overlay | 基于 `yaml.Node` 的极薄递归 | 候选类库都不能同时满足 Configra 的全部语义；只保留业务规则本身 |
| JSON 解析、校验、格式化 | `encoding/json` | 标准库已足够；保留 Token 级重复键和深度校验 |
| JSON 二路 overlay | 基于解码值的极薄递归 | 不使用 RFC 7396 实现，因为其 `null` 表示删除 |
| 应用日志 | `go.uber.org/zap` | 接受用户指定的成熟类库；直接使用强类型 `*zap.Logger`，不再包一层自有日志框架 |
| CLI | `github.com/spf13/cobra` | Configra 已有 Management/API 两个启动入口，达到使用子命令框架的门槛；不使用生成器铺脚手架 |

这里的“极薄递归”不是自研 YAML/JSON 引擎：解析、类型识别、AST、转义和输出仍全部交给成熟库，仅实现 [ADR-0005](../adr/0005-use-directional-two-way-merge.md) 定义的几条产品规则。

## Configra 必须保持的 Merge 语义

Source 定向覆盖 Target：mapping 递归合并；匹配的 scalar 和 sequence 由 Source 整体替换；Target-only key 保留；`null` 是普通值而不是删除指令。删除只发生在可编辑预览中，Replace 才是完整替换。格式化后的文本是权威 Revision；允许规范化空白和引号，但需要尽量保留注释和 key 顺序，见 [ADR-0004](../adr/0004-store-canonical-config-text.md) 与 [ADR-0005](../adr/0005-use-directional-two-way-merge.md)。

这组语义尤其排除了“名称也叫 merge，所以可以直接换上”的类库。

## YAML

### 采用：`go.yaml.in/yaml/v3`

仓库已经依赖 `go.yaml.in/yaml/v3`，因此它是依赖阶梯上的第一选择。官方 `Node` 暴露 mapping/sequence/scalar/alias、style、anchor、三个注释位置以及有序的 `Content`；官方也明确说明重新编码不会保留原始文本外观，但会尽力把注释保留在相关数据附近。这正好符合 Configra“规范化格式、保留语义注释和顺序，不保证原始空白/引号”的边界。[v3 package documentation](https://pkg.go.dev/go.yaml.in/yaml/v3)

V3 的 Decoder 不会替 Configra 完成全部信任边界校验，所以现有的单文档限制、scalar key、重复 key、alias cycle、大小限制仍应保留。这些是很薄的验证逻辑，不是重复造解析器。官方当前将 v3 定位为 API 稳定的 legacy 分支，常规新功能进入 v4；因此只做经过回归测试的 v3 patch 更新，不在 V1 中扩大 v3 API 用法。[go-yaml version intentions](https://github.com/yaml/go-yaml#version-intentions)

### 暂不采用：`go.yaml.in/yaml/v4`

截至调研日，pkg.go.dev 展示的是 `v4.0.0-rc.6`，并明确标记为非稳定版本。V4 的 `Load`/`Dump`、默认唯一键检查和可插拔深度/alias 限制很有价值，但不足以抵消生产系统现在迁移到 RC 的风险。[v4 package documentation](https://pkg.go.dev/go.yaml.in/yaml/v4)

决策：等 v4 stable 后，用现有 canonical/merge 回归集做一次迁移评估；现在不双栈，也不写兼容层。

### 暂不采用：`github.com/goccy/go-yaml`

这是成熟且有吸引力的备选：官方说明它没有第三方依赖，默认拒绝重复 mapping key，提供 tokenizer/parser/AST，并支持带 comments、anchors 和 aliases 的可逆变换。[project features](https://github.com/goccy/go-yaml#features) [parser options](https://pkg.go.dev/github.com/goccy/go-yaml/parser)

但它的内建 `ast.Merge` 不符合 Configra：mapping merge 只替换当前层的同名 value、不会递归进入同名 mapping，而 sequence merge 是 append；Configra 要求 mapping 递归、sequence 整体替换。[official mapping merge source](https://github.com/goccy/go-yaml/blob/master/ast/ast.go#L1142-L1159) [official sequence merge source](https://github.com/goccy/go-yaml/blob/master/ast/ast.go#L1496-L1506)

因此换库后仍要保留自定义 overlay，同时承担 AST/API 迁移成本。只有将来把“原始文本级 round-trip”提升为硬需求，或现有 v3 出现已复现且无法规避的问题时，才做隔离 spike；当前不新增它。

### 不采用：`github.com/mikefarah/yq/v4/pkg/yqlib`

Yq 的 `*` 操作符能深度合并对象，默认让右侧数组替换左侧数组，并保留左侧 style，表面上最接近需求；但它把 null 与 map/array 合并视为保留另一侧，而 Configra 要让 Source `null` 真正覆盖 Target 值。[yq multiply/merge semantics](https://mikefarah.gitbook.io/yq/operators/multiply-merge)

更重要的是，`yqlib` 是完整的多格式表达式引擎。其官方 `go.mod` 当前含表达式 parser、颜色、INI、HCL、TOML、Lua、CTY、Cobra、多个 YAML/JSON 实现等大量直接依赖。为一个固定 overlay 规则引入整套 DSL，不划算。[yq go.mod](https://github.com/mikefarah/yq/blob/master/go.mod)

结论：`yq` 适合作为运维 CLI，不作为 Configra 服务端内部 merge 库。

## JSON

### 采用：`encoding/json`

标准库已经提供 Token 流、`UseNumber` 和 `MarshalIndent`。当前 v1 API 对 map key 排序后编码，因此相同结构可以得到确定性输出；`UseNumber` 避免先把任意数字强制转为 `float64`。[encoding/json documentation](https://pkg.go.dev/encoding/json#Marshal) [UseNumber](https://pkg.go.dev/encoding/json#Decoder.UseNumber) [MarshalIndent](https://pkg.go.dev/encoding/json#MarshalIndent)

标准库当前 API 默认允许重复 object name，所以现有 Token 级 decoder 仍需负责重复键、最大深度、尾随内容和单文档校验；这比增加另一个 JSON parser 更小，也保住当前行为。[encoding/json security considerations](https://pkg.go.dev/encoding/json#hdr-Security_Considerations)

### 不采用：`github.com/evanphx/json-patch/v5`

该库成熟地实现了 RFC 6902 和 RFC 7396，但 RFC 7396 明确定义 object member 的 `null` 为删除，并指出该格式不适合需要显式 null 的 JSON。它的 non-object/array 整体替换行为虽与 Configra 部分相同，关键的 null 语义却相反。[RFC 7396 processing rules](https://www.rfc-editor.org/rfc/rfc7396.html#section-2) [json-patch project](https://github.com/evanphx/json-patch#readme)

不能先调用该库再补回 null：删除后已经丢失“该 key 是 Source 明确给 null，还是根本不存在”的信息。继续使用当前很短的递归 overlay 才是正确且更小的实现。

## 日志：采用 Zap

采用稳定的 `go.uber.org/zap` V1。官方将其 API 标记为 stable，基础 `Logger` 提供并发安全、强类型 structured fields 和面向低分配 hot path 的实现；官方 production preset 默认输出 JSON。[zap package](https://pkg.go.dev/go.uber.org/zap) [zap stability and benchmarks](https://github.com/uber-go/zap#development-status-stable)

`log/slog` 是可靠的标准库替代品：Go 官方说明它从 Go 1.21 起提供 structured logging，并为禁用日志、公共属性和常见少量属性做过性能设计。因此，如果没有明确偏好，`slog` 已足够。[Go slog design](https://go.dev/blog/slog)

本项目仍选择 Zap，原因是用户已明确指定、API Server 有 1000 QPS 目标，而且 Zap 已稳定。实现保持最小：启动时构造一次 `zap.NewProduction` 或一份很小的 `zap.Config`，把 `*zap.Logger` 直接传入需要记录日志的边界；不建立自有 `Logger` interface，不写自定义 encoder。日志是否经采集器或服务写入 ClickHouse是输送链路问题，不应塞进日志调用 API。

## CLI：采用 Cobra

标准库 `flag` 足以启动一个只有少量 flags 的单用途 server，也能用多个 `FlagSet` 手写 subcommand；如果 Configra 永远只有一个进程模式，应停在这里。[flag package](https://pkg.go.dev/flag)

但 Configra 已确定同一代码库/镜像具有 Management Server 和 API Server 两个独立入口，因此从第一版就有真实 subcommand，而不是“未来也许会有”。Cobra 提供子命令树、参数校验、POSIX/pflag、自动 help/version/completion；其 V1 module 已稳定并广泛使用。[Cobra package](https://pkg.go.dev/github.com/spf13/cobra) [Cobra capabilities](https://github.com/spf13/cobra#overview)

最小落地是一个 root command 加 `management`、`api` 两个 leaf command，leaf 只负责读取冷启动 YAML 后调用现有 `run` 函数。不要运行 `cobra-cli` 生成目录，不接 Viper，不自定义 help template；需要第三个真实运维命令时再增加。

## 实施约束

1. 现在不因调研修改 YAML/JSON 依赖：保留 `go.yaml.in/yaml/v3` 与 `encoding/json`，保留很薄且有语义回归测试的 overlay。
2. 创建 server entrypoint 时加入 Zap 和 Cobra，并固定 V1 版本；不要提前添加未被代码使用的依赖。
3. Merge 回归至少锁定 nested mapping、scalar/sequence replacement、Target-only key、Source `null`、注释/key 顺序和输入不可变性。
4. 只有稳定版本、可复现缺陷或新增硬需求能触发换库；“功能更多”本身不是迁移理由。
