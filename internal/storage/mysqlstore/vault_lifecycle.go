package mysqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type VaultLifecycleAction string

const (
	VaultArchive   VaultLifecycleAction = "archive"
	VaultUnarchive VaultLifecycleAction = "unarchive"
)

type VaultLifecycleChange struct {
	OperationID  string
	Actor        Actor
	Action       VaultLifecycleAction
	NamespaceKey string
	Key          string
}

type VaultLifecycleResult struct {
	Outcome      Outcome `json:"outcome"`
	NamespaceKey string  `json:"namespace_key"`
	Key          string  `json:"key"`
	Archived     bool    `json:"archived"`
}

func (store *Store) ApplyVaultLifecycleChange(ctx context.Context, request VaultLifecycleChange) (VaultLifecycleResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return VaultLifecycleResult{}, ErrValidation
	}
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VaultLifecycleResult{}, fmt.Errorf("begin Vault lifecycle change: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return VaultLifecycleResult{}, err
	}
	if replayed {
		var previous VaultLifecycleResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return VaultLifecycleResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if !validResourceKey(request.NamespaceKey) || !validResourceKey(request.Key) || (request.Action != VaultArchive && request.Action != VaultUnarchive) {
		return finishVaultLifecycleFailure(ctx, transaction, request, "vault.validation_failed")
	}
	var id []byte
	var archivedAt sql.NullTime
	if err := transaction.QueryRowContext(ctx, `
		SELECT id, archived_at FROM vault_items WHERE namespace_key = ? AND resource_key = ? FOR UPDATE
	`, request.NamespaceKey, request.Key).Scan(&id, &archivedAt); errors.Is(err, sql.ErrNoRows) {
		return finishVaultLifecycleFailure(ctx, transaction, request, "vault.not_found")
	} else if err != nil {
		return VaultLifecycleResult{}, fmt.Errorf("lock Vault lifecycle: %w", err)
	}
	result := VaultLifecycleResult{Outcome: OutcomeNoChange, NamespaceKey: request.NamespaceKey, Key: request.Key}
	if request.Action == VaultArchive {
		result.Archived = true
		if !archivedAt.Valid {
			if _, err := transaction.ExecContext(ctx, `UPDATE vault_items SET archived_at = UTC_TIMESTAMP(6) WHERE id = ?`, id); err != nil {
				return VaultLifecycleResult{}, fmt.Errorf("Archive Vault Item: %w", err)
			}
			result.Outcome = OutcomeSuccess
		}
	} else if archivedAt.Valid {
		if _, err := transaction.ExecContext(ctx, `UPDATE vault_items SET archived_at = NULL WHERE id = ?`, id); err != nil {
			return VaultLifecycleResult{}, fmt.Errorf("Unarchive Vault Item: %w", err)
		}
		result.Outcome = OutcomeSuccess
	}
	eventType := "vault.archived"
	if request.Action == VaultUnarchive {
		eventType = "vault.unarchived"
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: eventType, Action: "vault." + string(request.Action), NamespaceKey: request.NamespaceKey, ResourceType: "vault_item", ResourceKey: request.Key,
	}, result.Outcome == OutcomeSuccess); err != nil {
		return VaultLifecycleResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return VaultLifecycleResult{}, fmt.Errorf("commit Vault lifecycle change: %w", err)
	}
	return result, nil
}

func finishVaultLifecycleFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request VaultLifecycleChange,
	eventType string,
) (VaultLifecycleResult, error) {
	result := VaultLifecycleResult{Outcome: OutcomeValidationFailed, NamespaceKey: request.NamespaceKey, Key: request.Key}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: eventType, Action: "vault." + string(request.Action), NamespaceKey: request.NamespaceKey, ResourceType: "vault_item", ResourceKey: request.Key,
	}, false); err != nil {
		return VaultLifecycleResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return VaultLifecycleResult{}, fmt.Errorf("commit Vault lifecycle failure Audit: %w", err)
	}
	return result, ErrValidation
}
