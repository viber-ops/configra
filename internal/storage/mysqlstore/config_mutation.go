package mysqlstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
)

type Actor struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Outcome string

const (
	OutcomeSuccess          Outcome = "success"
	OutcomeNoChange         Outcome = "no_change"
	OutcomeConflict         Outcome = "conflict"
	OutcomeValidationFailed Outcome = "validation_failed"
)

var (
	ErrConflict       = errors.New("revision conflict")
	ErrNotFound       = errors.New("not found")
	ErrValidation     = errors.New("validation failed")
	ErrOperationReuse = errors.New("OperationID reused with a different request")
)

type ConfigCommit struct {
	OperationID      string
	Actor            Actor
	EnvironmentKey   string
	ConfigKey        string
	ConfigName       string
	ExpectedRevision uint64
	Format           configdoc.Format
	Content          []byte
}

type ConfigWarning struct {
	Code         string `json:"code"`
	NamespaceKey string `json:"namespace_key"`
	ItemKey      string `json:"item_key"`
	FieldKey     string `json:"field_key"`
}

type ConfigCommitResult struct {
	Outcome  Outcome         `json:"outcome"`
	Revision uint64          `json:"revision,omitempty"`
	Warnings []ConfigWarning `json:"warnings,omitempty"`
}

func (store *Store) CommitConfig(ctx context.Context, request ConfigCommit) (ConfigCommitResult, error) {
	return store.commitConfig(ctx, request, configCommitMeta{Action: "commit"})
}

type configCommitMeta struct {
	Action              string
	SuccessEvent        string
	SourceConfigID      []byte
	SourceEnvironmentID []byte
	SourceRevision      uint64
	RequestDigest       *[sha256.Size]byte
	CreateOnly          bool
}

func (meta configCommitMeta) event(suffix string) string {
	return "config." + meta.Action + "." + suffix
}

