# Documentation / 文档

## Using Configra / 使用 Configra

Start with the versioned website guides: [English](https://viber-ops.github.io/en/docs/configra/) ·
[中文](https://viber-ops.github.io/docs/configra/). They cover installation, configuration,
certificates, the Go SDK and Kubernetes consumption.

These repository guides describe the checked-out source:

| Task / 任务 | Guide / 文档 |
| --- | --- |
| Install a binary bundle / 安装二进制包 | [Release installation](release-installation.md) |
| Deploy the service on Kubernetes / 部署服务 | [Service deployment](../deploy/kubernetes/README.md) |
| Consume configuration in Pods / Pod 接入 | [CSI and native synchronization](../kubernetes/README.md) |
| Operate deployments / 部署 CLI | [configractl](cli.md) |
| Publish/rollback whole sets / 整套发布回滚 | [Release sets](release-sets.md) |
| Revoke human sessions / 撤销人工会话 | [Session control](session-control.md) |
| Monitor and retain data / 监控与数据保留 | [Metrics](../deploy/monitoring/README.md), [Outbox retention](data-retention.md) |
| Production roles and HA / 生产权限与 HA | [DB roles](../deploy/mysql/README.md), [HA overlay](../deploy/kubernetes/production/README.md) |
| Upgrade to v1.2.0 / 升级至 v1.2.0 | [Schema 3 → 4](releases/v1.2.0.md) |
| Automate scoped writes / 自动化范围写入 | [English](scoped-write-tokens.md) · [中文](scoped-write-tokens.zh-CN.md) |
| Upgrade to v1.1.0 / 升级至 v1.1.0 | [Release and migration](releases/v1.1.0.md) |
| Manage CAs and client certificates / 管理证书 | [Managed certificates](managed-certificates.md) |
| Back up and restore / 备份恢复 | [Backup and restore](../deploy/backup/README.md) |

## Develop and operate / 开发与维护

| Question / 问题 | Reference / 资料 |
| --- | --- |
| What remains before production? / 离生产可用还差什么？ | [Acceptance requirements and current status](production-readiness.md) |
| How does the domain work? / 领域模型是什么？ | [Terminology](../CONTEXT.md), [detailed design](design.md) |
| Why were these choices made? / 为什么这样设计？ | [Numbered architecture decisions](adr/) |
| What did the security review find? / 安全审查结论是什么？ | [Security and architecture review](security-architecture-review.md) |
| Which operational capabilities should come next? / 哪些运维与架构能力应优先补齐？ | [v1.1.0 operations and architecture review](research/operations-architecture-review-2026-10-02.md) |
| How should the UI behave? / 界面如何组织？ | [UI design, routes and screenshots](ui.md) |
| How are release materials assembled? / 如何维护发布材料？ | [Distribution tooling](../scripts/DISTRIBUTION.md) |
| What was actually tested? / 哪些操作真正验证过？ | [2026-10-03 verification](verification/2026-10-03.md), [2026-09-22 verification record](verification/2026-09-22.md), [September 12 release checks](verification/2026-09-12.md), [migration record](verification/2026-09-11-migration.md) |

[Contributing](../CONTRIBUTING.md) and [private security reporting](../SECURITY.md)
describe the contribution and disclosure process. [Research notes](research/) retain
upstream evidence; [release notes](releases/) describe published versions. Historical
verification records are not proof that a later candidate passed the same checks.

## Keep this directory small / 文档维护约定

- Keep current status and remaining gates in `production-readiness.md`, not in
  another progress file. 当前状态只维护一份。
- Put dated verification evidence in `verification/`; group related runs instead
  of adding one root-level report per change. 验证记录集中存放，不堆在根目录。
- Keep machine-readable license records beside their collectors. Link to them
  from prose instead of copying the JSON. 文档引用清单，不重复粘贴数据。
- Keep public screenshots in `viber-ops.github.io/public/assets/configra/`.
  Generated test captures belong in `.cache/ui-screenshots/`. 展示资源集中在网站仓库，测试截图不提交。
- Preserve numbered ADRs and published tag references. Update local links when
  moving a document; do not rewrite historical release instructions as current ones.
