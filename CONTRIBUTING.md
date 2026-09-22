# Contributing to Configra

Start with a reproducible problem or a concrete operator/developer workflow.
For a substantial interface, authorization or storage change, discuss the use
case in an issue before implementing it. Report vulnerabilities privately as
described in [SECURITY.md](SECURITY.md).

Use the [documentation index](docs/README.md) for design, current acceptance status
and historical evidence. Keep one current status record; place related verification
runs together under `docs/verification/`. Generated screenshots stay in `.cache/`.

## Work locally

Clone `configra` and `configra-go` into adjacent directories. Use the documented
Go and Node versions, and keep credentials out of source control.
Container/service checks also need Docker with Compose, curl, OpenSSL and
`unzip` on the host. Go race tests need a C compiler (GCC or Clang).

Use `main` for development or the README's tag to reproduce a released version.
Kubernetes and managed-PKI code are included in both. The sibling SDK checkout is
needed by `e2e` and `make service-test`; the Kubernetes module uses its
checksum-pinned public SDK.

```sh
go test -race ./...
go vet ./...
make web-test
make kubernetes-test
```

Storage, protocol, authentication and migration changes also need the relevant
real dependency tests (`make test-integration`, `make image-test`,
`make backup-test`). These start disposable local containers and create test
databases. Never point their DSNs or kubeconfig at a live deployment.

`make service-test` builds the source image and runs Management and API containers
with real HTTPS Casdoor, MySQL 8.0.22, NATS and ClickHouse. It checks authentication,
dependency stalls, durable Audit recovery, backup/restore, offline Master Key
rotation and the sibling SDK example against the recovered API.
Keep local fixture ports 33079, 42229, 8229, 8129, 9009 and 18080/18088/18089 free.
Each run owns a new project and removes its containers/tmpfs data in cleanup;
private diagnostics stay under `.cache/service-smoke-*`. This local development
fixture disables native AIO and is not the production capacity or Kubernetes gate.

`make kubernetes-service-test` uses the same real dependencies but runs Configra
Management/API, sync controllers and CSI provider in a new three-node kind
cluster (one control plane and two workers). Install
kind, kubectl, curl and OpenSSL first (set `CONFIGRA_TEST_KIND` if kind is outside
`PATH`), and have `busybox:1.37.0` available locally. It uses Kubernetes 1.35.0
and the fixed v1.6.1 CSI Driver source, downloaded over HTTPS. The dedicated
kubeconfig never replaces your current context. Driver images are pulled once
on the Docker host and imported into each node before the driver starts. The test also stops every service
Pod, backs up MySQL, runs offline Master Key rotation in hardened maintenance Pods, verifies
both keys' outcomes, replaces the fixture Secret, and checks restarted SDK,
CA/Audit and fresh native/CSI delivery. The generated keys are test-only.
It also pauses the worker holding the sync leader and checks service endpoint
removal, cross-node leadership, native/CSI updates and a fresh mount on the
surviving worker. Normal cleanup resumes that worker before removing the cluster.
All cluster resources and fixture databases are disposable; normal success/failure
cleanup removes the named cluster and Compose project. A hard kill may require deleting those
printed names manually. Do not run this and `service-test` concurrently: they
use the same loopback fixture ports. Neither is a production load test.

The required MySQL compatibility baseline remains **8.0.22**. Do not introduce
8.4-only SQL or relax a version/throughput assertion to make a test pass.
Performance changes must use the unchanged ten-minute gate in
[production-readiness.md](docs/production-readiness.md).
On a Mac, keep the lid open and connect power throughout the run. From the
repository root, `caffeinate -i make load-test` prevents idle sleep only while
the command runs; it does not change persistent power settings. Do not disable
thermal protection. Keep sleep-interrupted failures in the record, check
`pmset -g log`, and repeat before drawing capacity conclusions. Run builds and
other test suites separately from the load gate.

The CI workflows run on pull requests, `main` updates and a weekly schedule.
They cover race tests, vet, module integrity and reachable vulnerabilities. Service
CI also runs the real-dependency integration suite and UI regressions; its SDK
fixture is pinned in the workflow. Dependency updates remain reviewable PRs,
not automatic merges: update the reviewed license manifest when shipped imports
change. These checks do not replace the production-image and capacity gates.

Manual CI adds `all`, `capacity` or `kubernetes` acceptance modes. Use separate
runs when capacity and cluster checks need different hosts; selecting one does
not count the other as passed. A release needs both sets of evidence, tied to
its runtime source and image. Failed capacity runs report bounded metadata, not
raw HTTP errors or private diagnostics. `all` still fails if either gate fails.

## Submit a change

- Explain the user problem, behavior change and compatibility impact.
- Add regression coverage through the interface a caller actually uses.
- List the exact checks run and any skipped checks, including the reason.
- Run `gofmt` on changed Go files and `git diff --check` before submitting.
- Keep errors useful without exposing Tokens, keys, URLs with credentials or
  secret-bearing request/response values.
- Update user-facing documentation in Chinese and English. Execute the changed
  commands from a clean setup; record prerequisites, expected results and limits.
- Preserve third-party copyrights and licenses. Do not contribute material you
  are not authorized to publish.

Original contributions intentionally submitted for inclusion are under
[Apache-2.0](LICENSE), as described in section 5 of the license. Third-party
components retain their original terms. No separate contributor agreement is
currently required.

Keep discussion respectful and focused on the work. Do not harass people or
publish private information. Use GitHub's reporting tools for abuse rather than
escalating a public argument.
