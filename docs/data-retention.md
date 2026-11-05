# Data retention and completed delivery maintenance / 数据保留

Config/Vault Revisions, archived resource identities and Operation records remain
retained. This release does not silently truncate history or permit an old
Operation ID to execute again. Access has the existing ClickHouse 90-day TTL;
Audit remains retained. Choose log storage/backup retention and capacity budgets
for your deployment; metrics expose estimated MySQL rows and allocated bytes.

历史版本、归档资源身份和 Operation 防重放记录保持保留。分页不限制总体存储量；
先测量增长与备份时长，再选择保留期。不要直接删除 operations 或仍需恢复的版本。

## Preview and prune one completed batch

Use `configra prune-outbox` on a controlled maintenance runner. It needs no Master
Key and never starts public listeners. Its database account needs SELECT on
schema_migrations/outbox_events/notification_targets/notification_deliveries,
SELECT/INSERT/UPDATE on operations, and INSERT/DELETE on outbox_events plus DELETE
on notification_targets/notification_deliveries. It needs no access to Vault
ciphertext, Config values or OIDC sessions. Use a separate account from services.

Database-only bootstrap YAML:

```yaml
version: 1
mysql:
  dsn_env: CONFIGRA_MAINTENANCE_DSN
```

From the unpacked release directory, with the DSN supplied privately, first
preview an explicit retention cutoff (replace database name/date/path):

```sh
./configra prune-outbox --config /run/configra/metadata.yaml \
  --confirm-database configra --before 2026-07-01T00:00:00Z --limit 100
```

Without `--apply`, this returns a bounded candidate count and changes nothing.
Eligible records must be completed before the cutoff, with no pending/dead
notification targets. Pending/processing/dead Outbox Events are never candidates.

Prepare a private archive directory on durable storage, then apply:

```sh
./configra prune-outbox --config /run/configra/metadata.yaml \
  --confirm-database configra --before 2026-07-01T00:00:00Z --limit 100 \
  --archive /backup/outbox/batch-20261003-01.json \
  --operation-id outbox-20261003-01 --apply
```

The tool locks one bounded batch, durably saves its event payloads and associated
notification target/attempt metadata into a 0600 archive, then deletes that batch
and writes a maintenance Audit receipt in the same MySQL transaction. The success
receipt includes archive SHA-256; the Audit resource identifies that digest.
Archives are metadata, not an alternative backup for application state or secrets.
Copy/protect them with the existing backup/retention platform.

An archive failure rolls the database work back. Existing files are never
overwritten; an identical private regular archive can be reused after an uncertain
commit. Retry with the **same** Operation ID, cutoff, limit and archive path.
A committed operation replays its original result without another deletion or
archive. Change the Operation ID and archive path only for a new batch. A completed
empty batch also retains a replayable no-change result.

应用前先预览。每批最多 1000 个事件，默认 100；关联通知明细超过 10000 行或归档
超过 32 MiB 时拒绝并要求缩小批量。不会回收失败或待处理投递，也不会删除配置历史、
Operation 或 ClickHouse Audit。归档先持久化，数据库后提交；提交结果不确定时使用
原参数重放，不能换一个 ID 盲目重跑。已回收的成功通知明细只在归档中保留，管理端
不再提供它的在线明细或新发起的人工重投；此前已接受的 Operation 重放仍有效。

Run a fixed number of batches with a total time budget in the existing scheduler;
do not create an unbounded loop that competes with live traffic. The index added
by schema 4 supports completed_at selection, and READ COMMITTED avoids holding
unrelated insertion gaps. Preserve failed/dead queues for diagnosis and supported
redelivery. Alert on their age/size instead of treating deletion as recovery.
