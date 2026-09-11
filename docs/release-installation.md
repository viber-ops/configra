# Configra release bundle

This bundle includes `configra` (Management and machine API),
`configra-kubernetes` (CSI provider and native binding controller), bootstrap
examples, Kubernetes manifests, and backup scripts. The web UI is embedded in
`configra`; Node.js is not needed to run a release binary.

Documentation: https://viber-ops.github.io/docs/configra/

## Verify and run

Download `SHA256SUMS` and the archive for your operating system and architecture
from the same GitHub release. On macOS use `shasum -a 256`; on Linux use
`sha256sum`. Compare the result with the matching line in `SHA256SUMS` before
extracting. Checksums detect corruption, not an independently compromised release
account. `BUILD.json` records the exact source commit and target.

```sh
./configra --version
./configra --help
./configra management --config deploy/management.example.yaml
# In another process, using the same database and Master Key:
./configra api --config deploy/api.example.yaml
```

Edit the example YAML and provision its referenced TLS files, OIDC client,
Master Key, MySQL 8.0.22 database, NATS, and ClickHouse first. The examples are not
a zero-configuration local demo. Follow the local-experience guide for that.

Use macOS arm64 for Apple Silicon, macOS amd64 for Intel, and the matching Linux
CPU architecture for servers. The Kubernetes CSI node deployment is Linux-only;
the macOS helper is for development/controller use, not a macOS CSI node runtime.
Release binaries are not Apple-signed or notarized. Do not disable Gatekeeper
globally; inspect and approve a trusted binary using macOS system controls, or
build from the tagged source under your organization's software policy.

## Preview status

Versions containing `-rc` are prereleases, not a completed production acceptance.
Review the documented security boundaries and run an environment-specific rollout,
backup/restore drill, and load test before production. No availability or throughput
SLA is claimed. Do not use Configra to supply its own bootstrap secrets.

## Container images

The manifests intentionally contain example registry names. Build the two images
from the tagged `configra` source and its adjacent `configra-go` checkout, push to
your registry, and replace images/API names/OIDC settings in your overlays:

```sh
docker build -t registry.example.com/configra:YOUR_VERSION .
docker build -f kubernetes/Dockerfile \
  -t registry.example.com/configra-kubernetes:YOUR_VERSION ..
```

No prebuilt container image is implied by the binary release.
