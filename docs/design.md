# Configra --- 轻量配置与敏感信息管理中心设计文档

> V1 Design：短小精悍、语义明确、可审计、可验证。

## 1. 产品定位

Configra 是轻量配置与敏感信息管理中心：集中管理 YAML/JSON；使用任意
Environment 隔离配置；任意 Environment 间 Diff、Merge、Replace；Vault
管理密码、Token、私钥、证书和小型文件；Config 引用 Vault 时自动继承当前
Environment；Config/Vault 均有不可变版本历史；支持
Config Clone、Restore、Audit、Access Stats、Notification；人员使用 OIDC，程序使用
API Token。

核心原则：**Environment 是 Context，不是阶段。** 系统不预设 dev → test →
prod。`a / b / c / customer-x / beijing / cluster-01` 都只是平级
Environment。

核心原则：**Environment 本身没有方向，跨 Environment 的 Operation
有方向。** `a → b` 与 `b → a` 是不同操作。

核心原则：**Vault Namespace 只是身份段，不是权限边界。** Vault Item 由
`(namespace_key, item_key)` 唯一标识；Token 是否能读取仍只取决于请求的
Environment 是否在其 allowlist 中。

## 2. V1 非目标

不做：服务发现、Feature
Flag、灰度发布、审批流、Organization/Workspace/Team、复杂
RBAC/ACL/Policy Engine、动态
Secret、大文件管理、完整监控平台、消息队列/Redis/etcd
强依赖、TOML/properties/dotenv、固定环境流水线。

## 3. Environment

Environment 是系统级 Context，不属于 Config，也不属于 Vault。

``` text
                    Environment
                         │
              ┌──────────┴──────────┐
              │                     │
            Config                Vault
```

支持 Create、Rename、Archive、Unarchive。Archive 为可恢复软删除；Resource Key
永不复用。

## 4. Config

一个 Config 可以存在于任意多个 Environment 中，不要求覆盖全部
Environment。

``` text
Config: payment
├── a → Revisions
├── b → Revisions
└── f → Revisions
```

### 4.1 格式

V1 强约束只支持 **YAML / JSON**。Revision
保存 canonical text；canonical text 是权威内容，AST/Document 用于
Validate、Diff、Merge、Vault Reference 检测。不要只保存 `map[string]any`
后重序列化，以免破坏 YAML 注释、顺序和格式。

### 4.1.1 保存、校验与格式化

所有能够产生 Config Revision 的路径必须经过同一条 Commit Pipeline：

``` text
Manual Edit ─┐
Merge ───────┤
Replace ─────┤
Clone ───────┼→ Parse → Validate → Validate Vault References → Format → Re-validate → Diff → Commit
Restore ─────┤
API Write ───┘
```

> **Invariant：任何 committed Config Revision 都必须是合法且 canonical
> formatted 的 YAML/JSON。**

规则：

-   YAML / JSON 必须严格 Parse 成功才能保存。
-   JSON 使用严格 JSON，不接受 JSON5、注释、trailing comma。
-   YAML / JSON Duplicate Key 均视为 Error。
-   错误尽可能返回 line / column。
-   JSON 使用确定性的 canonical format（默认 2-space
    indentation、统一换行）。
-   YAML 使用确定性的 formatter，并尽可能保留 comments /
    ordering；禁止简单 `map[string]any → marshal` 作为格式化实现。
-   Format 后必须重新 Parse / Validate。
-   Format 后与当前 Revision canonical text 完全一致时返回
    `No Change`，不产生 Revision。
-   `Format` 本身只修改编辑内容，不产生 Revision。

Vault Reference 保存检查：

  检查                                级别      行为
  ----------------------------------- --------- ----------
  非四段式 `{vault.<namespace>.<item>.<field>}`  Error     禁止保存
  Vault Item 不存在                   Warning   允许保存
  Vault Field 不存在                  Warning   允许保存
  当前 Environment 无匹配 Variant     Warning   允许保存

允许 unresolved reference，以支持 Config 与 Vault 分步骤建立；真正执行
resolved read 时若仍无法解析，则返回 resolution error。

提供：

``` text
POST /v1/configs/validate
```

供 Management Web UI 与其他已认证管理客户端复用相同的 Parse / Validate /
Format 规则。Validate API 与 Commit 必须调用同一套 Domain
实现，禁止形成两套规则。

### 4.2 Revision / Restore

Revision 不可变，任何修改产生新 Revision。Restore
同样创建新版本：`current=v17; restore v12 → v18.content=v12.content`，而不是把
current pointer 指回旧版本。

## 5. Diff / Merge / Replace

任意两个 Environment + Revision 可以 Diff：`a@v17 ↔ b@v12`，也允许
`a@v15 ↔ a@v17`。

