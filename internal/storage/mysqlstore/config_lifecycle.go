package mysqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type ConfigLifecycleAction string

const (
	ConfigArchive   ConfigLifecycleAction = "archive"
	ConfigUnarchive ConfigLifecycleAction = "unarchive"
)

type ConfigLifecycleChange struct {
	OperationID string
	Actor       Actor
	Action      ConfigLifecycleAction
	Key         string
}

type ConfigLifecycleResult struct {
	Outcome  Outcome `json:"outcome"`
	Key      string  `json:"key"`
	Archived bool    `json:"archived"`
}

func (store *Store) ApplyConfigLifecycleChange(ctx context.Context, request ConfigLifecycleChange) (ConfigLifecycleResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ConfigLifecycleResult{}, ErrValidation
	}
	digest := configLifecycleDigest(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ConfigLifecycleResult{}, fmt.Errorf("begin Config lifecycle change: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return ConfigLifecycleResult{}, err
	}
	if replayed {
		var previous ConfigLifecycleResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return ConfigLifecycleResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if !validResourceKey(request.Key) || (request.Action != ConfigArchive && request.Action != ConfigUnarchive) {
		return finishConfigLifecycleFailure(ctx, transaction, request, "config.validation_failed")
	}

	var id []byte
	var archivedAt sql.NullTime
	if err := transaction.QueryRowContext(ctx, `
		SELECT id, archived_at FROM configs WHERE resource_key = ? FOR UPDATE
	`, request.Key).Scan(&id, &archivedAt); errors.Is(err, sql.ErrNoRows) {
		return finishConfigLifecycleFailure(ctx, transaction, request, "config.not_found")
	} else if err != nil {
		return ConfigLifecycleResult{}, fmt.Errorf("lock Config lifecycle: %w", err)
	}

	result := ConfigLifecycleResult{Outcome: OutcomeNoChange, Key: request.Key}
	if request.Action == ConfigArchive {
		result.Archived = true
		if !archivedAt.Valid {
			if _, err := transaction.ExecContext(ctx, `UPDATE configs SET archived_at = UTC_TIMESTAMP(6) WHERE id = ?`, id); err != nil {
				return ConfigLifecycleResult{}, fmt.Errorf("Archive Config: %w", err)
			}
			result.Outcome = OutcomeSuccess
		}
	} else if archivedAt.Valid {
		if _, err := transaction.ExecContext(ctx, `UPDATE configs SET archived_at = NULL WHERE id = ?`, id); err != nil {
			return ConfigLifecycleResult{}, fmt.Errorf("Unarchive Config: %w", err)
		}
		result.Outcome = OutcomeSuccess
	}
	if err := finishConfigLifecycleOperation(ctx, transaction, request, result, result.Outcome == OutcomeSuccess); err != nil {
		return ConfigLifecycleResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ConfigLifecycleResult{}, fmt.Errorf("commit Config lifecycle change: %w", err)
	}
	return result, nil
}

func configLifecycleDigest(request ConfigLifecycleChange) [sha256.Size]byte {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func finishConfigLifecycleFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request ConfigLifecycleChange,
	eventType string,
) (ConfigLifecycleResult, error) {
	result := ConfigLifecycleResult{Outcome: OutcomeValidationFailed, Key: request.Key}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type:         eventType,
		Action:       "config." + string(request.Action),
		ResourceType: "config",
		ResourceKey:  request.Key,
	}, false); err != nil {
		return ConfigLifecycleResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ConfigLifecycleResult{}, fmt.Errorf("commit Config lifecycle failure Audit: %w", err)
	}
	return result, withCommittedAudit(ErrValidation)
}

func finishConfigLifecycleOperation(
	ctx context.Context,
	transaction *sql.Tx,
	request ConfigLifecycleChange,
	result ConfigLifecycleResult,
	notify bool,
) error {
	eventType := "config.archived"
	if request.Action == ConfigUnarchive {
		eventType = "config.unarchived"
	}
	return finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type:         eventType,
		Action:       "config." + string(request.Action),
		ResourceType: "config",
		ResourceKey:  request.Key,
	}, notify)
}
