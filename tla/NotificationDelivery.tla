---------------------- MODULE NotificationDelivery ----------------------
EXTENDS Integers, FiniteSets

CONSTANTS Destinations, MaxAttempts

ASSUME /\ Destinations # {}
       /\ MaxAttempts \in Nat
       /\ MaxAttempts > 0

TargetStates == {"pending", "sending", "succeeded", "dead"}
TerminalStates == {"succeeded", "dead"}

VARIABLES targetStatus,
          attempts,
          outboxCompleted

vars == <<targetStatus, attempts, outboxCompleted>>

Init ==
    /\ targetStatus = [destination \in Destinations |-> "pending"]
    /\ attempts = [destination \in Destinations |-> 0]
    /\ outboxCompleted = FALSE

Send(destination) ==
    /\ ~outboxCompleted
    /\ targetStatus[destination] = "pending"
    /\ attempts[destination] < MaxAttempts
    /\ targetStatus' = [targetStatus EXCEPT ![destination] = "sending"]
    /\ attempts' = [attempts EXCEPT ![destination] = @ + 1]
    /\ UNCHANGED outboxCompleted

Succeeded(destination) ==
    /\ targetStatus[destination] = "sending"
    /\ targetStatus' = [targetStatus EXCEPT ![destination] = "succeeded"]
    /\ UNCHANGED <<attempts, outboxCompleted>>

RetryableFailure(destination) ==
    /\ targetStatus[destination] = "sending"
    /\ attempts[destination] < MaxAttempts
    /\ targetStatus' = [targetStatus EXCEPT ![destination] = "pending"]
    /\ UNCHANGED <<attempts, outboxCompleted>>

RetryLimitReached(destination) ==
    /\ targetStatus[destination] = "sending"
    /\ attempts[destination] = MaxAttempts
    /\ targetStatus' = [targetStatus EXCEPT ![destination] = "dead"]
    /\ UNCHANGED <<attempts, outboxCompleted>>

TerminalFailure(destination) ==
    /\ targetStatus[destination] \in {"pending", "sending"}
    /\ targetStatus' = [targetStatus EXCEPT ![destination] = "dead"]
    /\ UNCHANGED <<attempts, outboxCompleted>>

FinishOutbox ==
    /\ ~outboxCompleted
    /\ \A destination \in Destinations:
          targetStatus[destination] \in TerminalStates
    /\ outboxCompleted' = TRUE
    /\ UNCHANGED <<targetStatus, attempts>>

Next ==
    \/ \E destination \in Destinations: Send(destination)
    \/ \E destination \in Destinations: Succeeded(destination)
    \/ \E destination \in Destinations: RetryableFailure(destination)
    \/ \E destination \in Destinations: RetryLimitReached(destination)
    \/ \E destination \in Destinations: TerminalFailure(destination)
    \/ FinishOutbox

TypeOK ==
    /\ targetStatus \in [Destinations -> TargetStates]
    /\ attempts \in [Destinations -> 0..MaxAttempts]
    /\ outboxCompleted \in BOOLEAN

PendingHasRetryBudget ==
    \A destination \in Destinations:
        targetStatus[destination] = "pending" =>
            attempts[destination] < MaxAttempts

CompletedOnlyAfterAllTargetsTerminal ==
    outboxCompleted =>
        \A destination \in Destinations:
            targetStatus[destination] \in TerminalStates

TerminalTargetsNeverChange ==
    \A destination \in Destinations:
        targetStatus[destination] \in TerminalStates =>
            targetStatus'[destination] = targetStatus[destination]

Spec == Init /\ [][Next]_vars

=============================================================================