跨 Environment Mutation 必须明确 Source /
Target：`a@v17 → b@v12`。Source 只读，只有 Target 产生新 Revision。
Source Revision 与 Target Revision 均可选择任意不可变历史版本；各 Environment
独立编号，Revision 数字相同或不同都没有跨 Environment 语义。操作结果追加为 Target
Environment 的新 current Revision。
Merge / Replace 的提交预览以 `Target 变更前 → Result 变更后` 显示 Diff；Source
保留为已选择的只读输入，不重复占用预览代码栏。

> Invariant：跨 Environment Operation 永远不能修改 Source。

采用 optimistic concurrency。预览需要同时记录所选 Target Revision 和当时的
Target current Revision；如果提交时 current 已变化，必须返回 Conflict，禁止静默覆盖。
所选历史 Target 本身不可变，不承担并发锁职责。

底层明确区分 `Merge` 与 `Replace`；不要使用暗示环境层级的 `Upgrade`。

## 6. Config Clone / Copy

> **Clone 复制当前状态，不复制历史；新对象从 Revision 1
> 开始，并记录来源。**

-   Clone Config：把 Source 当前状态复制到指定 Environment 下的全新 Config
    Resource Key，新 Config 历史从 v1 开始并记录 Source Config、Environment 和
    Revision。

V1 不提供 Environment Clone 或 Vault Item Clone。前者会把多个 Config Revision、
多个 Vault Item Revision 和 Environment 创建合并成一次跨资源 Operation；后者还需
定义敏感值来源审计。在没有明确原子性、幂等与逐资源 Audit 语义前，不提供看似方便
但可能只复制一半的操作。管理员可显式创建 Environment、逐个 Clone Config，并通过
Vault Item 编辑为新 Environment 增加既有 Variant 绑定。

## 7. Vault：1Password 式 Item

Vault 不是简单 KV。

``` text
Vault
  └── Namespace Key（仅身份段）
       └── Item
            ├── Variant[]
            │    ├── Environments[]
            │    └── Fields[]
            └── Revisions[]
```

Namespace Key 与其他 Resource Key 使用相同格式，但 Namespace 不是独立资源：没有
Namespace 表、Display Name、ACL、Revision、Archive 或管理页。Item 的唯一身份是
`(namespace_key, item_key)`；`platform/redis` 与 `aliyun/redis` 是两个完全独立的
Item，可以拥有不同值与 Revision 历史。V1 不提供默认 Namespace，也不兼容三段式引用。

### 7.1 Item / Field

Item 是版本化最小单位。一个 Item 可以包含
`host / port / username / password / readonly_password / private_key / certificate / config.json`
等多个字段。

V1 Field Type 只保留：

-   `text`：普通文本，默认显示；
-   `secret`：敏感文本，默认隐藏，可 Reveal/Copy；
-   `file`：小型敏感文件，保存 filename/MIME/size/encrypted bytes。

File 适合 SSH key、PEM、P12、JKS、JSON
credential、license、小型二进制密钥。V1 不引入 S3，直接加密 BLOB 存
SQL；建议默认最大 5 MiB，可配置。

### 7.2 Variant

一个 Item 可针对不同 Environment 有不同的一整组 Field Value：

``` text
Item: platform/redis

Variant #1  envs=[a,b,c]
  host      10.0.0.1
  username  redis
  password  ****

Variant #2  envs=[d,e]
  host      10.0.1.1
  username  redis
  password  ****
```

> Invariant：同一个 Vault Item 当前 Revision 中，一个 Environment
> 最多属于一个 Variant。

把 Environment 加入另一个 Variant 时，语义是"移动"。

### 7.3 Item Revision

Revision 属于整个 Item，而不是 Field/Variant。修改任意
Field、替换文件、增加字段、移动 Environment、改变 Variant 都产生完整
Item 新 Revision。

``` text
v22 Replaced certificate
v21 Environment b moved to Variant #2
v20 Added api_token
v19 Changed host
v18 Changed password
```

Restore `v18` 时创建 `v23 = snapshot(v18)`。

## 8. Vault Reference 与 Environment 继承

Config 中统一使用且只接受四段式：`{vault.<namespace>.<item>.<field>}`。

``` yaml
database:
  username: "{vault.platform.mysql.username}"
  password: "{vault.platform.mysql.password}"
ssh:
  private_key: "{vault.infrastructure.server.private_key}"
```

引用不携带 Environment。若当前 Config 是 `payment/b`，解析
`{vault.aliyun.redis.password}` 时自动以 `env=b` 查找精确的
`(namespace=aliyun, item=redis)` Item 中包含 b 的 Variant，再读取 password
Field。另一个 Namespace 下同名的 `platform/redis` 不参与本次解析。

> Invariant：Vault Reference 默认继承当前 Config 所属 Environment。

