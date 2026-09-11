# Use a versioned machine-read HTTP contract

V1 exposes exact reads at `GET /v1/environments/{env}/configs/{config}` and `GET /v1/environments/{env}/vault-items/{namespace}/{item}/fields/{field}/content`. Resolved Config reads return a JSON envelope containing format, final content, Config Revision, and referenced Vault Item Revisions keyed as `<namespace>.<item>`, together with a standard composite ETag; File reads return raw bytes with filename, MIME type, and ETag. Content responses use `Cache-Control: no-store`; SDK Last-known-good storage remains process memory only.

Errors use stable machine codes and a server Request ID without values: invalid credentials return 401, a Token not authorized for the requested Environment returns 403, missing and Archived resources both return 404, unresolved Vault References return 422, rate limits return 429, and service or cryptographic-integrity faults return 5xx. A Resolved Config is all-or-nothing and never returns values from only the successfully decrypted references. V1 relies on Kubernetes Ingress limits, HTTP timeouts, and database connection-pool bounds instead of implementing shared cross-replica Token rate limiting.

V1 fixes the maximum canonical and Resolved Config size at 5 MiB, each Text or Secret Field at 512 KiB, each File Field at 5 MiB, and each current Vault Item Snapshot at 10 MiB.
