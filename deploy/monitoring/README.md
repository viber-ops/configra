# Operational monitoring / 运维监控

Configra exposes Prometheus metrics on a **separate internal HTTP listener**.
Add this to the relevant bootstrap YAML (use different ports when both processes
share a host):

```yaml
observability:
  listen: 127.0.0.1:9090
```

Scrape `/metrics` using an existing monitoring system. Use `0.0.0.0:9090` only
behind the deployment's private network policy/firewall. The listener contains
metadata and is not OIDC/mTLS authenticated; never expose it through public
ingress. An omitted listener disables exposure. The main API keeps its existing
TLS, Token and certificate checks.

指标是独立内网端口；同宿主机 Management/API 使用不同端口。不要暴露到公网。
采集本身不执行数据库查询：后台每 15 秒进行一次、最多 2 秒的元数据采样，失败
保留旧值并令 `configra_operational_sample_success` 为 0。

## Signals and alerts

- HTTP count/latency use only method/status categories, never raw paths or IDs.
- SQL pool state/waits, outstanding Outbox counts/oldest ages, worker state and
  last attempts distinguish serving readiness from background delivery health.
- Access drop counts cover observed publisher errors, slow-consumer drops,
  decoding errors and failed ClickHouse batches. They cannot count every loss
  possible in Core NATS; Access remains best effort.
- Credential inventory/expiry covers Token, client certificate and managed CA
  metadata; it does not expose credential IDs or values. Loaded server TLS expiry
  is reported separately. Choose alert lead times to match your rotation budget.
- Allocated table bytes and **estimated** InnoDB row counts support capacity
  planning. They are not a full-record integrity check or a disk-free measurement;
  use existing database/host exporters for disk, replication and backup storage.

Load `alerts.yaml` into Prometheus and route alerts through an independent
Alertmanager/incident channel. Set scrape job names to `configra-api`,
`configra-management`, and `configra-sync`, or adjust the sample selector. The
rules are starting thresholds, not an SLA. Validate changes with
`promtool check rules deploy/monitoring/alerts.yaml` from the repository root.

Audit, notification and Access consumption run independently. A consumer that
unexpectedly exits stops the owning Management process, allowing its supervisor
to restart it. Dependency outages retry and produce observable backlog/failures;
ClickHouse/NATS failure does not by itself remove the machine read API.

## Actual read and consumer freshness

Machine mutations have a per-process global rate budget and concurrency cap
before authentication. Defaults are 50 requests/s, burst 100 and four concurrent
writes. Authenticated callers additionally receive 25 requests/s with burst 50.
Configure `api_write_limits` in API bootstrap YAML; concurrency is bounded to 1–8
so write transactions cannot occupy all twenty pooled SQL connections.

Global/concurrency rejection happens before authentication and returns 429 plus
`Retry-After`, without accepting a mutation or attributing an unauthenticated
request to a Token. Per-Token rejections occur after authentication and retain the
durable, value-free rejection audit. Reads do not enter this gate. Limits apply
per API process; multiple replicas do not form a global distributed quota. Keep
the same Operation ID/input for an uncertain retry and honor the retry delay.

全局限流在认证/受理写操作之前拒绝；已认证 Token 的限流拒绝仍记耐久审计。
读取不经过写入门限。规则按实例执行，不能当成跨副本总配额。参数应通过实际混合
读写负载校准；不要以删除审计或取消证书校验来提高吞吐。

Use a least-privilege read identity and a known Config:

```sh
configractl --context production-probe check server --timeout 10s --json
```

The command returns metadata only, while verifying real HTTPS, Token/mTLS,
authorization and Config resolution. Probe failure must reach the monitoring
system even if Configra's own notification delivery is unavailable.

Kubernetes sync exposes standard controller-runtime metrics with
`--metrics-address` (default loopback `127.0.0.1:8080`; `0` disables it).
Binding `Ready`, `version` and `lastSyncedAt` remain available through the CRD.
Inspect failing Bindings with Kubernetes tooling; synchronization success does
not prove that an application reloaded its files or environment variables.

Go applications can export `ViperHandler.Status()` through their existing
metrics endpoint: last attempt, last successful accepted/unchanged fetch,
consecutive failures and safe error code. `LastSuccessAt` is a freshness signal,
not proof that post-install `OnChange` application work succeeded. An application
that needs a freshness deadline must define its own allowed staleness and alert;
Configra does not automatically kill otherwise running Last-known-good consumers.

应用应把 SDK 的刷新状态接入自身指标；Kubernetes 对象已同步与应用已生效是两个
阶段。用故障演练确认告警：停止 ClickHouse、阻断一个消费者、制造通知失败，观察
积压/刷新失败告警触发及恢复；不要为了让健康检查变绿而丢弃审计或关闭鉴权。
