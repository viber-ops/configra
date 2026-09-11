# Configra

Configra manages versioned application configuration and sensitive values for machine clients across explicit runtime contexts.

## Language

**Environment**:
A peer context that selects Config and Vault values. It is neither a lifecycle stage nor a tenant or security boundary.
_Avoid_: Stage, tenant

**API Token**:
A machine credential formatted as `cfg_<public-id>_<secret>` whose editable, possibly empty allowlist explicitly names every Environment it may access and includes all Config and Vault values in those Environments. Its plaintext 256-bit random Secret is shown once and only its hash is stored; at creation its immutable `allow_without_mtls` flag defaults to false but may explicitly permit Token-only Authentication. Allowlist changes take effect immediately and are audited, the Token defaults to 90-day expiry, may explicitly be non-expiring in either authentication mode, and cannot be restored after revocation.
_Avoid_: Global token

**Management Server**:
The human-facing Configra deployment that serves OIDC-authenticated management and mutations.
_Avoid_: System Settings, control plane

**Management UI**:
The English-and-Chinese human console for Environments, Configs, Vault, Notifications, Access, Audit, and Administration. It exposes Cold-start Configuration only as read-only deployment status and presents Environments as peers rather than stages.
_Avoid_: Cold-start configuration editor, stage pipeline

**API Server**:
The HTTPS-facing Configra deployment that serves exact-key reads of Resolved Configs and File Fields to Machine Clients. It supports Client Certificate authentication and explicitly permitted Token-only Authentication, but does not serve raw Configs, generic Vault Field reads, or resource listing and search.
_Avoid_: Machine API, Client API

**Resource Key**:
An immutable lowercase machine identifier for an Environment, Config, Vault Namespace, Vault Item, or Field. It matches `[a-z][a-z0-9_-]{0,62}`; `.` is reserved as the Vault Reference separator. Environment Keys participate in authorization; Vault Namespace and Item Keys identify values but do not grant access.
_Avoid_: Display name, mutable name

**Vault Namespace**:
The first immutable identity segment of a Vault Item, used only to organize and disambiguate Items. It has no separate record, Display Name, permissions, Revision history, Archive lifecycle, or management page; access remains determined exclusively by the requested Environment and API Token grant.
_Avoid_: Tenant, permission scope, folder resource

**Display Name**:
A mutable human-readable label that never identifies a resource in a client contract.
_Avoid_: Resource key

**Config**:
A named configuration that may exist in multiple Environments, with an independent immutable Revision history in each Environment. Revision numbers are Environment-local and imply no equality or ordering across Environments.
_Avoid_: Global revision stream

**Canonical Config Text**:
The validated and formatted YAML or JSON source stored as the authoritative content of a Config Revision. Comments and key ordering are preserved, but the caller's original whitespace and quoting are not authoritative.
_Avoid_: Raw submission

**Revision**:
An immutable snapshot of a Config in one Environment or of an entire Vault Item. An operation that produces no state change produces no Revision.
_Avoid_: Mutable version

**Target Revision**:
The immutable Target snapshot selected for a Merge or Replace; it may be current or historical. The resulting snapshot becomes the Target Environment's new current Revision.
_Avoid_: Expected Target Revision, shared Environment version

**Vault Item**:
A 1Password-like versioned record identified by the composite `(Vault Namespace Key, Item Resource Key)`, with shared Field definitions and one or more environment-specific Variants. The same Item Resource Key may exist in different Vault Namespaces without sharing values or Revision history.
_Avoid_: Key, secret map

**Encrypted Vault Snapshot**:
The complete value-bearing state of one Vault Item Revision encrypted as one AES-256-GCM Blob under a fresh 256-bit DEK. Resource structure remains queryable metadata, while Field values and File filename, MIME type, and bytes are encrypted; authenticated additional data binds the Blob to its Item, Revision, algorithm, and Key Version.
_Avoid_: Per-Field encryption, plaintext value cache

**Field**:
A named and typed value definition shared by every Variant of a Vault Item. Its Resource Key and type are immutable.
_Avoid_: Key

**Variant**:
A complete set of values for a Vault Item's Fields, bound to one or more Environments. One Environment belongs to at most one Variant in the same Vault Item.
_Avoid_: Environment stage

**Vault Reference**:
A full-scalar Config placeholder such as `{vault.platform.mysql.password}` that identifies a Vault Namespace, Item, and Field while inheriting the Config's Environment. Exactly four segments are required; there is no implicit Namespace or legacy three-segment form. It resolves against the current Vault Item Revision rather than pinning a historical Revision; doubled braces such as `{{vault.platform.mysql.password}}` produce the literal single-braced text. Management displays a copyable canonical reference beside each Text or Secret Field for direct use as a YAML/JSON scalar; File Fields instead point users to the Client bytes API.
_Avoid_: Version-pinned secret

**Resolved Config**:
A Config whose Vault References have been replaced from the matching current Variants within one consistent read. Its identity includes the Config Revision and every Vault Item Revision used, because either can change its effective content.
_Avoid_: Config Revision

**Merge**:
A directional two-way overlay from a read-only Source Revision into a selected Target Revision that preserves Target-only content.
_Avoid_: Promotion, upgrade, replace

**Replace**:
A directional operation that creates a new Target Revision equal to the read-only Source Revision, removing content from the selected Target Revision that is absent from Source.
_Avoid_: Merge

