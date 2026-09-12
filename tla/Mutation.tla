----------------------------- MODULE Mutation -----------------------------
EXTENDS Integers, Sequences, FiniteSets

CONSTANTS Environments, Values, OperationIDs, InitialValue

ASSUME /\ Environments # {}
       /\ OperationIDs # {}
       /\ InitialValue \in Values

MutationKinds == {"edit", "merge", "replace", "restore", "clone"}
LifecycleKinds == {"archive", "unarchive"}
OperationKinds == MutationKinds \cup LifecycleKinds
Outcomes == {"success", "no_change", "conflict", "archived"}
MaxRevision == Cardinality(OperationIDs) + 1

VARIABLES history,
          archived,
          results,
          pendingAudit,
          storedAudit,
          pendingNotification,
          deliveredNotification

vars == <<history, archived, results, pendingAudit, storedAudit,
          pendingNotification, deliveredNotification>>

CurrentRevision(environment) == Len(history[environment])
CurrentValue(environment) == history[environment][CurrentRevision(environment)]
UsedOperationIDs == {result.id : result \in results}

Result(operation, kind, target, expected, value, outcome, revision) ==
    [id |-> operation,
     kind |-> kind,
     target |-> target,
     expected |-> expected,
     value |-> value,
     outcome |-> outcome,
     revision |-> revision]

Init ==
    /\ history = [environment \in Environments |-> <<InitialValue>>]
    /\ archived = {}
    /\ results = {}
    /\ pendingAudit = {}
    /\ storedAudit = {}
    /\ pendingNotification = {}
    /\ deliveredNotification = {}

ChangedMutation ==
    \E operation \in OperationIDs,
       kind \in MutationKinds,
       target \in Environments,
       expected \in 0..MaxRevision,
       newValue \in Values:
        /\ operation \notin UsedOperationIDs
        /\ target \notin archived
        /\ expected = CurrentRevision(target)
        /\ newValue # CurrentValue(target)
        /\ history' = [history EXCEPT ![target] = Append(@, newValue)]
        /\ results' = results \cup
             {Result(operation, kind, target, expected, newValue,
                     "success", CurrentRevision(target) + 1)}
        /\ pendingAudit' = pendingAudit \cup {operation}
        /\ pendingNotification' = pendingNotification \cup {operation}
        /\ UNCHANGED <<archived, storedAudit, deliveredNotification>>

NoChangeMutation ==
    \E operation \in OperationIDs,
       kind \in MutationKinds,
       target \in Environments,
       expected \in 0..MaxRevision:
        /\ operation \notin UsedOperationIDs
        /\ target \notin archived
        /\ expected = CurrentRevision(target)
        /\ results' = results \cup
             {Result(operation, kind, target, expected, CurrentValue(target),
                     "no_change", 0)}
        /\ pendingAudit' = pendingAudit \cup {operation}
        /\ UNCHANGED <<history, archived, storedAudit,
                        pendingNotification, deliveredNotification>>

ConflictMutation ==
    \E operation \in OperationIDs,
       kind \in MutationKinds,
       target \in Environments,
       expected \in 0..MaxRevision,
       newValue \in Values:
        /\ operation \notin UsedOperationIDs
        /\ target \notin archived
        /\ expected # CurrentRevision(target)
        /\ results' = results \cup
             {Result(operation, kind, target, expected, newValue,
                     "conflict", 0)}
        /\ pendingAudit' = pendingAudit \cup {operation}
        /\ UNCHANGED <<history, archived, storedAudit,
                        pendingNotification, deliveredNotification>>

ArchivedMutation ==
    \E operation \in OperationIDs,
       kind \in MutationKinds,
       target \in archived,
       expected \in 0..MaxRevision,
       newValue \in Values:
        /\ operation \notin UsedOperationIDs
        /\ results' = results \cup
             {Result(operation, kind, target, expected, newValue,
                     "archived", 0)}
        /\ pendingAudit' = pendingAudit \cup {operation}
        /\ UNCHANGED <<history, archived, storedAudit,
                        pendingNotification, deliveredNotification>>

