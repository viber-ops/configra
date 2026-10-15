package mysqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/viber-ops/configra/internal/configdoc"
)

type ConfigTransfer struct {
	OperationID            string
	Actor                  Actor
	SourceEnvironmentKey   string
	SourceConfigKey        string
	SourceRevision         uint64
	TargetEnvironmentKey   string
	TargetConfigKey        string
	TargetRevision         uint64
	ExpectedTargetRevision uint64
}

type ConfigRestore struct {
	OperationID      string
	Actor            Actor
	EnvironmentKey   string
	ConfigKey        string
	SourceRevision   uint64
	ExpectedRevision uint64
}

type ConfigClone struct {
	OperationID          string
	Actor                Actor
	SourceEnvironmentKey string
	SourceConfigKey      string
	TargetEnvironmentKey string
	TargetConfigKey      string
	TargetConfigName     string
	SourceRevision       uint64 `json:",omitempty"`
}

type configRevisionSnapshot struct {
	Active        bool
	ConfigID      []byte
	EnvironmentID []byte
	DisplayName   string
	Format        configdoc.Format
	Content       []byte
}

func (store *Store) MergeConfig(ctx context.Context, request ConfigTransfer) (ConfigCommitResult, error) {
	return store.transferConfig(ctx, request, "merge", true)
}

func (store *Store) ReplaceConfig(ctx context.Context, request ConfigTransfer) (ConfigCommitResult, error) {
	return store.transferConfig(ctx, request, "replace", false)
}

func (store *Store) RestoreConfig(ctx context.Context, request ConfigRestore) (ConfigCommitResult, error) {
	return store.transferConfig(ctx, ConfigTransfer{
		OperationID:            request.OperationID,
		Actor:                  request.Actor,
		SourceEnvironmentKey:   request.EnvironmentKey,
		SourceConfigKey:        request.ConfigKey,
		SourceRevision:         request.SourceRevision,
		TargetEnvironmentKey:   request.EnvironmentKey,
		TargetConfigKey:        request.ConfigKey,
		TargetRevision:         request.ExpectedRevision,
		ExpectedTargetRevision: request.ExpectedRevision,
	}, "restore", false)
}

func (store *Store) CloneConfig(ctx context.Context, request ConfigClone) (ConfigCommitResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ConfigCommitResult{}, ErrValidation
	}
	digest := configCloneDigest(request)
	if result, found, err := store.replayConfigOperation(ctx, request.OperationID, digest); found || err != nil {
		return result, err
	}
	meta := configCommitMeta{
		Action:        "clone",
		SuccessEvent:  "config.cloned",
		RequestDigest: &digest,
		CreateOnly:    true,
	}
	commit := ConfigCommit{
		OperationID:    request.OperationID,
		Actor:          request.Actor,
		EnvironmentKey: request.TargetEnvironmentKey,
		ConfigKey:      request.TargetConfigKey,
		ConfigName:     request.TargetConfigName,
	}
	reject := func(outcome Outcome, resultErr error) (ConfigCommitResult, error) {
		return store.rejectConfigTransfer(ctx, commit, meta, digest, outcome, resultErr)
	}
	if err := validateConfigTransferKeys(request.SourceEnvironmentKey, request.SourceConfigKey, request.TargetEnvironmentKey, request.TargetConfigKey); err != nil {
		return reject(OutcomeValidationFailed, err)
	}
	var source configRevisionSnapshot
	var err error
	revision := request.SourceRevision
	if revision == 0 {
		source, revision, err = store.readCurrentConfig(ctx, request.SourceEnvironmentKey, request.SourceConfigKey)
	} else {
		source, err = store.readConfigRevision(ctx, request.SourceEnvironmentKey, request.SourceConfigKey, revision)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "source_revision", Reason: "source_revision_unavailable"})
	}
	if err != nil {
		return ConfigCommitResult{}, err
	}
	meta.SourceConfigID = source.ConfigID
	meta.SourceEnvironmentID = source.EnvironmentID
	meta.SourceRevision = revision
	commit.Format = source.Format
	commit.Content = source.Content
	if commit.ConfigName == "" {
		commit.ConfigName = source.DisplayName
	}
	return store.commitConfig(ctx, commit, meta)
}

