# Configra 生产运维部署契约

实现更新：已交付的 [角色模板与受限账号验收](../../deploy/mysql/README.md)、[HA overlay](../../deploy/kubernetes/production/README.md) 和 [备份调度](../../deploy/backup/README.md#scheduled-remote-backups--定时异地备份) 为当前使用入口。下文保留实现前的调查；其中 Management 每次启动需要 CREATE 的限制已消除，新增 `configra migrate`。实际 MySQL 8.0.22 角色测试确认 Sentinel 锁读还需要窄化的 `UPDATE(id)` 权限，不能直接套用下文 SELECT-only 推论。发布表 S/I/U、schema 3→4 及服务角色禁止 DDL/删除已经通过真实账号验收。

日期：2026-10-03；范围：`feat/operations-cli` 工作树、尚未发布的 schema 4。本文只读核对实现并给部署建议；没有执行授权、备份、清理或生产操作。
官方页面通过 HTTPS 读取；web 工具接口不可用。权限表来自实际 SQL，仍须在 MySQL **8.0.22** 用受限账号完成验收，不能把源码推导写成已通过的权限测试。

## 1. MySQL 账号、权限与 DSN

所有授权限定到 Configra 的指定数据库/表和实际连接来源，不给 `SUPER`、`PROCESS`、`FILE`、`GRANT OPTION` 或 `*.*`。下文 S/I/U/D 分别表示 `SELECT/INSERT/UPDATE/DELETE`。

| 身份 | 当前实现需要的权限与边界 |
| --- | --- |
| Management | 业务表 S；按下述写表范围授予 I/U/D；每次启动仍须对 `schema_migrations` 有 `CREATE`。初始化/迁移窗口另授指定库的 `CREATE, ALTER, REFERENCES`，以及迁移记录/初始 Sentinel 的 I。 |
| API，含 scoped-write | 下述内容/鉴权/通知元数据的 S，以及确切写表的 I/U；不需要 DELETE、DDL、人工 Session 表或 GC 权限。API 仅验证 schema，不能代替 Management 迁移。 |
| MySQL backup | 当前全部为 InnoDB 普通表，脚本最低为目标库 S；若引入 view/trigger，分别补 `SHOW VIEW/TRIGGER` 并复核备份完整性。无 Master Key。 |
| MySQL restore | 只对预先确定的**新恢复库名**授 `CREATE, DROP, ALTER, INSERT, SELECT, LOCK TABLES, REFERENCES`；无生产库权限。当前 dump 默认含 DROP/LOCK/索引控制语句，恢复失败清理还会 DROP 自建库。 |
| doctor | 恢复库 S，加单独挂载的匹配 Master Key；没有写入或迁移权限。 |
| prune preview | `schema_migrations, outbox_events, notification_targets` 的 S；不要求 Master Key。 |
| prune apply | `schema_migrations`: S；`operations`: S/I/U；`outbox_events`: S/I/D；`notification_targets, notification_deliveries`: S/D。无配置、Vault、CA、Session 表权限，无 Master Key。 |

Management 的启动限制来自 [`initializeOrVerify`](../../internal/storage/mysqlstore/bootstrap.go#L83)：即使已是 schema 4，也执行 `CREATE TABLE IF NOT EXISTS schema_migrations`；[schema 4](../../internal/storage/mysqlstore/schema_operations.go#L9) 新建 Session policy 表并 ALTER Outbox 索引。当前没有独立 migrate-only 入口，不能声称常驻 Management 已完全移除 DDL 权限。稳定 schema 可收回临时 ALTER/REFERENCES 和其他表 CREATE，保留 `schema_migrations` 的 CREATE；新版本升级前再受控授回。
外键创建要求父表 REFERENCES；ALTER TABLE 还要求 CREATE/INSERT。不要因缺少这些权限给整个实例 ALL PRIVILEGES。[MySQL 权限](https://docs.oracle.com/cd/E17952_01/mysql-8.0-en/privileges-provided.html)、[ALTER TABLE](https://docs.oracle.com/cd/E17952_01/mysql-8.0-en/alter-table.html)

API 表级起点（以[内容提交](../../internal/storage/mysqlstore/config_mutation.go#L81)、[Vault 提交](../../internal/storage/mysqlstore/vault_mutation.go#L227)、[凭据签发](../../internal/storage/mysqlstore/deployment_credential.go#L29)和[监控采样](../../internal/storage/mysqlstore/operational.go#L37)为依据）：
- S：`schema_migrations, crypto_sentinel, environments, configs, config_env_states, config_revisions, config_revision_vault_refs, vault_items, vault_fields, vault_item_revisions, vault_revision_fields, vault_revision_variants, vault_revision_variant_environments, api_tokens, api_token_environments, client_certificates, certificate_authorities, operations, outbox_events, notification_destinations, notification_subscriptions`。
- I：`configs, config_env_states, config_revisions, config_revision_vault_refs, vault_items, vault_fields, vault_item_revisions, vault_revision_fields, vault_revision_variants, vault_revision_variant_environments, api_tokens, api_token_environments, client_certificates, operations, outbox_events, notification_targets`。
- U：`configs, config_env_states, vault_items, vault_fields, api_tokens, client_certificates, operations, certificate_authorities`；其中 configs/CA 的锁读也会要求额外权限，不能仅按显式 UPDATE 语句生成 grants。
- 并行实现计划还会加入 `config_release_sets/config_release_states` 的 S/I/U；本次读取时文件尚未落盘，须在合并后的 schema 4 上复核，不能把计划当作已测试能力。

Management 在上述 API 写权限上增加 I：`schema_migrations, crypto_sentinel, environments, certificate_authorities, notification_destinations, notification_subscriptions, notification_deliveries, management_sessions, management_session_policies`；增加 U：`environments, notification_destinations, notification_targets, outbox_events, management_sessions, management_session_policies`；D 仅需 `api_token_environments, notification_subscriptions, management_sessions`。人工撤权通过[共享 generation/block policy](../../internal/storage/mysqlstore/session_policy.go#L70)生效，不是直接删所有 Session。
**锁读约束：** [CA 签发查询](../../internal/storage/mysqlstore/authority.go#L400)仍使用 FOR UPDATE，需要 S 加 U/D/LOCK TABLES 之一；可选 CA 表级 U，不应为此给 API 全库 LOCK TABLES。MySQL 8.0.22 的 FOR SHARE 则只需 S，Sentinel 和发布集读取不应因此额外扩大写权限。[MySQL locking reads](https://docs.oracle.com/cd/E17952_01/mysql-8.0-en/innodb-locking-reads.html)
`information_schema.tables` 的可见范围随账号权限变化；[OperationalStats](../../internal/storage/mysqlstore/operational.go#L93)返回行数估算和已分配字节，不代表全库/磁盘可用容量。由 Management 指标提供完整业务表概览，不为了 API 的容量图给它人工 Session 读取权限。

拆分 [base 共享 DSN](../../deploy/kubernetes/base/api.yaml#L51)：保留进程内变量名 `CONFIGRA_MYSQL_DSN`，将 API 的 secretKeyRef 指向独立 `configra-api-db/mysql-dsn`，Management 指向 `configra-management-db/mysql-dsn`。两账号仍连接同一权威数据库，不能把 API 随意指向异步只读副本。
备份账号只挂载 `mysql.cnf`；恢复、doctor、GC 各用独立临时身份与 Secret，不挂到常驻 Pod。GC 使用 database-only YAML 和 `CONFIGRA_GC_DSN`，不用服务完整 bootstrap。所有 DSN/密码均经 Secret/env/配置文件分发，不进命令行或仓库。
两 API 加两 Management 的默认连接池上限合计 80，再给备份/运维及数据库管理预留连接；见 [Store pool](../../internal/storage/mysqlstore/bootstrap.go#L46)。实际连接预算与 MySQL 故障切换地址由数据库负责人确认。

## 2. 最小 HA overlay

| 项目 | 最小可执行配置 |
| --- | --- |
| 副本 | 固定同一镜像 digest 的 API 2、Management 2；共用数据库、Master Key、OIDC 配置。现有 [base](../../deploy/kubernetes/base/kustomization.yaml) 各 1 副本，overlay 显式修改。 |
| 拓扑 | 每组件各自匹配 label；`topologySpreadConstraints` 在 hostname 及真实 zone 上 `maxSkew: 1`、`whenUnsatisfiable: DoNotSchedule`，支持时设 `minDomains: 2`。部署前确认至少两个真实合格故障域和剩余容量；不以两枚 kind worker 代替物理隔离。 |
| 中断预算 | 每组件一个 `policy/v1` PDB，2 副本时 `minAvailable: 1`；API 保留 RollingUpdate 的 `maxUnavailable: 0/maxSurge: 1`，为 surge 留容量。PDB 约束驱逐，不保证节点故障，也不限制 Deployment 的滚动更新/直接删除。 |
| Management 升级 | 保留 Recreate 和维护窗口约定；相同版本可双副本运行，跨 schema/静态资源版本升级不能靠改成 RollingUpdate 就保证兼容。先暂停写入并按版本运行手册迁移，再启动匹配 schema 的 API；旧 API 对新 schema 会拒绝启动。 |
| TLS 与退出 | API 入口保留 TCP/TLS pass-through；复用现有 5 秒 preStop 与 30 秒终止预算，在实际入口测传播/排空时间后调整。Secret 更新后受控重启，再检查新 TLS 连接实际提供的证书。 |
| metrics | 在两份 bootstrap 中设 `observability.listen: 0.0.0.0:9090`，Pod 增加命名端口，仅私网 ClusterIP/PodMonitor 抓取；不同 Pod 可同端口。不开公网 Ingress/NodePort/LoadBalancer。 |

拓扑约束可能在故障域不足时使替代 Pod Pending，值班手册应写清剩余副本承载能力和扩容动作。[Topology spread](https://kubernetes.io/docs/concepts/workloads/pods/pod-topology-spread-constraints/)、[PDB 限制](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/)
[metrics listener](../../internal/observability/metrics.go#L140)没有 OIDC/mTLS：使用支持 NetworkPolicy 的 CNI，默认拒绝后，仅允许指定监控 namespace **且** Prometheus Pod selector 访问 9090；同一 `from` 条目组合两个 selector，避免误写成 OR。另外显式保留业务入口端口及必需依赖流量。[NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
复用[现有规则](../../deploy/monitoring/alerts.yaml)与独立告警通道；运维端口故障不应暴露到公众。多副本不会消除 MySQL、OIDC、入口的单点，HA/PITR 产品选型和真实跨主机/可用区演练由运维认领。

## 3. 定时备份、归档与新鲜度

[MySQL 脚本](../../deploy/backup/mysql-backup.sh#L67)固定客户端/服务端 8.0.22，使用 single-transaction、no-tablespaces、GTID OFF；备份端无需 LOCK TABLES/PROCESS。运行期间禁止 schema DDL，并确保所有业务表为 InnoDB；[官方 mysqldump 约束](https://docs.oracle.com/cd/E17952_01/mysql-8.0-en/mysqldump.html)不能因“事务备份”而省略。
复用脚本的 CronJob 需要带 `sh/mysql/mysqldump/gzip/sha256sum` 的受控辅助镜像；服务 scratch 镜像不能直接跑 shell 脚本。按 RPO 选择频率，用固定 UTC 时区、`concurrencyPolicy: Forbid`、明确 `startingDeadlineSeconds`、`activeDeadlineSeconds`、`restartPolicy: Never` 和受控 `backoffLimit`，保留失败 Job。
CronJob 可能漏调度或重复创建，Forbid 只约束同一个 CronJob；不会阻止另一任务或 Management 执行 DDL。每次执行使用唯一、0700 输出目录，禁止覆盖旧恢复点；重试先核验既有产物，再使用新执行目录，不能为重试删除有效备份。[CronJob 官方限制](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)

每次任务执行现有命令 `sh deploy/backup/mysql-backup.sh /run/secrets/mysql.cnf configra /backup/唯一目录`，再上传 `mysql.sql.gz + manifest.sha256` 到独立故障域的加密存储，核对远端完整性后发布成功标记；不把临时 PVC 写成功当异地备份完成。
Master Key/冷配置保存在独立权限域；记录恢复点匹配的 key version/image/schema，保留仍被旧恢复点需要的旧 key。备份凭据不具有 Master Key 读取权。ClickHouse 用[现有脚本](../../deploy/backup/clickhouse-backup.sh)，其客户端/服务端固定 26.7.3.19；与 MySQL 独立调度、保留、验证，Core NATS 无备份流。

任务 wrapper 向已有批任务监控入口持久写出以下**新增部署指标**，当前 Configra metrics 不自带这些值：
- `configra_backup_last_success_timestamp_seconds{store}`：仅远端完整性核验成功后更新；失败保留旧值。
- `configra_backup_recovery_point_timestamp_seconds{store}`：成功产物的实际恢复点；无法精确取得 snapshot 时间时用 dump 开始时间作保守值，不用上传结束时间虚增新鲜度。
- `configra_backup_duration_seconds{store}`、最近执行成功/失败；`configra_restore_last_success_timestamp_seconds{store}` 与实际恢复耗时由恢复演练任务更新。
按 `time() - recovery_point > RPO`、指标缺失、任务失败/超时和恢复验证过期告警；持久保存最后成功值，使短命 Job 退出或 CronJob 永不启动时仍能告警。沿用现有 Pushgateway/批任务状态 exporter，避免另建 Configra 内置调度器。[Prometheus batch jobs](https://prometheus.io/docs/practices/instrumentation/#batch-jobs)

GC 单独安排在已验证备份之后；[Prune](../../internal/storage/mysqlstore/prune.go#L58)默认 preview，apply 每批 1–1000 条，先耐久归档再删除 completed 且所有通知目标 succeeded 的记录，保留 Operations、全部 Revision 与 pending/failed 数据。
使用 `configra prune-outbox --config /run/configra/gc.yaml --confirm-database configra --before 固定RFC3339时间 --limit 100` 先预览；apply 额外传固定 `--operation-id`、`--archive` 和 `--apply`。提交结果不确定时用完全相同参数重放，不生成新 OperationID。
[归档实现](../../internal/app/prune.go#L70)要求私有目录、0600 文件及支持 hard-link/fsync 的文件系统；必须挂载耐久卷，不用 emptyDir。随后复制到外部保留存储；若本地卷损失也不可接受，应先选择跨故障域耐久归档盘，当前回调不会等待远端对象存储确认。

## 4. 恢复与交付验收

1. 用[restore](../../deploy/backup/mysql-restore.sh#L60)校验 manifest/gzip，在明确的新隔离库恢复；匹配二进制/schema 与旧 Master Key，用 SELECT-only 账号执行 `configra doctor --config /run/configra/doctor.yaml --verify-vault --timeout 10m`。备份 SHA-256 检测损坏，不认证来源。
2. 对比 inventory/记录数、实际 Config/File 读取、CA 签发与吊销、Mutation Audit；恢复库中的通知只指向测试目标。单纯 readiness 或 doctor 不能证明备份完整和授权正确，沿用[恢复手册](../../deploy/backup/README.md)。
3. 核对恢复点之后的 Token/CA 吊销及 `management_session_policies` 的 block/invalidate；未确认的旧会话/身份先失效再开放流量，避免恢复较旧数据库重新放行已撤权用户。多副本上分别验证会话阻断。
4. 记录恢复点年龄、实际 RTO 与数据损失窗口；停止一个真实主机/切换 MySQL 主库，分别观测已有 LKG 客户端和冷启动客户端。kind 演练只证明已记录的同机进程/节点故障行为。
5. 受限账号验收覆盖：Management 重启/3→4 迁移/API 新旧 schema、Config/Vault/发布集写读、部署凭据签发/撤销、采样、GC preview/apply/replay、备份及恢复失败清理；特别覆盖 CA FOR UPDATE 和新发布表，确认 API 不能 DDL、Management 仅有约定的建表/迁移权限、服务账号不能执行 GC 删除。

托管 MySQL 的 HA、自动备份/PITR 可替代脚本调度；运维负责有效恢复范围、binlog/GTID 关联、保留、加密和权限。当前 dump 不记录恢复坐标也不归档 binlog，不能宣称自带 PITR；PITR 在完整备份后继续应用变化，仍须做上述应用与授权验证。[MySQL PITR](https://docs.oracle.com/cd/E17952_01/mysql-8.0-en/point-in-time-recovery.html)
