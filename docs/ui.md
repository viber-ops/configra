# Management UI

The console helps operators find a resource, inspect its state and make a
controlled change. Its list/detail layout takes cues from desktop credential
managers, while retaining Configra's own Environment-based model.

## Layout and interaction

- Use a compact navigation column, searchable resource list and focused detail
  pane. Show the readable name with the immutable key, without repeating badges.
- Keep resource actions beside the resource they change. Protect dirty edits and
  retain user input after conflicts; provide the request identifier for diagnosis.
- Conceal sensitive values until explicitly requested. Changing resource identity
  must require another reveal; long fields and Environment lists need bounded views.
- Preserve keyboard navigation, visible focus, reduced-motion support and useful
  empty/error states. Narrow screens show one focused pane with a way back.
- English and Chinese share the same functionality. Light and dark themes keep
  the same hierarchy and preserve contrast in editors, diffs and credential views.

The main palette is white paper, `#f3f5f9` canvas, `#162238` ink, `#142a4a`
navigation and `#126ef2` action blue. Use system UI fonts, with monospace for keys
and source. Existing tokens live in [styles.css](../web/src/styles.css) and
[workspace.css](../web/src/workspace.css); extend them instead of adding competing
per-page themes. Typical headings are 24–28 px, controls 13–14 px, with 8 px
spacing steps and 10–12 px panel corners.

## Pages and primary states

Confirmation prompts and transient loading/error messages are component states,
not separate routes. The browser regression suite is in [web/tests/](../web/tests/).

| Route | Primary states |
| --- | --- |
| `/` | Signed-out login, language/theme choice, SSO and license links |
| `#/overview` | Observed health, configuration gaps, recent changes |
| `#/environments` | Inventory and creation |
| `#/environments/:environment` | Environment details and linked Config/Vault filters |
| `#/configs` | Inventory and creation |
| `#/configs/:config` | Environment contexts for one Config identity |
| `#/configs/:config/:environment` | Current source, history, historical inspection, explicit resolved preview, comparison picker/diff, merge/replace preview, clone and not-found states |
| `#/vault` | Item inventory and creation |
| `#/vault/:namespace/:item` | Field metadata, explicit reveal, history, structured editing and archived states |
| `#/notifications` | Destination inventory/creation and delivery history |
| `#/access`, `#/audit` | Access and Audit records, filters, request/operation correlation |
| `#/administration` | Tokens, grants, CA/client certificates, notification settings and deployment status |
| Admin-only route as Viewer | Forbidden state without administrator-only requests |

The API/storage invariants remain in the [design](design.md), [terminology](../CONTEXT.md)
and [production acceptance matrix](production-readiness.md). This page describes
presentation and navigation, not additional permission boundaries.

## Revision history / 历史版本

Open a Config or Vault Item and select **History**. Each page shows up to 50
versions. Use **Next** for older versions and **Previous** to go back; the recent
version buttons above the tabs stay available. A failed page shows a retry button
and its Request ID. Restoring an old version creates a new current version and
returns the history to page one. It never rewrites the old version.

Config comparison and merge/replace use the same paging controls in each
Environment column. Changing pages does not change your selected Source or
Target. Check the Environment and version shown in those two slots before
committing a transfer.

进入 Config 或 Vault Item，打开“历史版本”。每页最多显示 50 个版本，点击“下一页”
查看更早的版本，“上一页”返回。顶部的近期版本按钮不随翻页变化。加载失败时可以
重试，并复制 Request ID 排查。恢复旧版本会生成一个新版本，历史列表回到第一页，
不会覆盖旧版本。Config 对比、合并和替换的两侧可以分别翻页；已经选中的源版本和
目标版本不会被翻页替换，提交前请核对两边显示的环境和版本号。

The console uses these **Management** endpoints, authenticated by the human
OIDC session, not by a machine API Token:

```text
GET /v1/environments/{environment}/configs/{config}/revisions?limit=50
GET /v1/vault-items/{namespace}/{item}/revisions?limit=50
```

