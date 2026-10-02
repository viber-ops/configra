# Scoped write tokens (v1.1.0)

[中文](scoped-write-tokens.zh-CN.md) · [Upgrade from v1.0.1](releases/v1.1.0.md)

An operator uses a `write-scoped` Token and a registered mTLS client certificate
on the API listener (`9443`). It never receives a human session, OIDC credentials,
a CA signing key or a Master Key. Existing Tokens migrate to `read-only`; their
expiry, Environment grants and explicitly permitted Token-only mode are retained.

## Issuance and model

1. An Administrator completes OIDC login and MFA, then opens **Administration →
   API tokens → New API token → Scoped write token** on Management (`8443`).
2. Select at least one existing Environment, optional Config key and Vault
   Namespace key allowlists, and an explicit future expiry. Resource keys are
   immutable identifiers (`server`), not Display Names or filenames (`server.yaml`).
3. Save the one-time Token securely. Issue/register the operator's client
   certificate through the existing Management certificate workflow. Give the
   operator the Token, leaf certificate/private key and HTTPS server trust roots.
4. Create a managed client CA as Administrator and give the operator its public
   ID for deployment issuance. Never distribute the CA private key to operators.

The Management `POST /v1/api-tokens` body extends the existing contract with
`kind: "write-scoped"`, `config_keys`, `namespace_keys` and required RFC3339
`expires_at`. Only an authenticated Administrator whose verified ID Token attests
MFA can issue a writer. Accepted `amr` evidence is `mfa`, or both `pwd` and `otp`.
Missing/malformed evidence fails with `403 mfa_required`; UserInfo, role claims
and request headers cannot attest MFA. Existing sessions need a fresh login after
the upgrade. Configure the identity provider to issue the signed `amr` claim;
there is no switch to bypass this check, including in the local demo.

Tokens retain the opaque `cfg_<public-id>_<secret>` format. Only the Secret digest
is stored. Scope metadata is returned in the existing administrator-only token
inventory, and issuance/revocation is audited. Writer and deployment Environment
grants are immutable; rotate credentials to change them. Existing standalone
read-only Tokens retain their editable Environment grants.

Writer expiry is mandatory and capped at 90 days. To lower the limit, set this in
Management's cold-start YAML and restart Management:

```yaml
write_tokens:
  max_ttl_days: 14 # 1..90; defaults to 90
```

Omitted/null Config or Namespace allowlists allow all keys inside the granted
Environments; explicit `[]` denies all in that dimension. The UI's blank allowlist
means omitted. Lists contain at most 256 unique resource keys. `never_expires`
and `allow_without_mtls` cannot be enabled for a writer. Server TLS validation,
certificate registration, expiry and CA/leaf revocation checks always apply.

## API contract and boundaries

Every mutation needs a caller-generated `Idempotency-Key` (8–128 ASCII letters,
digits, `_` or `-`). Reuse it only with identical content. Config/Vault writes use
`expected_revision`: `0` creates, the current revision updates, and stale values
return `409 revision_conflict`. Success/no-change replies contain `outcome` and
`revision`; changed content retains immutable history. Errors contain only a safe
code and Request ID, never request/response values.

For a Vault Item already used in another Environment, `expected_revision: 0`
also creates the selected Environment's missing Variant. Supply every Field value
with `PUT /vault-items/{namespace}/{item}`; existing Variants still require the
current global Item revision, and outside-scope values remain unreadable.

| Method and path under `/v1/environments/{environment}` | Body / result |
| --- | --- |
| `GET /configs/{config}` | Existing resolved Config read |
| `GET /configs/{config}/raw` | Writer-only current raw Config, with `revision` |
| `PUT /configs/{config}` | `name`, `format` (`yaml`/`json`), `content`, `expected_revision` |
| `GET /vault-items/{namespace}/{item}` | Writer-only current Item with only this Environment's Variant |
| `PUT /vault-items/{namespace}/{item}` | `display_name` (optional), `fields`, `values`, `expected_revision` |
| `PUT /vault-items/{namespace}/{item}/fields/{field}` | `name`, `type`, `value`, `expected_revision` |
| `DELETE /vault-items/{namespace}/{item}/fields/{field}` | JSON `expected_revision`; removes the shared Field definition |
| `DELETE /vault-items/{namespace}/{item}` | JSON `expected_revision`; archives the Item, retaining identity/history |
| `GET /vault-items/{namespace}/{item}/fields/{field}/content` | Existing File bytes read |
| `POST /deployment-credentials` | `display_name`, managed `authority_id`, explicit `expires_at` |
| `POST /deployment-credentials/{public_id}/revoke` | No body; revokes both the Token and its certificate |

