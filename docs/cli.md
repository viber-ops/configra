# configractl / 部署运维 CLI

`configractl` is the Go client executable, built with Cobra and `configra-go`.
It uses the machine API over HTTPS and the same scoped Token/mTLS checks as the
SDK. `configra` remains the service and database-maintenance executable.

这是独立 Go 客户端，复用 Cobra 命令解析和 SDK；部署机无需 Go 编译器、MySQL
账号或 Master Key。写令牌仍由管理端管理员经 OIDC+MFA 签发。

## Install and connect / 安装与连接

The new release bundle contains `configractl` beside `configra`. From a source
checkout, run `go build -o ./dist/configractl ./cmd/configractl` after creating
`dist/`. Verify `configractl --version` and use `--help` on any command.

Provide `CONFIGRA_URL`, `CONFIGRA_ENVIRONMENT`, `CONFIGRA_TOKEN_FILE`,
`CONFIGRA_CLIENT_CERT`, `CONFIGRA_CLIENT_KEY` and optional `CONFIGRA_SERVER_CA`,
or save file paths in a local context:

```sh
configractl context set testing --server https://configra.example.com \
  --environment testing --token-file /run/secrets/operator-token \
  --client-cert /run/secrets/operator.crt --client-key /run/secrets/operator.key \
  --server-ca /run/secrets/server-ca.crt
configractl context use testing
configractl context show
```

`--context-file` selects an explicit contexts file. A selected context supplies
the complete connection profile and ignores ambient connection variables;
explicit command flags override it. Without a selected context, environment
variables supply defaults. Relative paths saved by `context set` are converted
to absolute paths. Context files contain paths, never Token/key contents, and
are written atomically with mode 0600. `--timeout` bounds request/validator work
and stdin waiting; filesystem completion follows normal operating-system I/O
semantics.

已选 context 时，以其整套连接配置为准；显式 flag 可覆盖，避免混入其他环境的
环境变量。未选 context 时才使用环境变量。原始 Token 不接受命令行参数。

## Resource operations / 资源操作

Commands below assume a selected context. Replace resource keys, files,
Operation IDs and revisions with those of the intended deployment.

```sh
configractl config get server --raw --output ./server.download.yaml --json
configractl config put server --file ./server.yaml --format yaml \
  --expected-revision 12 --operation-id deploy-20261003-server --json
configractl vault item get deployment secrets --output ./secrets.download.json
configractl vault item put deployment secrets --file ./item.json \
  --expected-revision 8 --operation-id deploy-20261003-secrets --json
configractl vault file put deployment secrets signing_key --file ./signing.key \
  --expected-revision 9 --operation-id deploy-20261003-file --json
configractl vault file get deployment secrets signing_key --output ./signing.download.key
configractl vault field put deployment database password --type secret --file - \
  --expected-revision 3 --operation-id rotate-20261003-password --json
configractl vault field delete deployment database obsolete \
  --expected-revision 4 --operation-id archive-20261003-field --json
configractl vault item delete deployment obsolete \
  --expected-revision 7 --operation-id archive-20261003-item --json
```

`vault item put` accepts the SDK/API item document (`display_name`, `fields`,
`values`); the explicit CLI flags supply its Operation ID and expected revision.
Use this command to submit all 17 File fields as one existing Vault Item snapshot.
The `bytes` member in a JSON File value is base64. Individual File commands read
binary files directly. Shared schema/name/deletion rules still apply.

`--expected-revision` is mandatory: use explicit zero for creation and the current
revision for updates. Reuse one `--operation-id` only with exactly the same input
and expected revision after an uncertain result. A conflict requires reviewing
the new state and choosing a new Operation ID; automatic overwrite is unsupported.

下载默认只写入不存在的 0600 文件；不会覆盖已有文件。上传通过 `--file` 或 stdin
提供内容，不把秘密放进参数。需要向 stdout 返回敏感内容时，必须同时指定
`--output - --reveal`；此时不要将 stdout 发送到 CI 日志。普通 stdout 仅输出
无值元数据，`--json` 使用紧凑 JSON；stderr 只包含固定错误码和退出码。

## Complete releases / 整套发布

