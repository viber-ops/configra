package mysqlstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ConfigMutationError carries server-selected identifiers only. Config content,
// parser errors and Vault values must never become error or audit text.
type ConfigMutationError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (issue *ConfigMutationError) Error() string { return "Config operation rejected" }
func (issue *ConfigMutationError) Unwrap() error {
	switch issue.Reason {
	case "clone_target_exists", "target_config_exists", "target_revision_changed":
		return ErrConflict
	default:
		return ErrValidation
	}
}

func replayConfigResult(replay operationReplay) (ConfigCommitResult, error) {
	var previous ConfigCommitResult
	if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
		return ConfigCommitResult{}, errors.New("existing Operation failed integrity validation")
	}
	if previous.Failure != nil {
		return previous, withCommittedAudit(previous.Failure)
	}
	return previous, outcomeError(previous.Outcome)
}

// Completed retries do not depend on the current availability of their source.
// A mismatch falls through to beginOperation, which rejects reuse (or checks a
// compatible pre-upgrade digest). Concurrent first attempts still serialize there.
func (store *Store) replayConfigOperation(ctx context.Context, id string, digest [sha256.Size]byte) (ConfigCommitResult, bool, error) {
	var storedDigest []byte
	var replay operationReplay
	err := store.db.QueryRowContext(ctx, `SELECT request_sha256, outcome, response_json FROM operations WHERE operation_id = ?`, id).Scan(&storedDigest, &replay.Outcome, &replay.Response)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigCommitResult{}, false, nil
	}
	if err != nil {
		return ConfigCommitResult{}, false, fmt.Errorf("read Config Operation: %w", err)
	}
	if !bytes.Equal(storedDigest, digest[:]) {
		return ConfigCommitResult{}, false, nil
	}
	result, err := replayConfigResult(replay)
	return result, true, err
}
