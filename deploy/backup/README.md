# Backup and restore runbook

Configra has two independent recovery streams:

- MySQL contains all service state. A usable recovery point is its consistent
  backup plus the separately protected Cold-start Configuration and Master Key.
- ClickHouse contains retained Access/Audit logs. It is backed up independently
  and does not block recovery of the content-serving API.
- Core NATS is transient and is not backed up.

Run `make backup-test` before a release. It exercises both streams against the
pinned test containers, including corrupt/missing artifacts, existing targets,
plaintext leakage, the Crypto Sentinel with correct and incorrect keys, and the
read-only recovery command below. The MySQL drill also restores historical Vault
revisions, a CA signing key and notification credentials, then issues a client
certificate using the restored CA.

## MySQL 8.0.22

Run both tools with the MySQL 8.0.22 client image. Pass credentials in a mounted
`--defaults-extra-file`, never on the command line:

```ini
[client]
host=mysql.example.internal
port=3306
protocol=tcp
user=configra_backup
password=replace-through-secret-volume
```

From the repository root, with `/backup` already mounted and writable and the
matching MySQL 8.0.22 tools available:

```sh
cd deploy/backup
mkdir -m 700 /backup/new-point
sh mysql-backup.sh /run/secrets/mysql.cnf configra /backup/new-point
sh mysql-restore.sh /run/secrets/mysql.cnf /backup/new-point configra_restore_20260827
```

The backup tool uses `mysqldump --single-transaction`, emits `mysql.sql.gz` and a
SHA-256 manifest atomically, refuses overwrite, and checks that both client and
server are exactly 8.0.22. Do not run schema rollout concurrently with the dump.
The restore tool verifies the manifest before creating a new database and never
overwrites an existing database.

The output directory must already exist and contain no previous backup files.
Use a new directory and restore database name for each drill; do not remove a
recovery point merely to make a repeated command succeed. These exact-version
scripts are not a cross-version MySQL upgrade mechanism.

For host bind mounts, run a backup/restore helper container as the directory
owner (Docker `--user UID:GID`). The scripts deliberately create owner-only
files; a root-run helper leaves root-owned backups on native Linux. Do not make
backups world-readable to work around an ownership mismatch.

中文说明：用容器向宿主机目录备份时，让辅助容器以该目录所有者的 UID/GID
运行。备份文件只允许所有者读取；不要为了让普通用户能读取 root 生成的文件，
把备份改成所有人可读。

Do not promote a restored database merely because `/health/ready` succeeds.
Readiness checks connectivity and the active Crypto Sentinel, not every
historical encrypted record. Complete the following check, then start an
isolated service and exercise actual reads.

## Encrypted-state verification / 检查加密数据

**Source candidate only:** `doctor` is not in the published server rc.2 binary.
Use a binary built from this checkout until a new release includes the command.
From the repository root, with the supported Go toolchain installed:

```sh
mkdir -p .cache
go build -o .cache/configra ./cmd/configra
```

Save the following as `/run/configra/doctor.yaml`, adjusting the Master Key path.
Provide `CONFIGRA_RESTORE_DSN` through your secret manager or a Kubernetes Secret;
it must name the **new restored database**, not the live database. Use a MySQL
account with `SELECT` on that database. Do not give this check write/DDL privileges
or put the DSN/password in command arguments or YAML.

```yaml
version: 1
mysql:
  dsn_env: CONFIGRA_RESTORE_DSN
key_provider:
  master_key_file: /run/secrets/restored-master-key
```

Then, from the repository root:

```sh
./.cache/configra doctor --config /run/configra/doctor.yaml --verify-vault --timeout 10m
```

Exit code `0` and one JSON line mean that the complete scan succeeded. For the
automated drill's two Vault revisions, one CA and one destination, the output is:

```json
{"vault_revisions":2,"certificate_authorities":1,"notification_destinations":1}
```

The command checks the schema and Crypto Sentinel, decrypts **all** Vault revisions
(including archived items), verifies each CA certificate/private-key pair
(including expired or revoked CAs), and validates all notification credentials
(including disabled/archived destinations). It uses one read-only database
snapshot and processes one record at a time. It does not initialize, migrate,
repair, rotate keys, make HTTP requests, or start listeners and background workers.
Server YAML also works; only its MySQL and Master Key settings are required.

