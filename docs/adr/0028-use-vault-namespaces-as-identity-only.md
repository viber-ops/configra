# Use Vault Namespaces as identity only

A Vault Item is identified by the immutable pair `(namespace_key, resource_key)`, and references use exactly `{vault.<namespace>.<item>.<field>}`. The same Item Resource Key may exist in different Namespaces with independent values and Revision histories. MySQL enforces uniqueness on the pair; APIs, logs, notifications, UI routes, revision evidence, ETags, and File reads preserve both segments. Because Configra has not been released, V1 accepts no legacy three-segment reference, implicit default Namespace, or compatibility route.

A Namespace is deliberately not a managed resource. It has no table, Display Name, ACL, Revision, Archive lifecycle, or management page. The UI derives Namespace choices from existing Items and allows a new valid key during Item creation. API Token authorization remains exclusively the explicit Environment allowlist, so adding a Namespace cannot silently create or weaken a security boundary.
