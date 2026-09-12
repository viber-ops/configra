---
status: superseded by ADR-0011
---

# Persist detailed Access and Audit Events

Every read produces an Access Event and every mutation produces an Audit Event containing principal, resource key, and Revision metadata but never returned values. Mutations commit their Audit Event atomically, reads return only after their Access Event is durable, Audit Events are retained permanently, and Access Events default to 90-day retention; Configra accepts the additional write load and reduced availability to guarantee traceability.
