# Configra v1.1.0 运维与系统架构评审

评审日期：2026-10-02。服务端基线 `5bd686a23feeda93527b217a59e56811da888140`，SDK 基线 `88776a775b99edcbd5db499be26d4f17bcc4dd45`，均对应已发布 v1.1.0。

2026-10-03 补充：核对现有命令后，将配套 Go 客户端 CLI 单列为第 8 项 P1。它是发布、轮换和自动化的共同交付入口，应放入第一批。

本轮沿配置发布与读取、SDK/CSI/原生同步、机器与人工身份、审计投递、部署和恢复链路检查源码，并核对官方行业资料。关注会导致发布事故、到期停机、撤权滞后、静默故障或长期资源耗尽的能力。源码确认与风险推导分别说明；未读取现网部署，也没有把仓库缺少生产 overlay 推断为现网没有 HA。

## 结论与顺序

MySQL 事务、一致快照读取、加密 Vault 历史、Management/API 分部署、Outbox 审计和客户端 Last-known-good 是可以继续保留的基础。下一阶段最有价值的工作是安全发布、可观测性和身份生命周期，加上可执行的生产运维方案。

P1 表示建议在长期生产自动化依赖之前补齐；P2 表示随自动化写入规模、保留期和共享工作负载增长补齐。这里没有将设计上明确接受的短维护窗口定为缺陷。

| 顺序 | 重要缺口 | 优先级与触发条件 | 最小交付 |
| --- | --- | --- | --- |
| 1 | 业务校验、配套资源发布与完整回滚 | P1；尤其适用于 server.yaml 与 17 个关联 File | 生效前校验；记录完整版本组合；需要同时变化的资源支持发布集和一致读取 |
| 2 | 运维指标、告警及消费端新鲜度 | P1；任何长期生产部署 | 可抓取指标、少量告警规则、真实读取探测及运行手册 |
| 3 | Token/证书换新到实际消费的完整流程 | P1；服务寿命超过凭据寿命 | 到期预警、轮换影响清单、替换验证与失败处理 |
| 4 | 管理员紧急撤权和多会话失效 | P1；离职、降权、账号失陷 | 按用户吊销现有会话，明确 IdP 变更到应用拒绝的延迟 |
| 5 | HA、定时备份与恢复的生产实施方案 | P1，但主要是部署责任 | 生产 overlay/部署范本、备份新鲜度告警、明确 RPO/RTO 与故障切换演练 |
| 6 | 机器写入过载时保护正常读取 | P1 随 scoped-write 广泛接入；小规模先设简单预算 | 写入并发/速率预算、可识别过载响应、混合读写验收 |
| 7 | 持久数据增长和安全回收策略 | P2；至少先提供容量测量与告警 | 已完成 Outbox 生命周期、重放保留契约、历史与日志容量预算 |
| 8 | 可直接发行和用于 CI/operator 的 Go 客户端 CLI | P1；当前部署自动化即可受益 | 复用 SDK 的 Config/Vault/部署凭据命令、安全文件输出和稳定脚本契约 |

## 1. 安全发布：语义正确、成组生效、完整回滚

**已具备。** 单个 Config/Vault Mutation 具有事务、Revision、乐观并发控制和幂等性；一次 Resolved Config 从一致数据库快照解析其引用。17 个 File 放在同一个 Vault Item 时，可以一次提交完整快照。不能把这些已有保证说成缺少事务。

**确认的缺口。**

