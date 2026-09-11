# Configra 出站 Webhook / 飞书 HTTP 投递选型

调研日期：2026-08-27。范围是 Generic Webhook 与飞书自定义机器人投递，包括 SSRF、重定向、重试、签名和响应判定；不修改实现或依赖。

## 结论

V1 不新增 HTTP、SSRF、Retry 或飞书 SDK。使用 Go 标准库，建立一个 Configra 专用且全局复用的 `http.Client`，并把一次投递尝试保持为一次 HTTP 调用：

| 能力 | 选择 |
| --- | --- |
| URL 解析 | `net/url`，只接受结构明确的 HTTPS URL |
| DNS rebinding / SSRF | `net.Dialer.ControlContext` 在实际连接地址上执行策略 |
| HTTP | 专用 `net/http.Transport`，`Proxy: nil`，有界 timeout/header/body |
| 重定向 | `CheckRedirect` 返回 `http.ErrUseLastResponse` |
| Retry-After | `strconv.ParseUint` + `http.ParseTime` 的小型本地解析器 |
| Generic 签名 | `crypto/hmac` + `crypto/sha256` + `encoding/hex` |
| 飞书签名与响应 | 按飞书官方协议写两个很小的 provider 函数 |
| 重试调度 | 现有 MySQL Outbox 的 `next_attempt_at`，不在 HTTP Client 内 sleep |

这不是为了少依赖而缩减安全边界。恰恰相反，候选库都不能直接表达 Configra 已确定的“所有公网默认可达，加少量冷配置 internal Host/CIDR 例外”，引入后仍要绕开或重写其核心策略。标准库方案只保留产品真正需要的策略，并要求完整的安全回归测试。

[ADR-0021](../adr/0021-model-outbound-notification-destinations.md) 已将 Generic Webhook 与飞书建模为两种 Destination；[ADR-0022](../adr/0022-deliver-notifications-from-a-transactional-outbox.md) 已确定 public HTTPS、禁重定向、DNS 后拦截内网地址、最多八次尝试以及持久化 Outbox。本说明补足这两份 ADR 的实现边界。

## SSRF：检查真正要连接的 IP

