# Backup and restore runbook

Configra has two independent recovery streams:

- MySQL contains all service state. A usable recovery point is its consistent
  backup plus the separately protected Cold-start Configuration and Master Key.
- ClickHouse contains retained Access/Audit logs. It is backed up independently
  and does not block recovery of the content-serving API.
- Core NATS is transient and is not backed up.

Run `make backup-test` before a release. It exercises both streams against the
pinned test containers, including corrupt/missing artifacts, existing targets,
plaintext leakage, and the Crypto Sentinel with correct and incorrect keys.

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

```sh
sh mysql-backup.sh /run/secrets/mysql.cnf configra /backup/new-point
sh mysql-restore.sh /run/secrets/mysql.cnf /backup/new-point configra_restore_20260827
```

The backup tool uses `mysqldump --single-transaction`, emits `mysql.sql.gz` and a
SHA-256 manifest atomically, refuses overwrite, and checks that both client and
server are exactly 8.0.22. Do not run schema rollout concurrently with the dump.
The restore tool verifies the manifest before creating a new database and never
overwrites an existing database.

After restore, start an isolated Configra process against the new database with
the separately restored Master Key. Promote the new DSN only after `/health/ready`
succeeds; a Crypto Sentinel failure is a failed recovery, not a new installation.

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