**Machine Client**:
A configuration consumer authenticated with an API Token authorized for the requested Environment and, unless that Token explicitly permits Token-only Authentication, an accepted mTLS Client Certificate.
_Avoid_: Human session

**Client Certificate**:
An mTLS identity presented by a Machine Client and independently accepted or revoked by Configra. It never replaces the API Token, although a Token may explicitly permit a request that presents no Client Certificate.
_Avoid_: API Token

**Managed Certificate Authority**:
A Configra-managed issuer of Client Certificates whose signing key is retained securely for continued issuance. Revoking the Authority also invalidates every Client Certificate issued by it.
_Avoid_: Server TLS certificate, API Token issuer

**Credential Export**:
The one-time delivery of a newly generated Authority or Client Certificate's private key to its administrator. A completed export cannot be replayed; public certificates remain available for trust distribution.
_Avoid_: Recoverable private-key download

**Kubernetes Binding**:
A mapping from selected Configra resources to mounted files or native configuration objects in one Kubernetes namespace. It uses explicitly supplied Machine Client credentials and does not extend their Environment grants.
_Avoid_: Global cluster credential, ConfigMap storage backend

**Token-only Authentication**:
A request on the shared HTTPS API Server listener that presents no Client Certificate and is accepted only because its Active, unexpired API Token was created with `allow_without_mtls=true` and authorizes the requested Environment. Presented certificates are never ignored: an invalid, unregistered, or revoked certificate rejects the request, while a non-expiring Token-only Token is explicitly allowed.
_Avoid_: Plain HTTP, global mTLS disablement

**File Field**:
A small binary value stored in a Vault Item and retrieved as bytes through the Client rather than substituted into Config text.
_Avoid_: Base64 Secret Field

**Last-known-good**:
The most recent Config successfully fetched by a running Machine Client and retained only in that process for fallback.
_Avoid_: Persistent cache

**Config Watch**:
The single explicitly started Client loop for one Handler, stopped by its Context, that conditionally checks the API Server with the current composite ETag and atomically installs only a successfully parsed changed Resolved Config. It defaults to a 30-second interval with jitter, rejects intervals below 5 seconds, backs repeated failures off to at most 5 minutes, may skip intermediate Revisions, and retains the Last-known-good on failure.
_Avoid_: Server push, guaranteed revision stream

**Change Callback**:
A serialized post-install notification containing the previous and current read-only Viper Snapshots and their version identities. Callers may read or unmarshal them but must not mutate them; initial load, unchanged checks, and failed reloads do not invoke it, and callback failure is reported but cannot roll back an installed Snapshot.
_Avoid_: Transactional apply hook

**Archived Resource**:
A restorable Environment, Config, Vault Item, or Field retained for identity, history, and audit purposes but unavailable for new reads, resolution, or mutation. Its complete identity is never reused; for a Vault Item this means the `(Vault Namespace Key, Item Resource Key)` pair.
_Avoid_: Deleted resource

**OperationID**:
A caller-supplied identity for one Mutation, reusable only with the same request content to recover its original result.
_Avoid_: Request ID

**Cold-start Configuration**:
Configuration that Configra itself needs before it can operate. It always exists outside the Managed Configuration of the same Configra deployment.
_Avoid_: Self-hosted configuration

**Crypto Sentinel**:
A fixed encrypted record created by Management Server for a new database and verified by every Management and API Server startup. Missing or failed verification in an initialized database is fatal rather than treated as a new installation.
_Avoid_: Master Key fingerprint

**Managed Configuration**:
Application or business configuration stored and versioned by Configra for its consumers.
_Avoid_: Cold-start configuration, system configuration

**Notification Destination**:
An administrator-managed outbound target that subscribes to successful Mutation types. V1 supports Generic Webhook and Feishu Bot destinations; its provider-supplied URL and optional Secret are encrypted, editable credentials rather than credentials issued by Configra.
_Avoid_: Webhook receiver, generated signing secret

**Generic Webhook**:
A Notification Destination that receives Configra's value-free Event envelope and may authenticate it with an optional administrator-supplied HMAC Secret.
_Avoid_: Feishu payload

**Feishu Bot**:
A Notification Destination that renders a value-free Configra Mutation summary into Feishu's provider-specific message and optional signing protocol.
_Avoid_: Generic Webhook payload

**Notification Delivery**:
One persisted at-least-once attempt stream from a transactional Outbox Event to a Notification Destination. Deliveries may duplicate or arrive out of order; retries use the Destination's latest credentials and never persist response bodies or credential-bearing URLs.
_Avoid_: Exactly-once event, ordered event stream

**Access Event**:
An immutable metadata record emitted when API Server or Management Server returns managed content, identifying the machine or human principal, authentication context, resource, and version without recording returned values. Lists and metadata-only Management views, a `304 Not Modified` Config Watch check, and local Last-known-good use are metrics only, not Access Events.
_Avoid_: Access statistic

**Observed Request**:
An API Server request represented by a persisted Access Event. It is an operational observation rather than a complete count of actual requests.
_Avoid_: Audited request, total request

**Audit Event**:
An immutable metadata record of an authenticated Mutation attempt, identifying the principal, resource, outcome, and resulting Revision when one exists, without recording values.
_Avoid_: Access event
