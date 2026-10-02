# 有范围的写令牌（v1.1.0）

[English](scoped-write-tokens.md) · [从 v1.0.1 升级](releases/v1.1.0.md)

operator 在 API 端口（9443）持有 `write-scoped` Token 和已注册的 mTLS 客户端证书，
可以操作范围内的 Config、Vault 和只读部署凭据。它无需持有管理员登录、OIDC 凭据、
CA 签名私钥或 Master Key。旧 Token 自动成为 `read-only`，保留原过期、环境授权和
显式允许的 Token-only 行为。

## 签发与令牌模型

1. 管理员完成 OIDC 登录及 MFA，在管理端（8443）打开“管理 → API 令牌 → 新建 API
   令牌 → 有范围的写令牌”。
2. 选择至少一个已有 Environment，按需填写 Config 资源键、Vault namespace 白名单，
   设置明确的未来过期时间。白名单使用不可变资源键（例如 `server`），而不是显示名称
   或文件名（例如 `server.yaml`）。
3. 安全保存只展示一次的 Token；通过已有证书管理功能签发或注册 operator 客户端证书。
   operator 只持有 Token、客户端证书/私钥及 HTTPS 服务端信任根。
4. 管理员建立受管客户端 CA，向 operator 提供其公开 ID，用于签发部署主机证书。
   不向 operator 分发 CA 私钥。

管理端 `POST /v1/api-tokens` 在原请求中新增 `kind: "write-scoped"`、`config_keys`、
`namespace_keys`，并强制 RFC3339 格式的 `expires_at`。只有已认证的管理员且**已验证
签名的 ID Token** 声明 MFA 才能签发写令牌：`amr` 包含 `mfa`，或同时包含 `pwd` 和
`otp`。缺失或格式错误返回 `403 mfa_required`。UserInfo、角色及请求头不能作为 MFA
证据。升级前的 Session 需要重新登录；请配置 IdP 提供签名的 `amr` 声明。没有跳过
此检查的开关，本地演示环境也不例外。

令牌仍使用 `cfg_<public-id>_<secret>`，数据库只保存 Secret 摘要。管理端令牌列表
可查类型、范围、过期和吊销状态；签发、吊销均审计。写令牌及其部署 Token 的环境
授权不可编辑，变更范围时签发新凭据并轮换；旧独立只读 Token 仍可编辑环境授权。

写令牌必须显式过期，最长 90 天。可在 Management 冷启动 YAML 中收短期限并重启：

```yaml
write_tokens:
  max_ttl_days: 14 # 1..90，默认 90
```

未填写或 `null` 的 Config/namespace 白名单表示允许已授权环境内全部对应资源；明确
传入 `[]` 则拒绝该维度全部资源。UI 留空表示未填写。每个白名单最多 256 个不重复
资源键。写令牌禁止 `never_expires` 和 `allow_without_mtls`，每次请求都同时检查
Token、TLS、证书注册、过期及 CA/证书吊销状态。

## 写接口与权限边界

每个写请求必须提供 `Idempotency-Key`（8–128 个 ASCII 字母、数字、`_` 或 `-`）。
同一 key 只用于相同请求的重试。Config/Vault 使用 `expected_revision`：创建为 `0`，
更新为当前版本；旧版本返回 `409 revision_conflict`。成功/无变化返回 `outcome` 与
`revision`，有变化追加不可变历史。错误只返回安全错误码与 Request ID，不含值。

item 已存在于其他环境时，`expected_revision: 0` 也可创建所选环境尚不存在的 Variant。
请用 `PUT /vault-items/{namespace}/{item}` 提供全部字段值；已存在的 Variant 更新仍
必须匹配 item 当前全局版本，范围外的值不会返回给调用者。

以下路径均位于 `/v1/environments/{environment}` 下：

| 方法与路径 | 请求/结果 |
| --- | --- |
| `GET /configs/{config}` | 原有 resolved Config 读取 |
| `GET /configs/{config}/raw` | 写令牌专用，读取当前原文和 `revision` |
| `PUT /configs/{config}` | `name`、`format`（yaml/json）、`content`、`expected_revision` |
| `GET /vault-items/{namespace}/{item}` | 写令牌专用，只返回所选环境的当前 Variant |
| `PUT /vault-items/{namespace}/{item}` | 可选 `display_name`、`fields`、`values`、`expected_revision` |
| `PUT /vault-items/{namespace}/{item}/fields/{field}` | `name`、`type`、`value`、`expected_revision` |
| `DELETE /vault-items/{namespace}/{item}/fields/{field}` | JSON `expected_revision`，移除共享字段定义 |
| `DELETE /vault-items/{namespace}/{item}` | JSON `expected_revision`，归档 item，保留身份和历史 |
| `GET /vault-items/{namespace}/{item}/fields/{field}/content` | 原有 File bytes 下载 |
| `POST /deployment-credentials` | `display_name`、受管 `authority_id`、明确的 `expires_at` |
| `POST /deployment-credentials/{public_id}/revoke` | 无请求体，同时吊销 Token 与证书 |