- [`ValidateConfig`](../../internal/storage/mysqlstore/config_validation.go#L24) 执行格式规范化和引用检查，没有应用级 schema/语义校验。合法 YAML 中的错误端口、超时或不兼容字段仍可能发布。
- [SDK 的 `reloadLocked`](https://github.com/viber-ops/configra-go/blob/v1.1.0/handler.go#L177) 在执行 `OnChange` 之前存入新 Snapshot；回调失败不恢复旧 Snapshot，初次 Load 也没有业务校验入口。这个行为由 ADR 和测试明确规定，不能拿 `OnChange` 当作生效前拦截器。
- [operator](https://github.com/viber-ops/configra-go/blob/v1.1.0/examples/operator/main.go#L69) 先写 Vault，再写 Config，两次提交之间可被读取。第二次失败时，第一次已经生效。
- [Vault Reference 查询](../../internal/storage/mysqlstore/machine.go#L328) 使用 Item 的 `current_revision`。恢复旧 Config 并不恢复它当时所用的 Vault 值。[ADR-0003](../adr/0003-resolve-current-vault-revisions.md) 已明确将完整发布快照作为后续可能的演进。
- [Kubernetes Reader](../../kubernetes/internal/source/source.go#L88) 逐个请求 Config/File。一次目标对象写入虽是整体写入，但获取这组内容时可能跨越一个 Vault 更新，因此多个 File 的来源版本也可能不一致。

**实际影响。** 当配置与密钥必须配套时，消费者可能拿到旧 Config/新密钥，或者跨版本的文件组合。恢复 Config 历史不能单独证明业务已恢复。数据库读一致性在这里成立，缺的是业务发布单位。

**建议最小交付。**

1. 先由 CI/operator 执行与应用同源的结构和业务校验；SDK 增加可选的生效前 `Validate`，在 Load/Reload 安装 Snapshot 前执行，失败保留原 Snapshot 与 ETag。只输出安全的字段路径和错误码。
2. 对必须配套变化的内容记录一个不可变发布清单：Config Revision、Vault Item Revision、File 成员及发布 ID。清单只记录身份和版本，不额外复制明文。
3. 配套发布需要候选集验证后统一激活，并有按发布 ID 读取一组内容的契约。**仅加批量写事务仍不足以防止多个独立 GET 跨版本**；读取端也须绑定同一发布集。继续校验当前 Token、证书、环境和白名单权限。
4. 在一小组消费者验证后扩大范围，失败可恢复整个版本组合。早期由现有 CI/部署脚本负责观察和回滚即可；无需先自建完整审批/灰度平台。若目标数据库或第三方已撤销旧秘密，恢复历史文件不能代替目标系统的凭据恢复。

AWS AppConfig 提供 schema/语义验证以及渐进部署、观察期和告警回滚；Consul 提供有界的多项原子事务。这些支持上述能力的成熟性，但不能据此声称所有配置中心都必须采用相同发布模型。[AppConfig 验证器](https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-creating-configuration-and-profile-validators.html)、[部署策略](https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-creating-deployment-strategy.html)、[Consul 事务](https://developer.hashicorp.com/consul/api-docs/txn)

**验收。** 可解析但业务无效的配置不能替换 Current；第二项提交失败不暴露半套候选集；并发更新期间读取 YAML 与 17 个 File 只返回一个发布组合；回滚重新得到完整预期组合。

## 2. 可观测性：健康检查之外还需要可执行告警

**已具备。** 有 live/ready、结构化安全日志、Access/Audit 查询、通知重试以及 Kubernetes Binding 的 Ready、version、lastSyncedAt。读取失败时 SDK 保留已有 Snapshot，并可调用 `OnError`。

**确认的缺口。**

- [服务探针](../../internal/app/app.go#L211) 的 readiness 检查 MySQL/Crypto Sentinel；没有交付请求延迟、连接池等待、后台积压等可抓取指标与告警规则。
- [同步控制器](../../kubernetes/cmd/configra-kubernetes/main.go#L160) 明确以 `BindAddress: "0"` 关闭 controller-runtime metrics。
- [Log Worker](../../internal/app/app.go#L159) 意外返回时只记录错误，HTTP 服务继续运行；[worker 循环](../../internal/logworker/run.go#L55) 中审计和通知投递顺序执行，慢通知还会延迟下一轮审计投递。现有探针不能说明这些后台功能是否持续工作。
- Access 通过 Core NATS 尽力投递；[`TryPublish`](../../internal/accessnats/publisher.go#L76) 返回失败标志，[消费端](../../internal/accessnats/consumer.go#L69) 投递失败会丢弃该批事件。ADR-0011 写了 dropped-event metric 的意图，但当前运行接线未交付该指标。
- SDK 没有标准化的最近成功验证时间/陈旧时间输出。Binding 的同步成功只证明内容写入 Kubernetes 对象；服务端 Access 只证明返回了内容，均不能证明应用已经采用并正常运行。

**实际影响。** 页面和 API 仍可访问，但审计数小时不入库、证书信任刷新持续失败或应用长期保留旧配置，值班人员可能不知道。

**建议最小交付。** 暴露内部 metrics：请求量/状态/延迟、DB pool wait、Audit/Notification backlog 与最老年龄、dead/retry 数、Access dropped、worker 心跳、凭据到期、SDK/adapter 最近成功刷新。优先接现有 Prometheus/告警系统，提供少量规则和排障手册；不要将 Token、任意资源 ID、原始 URL 或值作为标签。告警出口应独立于 Configra 自身的变更通知链路。

意外退出的关键 worker 必须可被监督恢复或让实例明确失败。审计与慢通知应有独立执行预算；暂不需要拆成新的微服务。ClickHouse/NATS 临时故障也不应简单变成 API 读取的 readiness 失败。对于 Last-known-good，先提供陈旧度和告警，让应用选择允许的陈旧预算，不统一强制杀停所有使用旧配置的业务。

Vault 提供存储/请求/审计健康指标；External Secrets Operator 提供同步、provider、协调队列与 SLI 指标，说明“进程在运行”和“秘密正在正常投递”需要分别观察。[Vault 指标](https://developer.hashicorp.com/vault/docs/internals/telemetry/key-metrics)、[ESO 指标与 SLI](https://external-secrets.io/latest/api/metrics/)

**验收。** 停掉 ClickHouse、令 worker 退出、阻断某个消费者并制造一次同步失败，分别观察积压/心跳/陈旧度告警，并确认恢复后消退；不因此中断仍可正常鉴权和读取的 API。

## 3. 机器凭据：到期与轮换必须覆盖实际运行中的客户端

**已具备。** 签发、到期、吊销、mTLS、父子关系和证书绑定；管理端 Token 列表已经提供到期时间、父 Token ID 等元数据。不能把缺少轮换编排说成没有凭据管理。

**确认的缺口。**

- [部署凭据](../../internal/storage/mysqlstore/deployment_credential.go#L56) 的寿命不超过 writer/CA；[撤销 writer](../../internal/storage/mysqlstore/deployment_credential.go#L153) 级联吊销子 Token 与证书。这个安全规则正确，但轮换需要覆盖所有子凭据。
- [示例](https://github.com/viber-ops/configra-go/blob/v1.1.0/examples/operator/main.go#L97) 下发 24 小时部署凭据，是一次性交付示例，没有定时换新与验证循环。
- [Server TLS](../../internal/bootstrap/tls.go#L14) 只在启动时载入；更新 Kubernetes Secret 文件不会让这个进程自动采用新服务证书。
- [SDK 文件凭据](https://github.com/viber-ops/configra-go/blob/v1.1.0/client.go#L97) 在 NewClient 时读取。SDK 支持自定义 `GetClientCertificate` 的轮换和关闭空闲连接，但默认 TokenFile/证书文件不是自动监听器。Kubernetes adapter 每次 Read 重建 Client，不能与长期持有 Client 的 Go 应用混为一谈。
- 未发现到期扫描/预警、消费端采用确认，以及 routine rotation 的可执行批次状态。

**实际影响。** 如果 operator 仅在发版时执行一次，使用示例凭据的主机次日可能失去刷新和冷启动能力；运行中 Last-known-good 还可能掩盖这一故障。例行撤销旧 writer 也可能意外同时撤销仍被主机使用的子凭据。

**建议最小交付。** 复用现有凭据列表，补到期扫描和 writer→部署凭据→使用方的轮换视图；对真正提供服务的 TLS 证书做外部探测。提供一次可重试的流程：签发新身份→原子分发 Token/证书组合→重建 Client 或受控滚动重启→用新连接验证读取→记录完成→撤销旧身份。分发与验证失败时保留明确状态，不能直接撤销尚在使用的身份。

周期任务可以轮换部署凭据；writer 的续期仍由管理员 MFA 签发，保持现有权限边界。routine rotation 要先换子凭据；失陷应急则应立即级联撤销。无需为了无重启续期先建设一套工作负载身份平台。

AWS Secrets Manager 的轮换包含修改目标服务、测试候选再切换当前版本；cert-manager 也指出应用可能需要重新载入更新后的证书。存储里的值已更新不等于使用方轮换完成。[秘密轮换步骤](https://docs.aws.amazon.com/secretsmanager/latest/userguide/rotate-secrets_lambda-functions.html)、[cert-manager 证书生命周期](https://cert-manager.io/docs/usage/certificate/)

**验收。** 验证 writer 将到期/撤销、CA/服务 TLS 换新、存量连接与新连接、长寿命 SDK Client 和新 Pod；最后观察到业务使用新身份且旧身份已拒绝。

## 4. 人工身份：IdP 撤权要能终止已有管理会话

**确认的缺口。** [OIDC callback](../../internal/humanauth/oidc.go#L211) 在登录时保存 role/amr，之后 [`attachPrincipal`](../../internal/humanauth/oidc.go#L235) 直接从服务端 Session 读取。Session 的[绝对寿命为 8 小时、空闲 30 分钟](../../internal/humanauth/session.go#L16)。当前提供本人退出，没有按 issuer/subject 吊销全部会话、Back-Channel Logout 或周期权限复核。

**实际影响。** 在 IdP 禁用账号/移除 Admin 角色，会影响之后的新登录，但不能据当前代码推导出已建立 Configra Session 立即失效。持续活跃的旧会话可能保留原权限直到会话到期；MFA 曾通过也不能解决权限已撤销的问题。这是应急撤权能力缺口，不是机器 Token 吊销失败。

**建议最小交付。** 首先提供按 issuer+subject 使所有 Management 副本上的会话失效的受控管理/运维操作，并纳入人员离职和失陷处置。然后针对实际 IdP 支持接入已签名的 Back-Channel Logout，或设定有明确延迟上限的权限复核。角色变更不一定触发 IdP logout，必须验证所用 IdP 的禁用/降权流程。

OIDC Back-Channel Logout 定义了 Logout Token 校验以及按 `iss`、`sub`/`sid` 清除应用会话的机制；不能实现成接收任意 subject 的无验证 POST。[OIDC 标准 §2.6–2.7](https://openid.net/specs/openid-connect-backchannel-1_0.html)

**验收。** 管理员保持活跃浏览器会话，在 IdP 禁用/降权后，于约定时限内在每个副本上都无法执行高权限操作；伪造 logout 不能清除他人会话。

## 5. HA 与灾备：工具已有，生产实施和证据还需闭环

**已具备。** [备份、恢复、doctor、离线主密钥轮换](../../deploy/backup/README.md) 与真实依赖演练都已存在。不能重新列“没有备份恢复”或要求在线主密钥轮换来否定这些工作。

**确认的仓库边界。** [API base](../../deploy/kubernetes/base/api.yaml#L9) 和 [Management base](../../deploy/kubernetes/base/management.yaml#L9) 各一个副本，Management 使用 Recreate；base 没有生产 PDB/拓扑分散/备份调度 overlay。两者默认引用同一 `configra-runtime/mysql-dsn`，生产需要分别落实 Management DDL 与 API runtime 权限。文档明确将 HA 数据库、独立故障域、备份保留和 RPO/RTO 留给部署方。

每次机器读取依赖 MySQL 的授权与数据状态；[内存 Last-known-good](../adr/0007-use-unbounded-in-memory-fallback.md) 只帮助已成功加载的运行中进程，不能让新的 Pod/进程在数据库或 Configra 故障时启动。多个 Configra 副本也不能弥补单点 MySQL。

**建议最小交付。** 提供一个明确标注前提的生产参考配置：API 跨故障域副本、合适的 PDB/拓扑策略、TLS pass-through、独立 DB 用户、选定 MySQL HA 方案；Management 按固定版本运行的 HA 与有维护窗口的 schema 升级分别说明。数据库 HA/PITR 优先使用现有数据库平台能力，不在 Configra 内自研。

把已有备份脚本接入调度和独立故障域存储，独立保存 Master Key/冷配置，提供备份新鲜度和恢复演练失败告警。定义可接受数据丢失与恢复时间；在干净环境恢复并运行 doctor、真实 Config/File 读取、凭据鉴权与 Audit 验证，记录实际用时。仓库无法证明现网已经或尚未做到这些，需交付环境负责人认领。

**特别应纳入的恢复场景。** Token/CA 吊销状态也在 MySQL 中。从早于某次吊销的恢复点恢复，可能恢复当时仍有效的授权，这是根据存储模型推导的风险。恢复手册应核对备份点之后的应急吊销；无法确认时先重新撤销/替换受影响凭据，再开放服务。不要把“数据能够解密”当作恢复后授权状态已正确。

Consul 快照覆盖配置与 ACL 状态；Vault HA 同样依赖存储层可用性，应用副本数不能替代恢复与存储故障测试。[Consul 快照](https://developer.hashicorp.com/consul/commands/snapshot/save)、[Vault HA](https://developer.hashicorp.com/vault/docs/concepts/ha)

**验收。** 独立主机或 DB 主库故障后，记录已有客户端和冷启动客户端各自的行为；从异地备份恢复的时间/丢失窗口满足声明目标，并验证已撤销身份的处置。单宿主机 kind 演练仍有价值，但证据范围保持原有界定。

## 6. 写入过载保护：给关键读取保留资源

**已具备。** 有请求大小限制、20 秒请求 deadline、连接池限制、SDK Watch 抖动/退避。它们限制单个请求，不是按调用方或读写类别的资源预算。

**确认的缺口。** [9443 同一进程接入读写路由](../../internal/app/app.go#L175)，共用每 Store [最多 20 条 MySQL 连接](../../internal/storage/mysqlstore/bootstrap.go#L46)。机器写入还会执行 JSON/文件处理、加解密、Revision/Operation/Outbox 持久化和证书签发。没有发现 API 写入 admission limit、按身份限流或存储配额。原容量门槛主要测 resolved Config 读取，不能替代混合读写验收。

**实际影响。** 一个有权限但出错的 operator 紧密循环提交/重试，会与其他业务读取争用连接、CPU 和内存，即使它始终遵守 Environment scope。

**建议最小交付。** 先限制单实例同时执行的重 Mutation，并保留读取预算；按已认证 Token 增加简单速率规则，必要时拆出池预算。给出明确 429/过载错误与 Retry-After、客户端有界退避；入口方案必须保留真正的 mTLS，不因限流改造丢失证书身份。不要以丢弃已接受 Mutation 的审计换吞吐。暂不引入 Redis 或分布式配额系统。

Vault 使用 rate-limit quota 保护节点免受误行为客户端影响；Secrets Manager 对不同类别 API 使用不同配额。这是共享服务常见保护机制。[Vault 资源配额](https://developer.hashicorp.com/vault/docs/concepts/resource-quotas)、[Secrets Manager 配额](https://docs.aws.amazon.com/secretsmanager/latest/userguide/reference_limits.html)

**验收。** 并行运行正常读取、File 写入和凭据签发，再让一个 writer 持续重试；正常读取仍满足声明的延迟/成功率，等待队列和内存有界，超限无部分提交。

## 7. 数据生命周期：先测增长，再安全清理

**确认的缺口。** [已投递 Outbox](../../internal/storage/mysqlstore/outbox.go#L97) 只变成 completed；Operation、Revision 和投递历史没有交付清理流程。Access 表有[固定 90 天 TTL](../../internal/logstore/clickhouse.go#L381)，Audit 表没有 TTL。历史和 Archived 身份的保留是现有语义；分页不限制总存储量，也不限制备份/轮换所需时间。

**实际影响。** 自动化放大写入频率，File Revision 会保留完整加密快照。若持续有 1000 次每秒的带内容读取，理论上每天产生 8640 万条 Access Event；这只是容量估算，实际量受 304、真实流量和尽力交付影响，不能当作实测。固定 90 天保留期需要对应容量预算。

**建议最小交付。** 先展示/导出每类数据的数量、体积与增长率，提供磁盘/备份时长告警。优先对已成功投递、满足归档及引用要求的 Outbox/投递明细提供可中断的有界维护任务。未投递、失败待处置审计不能按年龄直接删除。

Operation 清理必须先定义重放契约：保留小型身份/摘要或明确的拒绝过期重放机制，不能删除记录后让旧 OperationID 被当成新写入。Vault/Config 历史销毁需要独立、明确的保留政策，不能为了 GC 静默破坏用户要求的历史和恢复能力；先量化增长比默认裁掉历史更合适。

Vault KV v2 提供版本保留控制；Secrets Manager 也定义旧版本回收条件和写入配额。这些证明需要生命周期契约，不意味着应照抄它们的默认保留数量。[Vault KV v2 配置](https://developer.hashicorp.com/vault/api-docs/secret/kv/kv-v2#configure-the-kv-engine)、[Secrets Manager 版本配额](https://docs.aws.amazon.com/secretsmanager/latest/userguide/reference_limits.html)

**验收。** 历史数据超过维护窗口后，清理不破坏当前读取、授权、未投递审计、承诺的历史恢复和 Operation 防重复；记录维护耗时及恢复耗时。

## 8. 配套 Go 客户端 CLI：把 API 能力变成可直接使用的运维工具

**确认的缺口。** 当前 [`configra` 命令](../../internal/app/app.go#L28) 提供 Management/API 启动、doctor 和离线主密钥轮换；它承担服务运行与维护。[SDK operator](https://github.com/viber-ops/configra-go/blob/v1.1.0/examples/operator/main.go) 是固定上传流程的示例，尚未形成覆盖通用部署操作、具有稳定参数/退出码及发行包的客户端 CLI。

**实际影响。** 每个 operator/CI 使用方都要自行编写 Go 或 HTTP 脚本，重复处理 mTLS、乐观版本、幂等重试、凭据文件权限和错误分类。API/SDK 可用仍不足以让运维直接、安全地使用这些能力。

**建议最小交付。** 用 Go 发行独立客户端命令 `configractl`，复用 `configra-go`，第一版覆盖已有 API：

- `config get/put`：下载与上传 Config，明确 Environment、预期 Revision 和 OperationID。
- `vault item/field/file`：范围内读写、File 上传/下载及删除，复用现有版本和归档语义。
- `deployment-credential issue/revoke`：签发或吊销只读 Token/证书组合，保存无值操作回执和私有凭据文件。
- 本地 context/profile：保存服务器、默认环境及凭据文件路径，写入时明确目标，便于 testing/production 脚本复用。

脚本契约同样属于第一版：稳定退出码、`--json` 元数据输出、超时/取消、显式预期版本、可复用的 OperationID。敏感输入使用文件或 stdin；敏感内容和一次性凭据默认写入 0600 文件，不自动打印到 CI 日志。提交结果不确定时保留同一 OperationID 查询重放；一次性凭据丢失不能伪装为可恢复明文，应按现有规则撤销重发。

CLI 使用 9443 的 Token+mTLS 和原有权限边界；写令牌签发仍由管理员在管理端完成 OIDC+MFA。CLI 不需要数据库或 Master Key。

后续再把发布 plan/apply、成组回滚和轮换流程放到这个入口。服务端尚无发布集、受限凭据清单或计划 API 的地方，应先补相应契约；不能把多个普通调用包装成一个 `apply` 就承诺原子发布。

**验收。** 在没有 Go 编译器的干净部署机上，仅用发行二进制和 scoped Token+mTLS，完成 YAML+17 File 上传及部署凭据签发/吊销；覆盖版本冲突、范围外 403、超时后的同 ID 重放、一次性导出丢失，以及 stdout/stderr 不泄露秘密。

## 有条件再做的能力

| 能力 | 值得投入的触发条件 |
| --- | --- |
| 人工 Environment/项目级权限、生产发布双人复核 | 第二个权限边界不同的团队接入。当前人类角色仍全局 Admin/Viewer；可先由现有 CI 保护生产发布，再决定细粒度 RBAC |
| 工作负载身份、外部 KMS/HSM | 大量短寿命主机/Pod 使分发静态身份成为负担，或组织要求外部密钥托管。KMS 本身不能解决应用进程已被控制的问题 |
| 完整秘密读取审计、外部不可变留存 | 需要证明每次秘密读取或抵抗日志管理权限篡改。当前 Mutation Audit 耐久，Access 是尽力交付，二者保证不同；先补丢失/积压信号。更强读取审计会涉及“审计失败是否拒绝返回内容”的可用性选择 |
| 动态数据库账号引擎、多地域双活、通用审批/策略平台 | 出现实际业务要求后再评估，不因竞品拥有就默认加入本轮 |

Vault KV 静态秘密不具备动态秘密的 lease，说明静态秘密中心无需默认复制动态凭据引擎。[Vault lease](https://developer.hashicorp.com/vault/docs/concepts/lease) 若需要强读审计，可参考 Vault 至少一个审计设备写入成功才允许相应请求的保证；若需要外部保留，优先对接已有日志平台/对象保留机制，不自行发明签名链。[Vault 审计可用性](https://developer.hashicorp.com/vault/docs/audit#availability-of-audit-devices)、[S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)

## 建议实施批次

1. **先让运维可直接使用、运行状态可知、身份可控。** 先交付复用 SDK 的 Go 客户端 CLI，随后承载受控轮换流程；补 metrics/告警、到期预警和按用户撤销管理会话，同时明确并演练生产备份与恢复责任。
2. **再保证变更安全。** 应用同源校验与 SDK 生效前校验先落地；围绕印小伴的 YAML+17 File 定义完整发布集、读取和回滚契约。先复用 CI 做少量实例验证。
3. **随后保障长期共享运行。** 混合读写预算与验收，存储增长观察、已完成投递回收及重放/历史保留政策。若已有高频自动写入，读写预算提前到第一批。

每一项都以对应故障场景的验收作为完成条件；不以新增页面、参数或测试数量代替闭环。

## 本轮验证边界

- 查看了服务、存储、Auth、PKI、日志 worker、SDK、Kubernetes 适配器、部署/备份及 ADR 的实际实现；没有以旧版 Environment-only 授权描述评价 v1.1.0 的机器权限。
- 重新运行 SDK 的 `TestViperHandlerInstallsBeforeReportingCallbackFailure` 和 `TestClientRotatesRuntimeCertificateAfterClosingIdleConnections`，`go test -race -count=1` 通过，确认回调与运行时证书行为。其余缺口为源码/契约和运维交付范围的评审，不伪称已进行破坏性现网实验。
- 官方资料于 2026-10-02 通过 HTTPS 直接读取；浏览工具接口不可用时未依赖搜索摘要或二手博客。
- 本报告提出后续完善项，不改变已发布 v1.1.0 的权限、历史、可用性或发行验收契约。
