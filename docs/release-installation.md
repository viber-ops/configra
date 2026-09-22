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

`LICENSE` and `NOTICE` apply to original Configra source. The Kubernetes module
also carries its own copies. `licenses/go1.26.7/` retains reviewed Go runtime and
standard-library notices, including source-embedded terms; its `SOURCE.md`
identifies the exact source and scope. `licenses/configra-modules/` and
`licenses/configra-kubernetes-modules/` retain dependency texts and source-access
instructions; `licenses/ui/` contains the browser materials. The exact MySQL
driver source ZIP is included under the service's module materials.

`SBOM.cdx.json` combines both executables, linked modules, Go runtime and UI
evidence. `INVENTORY.json` records every delivered file except itself, including
the SBOM, with size, mode and SHA-256. Neither is an independent digital signature.
Native binaries use host-supplied system trust; their archives do not pretend to
include the host operating system's CA store.

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

## Deployment checks

v1.0.0 is the first stable release. Review the documented security boundaries and
run an environment-specific rollout, backup/restore drill and load test before
production. The tested hardware and Kubernetes topology do not establish an
availability or throughput SLA for another deployment. Do not use Configra to
supply its own bootstrap secrets. Older versions containing `-rc` remain prereleases.

When upgrading from rc.2, custom Management clients must follow inventory and
revision-history pagination. Machine read endpoints and SDK calls are unchanged.
Master Key rotation is offline and requires the maintenance procedure in
`deploy/backup/README.md`; do not just replace the bootstrap key.

## Container images

The manifests intentionally contain example registry names. Build the two images
from the tagged `configra` source, push to
your registry, and replace images/API names/OIDC settings in your overlays:

```sh
docker build -t registry.example.com/configra:YOUR_VERSION .
docker build -f kubernetes/Dockerfile \
  -t registry.example.com/configra-kubernetes:YOUR_VERSION .
```

No prebuilt container image is implied by the binary release.

Images place their materials under `/licenses/configra/` or
`/licenses/configra-kubernetes/`. Their system CA collection includes the exact
Debian source archive, copyright record, MPL/GPL texts, selection configuration
and runtime-bundle hash. GPL terms apply to retained Debian script sources, not
to Configra's separate original code. The image's aggregate SBOM and inventory
live at the same license root. Build tools are absent from the scratch payload.

From the source checkout, `make image-license-test` builds both images and checks
their full exported payload, project/dependency/CA notice files, actual binary
targets, component-evidence relationships, the pinned SDK, and
`--version` under a non-root, read-only, network-disabled container. It creates
and removes only its inspection containers; it does not start dependencies or
prove production startup, recovery, or full third-party license compliance.