因此 `a→b` Merge 后引用文本保持不变，但在 b 中自然解析 b 对应的 Vault
Variant。

File Field 原始保存 bytes。V1 SDK 提供 bytes 获取能力；类似 `|base64`
的转换语法放到后续版本评估。

## 9. Vault 加密

采用 Envelope Encryption：随机 DEK 加密 Vault 数据，KEK/Master Key 加密
DEK。

V1：AES-256-GCM + cryptographically secure random nonce。Master Key
绝不能存业务 SQL 数据库。

V1 实现 `LocalKeyProvider`，Master Key 只从冷启动 YAML 指定的 Secret-mounted
file 读取。只有出现第二种实现时才提取 Provider interface；未来可增加 AWS KMS /
GCP KMS / Azure Key Vault / Vault Transit。

数据库保存
ciphertext、nonce、encrypted_dek、key_version、algorithm。历史 Revision
同样必须加密。明文生命周期尽可能短，禁止进入日志和普通缓存。

## 10. Authentication / Authorization

### Human：OIDC

Web UI 使用 OIDC，例如 Casdoor。Configra
不维护用户密码，只保留两个内部角色：`viewer`、`admin`，不做复杂 RBAC。
登录页不提供账号密码输入框，只跳转到配置的 Identity Provider；是否出现登录表单、MFA
或直接复用既有 SSO Session 由该 Provider 决定。Configra Logout 只销毁自身 Session，
V1 不声称同时注销企业 SSO Session。

viewer 可查看 Config、Diff、History、Vault metadata、Dashboard、Access
Stats；默认不能修改、Merge、Restore、管理 Token/Notification Destination 或查看 Secret
明文。admin 具有完整管理能力。

Management 使用标准 OIDC Authorization Code + PKCE，不绑定 Casdoor SDK。
生产发布物不内置 Identity Provider、本地账号、测试身份或认证绕过；登录必须跳转到
部署配置中的真实 OIDC Provider（当前选型为 Casdoor），回调后再建立 Configra Session。
一次性 State、Nonce 与 PKCE verifier 只保存在 MySQL 服务端 Session 中；登录后
立即轮换 Session ID。一个冷启动配置的 ID Token 或 UserInfo Claim 通过精确值
映射为 `viewer/admin`，未命中时拒绝登录；使用 UserInfo 时其 `sub` 必须与已验证
ID Token 一致。固定 Redirect URI 同时定义唯一允许的 Public Host。

Session Cookie 使用 `Secure`、`HttpOnly`、`SameSite=Lax` 与 `__Host-` 前缀，
绝对有效期 8 小时、空闲超时 30 分钟。状态变更接口由 Go 标准库
`http.CrossOriginProtection` 保护；本地退出仅支持同源 `POST /auth/logout` 并销毁
服务端 Session。详细决策见
[ADR-0026](adr/0026-use-standard-oidc-server-sessions-and-stdlib-csrf.md)。

### Machine：API Token

机器客户端使用 Configra API Token，例如 `cfg_<public-id>_<secret>`。创建时只
显示一次，DB 只保存 id/name/prefix/hash、Environment allowlist、
`allow_without_mtls`、created_at/expires_at/revoked_at。
Token 列表永远不返回 digest 或明文；明文只在成功创建的首次响应中出现，使用相同
Operation ID 重试只返回元数据。Environment allowlist 可以整体替换，Revoke
不可撤销。

V1 Token 默认只读，不设计复杂 Scope。Token 在其明确允许的 Environment 中
可以读取全部 Config 和 Vault 值，包括所有 Namespace 下的 Secret/File；Namespace
不参与授权，V1 没有 per-Namespace grant 或
`allow_secrets`。`allow_without_mtls` 仅决定是否允许 Token-only
Authentication，不改变 Environment 权限。

## 11. Config API / SDK / Viper

V1 Machine API 只提供两个 exact-key 内容读取端点：

``` text
GET /v1/environments/{env}/configs/{config}
GET /v1/environments/{env}/vault-items/{namespace}/{item}/fields/{field}/content
```

第一个端点只返回已经完成 Vault 替换的 Resolved Config JSON Envelope；第二个
端点返回 File Field 原始 bytes。Machine API 不暴露 unresolved Config、通用
Vault Field 读取、list 或 search。禁止日志记录 Resolved Config body 与 File
bytes。

Go SDK 提供 `ReadResolvedConfig` 与 `ReadFile`，负责 Token Authentication、
HTTPS/mTLS、Revision/ETag 和有界响应读取。`ViperHandler` 在此 Client 之上提供
`Load / Reload / Current / Watch`；只在进程内保留 Last-known-good，changed
Snapshot 原子安装后才串行调用 `OnChange`，并通过 `OnError` 报告 Watch 错误。
回调在 fetch/install 锁外运行；回调尚未结束时，无变化的 Reload 可以直接返回，
有变化的重叠 Reload 返回 `ErrReloadRunning` 且不得安装第二个 Snapshot。

