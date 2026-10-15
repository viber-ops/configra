# Config 跨环境复制

Config 键名在 Configra 全局唯一，各 Environment 的版本独立编号。
`test/server` 和 `production/server` 属于同一个 Config 在两个环境的配置，
无需为生产环境另起键名。

| 目的 | 操作 | 前提 |
| --- | --- | --- |
| 把 test 的同名配置首次放进 production | **复制到环境 / Copy to environment** | production 已存在且有效，该 Config 在 production 还没有版本 |
| 完整替换 production 的已有配置 | **替换 / Replace** | 选择源版本、目标版本并查看差异 |
| 保留 production 中源版本未包含的字段 | **合并 / Merge** | 两边格式相同，查看合并结果；数组按现有 overlay 规则替换 |
| 另建一个不同名称的 Config | **更多 → 克隆 / Clone** | 目标键名在所有环境中都未使用过 |

## 管理端流程

1. 打开 test 下的 Config。先保存要复制的修改。
2. 点击“复制到环境”，从可搜索的环境列表选择 production。
3. 核对页面上的源环境、Config 键名、已保存版本和原文，再创建首个版本。
4. 查看结果和 Vault 警告，通过“打开目标 Config”检查生产配置。

复制固定页面显示的已保存源版本。未保存的编辑和源历史不会复制。
`{vault.namespace.item.field}` 保留为引用，在 production 使用其对应的 Vault
Variant；不会把 test 的解析后密钥写入 production。缺少 Vault Item、Field 或环境
Variant 会作为警告显示，需要补齐后才能通过机器 API 读取解析后的配置。

目标已有配置时返回 409；并发创建目标也会触发相同保护。需要更新已有目标时，选择
Merge / Replace，确认方向和差异。预览后目标 current 改变会返回冲突，点击“刷新预览”
重新核对后再提交。恢复旧版本也会追加一个新版本。

## 错误与重试

错误显示具体原因，例如 Clone 的全局键名冲突、目标环境不存在或已归档、源版本不可用、
目标版本已变化、合并格式不同。错误不会包含 Config 原文、解析器原始报错或密钥值。

保留 Request ID 供请求排查；对于进入存储事务的写操作，可复制 **Operation / 操作 ID**
到 Audit 搜索该次操作。网络失败后，在相同表单中直接重试会使用相同 Operation ID，
重放原结果，不重复写版本或逻辑审计。已完成操作即使源环境后来归档，也能重放。
修改请求内容会使用新 Operation ID。浏览器刷新会丢弃内存中的重试编号；遇到不确定的
网络结果时，应先在 Audit 或目标历史中核实结果。

## 管理 API

仅管理员的人类会话可调用这些管理接口；不扩大 scoped write token 的权限。
同名首次复制使用现有 Replace 路由，必须明确传两个零值：

```http
POST /v1/environments/production/configs/server/replace
Content-Type: application/json
Idempotency-Key: <unique-operation-id>

{"source_environment":"test","source_config":"server","source_revision":3,"target_revision":0,"expected_target_revision":0}
```

创建 v1，记录 Source Config / Environment / Revision，写入 `config.replaced` 审计和
通知。Source 始终只读。Merge 和 Restore 仍要求有效的已有目标版本。
`transfer-preview` 用于已有目标的 Merge / Replace；首次复制的界面展示固定源原文。

Clone 请求可携带 `source_revision` 固定输入；省略或传零时兼容原先“读取当前版本”的
语义。Clone 仍只接受全新全局键名。写入错误保持原有 409/422 和 `error.code`，增加
安全的 `error.field`、`error.reason`；失败重放保留这些信息。

## 升级影响

本次修复没有数据库结构迁移。新的原因码保存在现有 Operation JSON 响应中，兼容旧
操作记录；旧记录无法补出的细分错误仍显示通用原因。旧版成功 Transfer 摘要可通过
不可变历史验证后重放，归档资源不因此允许新的写入。

管理端前端随服务端一起部署。更新后重新加载页面，再使用“复制到环境”和带源版本的
Clone 请求。仅格式变化（如正文相同的 JSON → YAML）现在正确创建新版本。
