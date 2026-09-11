# Configra

Configra is a V1 configuration and sensitive-value service: MySQL 8.0.22 stores
transactional state, ClickHouse stores Access/Audit logs, and machine clients read
resolved YAML/JSON or File fields over HTTPS with an Environment-scoped Token and,
by default, mTLS.

Vault Items use the composite identity `(namespace, item)` and Config references use
exactly four segments, for example `{vault.platform.mysql.password}`. Namespace is
organizational identity only; Token authorization remains Environment-scoped.

## Verify

Go 1.25.13 or newer is required; the production image uses the digest-pinned
Go 1.26.7 builder.

The integration and load suites use the sibling SDK checkout through `e2e/go.mod`.
Clone both repositories into the same parent directory before running those suites:

```sh
git clone https://github.com/viber-ops/configra.git
git clone https://github.com/viber-ops/configra-go.git
cd configra
```

```sh
make tla
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
make web-test
make test-integration
make image-test
make backup-test
make load-test
```

Container tests pin and query-check MySQL `8.0.22`. The ten-minute load report is
written to `.cache/load/latest.json`.

## Run and deploy

For the local development stack, run `make local-run`, then open
`https://localhost:18088`. The browser is redirected to the real Casdoor container at
`http://localhost:18080`; use `admin` / `configra-admin` or
`viewer` / `configra-viewer`. The local TLS certificate is self-signed.

- Product and protocol: [docs/design.md](docs/design.md)
- Release gates and current evidence: [docs/production-readiness.md](docs/production-readiness.md)
- Migration review and validation: [docs/migration-review.md](docs/migration-review.md)
- Security and architecture review: [docs/security-architecture-review.md](docs/security-architecture-review.md)
- Managed client certificates: [docs/managed-certificates.md](docs/managed-certificates.md)
- Kubernetes workload provider and synchronization: [kubernetes/README.md](kubernetes/README.md)
- Management bootstrap: [deploy/management.example.yaml](deploy/management.example.yaml)
- API bootstrap: [deploy/api.example.yaml](deploy/api.example.yaml)
- Kubernetes: [deploy/kubernetes/README.md](deploy/kubernetes/README.md)
- Backup and restore: [deploy/backup/README.md](deploy/backup/README.md)

The Go SDK and Viper Handler live in the sibling `configra-go` module.