Viper 作为 consumer/parser，不 fork Viper。Viper Handler 是通用 Go Client
之上的薄 Adapter。Snapshot 暴露只读版本信息与 `Unmarshal`，不把可变的
`*viper.Viper` 交给调用者。Go Client 与 Viper Handler 独立维护在
`configra-go` repo；Configra 对外契约始终是 V1，依赖模块自身的版本后缀不改变
该契约。

### 11.1 HTTPS / mTLS

生产环境禁止通过明文 HTTP 获取 Config 或 resolved Secret。Go Client
必须支持 HTTPS 和 mTLS；mTLS 负责认证调用方工作负载，API Token 仍负责
Configra 内的 Principal 与权限，两者不互相替代。

TLS Transport 只在通用 Go Client 中实现，Viper Handler 直接复用，禁止维护
第二套 TLS 配置。Go Client 接受调用方提供的标准库 `*tls.Config`，内部 Clone
后使用：

-   Server Root CA 内容或文件路径可以来自客户端自己的冷启动 YAML。
-   Client Certificate / Private Key 必须由调用方在运行时注入；需要动态轮换时
    使用 `tls.Config.GetClientCertificate`，替换证书后调用 Client
    `CloseIdleConnections` 强制下一次请求重新握手；不新增自定义证书 Provider
    接口。
-   用于连接 Configra 自身的 Client Certificate / Private Key 禁止从 Configra
    获取，以免形成启动循环；不得进入普通配置、缓存或日志。
-   Kubernetes 中优先由 Secret Volume、cert-manager 或同类机制提供 Client
    Certificate / Private Key，`configra-go` 不负责签发或持久化证书。

API 与 Management 的冷启动 YAML 都配置同一 Client CA。API Server 用它完成
TLS 链校验；Management 只用它校验管理员粘贴导入的 Client Certificate，随后按
leaf fingerprint 加入动态允许列表。Management 的浏览器 HTTPS Listener 始终为
`NoClientCert`，不会因配置这份 CA 而对人机界面启用 mTLS；Private Key 不允许导入。

## 12. Notification Destination

Notification 是 V1 能力。管理员在 Management 中配置两种有明确协议的
Destination：`generic_webhook` 与 `feishu_bot`。飞书不是 Generic Webhook 的
别名；它使用飞书自定义机器人的消息、签名与响应协议。Slack 等其他 Provider
等真实需求出现后再增加。

事件建议：

``` text
config.created / updated / merged / replaced / restored / cloned
vault.created / updated / restored / archived / unarchived
environment.created / renamed / archived / unarchived
token.created / revoked
```

Destination 配置：Key、Display Name、Provider、URL、Secret、Enabled、
Events\[\]，支持 Test、Archive、Delivery History 与手动 Redelivery。完整 URL
可能在 path/query 中携带凭据，与可选 Secret 一起通过 KeyProvider 加密；列表、
历史、Audit 与日志只暴露 safe Host、掩码和安全错误类别。

Generic 使用版本化 HMAC-SHA256 合约，Header 固定为
`X-Configra-Event / X-Configra-Delivery / X-Configra-Signature /
X-Configra-Timestamp`；飞书使用官方 timestamp/sign 算法与 `code == 0`
成功判定。

Vault Event 永远不能包含 plaintext Secret、文件内容或 resolved Config。

### Transactional Outbox

``` text
BEGIN
write revision
write audit outbox event
write notification outbox event
COMMIT
```

Management Server 内部 worker 每次只执行一次 HTTP 尝试；网络错误、429 与
5xx 使用 full-jitter exponential backoff，并取本地 backoff 与有界
`Retry-After` 的较大值写回 MySQL。每个 Destination 最多 8 次，3xx 与其他
4xx 直接 Dead；投递为 at-least-once，允许重复与乱序，无需消息队列。

出站 Client 只用标准库：禁止 Proxy 与 Redirect，在实际 socket connect 前校验
解析后的 IP，默认只允许公网 HTTPS/443。冷启动 YAML 可精确允许内部 Host/CIDR、
内部自定义端口并追加全局 Root CA；loopback、link-local/metadata、multicast、
unspecified 与危险 transition/NAT64 地址始终拒绝。飞书 URL 只接受
`open.feishu.cn/open-apis/bot/v2/hook/...`。

## 13. Audit

Audit 记录每个已认证的 mutation attempt 及其结果。最终 Audit Event
写入 ClickHouse；先在 MySQL 持久化 Audit Outbox Event，成功 mutation
与业务变更使用同一事务，其他 attempt 使用独立短事务。后台投递到
ClickHouse 成功后才确认 Outbox，因此允许重复投递，但不允许静默丢失。
投递按最多 500 条批量执行，失败记录下次尝试时间并保留 Outbox；损坏的
value-free Event 标记为 Dead 供管理员处理，而不是静默删除。