func (store *Store) commitConfig(ctx context.Context, request ConfigCommit, meta configCommitMeta) (ConfigCommitResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ConfigCommitResult{}, ErrValidation
	}
	digest := configCommitDigest(request, meta)
	if meta.RequestDigest != nil {
		digest = *meta.RequestDigest
	}
	validationErr := validateConfigCommit(request)
	var document configdoc.Document
	if validationErr == nil {
		document, validationErr = configdoc.Canonicalize(request.Format, request.Content)
	}

	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ConfigCommitResult{}, fmt.Errorf("begin Config Commit: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return ConfigCommitResult{}, err
	}
	if replayed {
		var previous ConfigCommitResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return ConfigCommitResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		result := ConfigCommitResult{Outcome: OutcomeValidationFailed}
		if err := finishConfigOperation(ctx, transaction, request, meta, result, meta.event("validation_failed"), false); err != nil {
			return ConfigCommitResult{}, err
		}
		if err := transaction.Commit(); err != nil {
			return ConfigCommitResult{}, fmt.Errorf("commit Config validation Audit: %w", err)
		}
		return result, fmt.Errorf("%w: %v", ErrValidation, validationErr)
	}

	var environmentID []byte
	err = transaction.QueryRowContext(ctx, `
		SELECT id FROM environments WHERE resource_key = ? AND archived_at IS NULL
	`, request.EnvironmentKey).Scan(&environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return finishConfigFailure(ctx, transaction, request, meta, OutcomeValidationFailed, meta.event("validation_failed"), ErrValidation)
	}
	if err != nil {
		return ConfigCommitResult{}, fmt.Errorf("read Config Environment: %w", err)
	}

	configID, archived, found, err := findConfig(ctx, transaction, request.ConfigKey)
	if err != nil {
		return ConfigCommitResult{}, err
	}
	if found && meta.CreateOnly {
		return finishConfigFailure(ctx, transaction, request, meta, OutcomeConflict, meta.event("conflict"), ErrConflict)
	}
	if archived {
		return finishConfigFailure(ctx, transaction, request, meta, OutcomeValidationFailed, meta.event("validation_failed"), ErrValidation)
	}
	if !found && request.ExpectedRevision != 0 {
		return finishConfigFailure(ctx, transaction, request, meta, OutcomeConflict, meta.event("conflict"), ErrConflict)
	}
	if !found {
		configID, err = randomID()
		if err != nil {
			return ConfigCommitResult{}, err
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO configs (id, resource_key, display_name) VALUES (?, ?, ?)
		`, configID, request.ConfigKey, request.ConfigName); err != nil {
			return ConfigCommitResult{}, fmt.Errorf("create Config: %w", err)
		}
	}

	currentRevision, currentContent, err := lockCurrentConfig(ctx, transaction, configID, environmentID)
	if err != nil {
		return ConfigCommitResult{}, err
	}
	if request.ExpectedRevision != currentRevision {
		return finishConfigFailure(ctx, transaction, request, meta, OutcomeConflict, meta.event("conflict"), ErrConflict)
	}
	warnings, err := configWarnings(ctx, transaction, environmentID, document.References)
	if err != nil {
		return ConfigCommitResult{}, err
	}
	if currentRevision > 0 && bytes.Equal(currentContent, document.Content) {
		result := ConfigCommitResult{Outcome: OutcomeNoChange, Revision: currentRevision, Warnings: warnings}
		if err := finishConfigOperation(ctx, transaction, request, meta, result, meta.event("no_change"), false); err != nil {
			return ConfigCommitResult{}, err
		}
		if err := transaction.Commit(); err != nil {
			return ConfigCommitResult{}, fmt.Errorf("commit Config no-change: %w", err)
		}
		return result, nil
	}

	nextRevision := currentRevision + 1
	contentDigest := sha256.Sum256(document.Content)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO config_revisions
			(config_id, environment_id, revision, format, content, content_sha256,
			 operation_id, actor_type, actor_id, source_config_id, source_environment_id, source_revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, configID, environmentID, nextRevision, string(request.Format), document.Content, contentDigest[:],
		request.OperationID, request.Actor.Type, request.Actor.ID, nullableBytes(meta.SourceConfigID),
		nullableBytes(meta.SourceEnvironmentID), nullableRevision(meta.SourceRevision)); err != nil {
		return ConfigCommitResult{}, fmt.Errorf("insert Config Revision: %w", err)
	}
	for _, reference := range document.References {
		if _, err := transaction.ExecContext(ctx, `
				INSERT INTO config_revision_vault_refs
					(config_id, environment_id, revision, namespace_key, item_key, field_key)
				VALUES (?, ?, ?, ?, ?, ?)
			`, configID, environmentID, nextRevision, reference.NamespaceKey, reference.ItemKey, reference.FieldKey); err != nil {
			return ConfigCommitResult{}, fmt.Errorf("insert Config Vault Reference: %w", err)
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO config_env_states (config_id, environment_id, current_revision)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE current_revision = VALUES(current_revision)
	`, configID, environmentID, nextRevision); err != nil {
		return ConfigCommitResult{}, fmt.Errorf("advance Config Revision: %w", err)
	}
	result := ConfigCommitResult{Outcome: OutcomeSuccess, Revision: nextRevision, Warnings: warnings}
	eventType := meta.SuccessEvent
	if eventType == "" {
		eventType = "config.updated"
		if currentRevision == 0 {
			eventType = "config.created"
		}
	}
	if err := finishConfigOperation(ctx, transaction, request, meta, result, eventType, true); err != nil {
		return ConfigCommitResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ConfigCommitResult{}, fmt.Errorf("commit Config Revision: %w", err)
	}
	return result, nil
}

func validateConfigCommit(request ConfigCommit) error {
	if !validResourceKey(request.EnvironmentKey) || !validResourceKey(request.ConfigKey) {
		return errors.New("invalid Resource Key")
	}
	if request.ConfigName == "" || len(request.ConfigName) > 255 {
		return errors.New("Config display name is required and must not exceed 255 bytes")
	}
	return nil
}