func (store *Store) transferConfig(
	ctx context.Context,
	request ConfigTransfer,
	action string,
	merge bool,
) (ConfigCommitResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ConfigCommitResult{}, ErrValidation
	}
	digest := configTransferDigest(request, action)
	if result, found, err := store.replayConfigOperation(ctx, request.OperationID, digest); found || err != nil {
		return result, err
	}
	meta := configCommitMeta{Action: action, SuccessEvent: "config." + action + "d", RequestDigest: &digest}
	commit := ConfigCommit{
		OperationID:      request.OperationID,
		Actor:            request.Actor,
		EnvironmentKey:   request.TargetEnvironmentKey,
		ConfigKey:        request.TargetConfigKey,
		ExpectedRevision: request.ExpectedTargetRevision,
	}
	reject := func(outcome Outcome, resultErr error) (ConfigCommitResult, error) {
		return store.rejectConfigTransfer(ctx, commit, meta, digest, outcome, resultErr)
	}
	if err := validateConfigTransferKeys(request.SourceEnvironmentKey, request.SourceConfigKey, request.TargetEnvironmentKey, request.TargetConfigKey); err != nil {
		return reject(OutcomeValidationFailed, err)
	}
	if request.SourceRevision == 0 {
		return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "source_revision", Reason: "source_revision_required"})
	}
	// Expected revision zero creates the first environment version only. The
	// normal commit CAS check rejects a target created concurrently.
	createTarget := action == "replace" && request.TargetRevision == 0 && request.ExpectedTargetRevision == 0
	if !createTarget && (request.TargetRevision == 0 || request.ExpectedTargetRevision == 0) {
		return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "target_revision", Reason: "target_revision_required"})
	}
	source, err := store.readConfigRevisionIncludingArchived(ctx, request.SourceEnvironmentKey, request.SourceConfigKey, request.SourceRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "source_revision", Reason: "source_revision_unavailable"})
	}
	if err != nil {
		return ConfigCommitResult{}, err
	}
	meta.SourceConfigID = source.ConfigID
	meta.SourceEnvironmentID = source.EnvironmentID
	meta.SourceRevision = request.SourceRevision
	commit.ConfigName = source.DisplayName
	commit.Format = source.Format
	commit.Content = source.Content

	targetActive := true
	if !createTarget {
		target, err := store.readConfigRevisionIncludingArchived(ctx, request.TargetEnvironmentKey, request.TargetConfigKey, request.TargetRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "target_revision", Reason: "target_revision_unavailable"})
		}
		if err != nil {
			return ConfigCommitResult{}, err
		}
		targetActive = target.Active
		if merge {
			if target.Format != source.Format {
				return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "target_revision", Reason: "config_format_mismatch"})
			}
			merged, err := configdoc.Merge(source.Format, source.Content, target.Content)
			if err != nil {
				return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "content", Reason: "config_document_invalid"})
			}
			commit.Content = merged.Content
		}
		// Validate the old digest using immutable history, including archived
		// resources. Only a matching completed operation may bypass lifecycle checks.
		legacy := configCommitDigest(commit, meta)
		meta.LegacyRequestDigest = &legacy
		if result, found, err := store.replayConfigOperation(ctx, request.OperationID, legacy); found || err != nil {
			return result, err
		}
	}
	if !source.Active {
		return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "source_revision", Reason: "source_revision_unavailable"})
	}
	if !targetActive {
		return reject(OutcomeValidationFailed, &ConfigMutationError{Field: "target_revision", Reason: "target_revision_unavailable"})
	}
	return store.commitConfig(ctx, commit, meta)
}

