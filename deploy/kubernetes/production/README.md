# Production reference overlay / 生产参考部署

Render with `kubectl kustomize deploy/kubernetes/production`. Customize the image
to an immutable digest, ConfigMap endpoints/OIDC settings, namespace, ingress and
monitor selectors before applying. Create the `configra` namespace separately.
This overlay needs at least **two real zones and two eligible hosts**, capacity
for API rollout surge, and a CNI that enforces NetworkPolicy. Insufficient domains
leave replacement Pods Pending instead of silently co-locating them.

It supplies two API and two same-version Management replicas, per-component PDBs,
zone/host spreading, independent database Secrets, private metrics and explicit
ingress rules. It retains Management `Recreate`: schema/static asset upgrades use
a maintenance window. A PDB constrains voluntary eviction, not node failure or
Deployment rollout; it is not a zero-downtime upgrade guarantee.

Create the base's TLS, Master Key, NATS and `configra-runtime` Secrets, plus:

| Secret | Required key |
| --- | --- |
| `configra-api-db` | `mysql-dsn`, using the API role |
| `configra-management-db` | `mysql-dsn`, using the Management role |

The legacy shared `mysql-dsn` is not consumed by this overlay. The runtime Secret
still holds ClickHouse and OIDC settings. Keep migration/backup/doctor/GC accounts
out of running service Pods; see [database roles](../../mysql/README.md).

Use TLS pass-through for 9443. The sample allows ingress-nginx Pods in its own
namespace and direct API clients in namespaces labeled `configra-client=true`.
Metrics on port 9090 are allowed only from Prometheus Pods in `monitoring`; both
selectors appear in the **same peer**, so they mean AND. Do not add an Ingress or
public Service for metrics. Customize egress controls in the platform overlay
for your actual MySQL/NATS/ClickHouse/OIDC/DNS destinations.

先执行 [v1.2.0 升级](../../../docs/releases/v1.2.0.md) 的停写、备份、临时迁移和
校验步骤，再启动同版本副本。数据库 HA、存储故障域和 OIDC 可用性仍由基础设施
保证。已有客户端可保留内存旧快照，新客户端仍需要在线读取；不能用副本数量证明
冷启动或数据库故障恢复成功。

Configure an actual authorized `configractl check` probe as well as scrape alerts.
Rehearse one real host/DB primary failure, observe both loaded and cold clients,
then perform the [restore drill](../../backup/README.md). Record RPO/RTO and
reapply post-backup revocations before opening recovered traffic. The repository's
kind/container tests establish reproducible software behavior, not physical
fault-domain or provider failover guarantees.
