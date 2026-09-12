# Management OIDC、Session 与 CSRF 选型

调研日期：2026-08-26。范围是 Configra Management Server 的人员登录、浏览器
Session 与 CSRF；Casdoor 是首个兼容目标，但实现保持标准 OIDC。本文中的
`/v2`、`/v3` 只出现在上游 Go Module 的包路径中，不代表 Configra 存在 V2；
Configra 的 API、Schema 与产品契约仍然都是 V1。

## 结论

| 能力 | V1 选择 | 最小理由 |
| --- | --- | --- |
| OIDC | `github.com/coreos/go-oidc/v3/oidc` + `golang.org/x/oauth2` | 标准 Discovery/JWKS/ID Token 校验与 Authorization Code + PKCE 已覆盖，不绑定 Casdoor |
| Session | `github.com/alexedwards/scs/v2` + 官方 `mysqlstore` | 服务端 Session、绝对/空闲超时、登录换号与服务端注销均已有成熟实现 |
| CSRF | Go 1.25+ `net/http.CrossOriginProtection` | 当前 Web UI 与 Management API 同源；标准库已能拒绝非安全的跨源浏览器请求，无需另一套 token/cookie |
| Casdoor SDK | 不引入 | Configra 不管理 Casdoor 用户/资源；标准 OIDC 足够且验证边界更清楚 |
| `gorilla/sessions` | 不引入 | CookieStore 简单，但主动注销、空闲超时和服务端撤销不如 SCS 直接 |
| `gorilla/csrf` | 暂不引入 | 功能成熟，但在当前同源部署中会重复标准库能力，并增加密钥、Cookie 和前端 token 协议 |

