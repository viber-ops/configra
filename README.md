# Configra

**Self-hosted application configuration and sensitive-value management.**

[Website](https://viber-ops.github.io/en/configra/) ·
[Documentation](https://viber-ops.github.io/en/docs/configra/) ·
[Downloads](https://github.com/viber-ops/configra/releases/tag/v0.1.0-rc.1) ·
[Go SDK](https://github.com/viber-ops/configra-go) · [中文](README.zh-CN.md)

Configra stores versioned YAML/JSON and shared Vault values for each environment.
Applications read resolved configuration through the Go SDK, CSI file mounts,
or native Kubernetes Secret/ConfigMap synchronization.

> **Preview: v0.1.0-rc.1.** The features below are in the tagged preview, not a
> completed production acceptance. Use the tag for these guides; the feature PR
> remains under review. [Read the boundaries](https://viber-ops.github.io/en/docs/configra/security/).

![Configra: resource navigation, Vault entries, environment variants and field details](https://viber-ops.github.io/assets/configra/vault-light.png)

*Actual management UI with synthetic demo data. Sensitive values are hidden.*

## Use cases

- You maintain different configuration for development, staging and production.
- Several applications reference the same passwords, identifiers or certificate files.
- Developers need usable configuration; operators need change history and credential control.
- Some workloads use Go, while others only understand files or environment variables.

## Capabilities

| Need | Configra provides |
| --- | --- |
| One source of configuration | Environment-specific YAML/JSON, history, comparison and cloning |
| Shared sensitive values | Namespaced Vault Items with Text, Secret and File fields |
| Machine access | HTTPS, Environment-scoped Tokens, mTLS and revocation checks |
| Less certificate work | Managed client CAs, encrypted signing keys and one-time private exports |
| Go integration | Resolved reads, file downloads and Viper snapshots with ETag polling |
| Kubernetes integration | CSI file mounts **and** native Secret/ConfigMap synchronization |

For example, a Config can reference a Vault value without copying it:

```yaml
database:
  username: "{vault.platform.database.username}"
  password: "{vault.platform.database.password}"
```

Applications receive the final document for the requested environment. References
must occupy the complete scalar; File fields are read separately.

## Try the workspace

With Docker Compose, Go 1.25.13+, Node.js 24, npm, Make and OpenSSL installed:

```sh
git clone --branch v0.1.0-rc.1 https://github.com/viber-ops/configra.git
git clone --branch v0.1.0-rc.1 https://github.com/viber-ops/configra-go.git
cd configra
make local-run
```

Open **https://localhost:18088** and sign in with the local-only account
`admin` / `configra-admin` (or `viewer` / `configra-viewer`). This command prepares
development dependencies, certificates, OIDC and the UI. It starts Management;
the machine API is a separate service. The local certificate is self-signed.
**Do not expose the demo stack or use its credentials in production.**

[Local walkthrough](https://viber-ops.github.io/en/docs/configra/quickstart/) ·
[Download macOS / Linux binaries](https://viber-ops.github.io/en/docs/configra/installation/)

## Connect your applications

| Application reads | Use | When changes take effect |
| --- | --- | --- |
| Go values | [Go SDK](https://viber-ops.github.io/en/docs/configra/go-sdk/) | Your application validates and applies a new snapshot |
| Files in a Pod | [CSI provider](https://viber-ops.github.io/en/docs/configra/kubernetes/) | Driver rotation refreshes files; the application reloads |
| Native volume or envFrom | [Binding controller](https://viber-ops.github.io/en/docs/configra/kubernetes/) | Kubernetes refreshes volumes; environment variables require Pod replacement |

Configra itself can run in Kubernetes as separate Management and API Deployments.
It needs MySQL, NATS, ClickHouse, OIDC and external bootstrap secrets. It **does not
use its own provider to obtain its Master Key or startup credentials**.

[Deploy the service](https://viber-ops.github.io/en/docs/configra/deployment/) ·
[Manage client certificates](https://viber-ops.github.io/en/docs/configra/certificates/) ·
[Backup and recovery](https://viber-ops.github.io/en/docs/configra/operations/)

## Permissions and limits

Machine permissions are **Environment-wide**, and human Admin/Viewer roles are
workspace-wide. Vault Namespaces organize items; they do not isolate untrusted
tenants. Configra is not a dynamic database-credential engine, HSM/KMS, or an
application restart controller.

The current 1000 QPS gate did not pass on the shared test host. Some gateway
rejections are not durable Audit events, inventory APIs still need bounded
server-side pagination, and automatic Master Key rotation is not implemented.
See the [security and architecture review](https://viber-ops.github.io/en/docs/configra/security/)
before a production rollout.

## Develop and verify

Clone the SDK next to this repository for integration tests. The release builder
uses Go 1.26.7 and embeds the production web build.

```sh
go test -race ./...
go vet ./...
make web-test
make kubernetes-test
make test-integration
make image-test
make backup-test
make load-test
```

The container suites use MySQL 8.0.22. `make load-test` is an acceptance check, not
a claim that every environment meets its target. Maintainer-level protocol,
review and verification records are indexed in [maintainer documentation](docs/README.md).

## License

Original project source is licensed under [Apache-2.0](LICENSE). Third-party
components retain their own licenses and notices. See [CONTRIBUTING.md](CONTRIBUTING.md)
and [SECURITY.md](SECURITY.md) for contributions and private vulnerability reports.
