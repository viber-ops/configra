-------------------------- MODULE BackupRestore --------------------------
EXTENDS FiniteSets

CONSTANTS Versions, CorrectKey, WrongKey

ASSUME /\ Versions # {}
       /\ CorrectKey # WrongKey

NoVersion == "none"
NoKey == "none"
BackupStates == {"none", "writing", "complete", "corrupt"}
RestoreStates == {"idle", "ready", "failed"}
KeyIDs == {CorrectKey, WrongKey}

VARIABLES liveVersion,
          backupState,
          backupVersion,
          backupKeyID,
          checksumValid,
          containsMasterKey,
          selectedKeyID,
          restoreState,
          restoreArtifactState,
          restoreChecksumValid,
          restoredVersion,
          restoredKeyID

vars == <<liveVersion, backupState, backupVersion, backupKeyID,
          checksumValid, containsMasterKey, selectedKeyID, restoreState,
          restoreArtifactState, restoreChecksumValid, restoredVersion,
          restoredKeyID>>

Init ==
    /\ liveVersion \in Versions
    /\ backupState = "none"
    /\ backupVersion = NoVersion
    /\ backupKeyID = NoKey
    /\ checksumValid = FALSE
    /\ containsMasterKey = FALSE
    /\ selectedKeyID = NoKey
    /\ restoreState = "idle"
    /\ restoreArtifactState = "none"
    /\ restoreChecksumValid = FALSE
    /\ restoredVersion = NoVersion
    /\ restoredKeyID = NoKey

Mutate ==
    /\ \E version \in Versions \ {liveVersion}: liveVersion' = version
    /\ UNCHANGED <<backupState, backupVersion, backupKeyID,
                    checksumValid, containsMasterKey, selectedKeyID,
                    restoreState, restoreArtifactState,
                    restoreChecksumValid, restoredVersion, restoredKeyID>>

StartBackup ==
    /\ backupState = "none"
    /\ backupState' = "writing"
    /\ backupVersion' = liveVersion
    /\ backupKeyID' = CorrectKey
    /\ checksumValid' = FALSE
    /\ UNCHANGED <<liveVersion, containsMasterKey, selectedKeyID,
                    restoreState, restoreArtifactState,
                    restoreChecksumValid, restoredVersion, restoredKeyID>>

FinishBackup ==
    /\ backupState = "writing"
    /\ backupState' = "complete"
    /\ checksumValid' = TRUE
    /\ UNCHANGED <<liveVersion, backupVersion, backupKeyID,
                    containsMasterKey, selectedKeyID, restoreState,
                    restoreArtifactState, restoreChecksumValid,
                    restoredVersion, restoredKeyID>>

CorruptBackup ==
    /\ backupState \in {"writing", "complete"}
    /\ backupState' = "corrupt"
    /\ checksumValid' = FALSE
    /\ UNCHANGED <<liveVersion, backupVersion, backupKeyID,
                    containsMasterKey, selectedKeyID, restoreState,
                    restoreArtifactState, restoreChecksumValid,
                    restoredVersion, restoredKeyID>>

SelectRestoreKey ==
    /\ restoreState = "idle"
    /\ selectedKeyID = NoKey
    /\ \E key \in KeyIDs: selectedKeyID' = key
    /\ UNCHANGED <<liveVersion, backupState, backupVersion, backupKeyID,
                    checksumValid, containsMasterKey, restoreState,
                    restoreArtifactState, restoreChecksumValid,
                    restoredVersion, restoredKeyID>>

AttemptRestore ==
    /\ restoreState = "idle"
    /\ selectedKeyID \in KeyIDs
    /\ restoreArtifactState' = backupState
    /\ restoreChecksumValid' = checksumValid
    /\ IF /\ backupState = "complete"
           /\ checksumValid
           /\ selectedKeyID = backupKeyID
          THEN /\ restoreState' = "ready"
               /\ restoredVersion' = backupVersion
               /\ restoredKeyID' = selectedKeyID
          ELSE /\ restoreState' = "failed"
               /\ restoredVersion' = NoVersion
               /\ restoredKeyID' = NoKey
    /\ UNCHANGED <<liveVersion, backupState, backupVersion, backupKeyID,
                    checksumValid, containsMasterKey, selectedKeyID>>

ResetFailedRestore ==
    /\ restoreState = "failed"
    /\ restoreState' = "idle"
    /\ selectedKeyID' = NoKey
    /\ restoreArtifactState' = "none"
    /\ restoreChecksumValid' = FALSE
    /\ restoredVersion' = NoVersion
    /\ restoredKeyID' = NoKey
    /\ UNCHANGED <<liveVersion, backupState, backupVersion, backupKeyID,
                    checksumValid, containsMasterKey>>

Next == Mutate
     \/ StartBackup
     \/ FinishBackup
     \/ CorruptBackup
     \/ SelectRestoreKey
     \/ AttemptRestore
     \/ ResetFailedRestore

TypeOK ==
    /\ liveVersion \in Versions
    /\ backupState \in BackupStates
    /\ backupVersion \in Versions \cup {NoVersion}
    /\ backupKeyID \in KeyIDs \cup {NoKey}
    /\ checksumValid \in BOOLEAN
    /\ containsMasterKey \in BOOLEAN
    /\ selectedKeyID \in KeyIDs \cup {NoKey}
    /\ restoreState \in RestoreStates
    /\ restoreArtifactState \in BackupStates
    /\ restoreChecksumValid \in BOOLEAN
    /\ restoredVersion \in Versions \cup {NoVersion}
    /\ restoredKeyID \in KeyIDs \cup {NoKey}

MasterKeyNeverEntersBackup == ~containsMasterKey

OnlyCompleteBackupHasValidChecksum ==
    checksumValid => backupState = "complete"

ReadyOnlyAfterVerifiedRestore ==
    restoreState = "ready" =>
        /\ restoreArtifactState = "complete"
        /\ restoreChecksumValid
        /\ restoredVersion = backupVersion
        /\ restoredVersion \in Versions
        /\ restoredKeyID = CorrectKey

FailedRestoreIsNotReadable ==
    restoreState # "ready" =>
        /\ restoredVersion = NoVersion
        /\ restoredKeyID = NoKey

Spec == Init /\ [][Next]_vars

=============================================================================