管理员可填写任意 Generic Webhook URL，这正是 OWASP 列出的 SSRF 场景。OWASP 同时建议禁用重定向，并对域名的 A 与 AAAA 结果实施公网地址判断；它也明确指出 DNS pinning/rebinding 会绕过只在输入阶段做的域名检查。[OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html#case-2-application-can-send-requests-to-any-external-ip-address-or-domain-name)

因此以下流程是错误的：

```text
Lookup(host) -> 校验结果 -> http.Client 再按 host 发请求
```

最后一步会再次解析 DNS，攻击者可让第一次返回公网 IP、第二次返回内网 IP。保存 Destination 时的 DNS 检查最多是 UX 检查，不能作为投递时的安全判定。

V1 使用一个专用 `Transport.DialContext`。每次调用复制基础 `net.Dialer`，在副本上安装一个闭包式 `ControlContext`：闭包记住原始 hostname，callback 则解析 Go 已解析出的实际 remote IP/port，执行地址策略后才允许 `connect`。Go 1.25.1 文档保证 Control hook 位于创建 socket 之后、真正 dial 之前；同版本源码显示传入 callback 的地址来自已经解析的 `raddr.String()`。[net.Dialer documentation](https://pkg.go.dev/net@go1.25.1#Dialer) [Go 1.25.1 socket source](https://github.com/golang/go/blob/go1.25.1/src/net/sock_posix.go#L92-L105)

伪代码只需要这一层结构：

```go
func dialContext(ctx context.Context, network, address string) (net.Conn, error) {
    originalHost, originalPort := splitAndNormalize(address)

    d := baseDialer // per-call copy: do not mutate a shared Dialer
    d.ControlContext = func(ctx context.Context, actualNetwork, actualAddress string, _ syscall.RawConn) error {
        actual := mustParseAddrPort(actualAddress)
        return policy.Check(originalHost, originalPort, actualNetwork, actual.Addr().Unmap(), actual.Port())
    }
    return d.DialContext(ctx, network, address)
}
```

这避免了 validate-then-resolve 的 TOCTOU，也保留标准库对多地址、IPv4/IPv6 fallback 和连接超时的处理。DNS 每次重试可以变化，但每个新连接都会在 socket 边界重新检查；已复用的 keep-alive 连接已经固定到先前获准的 peer，不会因 DNS 变化而改变目标。若同一域名同时返回允许和禁止地址，禁止地址绝不会建立连接，标准库仍可尝试另一个允许地址；为“发现任意禁止记录就让整个域名失败”另写 resolve-and-race 没有增加 SSRF 防护，V1 不做。

### Transport 的不可省略项

- `Proxy` 必须显式为 `nil`。标准库说明 `nil` 表示不使用代理；若继承 `http.DefaultTransport` 的 `ProxyFromEnvironment`，Dial hook 看到的是代理地址而不是最终目标，目标 IP 策略就失去作用。[http.Transport.Proxy](https://pkg.go.dev/net/http@go1.25.1#Transport) 如果以后必须使用统一 egress proxy，应把 SSRF 策略迁到该可信代理，这是另一种部署架构。
- `DialTLSContext` 和旧的 `DialTLS` 保持 `nil`，使 HTTPS 继续经过受保护的 `DialContext`；标准库会再使用 `TLSClientConfig` 完成 TLS。[http.Transport.DialTLSContext](https://pkg.go.dev/net/http@go1.25.1#Transport)
- 不设置 `InsecureSkipVerify`。冷启动 YAML 的全局 CA bundle 加入 `TLSClientConfig.RootCAs`；不能在单个 Destination 上跳过校验。
- 配置 `Dialer.Timeout`、`TLSHandshakeTimeout`、`ResponseHeaderTimeout`、`Client.Timeout`、`MaxResponseHeaderBytes`、连接池上限与 idle timeout。Go 文档提供这些独立边界，并要求复用并发安全的 Client/Transport。[net/http client and transport](https://pkg.go.dev/net/http@go1.25.1#hdr-Clients_and_Transports)
- 建议 `DisableCompression: true`，并用 `io.LimitReader(max+1)` 将响应 body 限制为一个很小的固定值，例如 64 KiB；随后总是关闭 body。Configra 不需要接收大响应。
- 不配置 Cookie Jar，不注册其他 URL protocol，不手动覆盖 `Request.Host`。

### URL 与地址策略

Generic URL 在保存、测试和每次实际发送前都做相同的结构校验：绝对 URL、scheme 严格为 `https`、host 非空、V1 默认只允许 443；拒绝 userinfo、fragment、opaque URL、IPv6 zone 和非数字 port。V1 hostname 只接受 ASCII DNS 名称或已转成 ASCII 的 `xn--` 名称，并统一为小写、移除一个尾随点；这样 internal Host 例外可以做精确字符串匹配，不引入 Unicode 同形字或自写 IDNA 逻辑。出现真实的非 443 需求后，再用冷启动配置增加显式 port allowlist。

飞书 Destination 更窄：host 必须精确等于 `open.feishu.cn`，port 为 443，path 必须以 `/open-apis/bot/v2/hook/` 开头且 token 非空；拒绝 query、fragment、userinfo。不能让“飞书类型”退化为可填写任意 URL 的第二个 Generic Webhook。官方给出的 webhook 格式就是该 HTTPS endpoint，并提醒完整 URL 必须保密。[飞书自定义机器人使用指南](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)

实际 remote address 使用 `net/netip` 解析，先拒绝 zone，再调用 `Unmap`，避免 `::ffff:127.0.0.1` 等 IPv4-mapped IPv6 绕过 IPv4 规则。不能只判断 `Addr.IsGlobalUnicast()`：Go 官方文档明确说明该方法对 RFC 1918 IPv4 与 IPv6 ULA 仍会返回 true。[netip.Addr.IsGlobalUnicast](https://pkg.go.dev/net/netip@go1.25.1#Addr.IsGlobalUnicast)

地址策略应同时使用 `IsPrivate`、`IsLoopback`、`IsLinkLocalUnicast`、`IsMulticast`、`IsUnspecified` 和一份带来源日期的静态 `netip.Prefix` 表，默认拒绝 IANA IPv4/IPv6 special-purpose ranges，包括 CGNAT、documentation、benchmark、transition/NAT64 等标准库基础谓词没有完整覆盖的空间。[IANA IPv4 Special-Purpose Registry](https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml) [IANA IPv6 Special-Purpose Registry](https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry.xhtml)

冷配置的 exact internal Host 或 CIDR 是唯一例外，并在实际 IP 检查时应用；不能从 Management UI 给单个 Destination 放开。建议 loopback、unspecified、multicast、link-local/metadata 始终 hard-deny，冷配置只放开明确的 RFC 1918、ULA、CGNAT 或企业网段，且启动时拒绝与 hard-deny 相交的 CIDR。Host 规则只精确匹配，不支持 wildcard、suffix 或 regex。

## 重定向：一跳也不跟

```go
CheckRedirect: func(*http.Request, []*http.Request) error {
    return http.ErrUseLastResponse
}
```

`ErrUseLastResponse` 会阻止下一跳，同时把当前响应及未关闭 body 返回给调用者，便于统一做有界读取和状态分类。[http.ErrUseLastResponse](https://pkg.go.dev/net/http@go1.25.1#ErrUseLastResponse) 所有 3xx 都是终态失败，不因同 host、同 scheme 或 307/308 而例外。这样 Generic secret header、飞书 token 和请求 body 不可能被 redirect 转发到第二个目标。

## Retry-After 与持久化重试

[RFC 9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110.html#name-retry-after) 定义 `Retry-After` 为非负十进制秒或 HTTP-date；[RFC 6585 §4](https://datatracker.ietf.org/doc/html/rfc6585#section-4) 允许 429 携带该 header。最小且完整的 parser 规则是：

1. 只接受恰好一个、trim 后非空的 header value；多值视为无效。
2. 先用 `strconv.ParseUint` 解析十进制秒；先做 overflow 检查，再转 `time.Duration`。
3. 否则用 `http.ParseTime` 解析日期。该标准库函数覆盖 HTTP 允许的三种日期格式，而不是只支持 RFC1123。[http.ParseTime](https://pkg.go.dev/net/http@go1.25.1#ParseTime)
4. 过去的日期返回 0；无效值退回本地 backoff。
5. Header 来自不可信服务端，必须限制为配置好的最大 retry delay，不能让一个超大值把该 Delivery 无限延期。

下一次时间为 `now + max(full-jitter exponential backoff, capped Retry-After)`。网络错误、timeout、429 和 5xx 可重试；其他 4xx 与所有 3xx 立即 Dead；总 attempt 数是 8（包含第一次）。一次 POST 可能已经被接收、但响应在途中丢失，因此重试天然可能重复；稳定的 Delivery ID 让接收端可以去重，这与 ADR 的 at-least-once 语义一致。

重试不应包在 `http.Client.Do` 外层循环并 sleep。每次 Outbox worker claim 只执行一次 HTTP 调用，失败后持久化 attempt、safe error class 与 `next_attempt_at`，释放 worker；进程崩溃或重启不会遗失计划。每次 attempt 重新构造 request 和时间相关签名，但保持 Delivery ID 及事件内容不变。

## Generic Webhook HMAC 合约

Webhook HMAC 没有统一行业 wire format，所以 Configra 必须先固定一个带版本的合约。建议延续设计文档中的四个 header：

```text
X-Configra-Timestamp: <Unix seconds>
X-Configra-Delivery: <stable delivery ID>
X-Configra-Event: <event type>
X-Configra-Signature: v1=<lowercase hex HMAC-SHA256>
```

MAC 输入精确定义为以下字节，最后一段是实际发送的原始 JSON body：

```text
v1\n<timestamp>\n<delivery-id>\n<event-type>\n<raw-body>
```

实现必须先把 body marshal 一次，再对同一 `[]byte` 签名和发送，不能 marshal 两次后假定字节相同。Secret 按管理员录入值的 UTF-8 原始字节使用，不自动猜测或 Base64 解码。每次 attempt 使用新的 timestamp 和 signature，Delivery ID 保持不变。

以下固定向量可直接锁定 wire contract：

```text
secret    = test-secret
timestamp = 1700000000
delivery  = del_01
event     = vault.updated
body      = {"revision":21,"vault":"redis"}
signature = v1=5686b164694544f1dfe884eeb0c401b9593efb86096e4504cda40455bb2d036f
```

标准库 `hmac.New(sha256.New, secret)` 已覆盖生成；接收方先解析 `v1=` 与 hex，再用 `hmac.Equal` 做常量时间比较，并检查一个较小的 timestamp 容差及按 Delivery ID 去重。Go 官方明确要求用 `hmac.Equal` 避免比较时序泄露。[crypto/hmac](https://pkg.go.dev/crypto/hmac@go1.25.1)

Generic signer 不能复用飞书 signer；两者算法不同。

## 飞书协议

飞书官方自定义机器人协议规定：

- request body 不能超过 20 KiB；单租户单机器人限制为 100 次/分钟、5 次/秒，拥塞时可出现 `11232` 限流错误。
- 开启签名后，`timestamp` 是秒级 Unix 时间且与当前时间相差不超过 3600 秒。
- `stringToSign = timestamp + "\n" + secret`，把该字符串作为 HMAC-SHA256 的 **key**，message 是空字节；结果做 Base64。请求 JSON 增加字符串字段 `timestamp` 与 `sign`。
- 成功 JSON 的权威字段是 `code: 0`；`StatusCode` 与 `StatusMessage` 是历史兼容冗余字段。
- `19021`、`19022`、`19024` 分别表示签名/时间、IP 白名单、关键词配置失败。

以上均来自[飞书自定义机器人使用指南](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)。由官方示例 `secret=demo`、`timestamp=1599360473` 独立计算出的回归向量是：

```text
l1N0gAcBjdwBvGm1xMjOF0XSyaLRpR7tuO5dHfhAYc8=
```

每次 retry 必须重新生成 timestamp 和 sign，否则延迟重试可能使用过期签名。一次飞书投递仅在 HTTP 为 2xx、有限响应 body 是合法 JSON 且 `code == 0` 时成功；`11232` 按限流重试，已知的 `19021/19022/19024` 是配置错误并立即 Dead。未知 non-zero provider code 默认终态失败，等官方文档确认某个 code 可重试后再加入小型 allowlist。

官方 `larksuite/oapi-sdk-go` 是面向飞书 OpenAPI、token 管理、事件和完整类型系统的 SDK，而自定义机器人这里只是一次带特定 JSON/HMAC 的 webhook POST；为两个小函数引入整套 SDK 没有收益。[official Feishu OpenPlatform Go SDK](https://github.com/larksuite/oapi-sdk-go)

## 敏感信息与错误

飞书 token 位于 URL path，Generic 凭证也可能位于 path/query，因此不能直接记录 `http.Client.Do` 返回的 error。Go 的 `*url.Error` 结构包含完整 URL，`Error()` 也会把它格式化进字符串。[Go 1.25.1 net/url source](https://github.com/golang/go/blob/go1.25.1/src/net/url/url.go#L26-L34)

日志和 delivery record 只保留 Destination ID、provider、safe host、attempt、latency、HTTP status、provider code 与归一化错误类别。不能记录 full URL、Location、request/response body、HMAC header、Secret 或原始 `url.Error`。错误链仍可在内存中用 `errors.Is/As` 分类，但写日志前必须转换成不含 URL 的自有 safe error。

## 候选库比较

| 选项 | 优点 | 与 Configra 的差距 | 决策 |
| --- | --- | --- | --- |
| Go 标准库 | 已有；socket seam、TLS、redirect、HTTP-date 与 HMAC 都齐全 | 需要维护一份 IANA special-use prefix 快照及测试 | **采用** |
| `doyensec/safeurl` v0.2.5 | 活跃维护；在 Dialer Control 检查实际地址；当前版已补齐近期 IPv6 ranges | `AllowedIPs/CIDR` 一旦设置就变成排他 allowlist，不能表达“全部公网 + 少量 internal 例外”；还需自行清 Proxy、禁 redirect、持久化 retry，并引入 `miekg/dns` 等依赖 | 不采用 |
| `coder/safedial` v0.2.0 | 功能上最接近：resolve/pin、connect-time backstop、special-use、CIDR 例外、清 proxy | 仓库 2026-08-20 才创建，当前 release 2026-08-25 发布，尚无足够生产历史 | 暂观察 |
| `mccutchen/safedialer` v0.1.0 | 很小、无依赖、使用 socket Control | 仅固定 public IP 与 80/443，不支持 Configra internal 例外；最后版本发布于 2024-02 | 不采用 |
| `hashicorp/go-retryablehttp` v0.7.8 | 成熟，支持 429/5xx、Retry-After 与重签 hook | 重试在进程内 sleep，不符合 durable Outbox；默认 logger 会保留 URL path，飞书 token 会泄露；其默认 HTTP-date parser 只试 RFC1123，且仍需大量自定义策略 | 不采用 |

`safeurl` 的实际连接检查与排他 allowlist 可见其[官方 v0.2.5 source](https://github.com/doyensec/safeurl/blob/v0.2.5/client.go#L33-L90)，依赖见[官方 go.mod](https://github.com/doyensec/safeurl/blob/v0.2.5/go.mod)。它在 v0.2.4 修复了遗漏四段新 IPv6 special-use ranges 的中危 SSRF advisory；v0.2.5 已修复，但这个案例也说明无论自有代码还是依赖都必须持续跟进 IANA registry。[GHSA-xgch-x3mx-cm3c](https://github.com/doyensec/safeurl/security/advisories/GHSA-xgch-x3mx-cm3c)

`coder/safedial` 的完整目标见[官方 README](https://github.com/coder/safedial) 与[v0.2.0 release](https://github.com/coder/safedial/releases/tag/v0.2.0)；等它有稳定 API 和一段真实采用历史后，可以用同一安全回归集重新评估。`mccutchen/safedialer` 的固定策略见[官方 package docs](https://pkg.go.dev/github.com/mccutchen/safedialer)。

`go-retryablehttp` 的官方源码显示 retry loop 使用 timer sleep，默认 logger 格式化 URL，而 `redactURL` 只遮 userinfo password、不遮 path/query；其 Retry-After 日期解析也只调用 `time.RFC1123`。[retry loop and logging](https://github.com/hashicorp/go-retryablehttp/blob/v0.7.8/client.go#L715-L790) [URL redaction](https://github.com/hashicorp/go-retryablehttp/blob/v0.7.8/client.go#L926-L938) [Retry-After parser](https://github.com/hashicorp/go-retryablehttp/blob/v0.7.8/client.go#L548-L600) 这些都能通过大量定制绕开，但到那时比 Outbox 上的本地状态转换更复杂。

## Go 1.25.1 的生产风险

上述 API 都存在于 Go 1.25.1，因此代码可以保留 1.25.1 的源码兼容性；但不能再用 1.25.1 工具链构建生产镜像。Go 官方只支持最近两个 major；Go 1.27.0 已于 2026-08-19 发布，所以 1.25 已退出支持。1.25.1 之后的 patch 已修复多项 `crypto/tls`、`crypto/x509`、`net/http`、`net/url` 与 `net` 安全问题；最终 1.25 patch 是 1.25.14。[Go release policy and history](https://go.dev/doc/devel/release)

最小迁移是：源码暂可保留 `go 1.25.1` 作为最低语言版本，但 CI/Docker builder 明确固定到仍受支持的 Go 1.26.7；团队准备好后升 1.27.x。若 major 升级暂时受阻，1.25.14 只能作为紧急短期补丁，不是受支持的长期选择。由于标准库被编译进 Go binary，关键的是生产构建所用的 toolchain，而不是开发机恰好安装了哪个版本。

## 必须锁定的安全回归

1. URL：非 HTTPS、userinfo、fragment、opaque、zone、异常 port、Feishu 非官方 host/path 全部拒绝。
2. 地址：IPv4/IPv6 private、loopback、link-local/metadata、CGNAT、ULA、multicast、unspecified、documentation、benchmark、NAT64/transition 与 IPv4-mapped IPv6 全部覆盖。
3. DNS：保存时公网但连接时变成私网仍在 `ControlContext` 被拒；internal exact Host/CIDR 只按冷配置放开；设置 `HTTPS_PROXY` 环境变量也不能绕过 `Proxy: nil`。
4. Redirect：301/302/303/307/308 的第二个 server 都收不到请求，响应 body 仍被有界读取和关闭。
5. Retry-After：秒、三种 HTTP-date、过去日期、负数、overflow、多个值、malformed 与上限；实际结果写入 Outbox `next_attempt_at`，不 sleep。
6. Generic HMAC：固定向量；body 的一个 byte、timestamp、Delivery ID 或 event type 变化都会验证失败；比较使用 `hmac.Equal`。
7. 飞书：上述官方向量、20 KiB 边界、2xx + `code:0` 才成功、`11232` 重试、`19021/19022/19024` Dead、每次 retry 重签。
8. 保密：网络失败、TLS 失败、redirect 与 provider error 的日志/数据库都不含 URL path/query、Secret、签名或 body。