Audit 绝不记录 Secret plaintext、file plaintext、API Token
plaintext、resolved Config body。

## 14. Access Statistics

读取统计与 Audit 分开。API Server 通过 Core NATS 最大努力发出
Access Event，由 Management Server 写入 ClickHouse；这条路径不得阻塞或
影响客户端读取。记录 resource、Vault Namespace（适用时）、environment、token/user、返回的 Revision 与时间，
不记录响应内容。V1 统计由这些成功内容响应聚合 request_count、Top Resource/
Environment/Principal 与 last_accessed_at；拒绝响应、304 和本地 Last-known-good
命中不属于 Access Event，且事件可能丢失，因此不能从它推导完整错误率或计费数据。
仪表盘直接基于 ClickHouse 聚合；V1 不在 MySQL 维护第二份统计表。

V1 Subject 固定为 `configra.v1.access`，Envelope 固定为
`schema_version: 1`。API Server 使用 8 MiB 有界重连缓冲；Management
Consumer 使用 8192 条有界 Channel，并以最多 500 条或 250 ms 的窗口写入
ClickHouse。缓冲耗尽或写入失败允许丢弃并记录不含值的告警。

## 15. Dashboard

Dashboard 只回答三个问题：MySQL 与 ClickHouse 当前是否可用、最近实际记录了哪些
Mutation、哪些启用的 Config 缺少可用 Environment Revision。最近变更直接读取
ClickHouse Audit Event，并按 Event ID 去重；不同资源、不同 Environment 的 Revision
绝不拼成一条虚假的时间轴。资源数量仅用于快速进入对应列表，最近一次客户端读取仅来自
最新 Access Event。

没有采集的数据不显示占位统计，无法观测的组件不伪装成健康状态，页面标题也不展示没有
操作意义的协议版本。Access 页面单独展示近期已观测内容响应和 Authentication 分布。
更长窗口的 Top Config/Environment/Principal 可直接查询 ClickHouse，不在 V1 内置
图表系统。

不要做成 Prometheus/Grafana 替代品。

## 16. UI 页面规划

建议 V1 约 11 个主要页面/流程：

1.  Dashboard / Overview
2.  Environment List / Detail
3.  Config List
4.  Config Detail / Editor / History
5.  Config Revision Compare
6.  Merge / Replace `A → B`
7.  Vault Item List
8.  Vault Item Detail / Fields / Variants / History / Restore
9.  API Token Management
10. Notification Destination Management + Delivery History
11. Access Statistics

另外提供 Audit Log 和 Administration。Administration 管理 API Token、客户端证书
和只读部署状态；Database、OIDC、KeyProvider、HTTP/TLS 等冷启动配置来自部署
YAML，不允许在管理平台修改。

Vault UI 学习 1Password：用户主要面对 Item，而不是数据库式 KV 表格。列表面向约
1000 个 Item 使用紧凑表格、搜索和 Namespace 筛选；Namespace 只是 Item 身份中的
不可变列，不做目录树或单独管理页。创建 Item 时可复用已有 Namespace Key 或直接输入
新的合法 Key。详情页为每个 Text/Secret Field 展示可复制的
`{vault.<namespace>.<item>.<field>}`，File Field 展示完整 ReadFile 路径。

Environment 列表使用紧凑表格；点击 Environment 进入详情，聚合展示该 Context 当前的
Config Revision 与 Vault Variant 绑定。Config/Vault 列表支持精确 Environment 筛选，
筛选条件保存在 hash query 中，进入详情后浏览器返回仍保留原筛选。Vault Item 列表仅增加
当前 Revision 的 Environment Key 元数据，不返回 Field Value。

Vault Item 详情展示引用它的当前、启用 Config Revision，并按 Field 列出
Environment、Config 与 Revision；该查询只返回元数据。删除仍被引用的 Field，或移除仍被
当前 Config 使用的 Environment 绑定时，UI 必须列出影响并要求二次确认，但服务端不硬阻止，
因为 V1 明确允许为分步部署保存 unresolved reference。

Config Editor 提供明确的 `Format / Compare / History / Save`；Format 调用
`POST /v1/configs/validate`，只把编辑区替换为服务端返回的 canonical text，不产生
Revision。Save 与该端点复用同一套 Domain 校验和格式化规则。存在 Warning 时展示后
允许继续保存，存在 Error 时禁止保存。Revision 轨道只显示最近八个快捷入口，完整 History
中的任意 Revision 都可点击查看，历史版本始终只读。Config/Vault 编辑器在离开脏稿或取消
编辑前使用浏览器原生确认；Conflict 保留原稿并显示稳定错误码对应文案与可复制 Request ID，
未修改或正在提交时禁用 Save，成功后明确显示新 Revision。