Vault 的 `fields` 为 `{key, name, type}` 数组；`values` 按 Field key 映射为
`{"text":"…"}`（Text/Secret）或 `{"file":{"filename":"…","content_type":"…",
"bytes":"<base64>"}}`（File）。文件名、MIME 与 bytes 均随快照加密。字段类型不可
修改；每个 File 最长 5 MiB，Text/Secret 最长 512 KiB，整个快照明文最长 10 MiB，
JSON 请求体最长 32 MiB。

值更新只影响所选环境；若 Variant 与其他环境共享，先拆分再更新。item 显示名称和
字段定义是共享的：修改名称/定义、增加/删除字段或删除 item，必须拥有**全部绑定
环境**的权限；无环境绑定的 Variant 也会阻止全局结构操作。其他值保留，新增加的
共享字段在全部已授权 Variant 中使用本次提供的值。字段结构需要独立演进时使用
不同 item。删除最后一个字段会被拒绝，请归档 item；写 API 不提供解归档或身份复用。

Environment、Config、namespace 范围外请求返回 `403`。Config 的每个 Vault 引用
namespace 在解析/条件 `304` 前也检查，写入范围外引用同样拒绝。API 不挂载环境
创建/删除、用户/角色管理、写令牌签发、CA 管理、Master Key 导出、全局列表或管理
读取接口。Token 过期/吊销，或证书缺失、吊销、绑定不匹配返回 `401`。v1.1.0 API 的
数据库账号需要运行时写权限；DDL 仍仅由 Management 执行。写入及重放时在事务内
持有 Token 行锁，与吊销串行，已建立连接不能绕过复核。

## 部署凭据、审计与轮换

部署凭据在一个事务中签发：单个已授权 Environment 的**只读** Token，加上有效受管
CA 签发的客户端证书。继承 Config/namespace 白名单，不能扩大权限；Token 和证书
均不晚于写令牌/CA 过期。Token 与本次证书绑定，不能搭配其他证书使用。

仅首次响应包含 `token.token` 和 `certificate.export_bundle`（base64 ZIP，含
`client.crt`、`client.key`、公开 `ca.crt`）；重放只返回元数据。响应丢失时吊销该
public ID 并重新签发。writer 只吊销自己签发的 pair；旧独立凭据及其他 writer 的
pair 由管理员管理。管理端吊销 writer 时同步原子吊销其所有部署 Token 和证书；
operator 自身证书仍独立由管理员管理。

成功、无变化、冲突和已认证的拒绝请求进入现有事务性 Audit Outbox。管理端审计
页面可查 Token public ID（`actor_type=token`）、操作、环境、namespace/item/field
或 Config 对象、时间、版本/结果及 TCP 来源 `source_ip`，支持搜索。不会信任转发
请求头。已接受操作的重放沿用原 Audit ID 和 IP。请求体、Config 原文、密钥值、
文件名/bytes、Token Secret、私钥导出均不进入日志和审计。拒绝审计无法落库时返回
`503 audit_unavailable`；ClickHouse 不可用时已提交审计留在 MySQL 待重试。

建议 writer 使用较短期限（例如 14 天），测试/生产分别签发，每个部署主机单独 pair。
轮换顺序：签发替代凭据 → 分发并验证 → 吊销旧凭据。退休 writer 前先替换子凭据，
以免级联吊销中断主机读取。Token/私钥以 `0600` 保存，通过私密渠道交付；只有
operator 持有写身份。

[configra-go operator 示例](https://github.com/viber-ops/configra-go/tree/feat/scoped-write-api/examples/operator)
会上传 `server.yaml` 与 17 个 File 字段，再落盘只读部署 pair。
`make test-integration` 使用真实 MySQL 8.0.22/NATS/ClickHouse 和 HTTPS/mTLS 执行
示例，检查范围、历史、吊销、审计、权限文件及敏感值脱敏。