落地时固定当前稳定版本：`go-oidc` `v3.20.0`、`x/oauth2`
`v0.36.0`、SCS `v2.9.0`；SCS 官方 MySQL adapter 尚以独立伪版本发布，应固定
精确 commit，不使用浮动的 `latest`。版本信息可由各包的官方 Go Module 页面核对：
[go-oidc](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc)、
[x/oauth2](https://pkg.go.dev/golang.org/x/oauth2)、
[SCS](https://pkg.go.dev/github.com/alexedwards/scs/v2)。

## OIDC：使用标准客户端

### 为什么选择 `go-oidc` + `x/oauth2`

`go-oidc` 通过 Issuer Discovery 构造 OAuth 端点和远程 JWKS verifier；它校验
签名算法、签名、Issuer、Audience/Client ID、Expiry 和 `nbf`，远程 KeySet 会
缓存 Key，并在遇到未知 `kid` 时重新获取。不能启用 `SkipIssuerCheck`、
`SkipClientIDCheck`、`SkipExpiryCheck` 或 `InsecureSkipSignatureCheck`。
Nonce 是该库明确留给调用方校验的字段，必须由 Configra 补上。
[go-oidc verifier 文档](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc#IDTokenVerifier.Verify)
[go-oidc verifier 源码](https://github.com/coreos/go-oidc/blob/v3/oidc/verify.go)

`x/oauth2` 已提供 `GenerateVerifier`、`S256ChallengeOption` 与
`VerifierOption`，无需自行实现 PKCE。每次授权请求必须生成独立 verifier，并且
只使用 S256。[x/oauth2 PKCE API](https://pkg.go.dev/golang.org/x/oauth2#GenerateVerifier)
[RFC 7636](https://www.rfc-editor.org/rfc/rfc7636.html#section-4)

OIDC Core 要求 ID Token 的 `iss`、`aud`、签名、`exp` 和 Nonce 满足对应的
授权请求；`sub` 是稳定身份，不应拿 Email 当主键。若 Audience 有多个值，还要
检查 `azp` 等于 Configra Client ID。`go-oidc` 完成前一组通用校验，Configra
负责 Nonce、`sub` 非空以及多 Audience 时的 `azp`。
[OIDC Core ID Token validation](https://openid.net/specs/openid-connect-core-1_0-final.html#IDTokenValidation)

OAuth Security BCP 要求重定向 URI 精确匹配、客户端不得成为 Open Redirector，
并推荐 Authorization Code 而不是把 Token 放进前端重定向。Configra 因此只
接受冷启动配置中的固定 HTTPS callback URI，不能根据请求参数、`Host` 或
`X-Forwarded-*` 动态拼接。
[RFC 9700 redirect-based flows](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)

### 为什么不用 Casdoor Go SDK

Casdoor 官方支持标准 OIDC Discovery，Casdoor 自己也建议标准客户端用于普通
OIDC 接入；其 SDK 的价值主要是用户、组织、Role、Permission、Resource 等
Casdoor 管理 API。[Casdoor standard OIDC client](https://casdoor.org/docs/how-to-connect/standard-oidc-client/)
[Casdoor SDK 说明](https://casdoor.org/docs/how-to-connect/sdk/)

官方 Go SDK 的示例交换 Code 后解析的是 Access Token；其 `ParseJwtToken`
使用静态证书与算法白名单验签，但 API 没有传入预期 Issuer/Audience，也没有
组合 Discovery、JWKS Rotation、PKCE、State 和 Nonce。Configra 若只为登录引入
它，仍需自己补标准 OIDC 验证，同时带入大量不使用的 Casdoor 管理接口。
[Casdoor Go SDK callback 示例](https://github.com/casdoor/casdoor-go-sdk#authentication)
[ParseJwtToken 源码](https://github.com/casdoor/casdoor-go-sdk/blob/master/casdoorsdk/jwt.go)

结论不是 Casdoor SDK 不可用，而是它不适合 Configra 的这个窄边界。Casdoor
继续作为普通 OIDC Provider 接入，未来只有 Configra 真的要调用 Casdoor 管理
API 时才加入 SDK。

## 最小安全登录流程

### 启动

1. 从冷启动 YAML/Secret 读取 Issuer、Client ID、Client Secret、固定 Redirect
   URI、额外 Scope、Role Claim 来源与映射。
2. Issuer、Redirect URI 必须是 HTTPS；只允许显式的本地开发模式使用 loopback
   HTTP，生产不可降级，也不可 `InsecureSkipVerify`。
3. 通过 `oidc.NewProvider` 做 Discovery，复用一个 Provider、Verifier 与带明确
   connect/response timeout 的 HTTP Client，不要每次登录重新构造。
4. 启动时拒绝空的 role claim、空映射、Viewer/Admin 值重叠及非 HTTPS
   public callback。OIDC 不可用时 Management 启动失败，不能静默变成免登录。

### 发起登录

1. 生成互不复用的随机 `state`、`nonce` 和 PKCE verifier；State/Nonce 至少来自
   32 bytes `crypto/rand`，PKCE 使用 `oauth2.GenerateVerifier()`。
2. 把 `{state, nonce, verifier, created_at, return_to}` 放在服务端 Session；V1
   每个浏览器 Session 只保留一个未完成登录，新登录使旧 callback 失效。
3. `return_to` 只保存站内绝对路径：必须以单个 `/` 开头，拒绝 `//`、Scheme 和
   Host；非法值回到 `/`，避免 callback 成为 Open Redirector。
4. 调用 `AuthCodeURL(state, oidc.Nonce(nonce),
   oauth2.S256ChallengeOption(verifier))`。Scope 至少含 `openid`，通常再加
   `profile email` 和 Provider 返回 Role Claim 所需的 Scope。

State 是绑定授权响应与浏览器 Session 的一次性值；PKCE 防止截获的 Code 被其他
客户端兑换；Nonce 把最终 ID Token 绑定到本次认证。三者很便宜，因此全部使用，
不把它们当互斥替代品。[OAuth `state`](https://www.rfc-editor.org/rfc/rfc6749.html#section-10.12)
[OAuth Security BCP PKCE/CSRF](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.7.1)

### Callback

按以下顺序 Fail Closed：

1. 只接受固定 callback 路径与 GET；读取服务端 Session 中的待处理登录。
2. 对返回 State 做常量时间比较并检查 10 分钟 TTL；无论后续成功与否，先把
   `state/nonce/verifier` 从 Session 中弹出，保证 callback 单次使用。
3. Provider 返回 `error`、缺 Code、State 缺失/错误/过期都拒绝，不调用 Token
   Endpoint。
4. 用固定 Redirect URI 和 `oauth2.VerifierOption(verifier)` 兑换 Code；要求响应
   存在 `id_token`。
5. 用 Provider Verifier 校验 ID Token 的签名、算法、Issuer、Audience、Expiry；
   再手工校验 Nonce、`sub`，以及多 Audience 时的 `azp`。
6. 读取 Role Claim，映射成功后先调用 SCS `RenewToken`，再清除预登录数据并写入
   `{issuer, subject, role, display_name, email}`。Actor ID 使用 `(issuer, sub)`，
   Email 只用于显示。
7. 丢弃 Access Token、Refresh Token 和原始 ID Token；V1 不调用下游资源 API，
   没有继续保存它们的理由。响应使用 303 跳到已验证的站内 `return_to`，清除
   带 Code 的地址。

日志不得记录 callback Query、Authorization Header、Token、State、Nonce、PKCE
verifier 或 Session Cookie；错误只记录安全的阶段、Provider 与关联 ID。

## Claim 到 `viewer/admin` 的映射

只支持一个冷启动配置的顶层 Claim，其值必须是 string 或 string array。配置包含
互不重叠的 `viewer_values` 与 `admin_values`，使用区分大小写的精确匹配：

1. 命中任一 Admin 值，得到 `admin`；
2. 否则命中任一 Viewer 值，得到 `viewer`；
3. Claim 缺失、类型错误、空值或没有命中时拒绝登录，不提供默认 Viewer；
4. 不自动把 Casdoor 的全局管理员或 Email Domain 映射为 Configra Admin。

Role Claim 来源显式配置为 `id_token` 或 `userinfo`，不要不透明地在两处拼接。
Casdoor 的标准 UserInfo 模型包含 `groups`、`roles`、`permissions` 字符串数组，
所以推荐 Casdoor 使用 `source: userinfo` 与 `claim: groups`（或 `roles`）。
[Casdoor UserInfo 定义](https://github.com/casdoor/casdoor-go-sdk/blob/master/casdoorsdk/user.go#L61-L77)

使用 UserInfo 时只在登录 callback 调用一次，并必须验证 UserInfo 的 `sub` 与已
验证 ID Token 的 `sub` 完全相同；OIDC Core 明确要求不匹配时不得使用响应中的
Claims。[OIDC Core UserInfo response](https://openid.net/specs/openid-connect-core-1_0-final.html#UserInfoResponse)
[go-oidc UserInfo API](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc#Provider.UserInfo)

Role 在 Session 建立时快照化。V1 中 IdP Role 变化最迟在本地 Session 绝对过期
或用户重新登录后生效；如果将来要求即时移除所有登录，再增加按 `(issuer, sub)`
枚举并销毁 Session 或 OIDC Back-Channel Logout，不在首版轮询 UserInfo。

## Session：选择 SCS

SCS 使用浏览器中的随机 Session ID 和服务端数据，原生提供绝对 Lifetime、Idle
Timeout、`RenewToken`、`Destroy`、持久化 Store 与过期清理；官方明确要求在登录
等权限变化前换 Session Token。[SCS 文档](https://github.com/alexedwards/scs#preventing-session-fixation)

使用官方 MySQL Store 复用 Configra 已有 MySQL，不引入 Redis。Store 只需要
`token/data/expiry` 表，支持后台清理；使用独立表名 `management_sessions`，固定
精确 adapter commit，并在 Management 优雅退出时停止 cleanup goroutine；Store
迁移与过期清理的集成测试固定使用线上同版 MySQL 8.0.22。
[SCS MySQL Store](https://github.com/alexedwards/scs/tree/master/mysqlstore)

生产默认值：

``` text
Lifetime:        8h absolute
IdleTimeout:     30m
HashTokenInStore:true
Cookie.Name:     __Host-configra_session
Cookie.Domain:   empty
Cookie.Path:     /
Cookie.Secure:   true
Cookie.HttpOnly: true
Cookie.SameSite: Lax
Cookie.Persist:  false
```

`HashTokenInStore` 防止仅凭 Session 表中的 Token 直接重放。Session Data 只存
身份、内部 Role 与未完成的短期登录参数，不存 OAuth Token、Vault 值或 Secret。
Session Store 读取/写入错误返回 500 并 Fail Closed，生产不得退回内存 Store。

`SameSite=Lax` 是有意选择：OIDC Provider 到 callback 是跨站顶层 GET，需要带回
保存 State/Nonce/Verifier 的 Session Cookie。更严格的 `SameSite=Strict` 会让这
条正常回调丢失 Cookie；普通写请求则由下节的同源检查保护。Cookie 同时使用
`Secure`、`HttpOnly`、Host-only、`Path=/`；当前 BFF 安全建议同样要求这些属性，
并建议用 Host 前缀限制子域共享。[RFC 10017 BFF cookie guidance](https://www.rfc-editor.org/rfc/rfc10017.html#section-6.1.3.2)

### 为什么不用 `gorilla/sessions`

`gorilla/sessions` 是成熟库，CookieStore 支持签名、可选加密与 Key Rotation；若
需求只是少量无状态偏好，它更简单。[gorilla/sessions](https://github.com/gorilla/sessions)

但管理平台 Session 需要服务端注销、空闲/绝对过期和登录后换号。把完整身份放进
CookieStore 后，已签发 Cookie 在到期前不能由服务端可靠撤销；FilesystemStore
官方仍标记为实验性，也不适合容器。SCS + 现有 MySQL 更直接，不需要再挑一个
第三方 Gorilla MySQL Store。

## CSRF：选择 Go 1.25+ 标准库

`http.CrossOriginProtection` 对 GET/HEAD/OPTIONS 放行，对其他方法先看
`Sec-Fetch-Site`，只接受 `same-origin`/`none`，因此来自同一 eTLD+1 但不同子域
的 `same-site` 请求也会拒绝；旧浏览器路径再比较 Origin 与 Host。缺少
Fetch/Origin Header 时，标准库会假定请求同源或来自非浏览器并放行；这不是
密码学证明，因此生产兼容基线必须是支持 Fetch Metadata 的现代浏览器，并同时
启用 HTTPS/HSTS 与下文的 SameSite Cookie。
[Go 1.25 release notes](https://go.dev/doc/go1.25#net/http)
[CrossOriginProtection source](https://go.dev/src/net/http/csrf.go)

V1 策略：

1. 用 `CrossOriginProtection.Handler` 包住全部 Management Web/API 路由，置于
   Session DB 读取之前；不配置 Trusted Origin，也不配置 bypass pattern。
2. 所有状态变化只允许 POST/PUT/PATCH/DELETE；GET/HEAD/OPTIONS 永不写状态。
3. UI 与 Management API 必须同源，不开启带 Credentials 的跨源 CORS。
4. OAuth callback 的 GET 不依赖通用 CSRF middleware，而由一次性 State + PKCE
   + Nonce 单独保护；Logout 使用 POST。
5. 前端不需要获取或回传通用 CSRF Token。这里的“无 token”不包括 OAuth State。

Go 1.25.0 的 bypass pattern 曾有已修复漏洞，因此最低版本为 1.25.1，部署时应
使用受支持 Go 分支的最新 Patch；本方案也不调用 bypass API。
[Go vulnerability GO-2025-3955](https://pkg.go.dev/vuln/GO-2025-3955)

### 为什么暂不使用 `gorilla/csrf`

`gorilla/csrf` 提供带掩码 token、签名 Cookie、Header/Form 校验和 Origin/Referer
检查，是需要兼容旧浏览器或跨源 UI 时的成熟选择。
[gorilla/csrf](https://github.com/gorilla/csrf)

当前同源 UI 使用它会额外引入一把长期 CSRF Key、一个 Cookie、一条前端 token
获取/刷新协议和依赖，而 Go 1.25 已覆盖当前威胁面。若未来 UI 与 Management API
分离到不同 Origin、必须支持 2023 年前浏览器，或代理无法保留 Fetch Metadata 与
Origin，再改用 `gorilla/csrf`；不要在现在双重启用两套机制。

## Logout 与过期

`POST /auth/logout` 先通过跨源检查，再调用 SCS `Destroy`，删除 MySQL Session、
发出过期 Cookie，最后 303 到固定站内地址。GET Logout 不存在。Idle/Absolute
过期都返回 401，由前端跳到登录页；不能在过期时静默沿用旧 Role。

V1 只保证 Configra 本地注销。OIDC RP-Initiated Logout 需要 Discovery 中的
`end_session_endpoint`、通常还要保留 `id_token_hint` 和注册固定
`post_logout_redirect_uri`；只有明确需要“同时退出 Casdoor/其他应用”时再加入，
不为普通本地注销保存原始 ID Token。
[OIDC RP-Initiated Logout](https://openid.net/specs/openid-connect-rpinitiated-1_0.html)

## 反向代理与部署约束

1. 冷启动配置给出唯一、固定的外部 Redirect URI；它必须与 Provider 注册值精确
   一致。应用从该 URI 得到允许的 Public Host，并拒绝其他 Host。
2. 反向代理必须保留原始 Host、Origin、Sec-Fetch-Site；不得让公网直接访问后端
   Listener。Host 不匹配直接 400，不能靠 `X-Forwarded-Host` 修正。
3. TLS 即使在 Ingress 终止，应用仍无条件发 `Secure` Cookie；Redirect 和安全
   决策不根据 `r.TLS` 或不受信任的 Forwarded Header 推断。
4. 只在显式可信代理 CIDR 内使用 Forwarded Header 记录真实 Client IP；它不参与
   OIDC Redirect、Cookie 或授权判断。
5. Proxy/应用日志不记录 Query String 与 Cookie；callback 完成后立刻重定向到
   无 Code/State 的页面。Ingress 同样要对 `/auth/callback` 做 Query Redaction。
6. 通过真实 Ingress 做一条集成测试，证明 Host/Origin/Fetch Metadata 未被删除或
   改写，并证明注册的 Redirect URI 在外部视角完全一致。

## 实施验收矩阵

Happy Path 至少证明：Viewer/Admin 分别登录；PKCE S256；Casdoor UserInfo Role
映射；登录前后 Session ID 改变；Idle/Absolute 过期；POST Logout 后旧 Cookie
不可重放；代理后的 Redirect URI 正确。

Unhappy Path 至少证明：State 缺失/错误/过期/重放，Nonce 错误，PKCE verifier
错误，Token Endpoint 失败，ID Token 缺失、坏签名、错误 Issuer/Audience/AZP、
过期，UserInfo `sub` 不一致，Role Claim 缺失/类型错误/未命中，Session DB 故障，
错误 Host，`cross-site` 与 `same-site` 非同源 POST，以及任何 GET 变更状态都被
拒绝。测试日志与错误响应还要断言不含 Token、Code、Cookie、State、Nonce、
Verifier、Email 或 Claim 原文。
