# Configra

**把应用配置、敏感值和机器访问，放在一个地方管理。**

[产品网站](https://viber-ops.github.io/configra/) · [使用文档](https://viber-ops.github.io/docs/configra/) · [下载](https://viber-ops.github.io/docs/configra/installation/) · [English](README.md)

Configra 是面向研发与运维的自托管配置服务。如果你正在多个仓库、部署脚本和集群之间反复复制配置，它可以提供统一工作台：管理多环境 YAML / JSON、引用共享 Vault 值，再通过 Go SDK 或 Kubernetes 交给应用。

> 当前稳定版为 **v1.0.1**。请使用文档指定的标签，并在自己的部署环境验证入口、容量和恢复流程。

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
git clone --branch v1.0.1 https://github.com/viber-ops/configra.git
git clone --branch v1.0.0 https://github.com/viber-ops/configra-go.git
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

v1.0.1 的运行时代码已在 4 vCPU Linux 主机上通过十分钟 1000 QPS 测试，API 限制为 2 CPU / 512 MiB，60 万次读取全部成功。[测试记录](docs/verification/2026-09-22.md#final-runtime-native-linux-acceptance)包含具体环境和失败记录，不能直接作为所有部署的容量保证。

v1.0.1 新增后端列表分页、只读恢复检查和[维护窗口内的主密钥轮换](deploy/backup/README.md#offline-master-key-rotation)。从 rc.2 升级时，自编 Management 客户端需要[适配分页](docs/ui.md)；机器读取接口和 SDK 调用方式不变。生产入口、跨主机故障仍需在自己的环境验证。上线前请查看[当前验收状态](docs/production-readiness.md)。

[本地体验](https://viber-ops.github.io/docs/configra/quickstart/) · [服务部署](https://viber-ops.github.io/docs/configra/deployment/) · [备份与排障](https://viber-ops.github.io/docs/configra/operations/)

开发与维护资料见[仓库文档目录](docs/README.md)，包括设计、当前生产验收状态和验证记录。

## 许可证

项目原创代码采用 [Apache-2.0](LICENSE)，第三方组件保留各自的许可证与声明。
贡献方式见 [CONTRIBUTING.md](CONTRIBUTING.md)，安全问题请通过 [SECURITY.md](SECURITY.md) 中的私密渠道报告。