func (store *Store) readConfigRevision(ctx context.Context, environmentKey, configKey string, revision uint64) (configRevisionSnapshot, error) {
	snapshot, err := store.readConfigRevisionIncludingArchived(ctx, environmentKey, configKey, revision)
	if err == nil && !snapshot.Active {
		return configRevisionSnapshot{}, sql.ErrNoRows
	}
	return snapshot, err
}

func (store *Store) readConfigRevisionIncludingArchived(
	ctx context.Context,
	environmentKey string,
	configKey string,
	revision uint64,
) (configRevisionSnapshot, error) {
	var snapshot configRevisionSnapshot
	if err := store.db.QueryRowContext(ctx, `
		SELECT cr.config_id, cr.environment_id, c.display_name, cr.format, cr.content, (c.archived_at IS NULL AND e.archived_at IS NULL)
		FROM config_revisions cr
		JOIN configs c ON c.id = cr.config_id
		JOIN environments e ON e.id = cr.environment_id
		WHERE c.resource_key = ? AND e.resource_key = ?
		  AND cr.revision = ?
	`, configKey, environmentKey, revision).Scan(
		&snapshot.ConfigID, &snapshot.EnvironmentID, &snapshot.DisplayName, &snapshot.Format, &snapshot.Content, &snapshot.Active,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return configRevisionSnapshot{}, sql.ErrNoRows
		}
		return configRevisionSnapshot{}, fmt.Errorf("read Config Revision: %w", err)
	}
	return snapshot, nil
}

func (store *Store) readCurrentConfig(
	ctx context.Context,
	environmentKey string,
	configKey string,
) (configRevisionSnapshot, uint64, error) {
	var snapshot configRevisionSnapshot
	var revision uint64
	if err := store.db.QueryRowContext(ctx, `
		SELECT cr.config_id, cr.environment_id, c.display_name, cr.format, cr.content, cr.revision
		FROM config_env_states state
		JOIN config_revisions cr
		  ON cr.config_id = state.config_id
		 AND cr.environment_id = state.environment_id
		 AND cr.revision = state.current_revision
		JOIN configs c ON c.id = state.config_id
		JOIN environments e ON e.id = state.environment_id
		WHERE c.resource_key = ? AND c.archived_at IS NULL
		  AND e.resource_key = ? AND e.archived_at IS NULL
	`, configKey, environmentKey).Scan(
		&snapshot.ConfigID, &snapshot.EnvironmentID, &snapshot.DisplayName,
		&snapshot.Format, &snapshot.Content, &revision,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return configRevisionSnapshot{}, 0, sql.ErrNoRows
		}
		return configRevisionSnapshot{}, 0, fmt.Errorf("read current Config Revision: %w", err)
	}
	return snapshot, revision, nil
}

func configTransferDigest(request ConfigTransfer, action string) [sha256.Size]byte {
	encoded, _ := json.Marshal(struct {
		Action  string
		Request ConfigTransfer
	}{action, request})
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func configCloneDigest(request ConfigClone) [sha256.Size]byte {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func (store *Store) rejectConfigTransfer(
	ctx context.Context,
	request ConfigCommit,
	meta configCommitMeta,
	digest [sha256.Size]byte,
	outcome Outcome,
	resultErr error,
) (ConfigCommitResult, error) {
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ConfigCommitResult{}, fmt.Errorf("begin Config %s rejection: %w", meta.Action, err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return ConfigCommitResult{}, err
	}
	if replayed {
		return replayConfigResult(replay)
	}
	return finishConfigFailure(ctx, transaction, request, meta, outcome, meta.event(string(outcome)), resultErr)
}

func validateConfigTransferKeys(sourceEnv, sourceConfig, targetEnv, targetConfig string) error {
	for _, key := range []struct{ field, value string }{
		{"source_environment", sourceEnv}, {"source_config", sourceConfig},
		{"target_environment", targetEnv}, {"target_config", targetConfig},
	} {
		if !validResourceKey(key.value) {
			return &ConfigMutationError{Field: key.field, Reason: key.field + "_invalid"}
		}
	}
	return nil
}