## 17. Storage

MySQL 是常规业务与事务存储；ClickHouse 是日志和统计查询存储。
V1 直接以 MySQL 为目标，不为未确定的 PostgreSQL 兼容性增加抽象。
线上兼容基线固定为 MySQL `8.0.22`；容器集成测试与发布门禁必须运行同一
精确版本，不得用 `8.0` 或 `latest` 代替。

MySQL 主要表：

``` text
environments
configs
config_env_states
config_revisions
config_revision_vault_refs
vault_items
vault_item_revisions
api_tokens
notification_destinations
notification_subscriptions
outbox_events
notification_targets
notification_deliveries
```

Configra 自身管理的 ClickHouse 日志表：

``` text
access_events
audit_events
```

Zap 应用/安全运行日志写结构化 stdout，由 Kubernetes 的现有日志采集链路送入
ClickHouse；Configra V1 不再内置第二套通用日志采集器或假定运维侧表结构。业务内容
读取与 Mutation 的产品级明细分别以 `access_events`、`audit_events` 为权威记录，
任何持久日志都不写 MySQL。

`vault_items` 以 `(namespace_key, resource_key)` 建立唯一约束；
`config_revision_vault_refs` 同时保存 `namespace_key / item_key / field_key`，防止
跨 Namespace 串值。Vault Revision 使用 snapshot-oriented schema，并保证 Revision
immutable、Environment binding 唯一性和 Mutation 原子性。

Audit Event 通过 MySQL Transactional Outbox 至少一次投递到
ClickHouse；Access Event 继续通过 Core NATS 最大努力投递。ClickHouse
不参与 API 内容读取的同步路径，宕机时只影响日志写入/查询及统计新鲜度。
V1 的 `access_events` 默认保留 90 天；`audit_events` 不设置自动 TTL。

灾难恢复分为两条独立链路。服务状态由一个事务一致的 MySQL 快照与独立保管的
冷启动配置、Master Key 恢复；Master Key 不得进入数据库备份，恢复后必须先通过
Crypto Sentinel 校验才可 Ready。ClickHouse 使用自身的原生备份保存 Access/Audit
日志；它不阻塞内容服务恢复，但日志丢失属于明确的审计留存故障。Core NATS 是瞬时
传输，不进入备份集。详见
[ADR-0027](adr/0027-separate-service-state-and-log-recovery.md)。

## 18. Go 后端结构

``` text
cmd/configra
internal/
  accessnats/
  bootstrap/
  clientcert/
  configdoc/
  humanauth/
  logstore/
  logworker/
  machine/
  management/
  notification/
  storage/mysqlstore/
  vaultcrypto/
  vaultdoc/
web/
tla/

# 独立仓库 configra-go
client.go
handler.go
```

纯文档与加密 invariant 位于 `configdoc`、`vaultdoc`、`vaultcrypto`，不依赖
HTTP/SQL；需要锁、幂等 Operation 和 Transactional Outbox 的状态变更直接封装在
`mysqlstore` 的单个事务内，HTTP Handler 保持薄层。OIDC、Notification 与日志存储
保持外部边界。Go Client SDK 与 Viper Handler 独立维护在 `configra-go` repo；
Viper Handler 只是通用 Go Client 之上的薄 Adapter。

Go 基础设施能力优先使用维护活跃、生产验证充分的成熟类库，并保持直接、薄的
集成：日志使用 Zap；进程入口使用 Cobra 提供 `management` 与 `api`
子命令。YAML 由 `go.yaml.in/yaml/v3` 负责解析、AST 与编码，JSON 使用标准库
`encoding/json`。Configra 只保留成熟类库无法表达的业务规则，例如
[ADR-0005](adr/0005-use-directional-two-way-merge.md) 的确定性二路 overlay；
不自研 parser、formatter、日志框架或 CLI 框架，也不为单一能力引入完整 DSL
引擎。具体比较见 [Go 类库选型](research/go-library-selection.md)。

### 18.1 Deployment / Configuration Boundary

Configra 发布为 Docker Image，目标运行环境是 Kubernetes。Server 使用 YAML
完成冷启动配置；非敏感 YAML 适合放入 ConfigMap，密码、Master Key、TLS
Private Key 等只通过环境变量或 Secret Volume 引用，禁止把明文写入 YAML。
可直接从
[`deploy/management.example.yaml`](../deploy/management.example.yaml) 与
[`deploy/api.example.yaml`](../deploy/api.example.yaml) 开始部署配置。可渲染的
Kustomize 基础清单位于
[`deploy/kubernetes/base`](../deploy/kubernetes/base)，部署所需 Secret、入口 TLS
边界与发布镜像验证命令见
[`deploy/kubernetes/README.md`](../deploy/kubernetes/README.md)。Management 固定
单副本；API 初始单副本并可独立横向扩展。

