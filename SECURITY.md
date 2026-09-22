# Security policy

## Report privately

Use [GitHub private vulnerability reporting](https://github.com/viber-ops/configra/security/advisories/new).
For SDK-only issues, use the [SDK's private reporting channel](https://github.com/viber-ops/configra-go/security/advisories/new).

Do not put exploitable details, live Tokens, private keys, database dumps or real
configuration values in a public issue. Include the affected release/commit,
deployment mode, impact and a minimal reproduction using synthetic data.
If the private reporting form is unavailable, open a public issue asking for a
private contact **without disclosing the vulnerability**.

## Supported releases

The supported stable line starts at v1.0.1. Use its latest patch release when
reporting an issue. Older `-rc` versions are superseded by the stable release.
Security fixes are published in new versions; existing tags are not rewritten.
We do not promise a fixed response or remediation SLA.

MySQL 8.0.22 remains an application-compatibility requirement. That does not
extend the database vendor's security support. Operators remain responsible for
database isolation, credential management, patch policy and backup access.

## Security model

- Machine Tokens grant Environment-wide access; Vault Namespaces and Kubernetes
  namespaces are not narrower server-side grants.
- Human Admin/Viewer roles are workspace-wide. Untrusted tenants need separate
  trust domains, not naming conventions.
- API HTTPS verification and credential authorization must remain enabled.
- Master Keys and service bootstrap credentials stay outside Configra and must be
  backed up separately from encrypted database state.
- Revocation prevents later reads. It cannot erase values already delivered to
  applications, caches, files or Kubernetes objects.

See [the review and open findings](docs/security-architecture-review.md) and
[the production acceptance contract](docs/production-readiness.md).
