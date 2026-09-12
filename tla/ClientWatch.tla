--------------------------- MODULE ClientWatch ----------------------------
EXTENDS Integers, Sequences, FiniteSets

CONSTANTS MaxVersion, GoodVersions

ASSUME /\ MaxVersion >= 2
       /\ GoodVersions \subseteq 1..MaxVersion
       /\ 1 \in GoodVersions

VARIABLES networkUp,
          watching,
          serverVersion,
          loaded,
          clientVersion,
          installHistory,
          callbackPending,
          pendingPrevious,
          pendingCurrent,
          callbacks,
          callbackFailures

vars == <<networkUp, watching, serverVersion, loaded, clientVersion,
          installHistory, callbackPending, pendingPrevious, pendingCurrent,
          callbacks, callbackFailures>>

StrictlyIncreasing(sequence) ==
    \A left \in 1..Len(sequence), right \in 1..Len(sequence):
        left < right => sequence[left] < sequence[right]

Init ==
    /\ networkUp = TRUE
    /\ watching = FALSE
    /\ serverVersion = 1
    /\ loaded = FALSE
    /\ clientVersion = 0
    /\ installHistory = <<>>
    /\ callbackPending = FALSE
    /\ pendingPrevious = 0
    /\ pendingCurrent = 0
    /\ callbacks = <<>>
    /\ callbackFailures = 0

SetNetworkDown ==
    /\ networkUp
    /\ networkUp' = FALSE
    /\ UNCHANGED <<watching, serverVersion, loaded, clientVersion,
                    installHistory, callbackPending, pendingPrevious,
                    pendingCurrent, callbacks, callbackFailures>>

SetNetworkUp ==
    /\ ~networkUp
    /\ networkUp' = TRUE
    /\ UNCHANGED <<watching, serverVersion, loaded, clientVersion,
                    installHistory, callbackPending, pendingPrevious,
                    pendingCurrent, callbacks, callbackFailures>>

AdvanceServer ==
    /\ serverVersion < MaxVersion
    /\ serverVersion' = serverVersion + 1
    /\ UNCHANGED <<networkUp, watching, loaded, clientVersion,
                    installHistory, callbackPending, pendingPrevious,
                    pendingCurrent, callbacks, callbackFailures>>

InitialLoad ==
    /\ ~loaded
    /\ networkUp
    /\ serverVersion \in GoodVersions
    /\ loaded' = TRUE
    /\ clientVersion' = serverVersion
    /\ installHistory' = <<serverVersion>>
    /\ UNCHANGED <<networkUp, watching, serverVersion, callbackPending,
                    pendingPrevious, pendingCurrent, callbacks,
                    callbackFailures>>

StartWatch ==
    /\ loaded
    /\ ~watching
    /\ watching' = TRUE
    /\ UNCHANGED <<networkUp, serverVersion, loaded, clientVersion,
                    installHistory, callbackPending, pendingPrevious,
                    pendingCurrent, callbacks, callbackFailures>>

StopWatch ==
    /\ watching
    /\ watching' = FALSE
    /\ UNCHANGED <<networkUp, serverVersion, loaded, clientVersion,
                    installHistory, callbackPending, pendingPrevious,
                    pendingCurrent, callbacks, callbackFailures>>

InstallChanged ==
    /\ loaded
    /\ networkUp
    /\ serverVersion \in GoodVersions
    /\ serverVersion # clientVersion
    /\ ~callbackPending
    /\ clientVersion' = serverVersion
    /\ installHistory' = Append(installHistory, serverVersion)
    /\ callbackPending' = TRUE
    /\ pendingPrevious' = clientVersion
    /\ pendingCurrent' = serverVersion
    /\ UNCHANGED <<networkUp, watching, serverVersion, loaded,
                    callbacks, callbackFailures>>

WatchInstallChanged == InstallChanged /\ watching

FinishCallback ==
    /\ callbackPending
    /\ callbacks' = Append(callbacks, <<pendingPrevious, pendingCurrent>>)
    /\ callbackFailures' \in {callbackFailures, callbackFailures + 1}
    /\ callbackPending' = FALSE
    /\ pendingPrevious' = 0
    /\ pendingCurrent' = 0
    /\ UNCHANGED <<networkUp, watching, serverVersion, loaded,
                    clientVersion, installHistory>>

Next == SetNetworkDown
     \/ SetNetworkUp
     \/ AdvanceServer
     \/ InitialLoad
     \/ StartWatch
     \/ StopWatch
     \/ InstallChanged
     \/ FinishCallback

TypeOK ==
    /\ networkUp \in BOOLEAN
    /\ watching \in BOOLEAN
    /\ serverVersion \in 1..MaxVersion
    /\ loaded \in BOOLEAN
    /\ clientVersion \in 0..MaxVersion
    /\ installHistory \in Seq(1..MaxVersion)
    /\ callbackPending \in BOOLEAN
    /\ pendingPrevious \in 0..MaxVersion
    /\ pendingCurrent \in 0..MaxVersion
    /\ callbacks \in Seq((1..MaxVersion) \X (1..MaxVersion))
    /\ callbackFailures \in 0..MaxVersion

InstalledSnapshotsAreGood ==
    /\ (loaded <=> Len(installHistory) > 0)
    /\ (loaded => clientVersion = installHistory[Len(installHistory)])
    /\ \A version \in {installHistory[index] : index \in 1..Len(installHistory)}:
           version \in GoodVersions
    /\ StrictlyIncreasing(installHistory)

CallbacksFollowInstallation ==
    /\ Len(callbacks) + (IF callbackPending THEN 1 ELSE 0)
         = (IF loaded THEN Len(installHistory) - 1 ELSE 0)
    /\ \A index \in 1..Len(callbacks):
           callbacks[index] = <<installHistory[index], installHistory[index + 1]>>
    /\ (callbackPending =>
           /\ pendingPrevious = installHistory[Len(installHistory) - 1]
           /\ pendingCurrent = clientVersion)
    /\ (~callbackPending => pendingPrevious = 0 /\ pendingCurrent = 0)
    /\ callbackFailures <= Len(callbacks)

WatchMakesProgress ==
    \A version \in GoodVersions:
        (watching /\ loaded /\ networkUp
          /\ serverVersion = version
          /\ version # clientVersion
          /\ ~callbackPending)
        ~> (clientVersion = version
             \/ serverVersion # version
             \/ ~watching
             \/ ~networkUp)

Spec == Init
     /\ [][Next]_vars
     /\ WF_vars(WatchInstallChanged)
     /\ WF_vars(FinishCallback)

=============================================================================