func configCommitDigest(request ConfigCommit, meta configCommitMeta) [sha256.Size]byte {
	encoded, _ := json.Marshal(struct {
		Request             ConfigCommit
		Action              string
		SourceConfigID      []byte
		SourceEnvironmentID []byte
		SourceRevision      uint64
		CreateOnly          bool
	}{request, meta.Action, meta.SourceConfigID, meta.SourceEnvironmentID, meta.SourceRevision, meta.CreateOnly})
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

type operationReplay struct {
	Outcome  Outcome
	Response []byte
}

func beginOperation(
	ctx context.Context,
	transaction *sql.Tx,
	operationID string,
	digest [sha256.Size]byte,
	actor Actor,
) (bool, operationReplay, error) {
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO operations (operation_id, request_sha256, actor_type, actor_id, outcome)
		VALUES (?, ?, ?, ?, 'pending')
		ON DUPLICATE KEY UPDATE operation_id = operation_id
	`, operationID, digest[:], actor.Type, actor.ID)
	if err != nil {
		return false, operationReplay{}, fmt.Errorf("begin Operation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, operationReplay{}, fmt.Errorf("inspect Operation insert: %w", err)
	}
	if rows == 1 {
		return false, operationReplay{}, nil
	}
	var storedDigest, response []byte
	var outcome Outcome
	if err := transaction.QueryRowContext(ctx, `
		SELECT request_sha256, outcome, response_json
		FROM operations WHERE operation_id = ? FOR UPDATE
	`, operationID).Scan(&storedDigest, &outcome, &response); err != nil {
		return false, operationReplay{}, fmt.Errorf("read existing Operation: %w", err)
	}
	if !bytes.Equal(storedDigest, digest[:]) {
		return false, operationReplay{}, ErrOperationReuse
	}
	if outcome == "pending" || len(response) == 0 {
		return false, operationReplay{}, errors.New("existing Operation is incomplete")
	}
	return true, operationReplay{Outcome: outcome, Response: response}, nil
}

func findConfig(ctx context.Context, transaction *sql.Tx, key string) ([]byte, bool, bool, error) {
	var id []byte
	var archivedAt sql.NullTime
	err := transaction.QueryRowContext(ctx, `
		SELECT id, archived_at FROM configs WHERE resource_key = ? FOR UPDATE
	`, key).Scan(&id, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, fmt.Errorf("read Config: %w", err)
	}
	return id, archivedAt.Valid, true, nil
}

func lockCurrentConfig(
	ctx context.Context,
	transaction *sql.Tx,
	configID []byte,
	environmentID []byte,
) (uint64, []byte, error) {
	var revision uint64
	err := transaction.QueryRowContext(ctx, `
		SELECT current_revision
		FROM config_env_states
		WHERE config_id = ? AND environment_id = ?
		FOR UPDATE
	`, configID, environmentID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("lock current Config Revision: %w", err)
	}
	var content []byte
	if err := transaction.QueryRowContext(ctx, `
		SELECT content FROM config_revisions
		WHERE config_id = ? AND environment_id = ? AND revision = ?
	`, configID, environmentID, revision).Scan(&content); err != nil {
		return 0, nil, fmt.Errorf("read current Config content: %w", err)
	}
	return revision, content, nil
}

func configWarnings(
	ctx context.Context,
	transaction *sql.Tx,
	environmentID []byte,
	references []configdoc.Reference,
) ([]ConfigWarning, error) {
	var warnings []ConfigWarning
	for _, reference := range references {
		var itemID []byte
		var revision uint64
		err := transaction.QueryRowContext(ctx, `
				SELECT id, current_revision FROM vault_items
				WHERE namespace_key = ? AND resource_key = ? AND archived_at IS NULL
			`, reference.NamespaceKey, reference.ItemKey).Scan(&itemID, &revision)
		if errors.Is(err, sql.ErrNoRows) {
			warnings = append(warnings, ConfigWarning{Code: "missing_vault_item", NamespaceKey: reference.NamespaceKey, ItemKey: reference.ItemKey, FieldKey: reference.FieldKey})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("validate Config Vault Item: %w", err)
		}
		var fieldExists bool
		if err := transaction.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM vault_revision_fields
				WHERE item_id = ? AND revision = ? AND resource_key = ? AND archived = FALSE
			)
		`, itemID, revision, reference.FieldKey).Scan(&fieldExists); err != nil {
			return nil, fmt.Errorf("validate Config Vault Field: %w", err)
		}
		if !fieldExists {
			warnings = append(warnings, ConfigWarning{Code: "missing_vault_field", NamespaceKey: reference.NamespaceKey, ItemKey: reference.ItemKey, FieldKey: reference.FieldKey})
			continue
		}
		var variantExists bool
		if err := transaction.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM vault_revision_variant_environments
				WHERE item_id = ? AND revision = ? AND environment_id = ?
			)
		`, itemID, revision, environmentID).Scan(&variantExists); err != nil {
			return nil, fmt.Errorf("validate Config Vault Environment binding: %w", err)
		}
		if !variantExists {
			warnings = append(warnings, ConfigWarning{Code: "missing_vault_environment_variant", NamespaceKey: reference.NamespaceKey, ItemKey: reference.ItemKey, FieldKey: reference.FieldKey})
		}
	}
	return warnings, nil
}

func finishConfigFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request ConfigCommit,
	meta configCommitMeta,
	outcome Outcome,
	eventType string,
	resultErr error,
) (ConfigCommitResult, error) {
	result := ConfigCommitResult{Outcome: outcome}
	if err := finishConfigOperation(ctx, transaction, request, meta, result, eventType, false); err != nil {
		return ConfigCommitResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ConfigCommitResult{}, fmt.Errorf("commit Config failure Audit: %w", err)
	}
	return result, resultErr
}

func finishConfigOperation(
	ctx context.Context,
	transaction *sql.Tx,
	request ConfigCommit,
	meta configCommitMeta,
	result ConfigCommitResult,
	eventType string,
	notify bool,
) error {
	return finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, result.Revision, result, mutationEvent{
		Type:           eventType,
		Action:         "config." + meta.Action,
		EnvironmentKey: request.EnvironmentKey,
		ResourceType:   "config",
		ResourceKey:    request.ConfigKey,
	}, notify)
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func nullableRevision(value uint64) any {
	if value == 0 {
		return nil
	}
	return value
}

type mutationEvent struct {
	Type           string
	Action         string
	EnvironmentKey string
	NamespaceKey   string
	ResourceType   string
	ResourceKey    string
}

func finishOperation(
	ctx context.Context,
	transaction *sql.Tx,
	operationID string,
	actor Actor,
	outcome Outcome,
	revision uint64,
	result any,
	event mutationEvent,
	notify bool,
) error {
	response, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode Operation response: %w", err)
	}
	var resultingRevision any
	if outcome == OutcomeSuccess && revision > 0 {
		resultingRevision = revision
	}
	if _, err := transaction.ExecContext(ctx, `
		UPDATE operations
		SET outcome = ?, response_json = ?, resulting_revision = ?, completed_at = UTC_TIMESTAMP(6)
		WHERE operation_id = ? AND outcome = 'pending'
	`, outcome, response, resultingRevision, operationID); err != nil {
		return fmt.Errorf("finish Operation: %w", err)
	}
	payload := struct {
		Time           time.Time `json:"time"`
		OperationID    string    `json:"operation_id"`
		Actor          Actor     `json:"actor"`
		Action         string    `json:"action"`
		Outcome        Outcome   `json:"outcome"`
		EnvironmentKey string    `json:"environment_key"`
		NamespaceKey   string    `json:"namespace_key,omitempty"`
		ResourceType   string    `json:"resource_type"`
		ResourceKey    string    `json:"resource_key"`
		Revision       uint64    `json:"revision,omitempty"`
	}{
		Time:           time.Now().UTC(),
		OperationID:    operationID,
		Actor:          actor,
		Action:         event.Action,
		Outcome:        outcome,
		EnvironmentKey: event.EnvironmentKey,
		NamespaceKey:   event.NamespaceKey,
		ResourceType:   event.ResourceType,
		ResourceKey:    event.ResourceKey,
	}
	if outcome == OutcomeSuccess {
		payload.Revision = revision
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Audit Event: %w", err)
	}
	if _, err := insertOutbox(ctx, transaction, "audit", event.Type, operationID, encodedPayload); err != nil {
		return err
	}
	if notify {
		outboxID, err := insertOutbox(ctx, transaction, "notification", event.Type, operationID, encodedPayload)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO notification_targets (outbox_event_id, destination_id)
			SELECT ?, destination.id
			FROM notification_destinations AS destination
			JOIN notification_subscriptions AS subscription ON subscription.destination_id = destination.id
			WHERE destination.enabled = TRUE AND destination.archived_at IS NULL AND subscription.event_type = ?
		`, outboxID, event.Type); err != nil {
			return fmt.Errorf("snapshot Notification targets: %w", err)
		}
	}
	return nil
}

func insertOutbox(
	ctx context.Context,
	transaction *sql.Tx,
	kind string,
	eventType string,
	operationID string,
	payload []byte,
) ([]byte, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO outbox_events (id, kind, event_type, operation_id, payload)
		VALUES (?, ?, ?, ?, ?)
	`, id, kind, eventType, operationID, payload); err != nil {
		return nil, fmt.Errorf("insert %s Outbox Event: %w", kind, err)
	}
	return id, nil
}

func randomID() ([]byte, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("generate ID: %w", err)
	}
	return id, nil
}

func outcomeError(outcome Outcome) error {
	switch outcome {
	case OutcomeConflict:
		return ErrConflict
	case OutcomeValidationFailed:
		return ErrValidation
	default:
		return nil
	}
}

func validOperationID(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func validActor(actor Actor) bool {
	return (actor.Type == "user" || actor.Type == "system") && actor.ID != "" && len(actor.ID) <= 255
}

func validResourceKey(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
	}
	return true
}
