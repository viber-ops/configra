package mysqlstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

type VaultCommit struct {
	OperationID      string
	Actor            Actor
	NamespaceKey     string
	ItemKey          string
	ItemName         string
	ExpectedRevision uint64
	Snapshot         vaultdoc.Snapshot
	requestDigest    *[sha256.Size]byte
	validationErr    error
	action           string
	successEvent     string
	restoredFrom     uint64
}

type VaultRestore struct {
	OperationID      string
	Actor            Actor
	NamespaceKey     string
	ItemKey          string
	SourceRevision   uint64
	ExpectedRevision uint64
}

type VaultCommitResult struct {
	Outcome    Outcome  `json:"outcome"`
	Revision   uint64   `json:"revision,omitempty"`
	VariantIDs []string `json:"variant_ids,omitempty"`
}

func (store *Store) CommitVault(ctx context.Context, request VaultCommit) (VaultCommitResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return VaultCommitResult{}, ErrValidation
	}
	digest := vaultCommitDigest(request)
	if request.requestDigest != nil {
		digest = *request.requestDigest
	}
	validationErr := request.validationErr
	if validationErr == nil {
		validationErr = validateVaultCommit(request)
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VaultCommitResult{}, fmt.Errorf("begin Vault Commit: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return VaultCommitResult{}, err
	}
	if replayed {
		var previous VaultCommitResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return VaultCommitResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return finishVaultFailure(ctx, transaction, request, OutcomeValidationFailed, "vault.validation_failed", validationErr)
	}

	itemID, currentRevision, archived, found, err := lockVaultItem(ctx, transaction, request.NamespaceKey, request.ItemKey)
	if err != nil {
		return VaultCommitResult{}, err
	}
	if archived {
		return finishVaultFailure(ctx, transaction, request, OutcomeValidationFailed, "vault.archived", ErrValidation)
	}
	if request.ExpectedRevision != currentRevision {
		return finishVaultFailure(ctx, transaction, request, OutcomeConflict, "vault.conflict", ErrConflict)
	}
	if !found {
		itemID, err = randomID()
		if err != nil {
			return VaultCommitResult{}, err
		}
	}

	snapshot := cloneVaultSnapshot(request.Snapshot)
	for index := range snapshot.Variants {
		if snapshot.Variants[index].ID == "" {
			id, err := randomID()
			if err != nil {
				return VaultCommitResult{}, err
			}
			snapshot.Variants[index].ID = hex.EncodeToString(id)
		}
	}
	encoded, err := vaultdoc.Encode(snapshot)
	if err != nil {
		return finishVaultFailure(ctx, transaction, request, OutcomeValidationFailed, "vault.validation_failed", err)
	}
	defer clear(encoded.Plaintext)

	fields, err := prepareVaultFields(ctx, transaction, itemID, found, snapshot.Fields)
	if errors.Is(err, ErrValidation) {
		return finishVaultFailure(ctx, transaction, request, OutcomeValidationFailed, "vault.field_type_immutable", err)
	}
	if err != nil {
		return VaultCommitResult{}, err
	}
	environments, err := validateVaultEnvironments(ctx, transaction, itemID, currentRevision, snapshot.Variants)
	if errors.Is(err, ErrValidation) {
		return finishVaultFailure(ctx, transaction, request, OutcomeValidationFailed, "vault.environment_invalid", err)
	}
	if err != nil {
		return VaultCommitResult{}, err
	}
	variantIDs := make([]string, len(snapshot.Variants))
	for index, variant := range snapshot.Variants {
		variantIDs[index] = variant.ID
	}
	slices.Sort(variantIDs)

	if currentRevision > 0 {
		currentPlaintext, currentStructure, err := store.currentVaultSnapshot(ctx, transaction, itemID, currentRevision)
		if err != nil {
			return VaultCommitResult{}, err
		}
		var currentItemName string
		if err := transaction.QueryRowContext(ctx, `
			SELECT item_display_name FROM vault_item_revisions WHERE item_id = ? AND revision = ?
		`, itemID, currentRevision).Scan(&currentItemName); err != nil {
			clear(currentPlaintext)
			return VaultCommitResult{}, fmt.Errorf("read current Vault Item display name: %w", err)
		}
		same := currentItemName == request.ItemName && currentStructure == encoded.StructureSHA256 && bytes.Equal(currentPlaintext, encoded.Plaintext)
		clear(currentPlaintext)
		if same {
			result := VaultCommitResult{Outcome: OutcomeNoChange, Revision: currentRevision, VariantIDs: variantIDs}
			if err := finishVaultOperation(ctx, transaction, request, result, "vault.no_change", false); err != nil {
				return VaultCommitResult{}, err
			}
			if err := transaction.Commit(); err != nil {
				return VaultCommitResult{}, fmt.Errorf("commit Vault no-change: %w", err)
			}
			return result, nil
		}
	}

	nextRevision := currentRevision + 1
	encrypted, err := store.provider.EncryptSnapshot(vaultcrypto.SnapshotIdentity{
		ItemID:   hex.EncodeToString(itemID),
		Revision: nextRevision,
	}, encoded.Plaintext)
	if err != nil {
		return VaultCommitResult{}, fmt.Errorf("encrypt Vault Snapshot: %w", err)
	}
	if !found {
		if _, err := transaction.ExecContext(ctx, `
				INSERT INTO vault_items (id, namespace_key, resource_key, display_name, current_revision)
				VALUES (?, ?, ?, ?, ?)
			`, itemID, request.NamespaceKey, request.ItemKey, request.ItemName, nextRevision); err != nil {
			return VaultCommitResult{}, fmt.Errorf("create Vault Item: %w", err)
		}
	}
	if err := writeVaultFields(ctx, transaction, itemID, fields); err != nil {
		return VaultCommitResult{}, err
	}
	var restoredFrom any
	if request.restoredFrom > 0 {
		restoredFrom = request.restoredFrom
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO vault_item_revisions
			(item_id, revision, item_display_name, structure_sha256, algorithm, key_version, nonce, ciphertext,
			 encrypted_dek, ciphertext_size, operation_id, actor_type, actor_id, restored_from_revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, itemID, nextRevision, request.ItemName, encoded.StructureSHA256[:], encrypted.Algorithm, encrypted.KeyVersion,
		encrypted.Nonce, encrypted.Ciphertext, encrypted.EncryptedDEK, len(encrypted.Ciphertext),
		request.OperationID, request.Actor.Type, request.Actor.ID, restoredFrom); err != nil {
		return VaultCommitResult{}, fmt.Errorf("insert Vault Item Revision: %w", err)
	}
	if err := writeVaultRevisionMetadata(ctx, transaction, itemID, nextRevision, fields, snapshot.Variants, environments); err != nil {
		return VaultCommitResult{}, err
	}
	if found {
		if _, err := transaction.ExecContext(ctx, `
			UPDATE vault_items SET display_name = ?, current_revision = ? WHERE id = ?
		`, request.ItemName, nextRevision, itemID); err != nil {
			return VaultCommitResult{}, fmt.Errorf("advance Vault Item Revision: %w", err)
		}
	}
	result := VaultCommitResult{Outcome: OutcomeSuccess, Revision: nextRevision, VariantIDs: variantIDs}
	eventType := "vault.updated"
	if currentRevision == 0 {
		eventType = "vault.created"
	}
	if request.successEvent != "" {
		eventType = request.successEvent
	}
	if err := finishVaultOperation(ctx, transaction, request, result, eventType, true); err != nil {
		return VaultCommitResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return VaultCommitResult{}, fmt.Errorf("commit Vault Item Revision: %w", err)
	}
	return result, nil
}

func (store *Store) RestoreVault(ctx context.Context, request VaultRestore) (VaultCommitResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return VaultCommitResult{}, ErrValidation
	}
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	commit := VaultCommit{
		OperationID:      request.OperationID,
		Actor:            request.Actor,
		NamespaceKey:     request.NamespaceKey,
		ItemKey:          request.ItemKey,
		ExpectedRevision: request.ExpectedRevision,
		requestDigest:    &digest,
		action:           "restore",
		successEvent:     "vault.restored",
		restoredFrom:     request.SourceRevision,
	}
	if !validResourceKey(request.NamespaceKey) || !validResourceKey(request.ItemKey) || request.SourceRevision == 0 {
		commit.validationErr = ErrValidation
		return store.CommitVault(ctx, commit)
	}
	source, err := store.ReadVaultItem(ctx, request.NamespaceKey, request.ItemKey, request.SourceRevision, true)
	if errors.Is(err, ErrNotFound) {
		commit.validationErr = ErrValidation
		return store.CommitVault(ctx, commit)
	}
	if err != nil {
		return VaultCommitResult{}, err
	}
	commit.ItemName = source.DisplayName
	commit.Snapshot = source.Snapshot
	return store.CommitVault(ctx, commit)
}