冷启动配置包括 Server、MySQL、ClickHouse、NATS、OIDC、KeyProvider、
HTTP/TLS/mTLS、日志，以及 Notification 出站网络例外与全局 Root CA 等启动
依赖，修改后通过重启生效。Server Certificate / Private Key 与用于校验
Client Certificate 的 CA 都属于冷启动配置。

Environment、Config、Vault Item、API Token、Client Certificate、Notification
Destination 等明确业务资源由管理平台或 API 管理。Configra 不使用自身
托管的 Config 作为冷启动依赖，避免启动循环。

### 18.2 V1 Management HTTP

Management HTTP 只使用 `/v1`。Vault Item 的列表、详情和历史版本默认只返回
Field/Variant/Environment 元数据并移除所有 Value；只有 Admin 可以调用
`/values` 读取完整值。当前与历史版本分别使用：

``` text
GET  /v1/environments
POST /v1/environments
PATCH /v1/environments/{environment}
POST /v1/environments/{environment}/archive
POST /v1/environments/{environment}/unarchive

GET  /v1/configs
POST /v1/configs/validate
POST /v1/configs/{config}/archive
POST /v1/configs/{config}/unarchive
GET  /v1/environments/{environment}/configs/{config}
PUT  /v1/environments/{environment}/configs/{config}
GET  /v1/environments/{environment}/configs/{config}/resolved-preview
GET  /v1/environments/{environment}/configs/{config}/revisions
GET  /v1/environments/{environment}/configs/{config}/revisions/{revision}
POST /v1/environments/{environment}/configs/{config}/transfer-preview
POST /v1/environments/{environment}/configs/{config}/merge
POST /v1/environments/{environment}/configs/{config}/replace
POST /v1/environments/{environment}/configs/{config}/restore
POST /v1/environments/{environment}/configs/{config}/clone

GET  /v1/vault-items
GET  /v1/vault-items/{namespace}/{item}
GET  /v1/vault-items/{namespace}/{item}/usages
GET  /v1/vault-items/{namespace}/{item}/values
GET  /v1/vault-items/{namespace}/{item}/revisions
GET  /v1/vault-items/{namespace}/{item}/revisions/{revision}
GET  /v1/vault-items/{namespace}/{item}/revisions/{revision}/values
PUT  /v1/vault-items/{namespace}/{item}
POST /v1/vault-items/{namespace}/{item}/restore
POST /v1/vault-items/{namespace}/{item}/archive
POST /v1/vault-items/{namespace}/{item}/unarchive

GET  /v1/api-tokens
POST /v1/api-tokens
PUT  /v1/api-tokens/{public_id}/environments
POST /v1/api-tokens/{public_id}/revoke

GET  /v1/client-certificates
POST /v1/client-certificates
POST /v1/client-certificates/{fingerprint}/revoke

GET  /v1/notification-destinations
PUT  /v1/notification-destinations/{destination}
POST /v1/notification-destinations/{destination}/archive
POST /v1/notification-destinations/{destination}/unarchive
POST /v1/notification-destinations/{destination}/test
GET  /v1/notification-destinations/{destination}/deliveries
POST /v1/notification-destinations/{destination}/deliveries/{delivery}/redeliver

GET  /v1/access
GET  /v1/audit?limit={1..500}&offset={n}&q={metadata search}
```

Audit 列表由 ClickHouse 执行搜索和分页；`q` 只匹配 Operation、Actor、Action、Outcome、
Environment、Namespace 与资源标识等 value-free 元数据。响应以 `has_more` 指示下一页，
Management UI 默认每页读取 25 条，不会把完整审计历史加载到浏览器。

`/v1/vault-items/{namespace}/{item}/usages` 允许 Viewer/Admin 读取，只返回当前且启用的
Config 引用元数据：Field Key、Environment Key、Config Key/Display Name 和 Config
Revision。它排除历史 Revision、已 Archive Config 与已 Archive Environment，不返回
Config 原文、Vault Value 或 Secret。

成功返回完整值后，Management 使用已有 NATS 连接发送
`authentication=oidc`、`resource_type=vault_item` 且包含 Namespace 的 value-free Access Event；
发布失败不影响响应。元数据读取和拒绝的读取不发送 Access Event。
API Token 与 Client Certificate 路由只允许 Admin；列表只返回元数据。证书导入
请求只接受公钥证书 PEM/chain，并在写入 MySQL 前使用冷启动 Client CA 与
ClientAuth EKU 校验。

## 19. TDD

优先测试 Domain invariant，而不是堆 HTTP Handler 测试。