Responses contain `items`, ordered by descending revision. If `next_before` is
present, pass that value as `before` on the next request, keeping the same resource
and limit. `before` is exclusive. For example, `?limit=50&before=957` returns only
versions below 957. Omit `before` to return to the newest page. There is no
automatic full-history fetch or total-count query.

`limit` defaults to 50 and accepts 1–100. Unknown, duplicate, malformed or empty
query parameters return `400 invalid_request`. A known resource past the end of
its history returns `200` with `items: []`; an unknown resource returns `404`.
Archived resources retain readable history. Concurrent new versions do not
change the older pages selected by an existing cursor. Resource inventories use
the separate offset-based contract below.

这些是人类用户 OIDC 会话使用的 Management 接口，不是机器 Token 接口。响应的
`items` 按版本号倒序排列；有 `next_before` 时，将它作为下一次请求的 `before`，
没有则表示已到末尾。`before` 不包含该版本本身。`limit` 默认 50，范围为 1–100。
无效或重复参数返回 400；已存在资源翻到末尾返回空列表，资源不存在返回 404。
资源归档后仍可查询历史。资源列表使用下文单独说明的 offset 分页约定。

This behavior starts in v1.0.0; rc.2 returned full revision
history. Custom Management clients must follow `next_before` when upgrading.
The machine read API and Go SDK contract are unchanged.
此行为从 v1.0.0 开始，rc.2 返回完整历史。自编 Management 客户端升级时需要
处理 `next_before`；机器读取接口和 Go SDK 的用法没有变化。

## Resource lists and grants / 资源列表与授权

Environment, Config, Vault Item, API Token, CA, client certificate and notification
destination lists show 50 entries per page.
Search runs on the server, so it also finds entries beyond the current page.
Environment selectors have their own search and page controls. Switching pages
does not clear a selection or a pending grant edit. Overview counts come from
the server, not from the number of rows loaded into the browser.

Config, Vault and Token rows show at most three associated environments and the
number of additional associations. Open a Config to page through all its
environments. Vault details retain the complete revision snapshot. For a Token,
open **Edit environments** to inspect and change grants. Only environments you
check or uncheck are changed on save; grants on other pages stay untouched.
New Vault variants start with no environment selected: choose where the values
should be used instead of relying on an automatically selected first environment.

环境、Config、Vault Item、API Token、CA、客户端证书和通知目标列表每页显示 50 条。搜索会查服务端，不限于
当前页。选择环境时也可以搜索、翻页；翻页不会清空已经选中的环境或待保存的授权。
概览中的数量是服务端返回的总数，不是浏览器当前加载的条数。

列表每行最多展示 3 个关联环境，其余显示数量。打开 Config 后可以分页查看全部环境；
Vault 详情仍展示该版本的完整快照。修改 Token 时，点击“编辑环境”，勾选需要增加的
环境、取消需要移除的环境，再保存。没有操作过的环境授权不会被覆盖。新建 Vault
Variant 不会自动绑定第一个环境，需要明确选择值应当用于哪些环境。

These are Management-session endpoints. Config, Vault and Environment metadata
are available to Viewer and Admin; Token lists, grant membership, CA, certificate
and notification destination lists require Admin.

```text
GET /v1/environments?limit=50&offset=0
GET /v1/configs?limit=50&offset=0&q=payment
GET /v1/vault-items?limit=50&namespace=platform&environment=production
GET /v1/api-tokens?limit=50&offset=0
GET /v1/environments?config=payment&limit=50&offset=50
GET /v1/environments?token=<public_id>&include_archived=true&limit=50
GET /v1/certificate-authorities?include_revoked=true&limit=50
GET /v1/certificate-authorities?usable=true&limit=50
GET /v1/client-certificates?include_revoked=true&q=payments&limit=50
GET /v1/notification-destinations?include_archived=true&limit=50
GET /v1/notification-destinations/operations/event-types?limit=50&offset=50
GET /v1/vault-items/platform/db/usages?limit=50&offset=50
```