On wrong/missing keys, corrupt records, insufficient permissions, cancellation or
timeout, the command exits nonzero **without a success report**. Data errors name
only the table, internal ID and revision, not decrypted values. Stop the recovery
and investigate; do not delete the Sentinel or initialize the restored database.
Increase `--timeout` only when the database size needs it. Prefer an isolated
restore target: a long snapshot against a busy primary can delay undo cleanup.

Counts are not proof of completeness: missing rows cannot be decrypted or counted.
Compare the recovery point with your recorded inventory and required recovery
time. The dump's checksum does not authenticate its origin. Neither this check nor
readiness proves that unencrypted metadata, grants, clients, OIDC or notification
delivery are correct. Before changing the live DSN, use the isolated service to
read a known resolved Config and File, exercise required authorization, issue a
test client from a restored active CA, and test an explicitly approved notification
destination. Keep test delivery away from real incident channels.

中文说明：这个命令检查恢复库，不替你恢复或修改数据。先从当前源码构建，
再用只读数据库账号、恢复库的 DSN 和单独保管的主密钥执行；已发布的 rc.2
还没有此命令。不需要准备 OIDC、TLS、NATS 或 ClickHouse 才能检查。
成功时输出三类记录数量；失败、取消或超时返回非零退出码，不输出成功结果。
历史版本、归档项、过期或撤销的 CA、停用通知凭证都在检查范围内。

输出成功仍不等于恢复完成：检查不出备份前已丢失的记录，也不验证所有权限和
外部依赖。核对恢复点后，先启动隔离服务，实际读取配置和文件、验证权限、
签发测试证书并向指定测试地址发送通知，再决定是否切换生产 DSN。
不要因为 `/health/ready` 正常就直接切换。主密钥轮换需要下面的维护流程；
不要直接替换 Kubernetes Secret 中的主密钥，这会使旧数据无法解密。

## Offline Master Key rotation

This command is also **source-candidate only**, not part of rc.2. It changes the
database key wrapping, not application passwords, CA signing keys, client
certificates or Tokens. If the old key may have been stolen together with a
database copy, treat the exposed values and CA keys as compromised: rewrapping
does not undo that exposure. Rotate those credentials separately.

Before the maintenance window:

1. Use the reviewed binary for the maintenance command and all service replicas.
   Rehearse on an isolated restored database with the same amount of history.
2. Verify a fresh database backup with `doctor`. Back up the old and replacement
   Master Keys separately from the database, under separate access controls.
   Keep the old key for recovery points still encrypted with it.
3. Prepare a **new**, unused key-file path. Generate one base64-encoded 32-byte
   random key using your secret manager; for a local maintenance runner,
   `openssl rand -base64 32` produces the required format. Save it privately; do
   not paste it into an issue, terminal recording, command argument or Configra.
4. Confirm the target DSN/database name. The maintenance account needs SELECT on
   `schema_migrations`, SELECT and UPDATE on the four encrypted tables
   (`crypto_sentinel`, `vault_item_revisions`, `certificate_authorities`,
   `notification_destinations`),
   SELECT/INSERT/UPDATE on `operations`, and INSERT on `outbox_events`. The six
   tables must be visible InnoDB tables. No DDL privilege is needed.

Stop every Management and API replica and disable HPA/GitOps/jobs that would
restart them. Record replica counts before scaling to zero. Check that the Pods
or processes have actually exited. Do not run schema changes or other database
writers during rotation. Current-code stale writers are fenced, but older
releases do not have that guard; `--confirm-offline` is your acknowledgment, not
automatic cluster shutdown or proof that every writer stopped.

On a maintenance runner with database access, the reviewed binary, current
bootstrap YAML, old key file and separately mounted new key file, run:

```sh
./.cache/configra rotate-master-key \
  --config /run/configra/doctor.yaml \
  --new-key-file /run/secrets/new-master-key \
  --confirm-database configra_restore_20260827 \
  --confirm-offline --timeout 30m
```