Vault `fields` entries use `{key, name, type}`. `values` maps each Field key to
`{"text":"…"}` for Text/Secret or `{"file":{"filename":"…","content_type":"…",
"bytes":"<base64>"}}` for File. File metadata and bytes are encrypted with the
snapshot. A Field's type is immutable; File size is capped at 5 MiB, Text/Secret at
512 KiB, encrypted snapshot plaintext at 10 MiB, and JSON request bodies at 32 MiB.

Value updates isolate the selected Environment, splitting a shared Variant when
necessary. Item Display Names and Field definitions are shared: changing them,
adding/removing Fields or deleting an Item requires grants for **every** bound
Environment. An unbound Variant also blocks these global changes. Other values
are retained; a newly added shared Field takes the supplied value in all authorized
Variants. Use separate Items when schemas must evolve independently. Deleting the
last Field is rejected; archive the Item instead. Archive does not reuse its key
or expose an unarchive operation to writers.

Environment, Config and Namespace checks reject outside-scope access with `403`.
Config reads also check every Vault-reference Namespace **before** resolution or
conditional `304`, and Config writes reject outside-scope references. The API
does not mount Environment/user/role administration, writer issuance, CA
administration, Master Key export, resource listing or global management reads.
Expired/revoked Tokens and missing/revoked/mismatched certificates return `401`.
The API database account needs runtime mutation privileges in v1.1.0; only
Management performs DDL migrations. Scope checks remain server-side, including
a Token row lock held through write commit/replay to serialize revocation.

## Deployment credentials, audit and rotation

Deployment issuance atomically creates a **read-only**, certificate-bound Token
for one permitted Environment and a client certificate from an active managed CA.
Config/Namespace restrictions are inherited and cannot be expanded. Both expire
no later than the writer or CA. The response contains `token.token` and
`certificate.export_bundle` (base64 ZIP: `client.crt`, `client.key`, public `ca.crt`)
once; replay returns metadata only. If delivery is lost, revoke that public ID and
issue a new pair. The writer can revoke pairs it issued; standalone credentials
and another writer's pairs remain administrator-managed. Revoking the writer in
Management also atomically revokes its deployment Tokens and certificates. Its
own operator certificate remains separately administrator-managed.

Successful, no-change, conflict and authenticated rejected mutation attempts enter
the existing transactional Audit Outbox. Management's Audit page/search shows
`actor_type=token`, public Token ID, action, Environment, Namespace/Item/Field or
Config identity, timestamp, revision/outcome and TCP-peer `source_ip`. Forwarding
headers are not trusted. Accepted-operation replays keep the original Audit ID
and IP. Bodies, Config text, secret values, File names/bytes, Token Secrets and
private key exports never enter audit or logs. A rejection fails closed with
`503 audit_unavailable` if its receipt cannot be persisted. ClickHouse outages
leave committed audit receipts in MySQL for retry.

Prefer short writer lifetimes (for example 14 days), separate testing/production
writers and named per-host deployment pairs. Rotate by issuing replacement
credentials, deploying and testing them, then revoking old credentials. Account
for issuer revocation cascading to deployed hosts: replace children before
retiring their writer. Distribute one-time exports through a private channel and
store Token/key files with mode `0600`; keep the write identity only on operators.

The [Go SDK operator example](https://github.com/viber-ops/configra-go/tree/feat/scoped-write-api/examples/operator)
uploads `server.yaml` and 17 File fields, then saves a read-only deployment pair.
`make test-integration` runs it over real HTTPS/mTLS with MySQL 8.0.22, NATS and
ClickHouse and verifies scopes, history, revocation, audit and secret redaction.
