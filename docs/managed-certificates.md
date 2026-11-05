# Managed client certificates

Administration now manages both certificate authorities and client certificates.
The CA is for Configra machine-client mTLS; it does not replace the independent
HTTPS certificate used by the Management or API listener.

## Find and copy identifiers

Open **Administration → Certificate authorities**. Each card shows the full
**CA ID** and **Fingerprint SHA-256**; click either value to copy it. CA ID is the
identifier used by `configractl deployment-credential issue --authority` and the
API `authority_id` field; the fingerprint
identifies the public certificate. Neither requires opening an API URL.
Client-certificate rows likewise expose a copyable fingerprint and issuer CA ID.
Copy success/failure is explicit; if the browser denies clipboard access, select
the visible full value and copy it manually.

管理端“证书颁发机构”卡片直接显示完整 CA ID 和 SHA-256 指纹，点击即可复制，并显示
成功或失败反馈。客户端证书列表也可复制指纹及所属 CA ID。签发时会预览到期日期和
有效天数；CA 更早到期时明确提示实际有效期会被缩短。

## Lifecycle

Create a CA, save its initial backup ZIP, then issue a client credential. The CA
defaults to five years (maximum ten); client certificates default to 90 days
(maximum one year) and cannot outlive their CA. Keys use ECDSA P-256 and PKCS#8.
Issued leaves have client-auth usage and cannot issue certificates themselves.

CA private keys are retained encrypted with the deployment Master Key, with
authenticated data binding the key to the CA identity and certificate. Issued
client private keys are never stored. Creation returns a ZIP only after its
transaction commits; operation replay contains metadata without private export
material. Concurrent requests to different Management replicas with the same
operation key produce one credential and at most one private response.

Save the ZIP before closing the export dialog. A lost initial response cannot be
recovered as another private-key download: revoke and replace the credential if
the client key was not saved. Public CA certificates remain downloadable. Keeping
a CA private-key backup is optional for online issuance, because Configra retains
the encrypted signing key; it is still useful for the operator's recovery policy.

Individual client revocation is permanent. CA revocation also revokes its issued
clients, including imported certificates signed with the exported managed CA key.
API request authorization checks certificate/issuer activity and validity on each
read, so an existing TLS connection does not bypass revocation. It cannot erase
configuration already delivered to clients.

## Kubernetes and recovery

State lives in MySQL, not Pod memory or local files. All replicas must use the same
external Master Key. Each API replica refreshes its immutable public CA trust pool
every five seconds; a new CA may need that interval to propagate. Refresh failure
retains the last verified pool, while authorization still fails closed when its
database checks fail. External client CA files can be configured in addition to
managed CAs.

Schema v2 is additive and preserves existing records. Management verifies the
Crypto Sentinel before migration under the database migration lock. Upgrade
Management before API replicas. Back up the database and Master Key together;
losing the Master Key makes the retained CA signing keys unusable. Existing
backup/restore tooling includes the new tables.

Authority creation, client issuance, revocation, and persisted validation failures
write value-free audit metadata. Private keys and exports are excluded from
operation replay and audit payloads. OIDC Admin access and cross-origin protection
guard the management endpoints.