至少覆盖：

``` text
a → b normal
b → a normal
source remains unchanged
stale target revision → conflict
operation retry → idempotent
restore creates new revision
clone starts at revision 1
clone does not copy history
vault overlapping env → reject/move
same item key in different vault namespaces → isolated items and revisions
three-segment vault reference → reject
namespace never changes Environment authorization
vault reference inherits current env
missing vault env → resolution error
field change → whole Item revision +1
file replacement → whole Item revision +1
secret never appears in audit/notification
invalid YAML rejected
invalid JSON rejected
duplicate YAML key rejected
duplicate JSON key rejected
format is deterministic
format-only change creates no revision
invalid vault reference syntax rejected
missing vault item produces warning
missing vault field produces warning
missing vault environment variant produces warning
every Config commit path runs the same validation/format pipeline
committed Config revision is always valid and canonical formatted
mTLS succeeds with trusted CA and runtime client certificate
mTLS rejects missing or untrusted client certificate
Viper Handler reuses the Go Client transport
```

Go 使用 table-driven tests；Repository/API 再做 integration tests。

## 20. TLA+

TLA+ 重点验证 Revision、并发、Merge/Replace、Clone、Vault
Binding、Operation Idempotency、Transactional Outbox。

V1 Model 建议包含
Environment、Config、Revision、VaultItem、Variant、Operation。

核心 invariants：

1.  Revision immutable。
2.  成功 Mutation 恰好创建一个新的 Target Revision。
3.  Merge/Replace 永远不修改 Source。
4.  预览时记录的 Target current revision 与提交时不匹配时不能提交。
5.  相同 OperationID 重试最多产生一个 Revision。
6.  Restore 不修改旧 Revision，只创建新 Revision。
7.  Config Clone 不复制历史，新对象从 Revision 1 开始。
8.  同一 Vault Item 当前状态下，一个 Environment 最多属于一个 Variant。
9.  `Config/env=x` 的 Vault Reference 只能解析包含 x 的 Variant。
10. Vault Item 任意状态变化都创建完整 Item 新 Revision。
11. Revision + Audit Outbox + Notification Outbox Event 在 MySQL 原子提交。
12. 任何 committed Config Revision 都满足 `ValidConfig`。
13. 非法 YAML/JSON 永远不能进入 committed revision state。
14. Config Mutation 只有在 expected target current revision 与 current revision
    一致时才能提交。
15. Vault Reference 只能解析精确的 `(Namespace, Item, Field)`，同名 Item 不得跨
    Namespace 串值。
16. 改变 Vault Namespace 不能改变 Token 的 Environment 授权结果。

## 21. 技术栈

``` text
Backend       Go
Database      MySQL
Log Store     ClickHouse
Frontend      React + CodeMirror 6
Auth          OIDC / Casdoor
Config        YAML + JSON only
YAML          go.yaml.in/yaml/v3
JSON          encoding/json
Logging       Zap
CLI           Cobra（management / api）
Crypto        AES-256-GCM + Envelope Encryption
Client        独立 configra-go repo（Go SDK + Viper Handler）
Client TLS    HTTPS + mTLS（runtime client certificate）
Deployment    Docker Image + YAML bootstrap，Kubernetes target
Notification  Generic HMAC / Feishu + SSRF-safe HTTP + Transactional Outbox
Testing       TDD
Formal Spec   TLA+
```

管理界面的 YAML/JSON 编辑、只读预览和 Revision Diff 统一使用 CodeMirror 6；
Diff 由 `@codemirror/merge` 提供行级与字符级变化标记，不自行实现编辑器或 Diff
渲染。CodeMirror 运行时样式使用每个 HTML 响应生成的 CSP nonce，禁止为编辑器放宽
为 `style-src 'unsafe-inline'`。

## 22. V1 封板范围

``` text
Configra
├── Dashboard
├── Environments: Create / Rename / Archive / Unarchive
├── Config: YAML/JSON / Revision / Diff / Merge / Replace / Restore / Clone
├── Vault: Item / Variant / text-secret-file / Encryption / History / Restore
├── Auth: OIDC viewer/admin + Environment-scoped read-only API Token
├── Notification: Generic / Feishu / HMAC / Outbox / Retry / Test / Redelivery
├── Access Stats
├── Audit
├── Client: 独立 configra-go（Go SDK + Viper Handler + HTTPS/mTLS）
├── Deployment: Docker + YAML bootstrap + Kubernetes
└── Go + MySQL + ClickHouse + React/Vue + TLA+ + TDD
```

## 23. 设计纪律

后续每增加一个能力都先问：

> **它是否破坏了 Configra 的精简模型？**

如果会引入复杂组织模型、权限继承、审批流水线、额外基础设施或模糊核心语义，优先放到未来版本，而不是进入
V1。