Use `release prepare/state/get/activate/rollback` for one Config plus its pinned
Vault dependencies and File members. Activation/rollback require the expected
stream generation, a stable Operation ID and a local validator executable. See
the [release set workflow and manifest](release-sets.md). Independent current-value
reads continue to follow current values; select release consumption explicitly.

## Deployment credentials / 部署凭据

```sh
configractl deployment-credential issue --authority CA_ID --name host-01 \
  --expires-at 2026-10-04T00:00:00Z --operation-id host-01-20261003 \
  --output-dir ./host-01-identity --json
configractl deployment-credential revoke PUBLIC_ID \
  --operation-id revoke-host-01-20261003 --json
```

Choose an explicit expiry before both the writer and CA expire. Issuance first
reserves a new 0700 directory and a value-free `receipt.json`. It then saves
`token`, `client.crt`, `client.key`, public `ca.crt`, `client.zip` and the receipt
as 0600 files. An existing output directory is refused before issuing credentials.

The receipt retains the stable Operation ID. If the response is lost, replay with
the same inputs and a new output directory to learn the result's public identity.
One-time material is not returned again; revoke that identity and issue a new one.
If disk output fails after issuance, use the receipt/public ID for revocation;
never treat a partial directory as a usable identity.

签发与证书在服务端原子提交，但一次性私钥不能通过重放恢复。网络/磁盘失败时
保留 receipt；不确定结果使用原 Operation ID 查询重放，再撤销并换发。不要丢掉
回执后无上限重试签发。替换部署身份后再撤销旧凭据；writer 撤销会级联其子凭据。

## Verify and rotate / 验证与轮换

`configractl identity --json` returns the current Token's scopes, parent ID and
effective presented-certificate expiry. `deployment-credential list` returns only
the selected writer's pairs in the selected Environment, in pages of at most 100;
pass `--after` with `next_cursor` to continue. These endpoints require the new
server version. They never return Token secrets or private keys.

Prepare a replacement while the old pair remains usable:

```sh
configractl deployment-credential rotate --replaces OLD_PUBLIC_ID \
  --authority CA_ID --name host-01 --expires-at 2026-10-04T00:00:00Z \
  --operation-id host-01-rotation-20261003 --output-dir ./host-01-new \
  --verify-config server --json
```

`rotate` uses the issuance flow, saves the private bundle/receipt, then creates a
fresh client using that pair and performs identity plus actual Config reads. It
does not revoke the old pair or claim that a business application has reloaded.
Deploy the complete new directory/identity atomically, restart or reconstruct the
consumer's client as needed, and verify real consumer health before finishing:

```sh
configractl deployment-credential finish-rotation --directory ./host-01-new \
  --operation-id host-01-retire-20261003 --consumer-confirmed --json
```

Finishing checks the receipt's origin/Environment, public ID, certificate and a
new read again, records the revocation Operation ID, then revokes the replaced
pair. It is replayable with the same ID. `--consumer-confirmed` records the
operator/deployment system's adoption decision; it is not automatic workload
attestation. Failure leaves a receipt for recovery and does not revoke the new
pair. When replacing a writer as well, finish with the old writer's context (same
origin/Environment), or have the administrator retire the old issuer after all
children have moved. One writer cannot revoke another writer's children.

先换发并验证新凭据，旧凭据仍有效；部署系统确认真实业务采用后，再执行 finish。
不能仅凭 CLI 自己读成功就声称业务已切换。更换 writer 时，使用旧 writer 完成旧子
凭据的撤销，或在全部子凭据替换后由管理员撤销旧 writer。写身份的重新签发仍需
管理端 MFA。长寿命服务应在凭据到期前安排周期任务并接入到期告警。

## Exit codes / 退出码

| Code | Meaning |
| --- | --- |
| 0 | Completed, or help/version displayed |
| 2 | Invalid command, local input or connection settings |
| 3 | Authentication/scope denied (401/403) |
| 4 | Revision or Operation conflict (409) |
| 5 | API validation/not-found response (400/404/422) |
| 6 | Remote, transport, timeout, cancellation or overload failure; completion may be uncertain |
| 7 | Local I/O failure |
| 8 | One-time credential export unavailable/incomplete; use the receipt to revoke/reissue |

The CLI's independent `put` commands retain the existing per-resource transaction
contract. They do not claim that a sequence of Config/Vault calls is one atomic
release. Complete release-set operations are documented separately when enabled.
