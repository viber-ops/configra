# Contributing to Configra

Start with a reproducible problem or a concrete operator/developer workflow.
For a substantial interface, authorization or storage change, discuss the use
case in an issue before implementing it. Report vulnerabilities privately as
described in [SECURITY.md](SECURITY.md).

## Work locally

Clone `configra` and `configra-go` into adjacent directories. Use the documented
Go and Node versions, and keep credentials out of source control.

Until the preview PR is merged, use the preview tag from the README or the active
`feat/security-pki-kubernetes-ui` branch for the Kubernetes and managed-PKI work.
The default branch does not yet contain all preview code.

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

The required MySQL compatibility baseline remains **8.0.22**. Do not introduce
8.4-only SQL or relax a version/throughput assertion to make a test pass.
Performance changes must use the unchanged ten-minute gate in
[production-readiness.md](docs/production-readiness.md).

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