These endpoints return `{ "items": [...], "total": 123 }`. `limit` defaults to 50,
accepts 1–100, and `offset` is a non-negative integer. Increase `offset` by
`limit` to continue. No request automatically collects the other pages. Results
use key order (Vault: namespace then key; Token/CA/certificate: newest creation,
then public ID/CA ID/fingerprint; usages: field, environment, then Config key).
They are live inventories, not snapshot exports: concurrent creates, archives or
renames may change the count or page membership. Refresh before making a decision
based on a complete inventory. Counts and substring searches can scan matching
records; bounded response size is not a constant-time query guarantee.

- `q` is a case-insensitive literal substring, at most 256 bytes. `%` and `_`
  are not wildcards. Config search also matches bound environment keys; Vault
  search includes `namespace.key`. It never searches secret values or source.
- `key` is exact (Token: public ID; CA: ID; certificate: SHA-256 fingerprint);
  combine it with `namespace` for Vault identity.
- Default lists exclude archived/revoked rows. Use `include_archived=true` or
  `include_revoked=true`, or `status=active|all|archived` (`revoked` for credentials).
  Do not combine `status` with an `include_*` flag.
- Config supports exact `environment` and `unbound=true` (no active environment).
  Vault supports exact `namespace` and `environment` filters. Environment lists
  support `config`, paired `namespace` + `item`, and Admin-only `token`.
- Config/Vault/Token summaries expose `environment_count` with a maximum of three
  `environments`/`environment_keys`. Configs filtered by environment also expose
  that environment's current `revision`. Scoped Environment rows include the
  Config `revision` or Token `granted` flag; absent `granted` means false.
- Malformed numbers, unsupported or repeated parameters return `400`.
  Empty searches/optional filters are allowed. A page past the end returns an
  empty `items` array and the matching total, not `404`.

列表返回 `items` 和匹配条件的 `total`。`limit` 默认 50、最多 100，`offset` 从 0
开始，每次增加 `limit`。这是实时列表，不是导出快照；有人同时新增、归档或修改资源时，
总数和分页内容可能变化。搜索不读取配置正文或 Secret 值，`%` 和 `_` 按普通字符处理。
Config/Vault/Token 的关联数组只是最多 3 个环境的摘要，不能当成完整授权或绑定集合。
证书中的 `authority_name` 是服务端返回的颁发机构名称；不用先下载全部 CA 历史来匹配。
`status=active` 只排除吊销记录，不代表证书尚未过期。签发时使用 `usable=true`，它会同时
排除已吊销、已过期和尚未生效的 CA。服务端最多允许 32 个未吊销且未过期的管理 CA。
管理页面的分页不会截断服务端 TLS 信任集合。

CA `usable=true` selects unrevoked, currently valid issuers, not merely
`status=active` records. At most 32 unexpired, unrevoked managed CAs are admitted,
so the default page contains all usable issuers. TLS trust loading remains a
separate, complete read. Certificate rows include `authority_name` even when the
issuer is on another history page or is revoked/expired.

Notification rows contain at most three `event_types` and their full
`event_type_count`. Open the additional-count button to page through the remaining
subscriptions. The `/event-types` endpoint returns string items; it also accepts
`q`. Archived destinations retain readable subscription metadata. It does not
return webhook URLs or signing secrets. Neither this endpoint nor `/usages`
accepts `key`, `status` or `include_*` filters. A missing parent returns `404`;
an existing parent with no matching rows returns an empty page with `total: 0`.

通知目标只预览 3 个订阅类型，`event_type_count` 是完整数量。点击剩余数量可以继续
翻页查看；归档后仍可查看订阅，但不会返回 Webhook 地址或签名密钥。Vault 的“当前引用”
也可以搜索、翻页，只展示未归档 Config 在未归档环境中的当前版本引用。

### Vault edit impact / Vault 修改影响

Before saving an existing Vault item, the UI sends only the proposed field and
environment keys, never their values:

```http
POST /v1/vault-items/platform/db/impact-preview?limit=50&offset=0
Content-Type: application/json

{"fields":["host","password"],"environments":["production"]}
```

