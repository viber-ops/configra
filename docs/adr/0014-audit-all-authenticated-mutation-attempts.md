# Audit all authenticated Mutation attempts

Every authenticated Mutation request produces an Audit Event with a success, no-change, conflict, or validation-failed outcome, while only successful state changes carry a resulting Revision. Unauthenticated failures remain security logs, giving operators a complete account of authorized write attempts without pretending that rejected operations changed domain history.
