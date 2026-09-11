# Defer Local Master Key rotation

V1 LocalKeyProvider reads exactly one base64-encoded 32-byte Master Key from the Secret-mounted file named by Cold-start Configuration; plaintext is never embedded in YAML, an environment variable, or SQL. Management Server creates the encrypted Crypto Sentinel only for a confirmed new database, while every later Management and API Server startup refuses to run when the Sentinel is missing or cannot be decrypted. Replacing the Master Key is unsupported because it would strand historical Vault Revisions; multi-key Keyring support and DEK rewrap are deferred until online rotation is required.

MySQL and the Master Key are backed up separately by deployment infrastructure rather than Configra. A read-only `configra doctor --verify-vault` command validates the Sentinel and authenticates every Vault Revision without printing values; losing the original Master Key makes Vault data unrecoverable.
