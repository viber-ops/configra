# Production database identities / 生产数据库身份

Review the database name in [migration-role.sql](migration-role.sql), provision
the temporary migration account and run `configra migrate`. **After schema 4
exists**, apply [roles.sql](roles.sql) for service/maintenance accounts using a
database administrator. These scripts define roles without passwords. Provision separate accounts
with restricted connection hosts and TLS, grant exactly one appropriate role,
and set it as the account's default role. The template adds privileges; it does
not remove older excess privileges. No Configra process needs instance-wide
`ALL`, `SUPER`, `PROCESS`, `FILE` or `GRANT OPTION`.

| Role | Use |
| --- | --- |
| `configra_migrate` | Temporary offline initialization/schema migration; SELECT/INSERT/CREATE/ALTER/REFERENCES in the named database |
| `configra_management` | Running Management, including shared sessions and delivery workers; no DDL |
| `configra_api` | Content/auth reads, scoped writes and deployment credentials; no DDL, deletion, human Session reads or Environment creation |
| `configra_backup` | Consistent dump, SELECT only; no Master Key |
| `configra_doctor` | SELECT-only verification with the separately mounted matching Master Key |
| `configra_gc` | Five metadata tables only, bounded archived Outbox pruning; no Master Key/Vault access |

The API and Management roles reference the same authoritative MySQL database,
using different DSNs/Secrets. Do not send authenticated reads to an asynchronous
replica. Four service replicas can use up to 80 pooled connections; budget
additional capacity for migration, backup, recovery and database administration.

Keep the locking-read grants: CA signing uses `FOR UPDATE`; the tested MySQL
8.0.22 role-based Sentinel `FOR SHARE` also required an additional write privilege.
The template limits the latter to `UPDATE(id)` on `crypto_sentinel`, without
granting updates to its encrypted columns. Removing it made the real restricted
account acceptance fail. Service roles cannot run offline Master Key rotation;
grant that separate maintenance identity only during the controlled window.

先运行迁移，再启用常驻账号。普通 Management 重启先读取 schema 版本，已存在的
schema 不再要求 CREATE；只有初始化/升级操作需要临时 DDL 权限。不要把迁移角色
长期授予服务账号。实际 MySQL 8.0.22 测试覆盖 Config/Vault/发布集、CA 签发与吊销、
监控采样、会话撤销、GC 和 3→4 迁移，并验证禁止操作确实被数据库拒绝。

```sh
configra migrate --config /run/configra/migration.yaml \
  --confirm-database configra --timeout 10m
configra doctor --config /run/configra/doctor.yaml --verify-vault --timeout 10m
```

Both files can use the existing storage-only bootstrap (`version: 1`,
`mysql.dsn_env`, `key_provider.master_key_file`). Supply DSNs through mounted
Secrets/environment references; never pass their values as command arguments.
Migration takes the existing database lock and checks the Crypto Sentinel before
upgrading. Use the [versioned upgrade procedure](../../docs/releases/v1.2.0.md)
for the maintenance window and rollback boundary.

Restoration uses a separate account limited to a **new recovery database** and
CREATE/DROP/ALTER/INSERT/SELECT/LOCK TABLES/REFERENCES there. It must not have
production database privileges. The backup script uses `--single-transaction`
and `--no-tablespaces`, so backup itself does not need LOCK TABLES or PROCESS.