This Admin-only read returns `{ "items": [...], "total": 123 }`. A reference is
affected if its field or environment is absent from the proposed sets. The count
covers all current references, including those on pages the user has not opened.
Both arrays are required, may be empty, and must contain distinct valid keys.
Only `limit` and `offset` are accepted; a search must not hide part of the impact.
Count and preview rows are read from one database snapshot. No operation ID is
needed, and this read does not create a mutation audit.

The UI checks again on save and asks for confirmation when references would be
left unresolved. A failed or malformed preview blocks saving, keeps the draft,
and offers retry; server errors include a copyable Request ID. Changing the draft
clears confirmation. This is a live warning, not a transactional lock: other
users can change Config references after the check. Direct `PUT`, restore and
archive requests do not require this preview or its confirmation.

修改已有 Vault Item 时，页面会把准备保留的字段、环境 key 发给服务端，计算完整影响
数量，不会拿当前这一页引用来判断安全。查询失败时保留草稿、暂停保存，可以重试；
服务端错误会显示可复制的 Request ID。删除仍被引用的字段或环境绑定，需要再次确认。
修改草稿会清除之前的确认。这个提示不会锁住其他人的配置操作；检查之后引用仍可能
变化。直接调用 `PUT`、恢复版本或归档接口，不强制执行这一步检查。

### Token grant edits / Token 授权修改

The UI saves grant edits with an incremental request:

```http
PATCH /v1/api-tokens/<public_id>/environments
Content-Type: application/json
Idempotency-Key: <unique operation ID>

{"add":["staging"],"remove":["development"]}
```

Use the same operation ID and body when retrying an ambiguous response. The reply
contains `outcome` and `public_id`, not the full grant set. Changes are atomic and
audited; adding an existing grant or removing an absent grant is a no-op. An
environment cannot appear twice or in both arrays. Additions require active
environments; removals can remove archived environments. Revoked Tokens cannot
be edited. The existing `PUT` still replaces **all** grants and must receive the
complete intended set, never the three-item summary or one page of checkboxes.

`PATCH` 只修改 `add`、`remove` 中明确列出的环境；重试不确定是否成功的请求时，沿用
原来的操作 ID 和请求内容。添加已存在的授权、移除已不存在的授权都不会重复产生变更。
不能重复填写同一个环境，也不能同时添加和移除它。已归档环境不能新增授权，但可以
移除旧授权；已撤销 Token 不能修改。旧 `PUT` 仍是整组覆盖，必须提供完整目标集合，
不能把一页数据或列表摘要直接拿去覆盖。

This behavior starts in v1.0.0. rc.2 returned complete inventories
and association arrays; custom Management clients must adopt pagination and scoped
association reads when upgrading. Machine reads and `configra-go` are unchanged.
此行为从 v1.0.0 开始；升级自编 Management 客户端时要同步处理分页和关联摘要。

## Screenshots

Public screenshots use synthetic data and live in the website repository. Keep
their URLs stable; do not copy another gallery into this repository:

- [Light workspace](https://viber-ops.github.io/assets/configra/vault-light.png)
- [Dark workspace](https://viber-ops.github.io/assets/configra/vault-dark.png)
- [Mobile workspace](https://viber-ops.github.io/assets/configra/vault-mobile.png)
- [Certificate authorities](https://viber-ops.github.io/assets/configra/authorities.png)

For local review, use the prerequisites in [Try the workspace](../README.md#try-the-workspace),
but run `make local-run` from the root of this checkout, not an older release tag.
Keep it running. In a second terminal in the same directory, run `make local-seed`
to create synthetic data, then `make local-screenshots` to capture the pages at
1600 × 1000. These commands use the disposable development fixture and its
OIDC/HTTPS endpoints; do not point them at production. Captures default to
`.cache/ui-screenshots/` and must not be committed. `CONFIGRA_SCREENSHOT_DIR` can
select another reviewed output location.

Earlier text-only UI plans are in the optional
[review archive](https://github.com/viber-ops/configra/releases/download/v0.1.0-rc.2/review-records-2026-09-12.tar.gz).
Old screenshot galleries are not part of the source checkout or this archive.
