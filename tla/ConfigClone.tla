--------------------------- MODULE ConfigClone ---------------------------
EXTENDS Integers, Sequences, FiniteSets

CONSTANTS Configs, Values, OperationIDs,
          ExistingA, ExistingB, EmptyConfig,
          InitialA1, InitialA2, InitialB

ASSUME /\ Configs = {ExistingA, ExistingB, EmptyConfig}
       /\ ExistingA # ExistingB
       /\ ExistingA # EmptyConfig
       /\ ExistingB # EmptyConfig
       /\ InitialA1 \in Values
       /\ InitialA2 \in Values
       /\ InitialB \in Values
       /\ OperationIDs # {}

MaxRevision == Cardinality(OperationIDs) + 2
Outcomes == {"success", "conflict"}
Kinds == {"edit", "clone"}

VARIABLES histories, results

vars == <<histories, results>>

UsedOperationIDs == {result.id : result \in results}
Current(config) == histories[config][Len(histories[config])]

Result(operation, kind, source, sourceRevision, target,
       targetRevision, value, outcome) ==
    [id |-> operation,
     kind |-> kind,
     source |-> source,
     sourceRevision |-> sourceRevision,
     target |-> target,
     targetRevision |-> targetRevision,
     value |-> value,
     outcome |-> outcome]

Init ==
    /\ histories = [config \in Configs |->
          CASE config = ExistingA -> <<InitialA1, InitialA2>>
            [] config = ExistingB -> <<InitialB>>
            [] config = EmptyConfig -> <<>>]
    /\ results = {}

Edit ==
    \E operation \in OperationIDs,
       target \in Configs,
       value \in Values:
        /\ operation \notin UsedOperationIDs
        /\ Len(histories[target]) \in 1..(MaxRevision - 1)
        /\ value # Current(target)
        /\ histories' = [histories EXCEPT ![target] = Append(@, value)]
        /\ results' = results \cup
             {Result(operation, "edit", target, Len(histories[target]),
                     target, Len(histories[target]) + 1, value, "success")}

Clone ==
    \E operation \in OperationIDs,
       source \in Configs,
       target \in Configs:
        /\ operation \notin UsedOperationIDs
        /\ source # target
        /\ Len(histories[source]) > 0
        /\ Len(histories[target]) = 0
        /\ histories' = [histories EXCEPT ![target] = <<Current(source)>>]
        /\ results' = results \cup
             {Result(operation, "clone", source, Len(histories[source]),
                     target, 1, Current(source), "success")}

CloneConflict ==
    \E operation \in OperationIDs,
       source \in Configs,
       target \in Configs:
        /\ operation \notin UsedOperationIDs
        /\ source # target
        /\ Len(histories[source]) > 0
        /\ Len(histories[target]) > 0
        /\ histories' = histories
        /\ results' = results \cup
             {Result(operation, "clone", source, Len(histories[source]),
                     target, 0, Current(source), "conflict")}

Next == Edit \/ Clone \/ CloneConflict

HistoryDomainOK == DOMAIN histories = Configs

HistoryValuesOK ==
    \A config \in Configs: histories[config] \in Seq(Values)

HistoryLengthsOK ==
    \A config \in Configs: Len(histories[config]) <= MaxRevision

ResultsOK ==
    results \subseteq
         [id : OperationIDs,
          kind : Kinds,
          source : Configs,
          sourceRevision : 1..MaxRevision,
          target : Configs,
          targetRevision : 0..MaxRevision,
          value : Values,
          outcome : Outcomes]

TypeOK ==
    /\ HistoryDomainOK
    /\ HistoryValuesOK
    /\ HistoryLengthsOK
    /\ ResultsOK

OneResultPerOperation ==
    \A left \in results, right \in results:
        left.id = right.id => left = right

RevisionEvidence ==
    \A result \in results:
        result.outcome = "success" =>
            /\ result.targetRevision \in 1..Len(histories[result.target])
            /\ histories[result.target][result.targetRevision] = result.value

CloneCopiesOneImmutableSnapshot ==
    \A result \in results:
        result.kind = "clone" /\ result.outcome = "success" =>
            /\ result.source # result.target
            /\ result.targetRevision = 1
            /\ result.sourceRevision \in 1..Len(histories[result.source])
            /\ histories[result.source][result.sourceRevision] = result.value
            /\ histories[result.target][1] = result.value

Spec == Init /\ [][Next]_vars

=============================================================================
