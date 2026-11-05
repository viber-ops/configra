# Administrator session control / 管理员撤权

Management's **Administrator sessions** tab supports three audited actions on an
exact OIDC issuer and subject (`sub`):

- `invalidate`: invalidate existing sessions; a fresh authorized sign-in is allowed.
- `block`: invalidate sessions and deny new sign-ins for this identity locally.
- `unblock`: permit a fresh authorized sign-in; old cookies remain invalid.

Changes require an Administrator with verified OIDC MFA. Each Management replica
checks shared policy on authenticated requests and fails closed when that state
cannot be read. An absent policy preserves existing sessions during upgrade;
invalidation applies to subsequent requests, not rollback of already accepted work.

管理员在管理端核对准确的 issuer 与 sub 后提交，不能仅凭显示名称或邮箱猜测。
变更需要 MFA 并记录审计；所有副本共享失效代数。解除阻止后必须重新登录，旧 Cookie
不会恢复。数据库不可用时管理鉴权失败关闭；机器读取保持自己的 Token/mTLS 鉴权。

The HTTP contract is `GET /v1/session-policies` (Admin, bounded `limit`, `offset`,
`q`) and `POST /v1/session-policies` (Admin+MFA, `Idempotency-Key`) with:

```json
{"issuer":"https://identity.example.com","subject":"exact-idp-sub","action":"block"}
```

The response contains an opaque identity ID, generation, blocked state and outcome.
It contains no session cookie or login token. Retain the Operation ID across an
uncertain retry. A fresh login must use an ID Token issued after the latest policy
change; allow for the IdP's timestamp resolution before retrying a fresh sign-in.

## Identity-provider notifications / 身份提供方通知

For an IdP supporting OpenID Connect Back-Channel Logout, register
`https://MANAGEMENT_HOST/auth/backchannel-logout` for this Configra client. The
POST accepts a form-encoded `logout_token` and does not rely on browser cookies.
It verifies the configured issuer's signature/JWKS, audience, logout event,
absence of nonce, JTI and `sub` or `sid`. Issued-at must be within ten minutes
(at most one minute ahead); optional expiry/not-before claims are enforced.

A notice with `sid` invalidates that OP session; without `sid`, it invalidates
the subject's sessions. Successful changes are audited. JTI replay is idempotent
and cannot invalidate a later fresh login a second time. An existing local block
remains in force. Signing keys, raw notices and session cookies are never stored
in the audit record.

IdP logout support does not guarantee that its account-disable or role-change
actions emit notices. Verify those actions for the selected provider; use the
controlled block operation for offboarding/incident handling when they do not,
or when delivery could not be confirmed. Keep clocks synchronized. Protocol:
[OpenID Connect Back-Channel Logout](https://openid.net/specs/openid-connect-backchannel-1_0.html).

支持该标准的 IdP 可以发送已签名登出通知；服务端不会信任浏览器传来的用户标识
作为撤权证明。JTI 重放不会再次影响后来重新登录的会话。实际禁用/降权是否触发
回调需在所用 IdP 上验证，未触发或投递不确定时执行本地 block。

## Controlled maintenance / 受控维护

If Management or the identity provider cannot complete the incident workflow, an
authorized maintenance runner can use the existing storage-only bootstrap and
Master Key verification:

```sh
configra sessions --config /run/configra/maintenance.yaml \
  --issuer https://identity.example.com --subject exact-idp-sub \
  --action block --operation-id incident-20261003-user-block --timeout 30s
```

This is the server maintenance executable, not the scoped `configractl` client.
It does not initialize/migrate the database or grant roles. The account needs
SELECT on schema/Sentinel/session policy, INSERT/UPDATE on session policy and
operations, and INSERT on outbox_events. Keep this access outside ordinary operator
hosts; the infrastructure execution audit identifies the human maintenance runner.
The Configra receipt uses actor `system / maintenance-session-control`.

Before offboarding, disable or downgrade the identity at the IdP **and** invalidate
or block its Configra sessions. IdP role changes do not necessarily emit logout
events. For a compromised administrator, also review recent credential issuance
in Audit and rotate/revoke affected machine credentials and CAs. Human session
invalidation deliberately does not silently revoke institutional deployment tokens.

离职与应急流程同时处理 IdP 和 Configra 的现有会话。被盗管理员可能已签发机器
身份，需要从审计盘点并单独撤销；不要误以为退出浏览器能收回已经签发的部署凭据。
从旧备份恢复后也要重新核对恢复点之后的撤权/阻止记录，再开放访问。