Archive ==
    \E operation \in OperationIDs,
       target \in Environments:
        /\ operation \notin UsedOperationIDs
        /\ target \notin archived
        /\ archived' = archived \cup {target}
        /\ results' = results \cup
             {Result(operation, "archive", target, CurrentRevision(target),
                     CurrentValue(target), "success", 0)}
        /\ pendingAudit' = pendingAudit \cup {operation}
        /\ pendingNotification' = pendingNotification \cup {operation}
        /\ UNCHANGED <<history, storedAudit, deliveredNotification>>

Unarchive ==
    \E operation \in OperationIDs,
       target \in archived:
        /\ operation \notin UsedOperationIDs
        /\ archived' = archived \ {target}
        /\ results' = results \cup
             {Result(operation, "unarchive", target, CurrentRevision(target),
                     CurrentValue(target), "success", 0)}
        /\ pendingAudit' = pendingAudit \cup {operation}
        /\ pendingNotification' = pendingNotification \cup {operation}
        /\ UNCHANGED <<history, storedAudit, deliveredNotification>>

DeliverAudit ==
    \E operation \in pendingAudit:
        /\ pendingAudit' = pendingAudit \ {operation}
        /\ storedAudit' = storedAudit \cup {operation}
        /\ UNCHANGED <<history, archived, results,
                        pendingNotification, deliveredNotification>>

DeliverNotification ==
    \E operation \in pendingNotification:
        /\ pendingNotification' = pendingNotification \ {operation}
        /\ deliveredNotification' = deliveredNotification \cup {operation}
        /\ UNCHANGED <<history, archived, results,
                        pendingAudit, storedAudit>>

Next == ChangedMutation
     \/ NoChangeMutation
     \/ ConflictMutation
     \/ ArchivedMutation
     \/ Archive
     \/ Unarchive
     \/ DeliverAudit
     \/ DeliverNotification

TypeOK ==
    /\ history \in [Environments -> Seq(Values)]
    /\ \A environment \in Environments: Len(history[environment]) >= 1
    /\ archived \subseteq Environments
    /\ results \subseteq
         [id : OperationIDs,
          kind : OperationKinds,
          target : Environments,
          expected : 0..MaxRevision,
          value : Values,
          outcome : Outcomes,
          revision : 0..MaxRevision]
    /\ pendingAudit \subseteq OperationIDs
    /\ storedAudit \subseteq OperationIDs
    /\ pendingNotification \subseteq OperationIDs
    /\ deliveredNotification \subseteq OperationIDs

OneResultPerOperation ==
    \A left \in results, right \in results:
        left.id = right.id => left = right

AuditCreatedAtomically ==
    /\ UsedOperationIDs = pendingAudit \cup storedAudit
    /\ pendingAudit \cap storedAudit = {}

NotifiableResults ==
    {result \in results :
       /\ result.outcome = "success"
       /\ (result.revision > 0 \/ result.kind \in LifecycleKinds)}

NotifiableOperationIDs ==
    {result.id : result \in NotifiableResults}

NotificationCreatedAtomically ==
    /\ NotifiableOperationIDs = pendingNotification \cup deliveredNotification
    /\ pendingNotification \cap deliveredNotification = {}

RevisionEvidence ==
    /\ \A result \in results:
          result.revision > 0 =>
              /\ result.outcome = "success"
              /\ result.revision <= Len(history[result.target])
              /\ history[result.target][result.revision] = result.value
    /\ \A environment \in Environments:
          \A revision \in 2..Len(history[environment]):
              \E result \in results:
                  /\ result.target = environment
                  /\ result.revision = revision
                  /\ result.value = history[environment][revision]

LifecycleDoesNotCreateContentRevision ==
    \A result \in results:
        result.kind \in LifecycleKinds => result.revision = 0

AuditEventuallyStored ==
    \A operation \in OperationIDs:
        (operation \in UsedOperationIDs) ~> (operation \in storedAudit)

Spec == Init /\ [][Next]_vars /\ WF_vars(DeliverAudit)

=============================================================================