func validateVaultCommit(request VaultCommit) error {
	if !validResourceKey(request.NamespaceKey) || !validResourceKey(request.ItemKey) {
		return errors.New("invalid Vault Namespace or Item Resource Key")
	}
	if request.ItemName == "" || len(request.ItemName) > 255 {
		return errors.New("Vault Item display name is required and must not exceed 255 bytes")
	}
	return nil
}

func vaultCommitDigest(request VaultCommit) [sha256.Size]byte {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func cloneVaultSnapshot(source vaultdoc.Snapshot) vaultdoc.Snapshot {
	clone := vaultdoc.Snapshot{Fields: slices.Clone(source.Fields), Variants: make([]vaultdoc.Variant, len(source.Variants))}
	for index, variant := range source.Variants {
		clone.Variants[index] = vaultdoc.Variant{
			ID:           variant.ID,
			Environments: slices.Clone(variant.Environments),
			Values:       make(map[string]vaultdoc.Value, len(variant.Values)),
		}
		for key, value := range variant.Values {
			clonedValue := value
			if value.Text != nil {
				text := *value.Text
				clonedValue.Text = &text
			}
			if value.File != nil {
				file := *value.File
				file.Bytes = bytes.Clone(value.File.Bytes)
				clonedValue.File = &file
			}
			clone.Variants[index].Values[key] = clonedValue
		}
	}
	return clone
}

func lockVaultItem(ctx context.Context, transaction *sql.Tx, namespaceKey, itemKey string) ([]byte, uint64, bool, bool, error) {
	var id []byte
	var revision uint64
	var archivedAt sql.NullTime
	err := transaction.QueryRowContext(ctx, `
		SELECT id, current_revision, archived_at
			FROM vault_items WHERE namespace_key = ? AND resource_key = ? FOR UPDATE
		`, namespaceKey, itemKey).Scan(&id, &revision, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, false, nil
	}
	if err != nil {
		return nil, 0, false, false, fmt.Errorf("lock Vault Item: %w", err)
	}
	return id, revision, archivedAt.Valid, true, nil
}

type vaultFieldRecord struct {
	id       []byte
	key      string
	name     string
	typeName vaultdoc.FieldType
	active   bool
}

func prepareVaultFields(
	ctx context.Context,
	transaction *sql.Tx,
	itemID []byte,
	itemExists bool,
	requested []vaultdoc.Field,
) ([]vaultFieldRecord, error) {
	existing := make(map[string]vaultFieldRecord)
	if itemExists {
		rows, err := transaction.QueryContext(ctx, `
			SELECT id, resource_key, display_name, field_type FROM vault_fields WHERE item_id = ?
		`, itemID)
		if err != nil {
			return nil, fmt.Errorf("read Vault Fields: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var field vaultFieldRecord
			if err := rows.Scan(&field.id, &field.key, &field.name, &field.typeName); err != nil {
				return nil, fmt.Errorf("scan Vault Field: %w", err)
			}
			existing[field.key] = field
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate Vault Fields: %w", err)
		}
	}
	for _, field := range requested {
		record, found := existing[field.Key]
		if found && record.typeName != field.Type {
			return nil, fmt.Errorf("Vault Field %s type cannot change: %w", field.Key, ErrValidation)
		}
		if !found {
			id, err := randomID()
			if err != nil {
				return nil, err
			}
			record = vaultFieldRecord{id: id, key: field.Key, typeName: field.Type}
		}
		record.name = field.Name
		record.active = true
		existing[field.Key] = record
	}
	fields := make([]vaultFieldRecord, 0, len(existing))
	for _, field := range existing {
		fields = append(fields, field)
	}
	slices.SortFunc(fields, func(left, right vaultFieldRecord) int {
		return bytes.Compare([]byte(left.key), []byte(right.key))
	})
	return fields, nil
}

func validateVaultEnvironments(
	ctx context.Context,
	transaction *sql.Tx,
	itemID []byte,
	currentRevision uint64,
	variants []vaultdoc.Variant,
) (map[string][]byte, error) {
	currentBindings := make(map[string]string)
	if currentRevision > 0 {
		rows, err := transaction.QueryContext(ctx, `
			SELECT environment.resource_key, HEX(binding.variant_id)
			FROM vault_revision_variant_environments AS binding
			JOIN environments AS environment ON environment.id = binding.environment_id
			WHERE binding.item_id = ? AND binding.revision = ?
		`, itemID, currentRevision)
		if err != nil {
			return nil, fmt.Errorf("read current Vault Environment bindings: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key, variantID string
			if err := rows.Scan(&key, &variantID); err != nil {
				return nil, fmt.Errorf("scan current Vault Environment binding: %w", err)
			}
			currentBindings[key] = string(bytes.ToLower([]byte(variantID)))
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate current Vault Environment bindings: %w", err)
		}
	}
	requestedBindings := make(map[string]string)
	for _, variant := range variants {
		for _, environment := range variant.Environments {
			requestedBindings[environment] = variant.ID
		}
	}
	environments := make(map[string][]byte, len(requestedBindings))
	for key, variantID := range requestedBindings {
		var id []byte
		var archivedAt sql.NullTime
		err := transaction.QueryRowContext(ctx, `
			SELECT id, archived_at FROM environments WHERE resource_key = ?
		`, key).Scan(&id, &archivedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Environment %s does not exist: %w", key, ErrValidation)
		}
		if err != nil {
			return nil, fmt.Errorf("read Vault Environment: %w", err)
		}
		if archivedAt.Valid && currentBindings[key] != variantID {
			return nil, fmt.Errorf("archived Environment %s binding cannot change: %w", key, ErrValidation)
		}
		environments[key] = id
	}
	for key, variantID := range currentBindings {
		var archived bool
		if err := transaction.QueryRowContext(ctx, `
			SELECT archived_at IS NOT NULL FROM environments WHERE resource_key = ?
		`, key).Scan(&archived); err != nil {
			return nil, fmt.Errorf("read current Vault Environment lifecycle: %w", err)
		}
		if archived && requestedBindings[key] != variantID {
			return nil, fmt.Errorf("archived Environment %s binding must be retained: %w", key, ErrValidation)
		}
	}
	return environments, nil
}

func (store *Store) currentVaultSnapshot(
	ctx context.Context,
	transaction *sql.Tx,
	itemID []byte,
	revision uint64,
) ([]byte, [sha256.Size]byte, error) {
	var encrypted vaultcrypto.EncryptedSnapshot
	var structure [sha256.Size]byte
	var structureBytes []byte
	if err := transaction.QueryRowContext(ctx, `
		SELECT structure_sha256, algorithm, key_version, nonce, ciphertext, encrypted_dek
		FROM vault_item_revisions WHERE item_id = ? AND revision = ?
	`, itemID, revision).Scan(&structureBytes, &encrypted.Algorithm, &encrypted.KeyVersion,
		&encrypted.Nonce, &encrypted.Ciphertext, &encrypted.EncryptedDEK); err != nil {
		return nil, structure, fmt.Errorf("read current Vault Snapshot: %w", err)
	}
	if len(structureBytes) != sha256.Size {
		return nil, structure, errors.New("current Vault structure digest failed integrity validation")
	}
	copy(structure[:], structureBytes)
	plaintext, err := store.provider.DecryptSnapshot(vaultcrypto.SnapshotIdentity{
		ItemID: hex.EncodeToString(itemID), Revision: revision,
	}, encrypted)
	if err != nil {
		return nil, structure, fmt.Errorf("decrypt current Vault Snapshot: %w", vaultcrypto.ErrIntegrity)
	}
	if _, err := vaultdoc.Decode(plaintext); err != nil {
		clear(plaintext)
		return nil, structure, fmt.Errorf("decode current Vault Snapshot: %w", vaultcrypto.ErrIntegrity)
	}
	return plaintext, structure, nil
}

func writeVaultFields(ctx context.Context, transaction *sql.Tx, itemID []byte, fields []vaultFieldRecord) error {
	for _, field := range fields {
		if field.active {
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO vault_fields (item_id, id, resource_key, display_name, field_type, archived_at)
				VALUES (?, ?, ?, ?, ?, NULL)
				ON DUPLICATE KEY UPDATE display_name = VALUES(display_name), archived_at = NULL
			`, itemID, field.id, field.key, field.name, string(field.typeName)); err != nil {
				return fmt.Errorf("write active Vault Field: %w", err)
			}
		} else if _, err := transaction.ExecContext(ctx, `
			UPDATE vault_fields SET archived_at = COALESCE(archived_at, UTC_TIMESTAMP(6))
			WHERE item_id = ? AND id = ?
		`, itemID, field.id); err != nil {
			return fmt.Errorf("archive removed Vault Field: %w", err)
		}
	}
	return nil
}

func writeVaultRevisionMetadata(
	ctx context.Context,
	transaction *sql.Tx,
	itemID []byte,
	revision uint64,
	fields []vaultFieldRecord,
	variants []vaultdoc.Variant,
	environments map[string][]byte,
) error {
	for _, field := range fields {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO vault_revision_fields
				(item_id, revision, field_id, resource_key, display_name, field_type, archived)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, itemID, revision, field.id, field.key, field.name, string(field.typeName), !field.active); err != nil {
			return fmt.Errorf("insert Vault Revision Field: %w", err)
		}
	}
	for index, variant := range variants {
		variantID, _ := hex.DecodeString(variant.ID)
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO vault_revision_variants (item_id, revision, variant_id, ordinal)
			VALUES (?, ?, ?, ?)
		`, itemID, revision, variantID, index+1); err != nil {
			return fmt.Errorf("insert Vault Revision Variant: %w", err)
		}
		for _, environment := range variant.Environments {
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO vault_revision_variant_environments
					(item_id, revision, variant_id, environment_id)
				VALUES (?, ?, ?, ?)
			`, itemID, revision, variantID, environments[environment]); err != nil {
				return fmt.Errorf("insert Vault Revision Environment binding: %w", err)
			}
		}
	}
	return nil
}

func finishVaultFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request VaultCommit,
	outcome Outcome,
	eventType string,
	cause error,
) (VaultCommitResult, error) {
	result := VaultCommitResult{Outcome: outcome}
	if err := finishVaultOperation(ctx, transaction, request, result, eventType, false); err != nil {
		return VaultCommitResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return VaultCommitResult{}, fmt.Errorf("commit Vault failure Audit: %w", err)
	}
	if outcome == OutcomeConflict {
		return result, withCommittedAudit(ErrConflict)
	}
	return result, withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
}

func finishVaultOperation(
	ctx context.Context,
	transaction *sql.Tx,
	request VaultCommit,
	result VaultCommitResult,
	eventType string,
	notify bool,
) error {
	action := request.action
	if action == "" {
		action = "commit"
	}
	return finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, result.Revision, result, mutationEvent{
		Type:         eventType,
		Action:       "vault." + action,
		NamespaceKey: request.NamespaceKey,
		ResourceType: "vault_item",
		ResourceKey:  request.ItemKey,
	}, notify)
}
