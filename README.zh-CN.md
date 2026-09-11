# Configra

**把应用配置、敏感值和机器访问，放在一个地方管理。**

[产品网站](https://viber-ops.github.io/configra/) · [使用文档](https://viber-ops.github.io/docs/configra/) · [下载](https://viber-ops.github.io/docs/configra/installation/) · [English](README.md)

Configra 是面向研发与运维的自托管配置服务。如果你正在多个仓库、部署脚本和集群之间反复复制配置，它可以提供统一工作台：管理多环境 YAML / JSON、引用共享 Vault 值，再通过 Go SDK 或 Kubernetes 交给应用。

> 当前为 **v0.1.0-rc.1 预发布**。以下功能对应发布标签，功能 PR 尚在审查，不代表已经完成生产验收。

![Configra Vault 工作台：资源导航、条目、环境变体与字段详情](https://viber-ops.github.io/assets/configra/vault-light.png)

*真实界面，使用演示数据；敏感值默认隐藏。*

## 你是否需要它

- 多个环境的配置难以对齐，需要查看历史、比较变更和克隆配置。
- 多份配置共用密码、标识或证书文件，希望通过引用维护。
- 研发需要可直接使用的配置，运维需要管理凭据和排查访问。
- 有些应用能接 SDK，有些只能读文件或环境变量。

## 能做什么

| 场景 | 能力 |
| --- | --- |
| 配置管理 | 多环境 YAML / JSON、版本历史、比较与克隆 |
| 敏感值管理 | Vault Text / Secret / File 字段和显式展示控制 |
| 机器读取 | HTTPS、Environment Token、mTLS 与吊销检查 |
| 证书管理 | 创建 CA、加密保留签名私钥、持续签发、首次私钥导出 |
| Go 应用 | SDK 读取最终配置，Viper 快照与 ETag 轮询 |
| Kubernetes 应用 | CSI 文件挂载，以及原生 Secret / ConfigMap 同步 |

```yaml
database:
  username: "{vault.platform.database.username}"
  password: "{vault.platform.database.password}"
```

应用读取时拿到当前环境解析后的完整文档，不需要自己拼接 Vault 引用。

## 开始体验

准备 Docker Compose、Go 1.25.13+、Node.js 24、npm、Make 和 OpenSSL：

```sh
git clone --branch v0.1.0-rc.1 https://github.com/viber-ops/configra.git
git clone --branch v0.1.0-rc.1 https://github.com/viber-ops/configra-go.git
cd configra
make local-run
```

启动命令会准备本地依赖、证书、OIDC 和 UI。打开 **https://localhost:18088**，使用本地账号 `admin` / `configra-admin`。证书为自签名；此栈只用于受控开发机，不能直接暴露公网或沿用到生产。

本地命令启动管理面，机器 API 单独部署。也可以[下载 macOS / Linux 二进制](https://viber-ops.github.io/docs/configra/installation/)，运行时不需要 Node.js。

## 按你的应用选择

- [Go SDK](https://viber-ops.github.io/docs/configra/go-sdk/)：应用验证并应用新快照。
- [CSI provider](https://viber-ops.github.io/docs/configra/kubernetes/)：轮换挂载文件，应用负责重读。
- [原生同步](https://viber-ops.github.io/docs/configra/kubernetes/)：兼容 Secret / ConfigMap volume 和 envFrom；环境变量变更需要替换 Pod。

Configra 自身可运行在 Kubernetes 中，Management 与 API 分开部署。MySQL、NATS、ClickHouse、OIDC 和 Master Key 是外部启动依赖，不通过 Configra 自己的 provider 获取。

## 先了解边界

Token 按 **Environment** 授权，Admin / Viewer 为工作区级角色，Vault Namespace 不是多租户隔离机制。不提供动态数据库凭据、HSM/KMS、应用自动重启或 Master Key 自动轮换。

本轮 1000 QPS 验收尚未通过，部分早期拒绝请求的持久审计和后端列表分页仍需改进。生产前先阅读[安全与发布状态](https://viber-ops.github.io/docs/configra/security/)，并完成自己的负载与恢复验证。

[本地体验](https://viber-ops.github.io/docs/configra/quickstart/) · [服务部署](https://viber-ops.github.io/docs/configra/deployment/) · [备份与排障](https://viber-ops.github.io/docs/configra/operations/)