Replace `configra_restore_20260827` with the **exact** database name in the DSN.
The example deliberately targets an isolated restore. The config still names
the old Master Key. The command refuses a matching old/new key, a mismatched
database name, nontransactional tables and corrupt encrypted records. It does
not overwrite either key file, edit bootstrap YAML or update a Kubernetes Secret.
It processes one encrypted record at a time, but the transaction covers the
whole database history; allow for its lock/undo cost in your maintenance window.

Exit 0 returns an Operation ID and the three encrypted-record counts. Its
`master_key.rotated` Audit event is stored in the same transaction, with actor
`system / offline-master-key-rotation`; the operator's identity belongs in your
infrastructure execution audit, not a fabricated OIDC identity. After success:

1. Make a copy of the maintenance YAML that points to the replacement key and
   run `doctor --verify-vault` against the same database. Compare the counts and
   read required Configs/Files using an isolated service. Do not restart live
   replicas on a failed check.
2. Update the externally managed Master Key Secret/file through your normal
   secret-management workflow, then restore the recorded replica counts using
   the reviewed image. Restart **all** replicas; changing a mounted file alone
   does not update the in-memory key.
3. Check readiness, a real authorized Config/File read and the rotation Audit
   event after the worker resumes. Take a new verified backup. Only then restore
   the autoscaler/GitOps reconciliation and normal traffic policy.

Errors before COMMIT leave the old wrapping intact. A COMMIT connection error or
a terminated CLI can leave the outcome **unknown**, even when MySQL committed.
Keep replicas stopped and run doctor with each key on the same target. If only
the new key succeeds, the rotation committed: follow the new-key steps. If only
the old key succeeds, investigate the error before retrying. If neither succeeds,
do not guess or initialize the database; investigate the recovery point. A
success-report write error explicitly says `COMMITTED` and needs new-key checks.

After a successful rotation, merely restoring the old Secret is **not** a rollback.
To return to the old key, run the same offline procedure in reverse (current/new
key in the config, previous key in `--new-key-file`), verify it, then restart.
Restoring the earlier database **and** matching key is a separate disaster-recovery
choice that can discard writes made since that backup. No rollback is automatic.

中文说明：主密钥换钥需要维护窗口，不能只修改 Kubernetes Secret。先在恢复库
演练，分别备份数据库、新旧密钥；停掉全部 Management/API 实例及自动拉起它们
的任务后，再执行 `rotate-master-key`。`--confirm-database` 必须与 DSN 中的库名
一致，`--confirm-offline` 表示你已确认停机和备份，不会替你停服务。

命令不改原始密钥文件、不替换应用密码或 CA 私钥，也不改配置版本和 ETag。
成功后先用新密钥执行 doctor，核对数量和实际读取结果，再通过原有密钥管理流程
更新 Secret、重启所有实例。保留旧密钥供旧备份恢复使用。提交时断线或进程被杀
不等于回滚；分别用新旧密钥检查，确定数据库使用哪一把之后再操作。成功后的
回退需要反向执行完整换钥，不能直接把旧 Secret 放回去。

## ClickHouse logs

Configure an allowed backup disk or object-storage destination as described by
the [ClickHouse native backup documentation](https://clickhouse.com/docs/concepts/features/backup-restore/local-disk).
The local test configuration is `deploy/clickhouse/backup_disk.xml`; production
should normally point the named disk at durable object storage.

Provide `clickhouse-client` credentials through a mounted config file, then run:

```sh
sh clickhouse-backup.sh /run/secrets/client.xml configra backups configra-20260827.zip
sh clickhouse-restore.sh /run/secrets/client.xml configra backups configra-20260827.zip configra_restore_20260827
```

The restore wrapper first creates a new target it can safely identify as its own;
on a normal restore failure it removes only that target. Never point queries at a
restored database until row-count and retention checks pass.

## Production controls outside this repository

Set an explicit RPO, RTO, retention period, and restore-drill schedule for each
deployment. Store artifacts on encrypted, access-controlled, immutable/off-site
storage; the SHA-256 manifest detects accidental corruption but is not an
authenticity signature. Never place the Master Key in the same backup artifact or
storage credential scope. A killed restore job can leave an isolated new target;
the next drill must detect and remove it before retrying.
