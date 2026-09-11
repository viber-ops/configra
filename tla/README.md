# Configra TLA+ models

The models verify state-machine decisions that are expensive to recover from after implementation:

- `Mutation` — optimistic concurrency, one result per OperationID, immutable Revision history, Archive behavior, and atomic Audit/Notification Outbox creation.
- `ConfigClone` — Clone reads one immutable current Source snapshot, never changes Source, creates only Target Revision 1, and rejects an existing Target.
- `MachineRead` — Token Environment grants, Namespace-independent authorization, exact `(Namespace, Item)` resolution and Revision evidence, conditional mTLS, presented-certificate rejection, archived/missing concealment, all-or-nothing resolution, ETags, and best-effort Access Events.
- `ClientWatch` — Last-known-good retention, immutable Snapshot installation, skipped intermediate Revisions, and serialized post-install callbacks that cannot roll back state.

Run all finite models with `make tla`. TLC is pinned and checksum-verified by the root Makefile. These models prove their stated finite-state invariants; executable contract, integration, race, fault, and load tests remain required.

`NotificationDelivery` checks bounded per-Destination attempts, terminal-state
stability, and the rule that an Outbox Event cannot complete before every target
has succeeded or become dead.

`BackupRestore` checks that a backup never contains the Master Key and a restored
target becomes readable only from a complete, checksum-valid snapshot verified by
the independently supplied key through the Crypto Sentinel.
