package mysqlstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func (store *Store) TokenByPublicID(ctx context.Context, publicID string) (machine.Token, error) {
	var (
		tokenID     []byte
		digest      []byte
		allowNoMTLS bool
		expiresAt   sql.NullTime
		revokedAt   sql.NullTime
	)
	err := store.db.QueryRowContext(ctx, `
		SELECT id, secret_digest, allow_without_mtls, expires_at, revoked_at
		FROM api_tokens
		WHERE public_id = ?
	`, publicID).Scan(&tokenID, &digest, &allowNoMTLS, &expiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return machine.Token{}, machine.ErrNotFound
	}
	if err != nil {
		return machine.Token{}, fmt.Errorf("read API Token: %w", err)
	}
	if len(tokenID) != 16 || len(digest) != sha256.Size {
		return machine.Token{}, errors.New("API Token record failed integrity validation")
	}
	token := machine.Token{
		PublicID:         publicID,
		AllowWithoutMTLS: allowNoMTLS,
		Revoked:          revokedAt.Valid,
	}
	copy(token.SecretDigest[:], digest)
	if expiresAt.Valid {
		token.ExpiresAt = expiresAt.Time.UTC()
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT environment.resource_key
		FROM api_token_environments AS grant_record
		JOIN environments AS environment ON environment.id = grant_record.environment_id
		WHERE grant_record.token_id = ?
		ORDER BY environment.resource_key
	`, tokenID)
	if err != nil {
		return machine.Token{}, fmt.Errorf("read API Token Environment grants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var environment string
		if err := rows.Scan(&environment); err != nil {
			return machine.Token{}, fmt.Errorf("scan API Token Environment grant: %w", err)
		}
		token.AllowedEnvironments = append(token.AllowedEnvironments, environment)
	}
	if err := rows.Err(); err != nil {
		return machine.Token{}, fmt.Errorf("iterate API Token Environment grants: %w", err)
	}
	return token, nil
}

func (store *Store) IsCertificateActive(ctx context.Context, fingerprint [sha256.Size]byte) (bool, error) {
	var active bool
	err := store.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM client_certificates
			WHERE fingerprint_sha256 = ?
			  AND revoked_at IS NULL
			  AND not_before <= UTC_TIMESTAMP(6)
			  AND not_after > UTC_TIMESTAMP(6)
		)
	`, fingerprint[:]).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("read Client Certificate status: %w", err)
	}
	return active, nil
}

func (store *Store) ReadResolvedConfig(
	ctx context.Context,
	environmentKey string,
	configKey string,
	ifNoneMatch string,
) (machine.ResolvedConfig, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return machine.ResolvedConfig{}, fmt.Errorf("begin resolved Config snapshot: %w", err)
	}
	defer transaction.Rollback()

	var (
		environmentID []byte
		configID      []byte
		revision      uint64
		format        string
		canonical     []byte
	)
	err = transaction.QueryRowContext(ctx, `
		SELECT environment.id, config.id, state.current_revision, revision_record.format, revision_record.content
		FROM environments AS environment
		JOIN config_env_states AS state ON state.environment_id = environment.id
		JOIN configs AS config ON config.id = state.config_id
		JOIN config_revisions AS revision_record
		  ON revision_record.config_id = state.config_id
		 AND revision_record.environment_id = state.environment_id
		 AND revision_record.revision = state.current_revision
		WHERE environment.resource_key = ?
		  AND environment.archived_at IS NULL
		  AND config.resource_key = ?
		  AND config.archived_at IS NULL
	`, environmentKey, configKey).Scan(&environmentID, &configID, &revision, &format, &canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return machine.ResolvedConfig{}, machine.ErrNotFound
	}
	if err != nil {
		return machine.ResolvedConfig{}, fmt.Errorf("read current Config Revision: %w", err)
	}
	if len(environmentID) != 16 || len(configID) != 16 || revision == 0 || len(canonical) > 5<<20 {
		return machine.ResolvedConfig{}, errors.New("Config Revision failed integrity validation")
	}

	references, err := readReferences(ctx, transaction, configID, environmentID, revision)
	if err != nil {
		return machine.ResolvedConfig{}, err
	}
	items, err := readVaultMetadata(ctx, transaction, environmentID, references)
	if err != nil {
		return machine.ResolvedConfig{}, err
	}
	etag := resolvedConfigETag(configID, revision, items)
	result := machine.ResolvedConfig{
		Format:         format,
		ConfigRevision: revision,
		VaultRevisions: make(map[string]uint64, len(items)),
		ETag:           etag,
	}
	for identity, item := range items {
		result.VaultRevisions[identity.revisionKey()] = item.revision
	}
	if ifNoneMatch == etag {
		if err := transaction.Commit(); err != nil {
			return machine.ResolvedConfig{}, fmt.Errorf("commit unchanged Config snapshot: %w", err)
		}
		return result, machine.ErrNotModified
	}

	values := make(map[configdoc.Reference]string, len(references))
	for identity, item := range items {
		itemValues, err := store.readVaultTextValues(ctx, transaction, item, references[identity])
		if err != nil {
			return machine.ResolvedConfig{}, err
		}
		for fieldKey, value := range itemValues {
			values[configdoc.Reference{NamespaceKey: identity.namespaceKey, ItemKey: identity.itemKey, FieldKey: fieldKey}] = value
		}
	}
	resolved, err := configdoc.Resolve(configdoc.Format(format), canonical, func(reference configdoc.Reference) (string, error) {
		value, ok := values[reference]
		if !ok {
			return "", machine.ErrUnresolved
		}
		return value, nil
	})
	if err != nil {
		return machine.ResolvedConfig{}, fmt.Errorf("resolve Config: %w", machine.ErrUnresolved)
	}
	if err := transaction.Commit(); err != nil {
		clear(resolved)
		return machine.ResolvedConfig{}, fmt.Errorf("commit resolved Config snapshot: %w", err)
	}
	result.Content = string(resolved)
	clear(resolved)
	return result, nil
}

func (store *Store) ReadFile(
	ctx context.Context,
	environmentKey string,
	namespaceKey string,
	itemKey string,
	fieldKey string,
	ifNoneMatch string,
) (machine.FileContent, error) {
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return machine.FileContent{}, fmt.Errorf("begin File snapshot: %w", err)
	}
	defer transaction.Rollback()
	var environmentID []byte
	err = transaction.QueryRowContext(ctx, `
		SELECT id FROM environments WHERE resource_key = ? AND archived_at IS NULL
	`, environmentKey).Scan(&environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return machine.FileContent{}, machine.ErrNotFound
	}
	if err != nil {
		return machine.FileContent{}, fmt.Errorf("read File Environment: %w", err)
	}
	metadata, err := readOneVaultMetadata(ctx, transaction, environmentID, vaultItemIdentity{namespaceKey: namespaceKey, itemKey: itemKey})
	if errors.Is(err, machine.ErrUnresolved) {
		return machine.FileContent{}, machine.ErrNotFound
	}
	if err != nil {
		return machine.FileContent{}, err
	}
	etag := vaultFileETag(metadata.itemID, metadata.revision, fieldKey)
	result := machine.FileContent{VaultRevision: metadata.revision, ETag: etag}
	if ifNoneMatch == etag {
		if err := transaction.Commit(); err != nil {
			return machine.FileContent{}, fmt.Errorf("commit unchanged File snapshot: %w", err)
		}
		return result, machine.ErrNotModified
	}
	fieldTypes, err := readFieldTypes(ctx, transaction, metadata, []string{fieldKey})
	if err != nil || fieldTypes[fieldKey] != "file" {
		return machine.FileContent{}, machine.ErrNotFound
	}
	payload, err := store.decryptVaultPayload(ctx, transaction, metadata)
	if err != nil {
		return machine.FileContent{}, err
	}
	variant, ok := payload.Variants[hex.EncodeToString(metadata.variantID)]
	if !ok {
		return machine.FileContent{}, machine.ErrNotFound
	}
	value, ok := variant.Values[fieldKey]
	if !ok || value.File == nil || len(value.File.Bytes) > 5<<20 {
		return machine.FileContent{}, machine.ErrNotFound
	}
	result.Bytes = bytes.Clone(value.File.Bytes)
	result.Filename = value.File.Filename
	result.ContentType = value.File.ContentType
	if err := transaction.Commit(); err != nil {
		clear(result.Bytes)
		return machine.FileContent{}, fmt.Errorf("commit File snapshot: %w", err)
	}
	return result, nil
}

type vaultMetadata struct {
	itemID    []byte
	revision  uint64
	variantID []byte
}

type vaultItemIdentity struct {
	namespaceKey string
	itemKey      string
}

func (identity vaultItemIdentity) revisionKey() string {
	return identity.namespaceKey + "." + identity.itemKey
}

func readReferences(
	ctx context.Context,
	transaction *sql.Tx,
	configID []byte,
	environmentID []byte,
	revision uint64,
) (map[vaultItemIdentity][]string, error) {
	rows, err := transaction.QueryContext(ctx, `
			SELECT namespace_key, item_key, field_key
		FROM config_revision_vault_refs
		WHERE config_id = ? AND environment_id = ? AND revision = ?
			ORDER BY namespace_key, item_key, field_key
	`, configID, environmentID, revision)
	if err != nil {
		return nil, fmt.Errorf("read Config Vault References: %w", err)
	}
	defer rows.Close()
	references := make(map[vaultItemIdentity][]string)
	for rows.Next() {
		var namespaceKey, itemKey, fieldKey string
		if err := rows.Scan(&namespaceKey, &itemKey, &fieldKey); err != nil {
			return nil, fmt.Errorf("scan Config Vault Reference: %w", err)
		}
		identity := vaultItemIdentity{namespaceKey: namespaceKey, itemKey: itemKey}
		references[identity] = append(references[identity], fieldKey)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Config Vault References: %w", err)
	}
	return references, nil
}

func readVaultMetadata(
	ctx context.Context,
	transaction *sql.Tx,
	environmentID []byte,
	references map[vaultItemIdentity][]string,
) (map[vaultItemIdentity]vaultMetadata, error) {
	items := make(map[vaultItemIdentity]vaultMetadata, len(references))
	for identity := range references {
		metadata, err := readOneVaultMetadata(ctx, transaction, environmentID, identity)
		if err != nil {
			return nil, err
		}
		items[identity] = metadata
	}
	return items, nil
}

func readOneVaultMetadata(
	ctx context.Context,
	transaction *sql.Tx,
	environmentID []byte,
	identity vaultItemIdentity,
) (vaultMetadata, error) {
	var metadata vaultMetadata
	err := transaction.QueryRowContext(ctx, `
		SELECT item.id, item.current_revision, binding.variant_id
		FROM vault_items AS item
		JOIN vault_revision_variant_environments AS binding
		  ON binding.item_id = item.id
		 AND binding.revision = item.current_revision
			WHERE item.namespace_key = ?
			  AND item.resource_key = ?
			  AND item.archived_at IS NULL
			  AND binding.environment_id = ?
		`, identity.namespaceKey, identity.itemKey, environmentID).Scan(&metadata.itemID, &metadata.revision, &metadata.variantID)
	if errors.Is(err, sql.ErrNoRows) {
		return vaultMetadata{}, machine.ErrUnresolved
	}
	if err != nil {
		return vaultMetadata{}, fmt.Errorf("read Vault Item metadata: %w", err)
	}
	if len(metadata.itemID) != 16 || len(metadata.variantID) != 16 || metadata.revision == 0 {
		return vaultMetadata{}, fmt.Errorf("Vault Item metadata: %w", machine.ErrIntegrity)
	}
	return metadata, nil
}

func (store *Store) readVaultTextValues(
	ctx context.Context,
	transaction *sql.Tx,
	metadata vaultMetadata,
	fieldKeys []string,
) (map[string]string, error) {
	fieldTypes, err := readFieldTypes(ctx, transaction, metadata, fieldKeys)
	if err != nil {
		return nil, err
	}
	payload, err := store.decryptVaultPayload(ctx, transaction, metadata)
	if err != nil {
		return nil, err
	}
	variant, ok := payload.Variants[hex.EncodeToString(metadata.variantID)]
	if !ok {
		return nil, machine.ErrUnresolved
	}
	values := make(map[string]string, len(fieldKeys))
	for _, fieldKey := range fieldKeys {
		if fieldTypes[fieldKey] != "text" && fieldTypes[fieldKey] != "secret" {
			return nil, machine.ErrUnresolved
		}
		value, ok := variant.Values[fieldKey]
		if !ok || value.Text == nil || len(*value.Text) > 512<<10 {
			return nil, machine.ErrUnresolved
		}
		values[fieldKey] = *value.Text
	}
	return values, nil
}

func readFieldTypes(
	ctx context.Context,
	transaction *sql.Tx,
	metadata vaultMetadata,
	fieldKeys []string,
) (map[string]string, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT resource_key, field_type
		FROM vault_revision_fields
		WHERE item_id = ? AND revision = ? AND archived = FALSE
	`, metadata.itemID, metadata.revision)
	if err != nil {
		return nil, fmt.Errorf("read Vault Field metadata: %w", err)
	}
	defer rows.Close()
	types := make(map[string]string, len(fieldKeys))
	for rows.Next() {
		var key, fieldType string
		if err := rows.Scan(&key, &fieldType); err != nil {
			return nil, fmt.Errorf("scan Vault Field metadata: %w", err)
		}
		types[key] = fieldType
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Vault Field metadata: %w", err)
	}
	return types, nil
}

func (store *Store) decryptVaultPayload(
	ctx context.Context,
	transaction *sql.Tx,
	metadata vaultMetadata,
) (vaultdoc.Payload, error) {
	var encrypted vaultcrypto.EncryptedSnapshot
	err := transaction.QueryRowContext(ctx, `
		SELECT algorithm, key_version, nonce, ciphertext, encrypted_dek
		FROM vault_item_revisions
		WHERE item_id = ? AND revision = ?
	`, metadata.itemID, metadata.revision).Scan(
		&encrypted.Algorithm,
		&encrypted.KeyVersion,
		&encrypted.Nonce,
		&encrypted.Ciphertext,
		&encrypted.EncryptedDEK,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return vaultdoc.Payload{}, machine.ErrUnresolved
	}
	if err != nil {
		return vaultdoc.Payload{}, fmt.Errorf("read encrypted Vault Item Revision: %w", err)
	}
	if len(encrypted.Ciphertext) > vaultdoc.MaxSnapshotBytes+32 {
		return vaultdoc.Payload{}, machine.ErrIntegrity
	}
	plaintext, err := store.provider.DecryptSnapshot(vaultcrypto.SnapshotIdentity{
		ItemID:   hex.EncodeToString(metadata.itemID),
		Revision: metadata.revision,
	}, encrypted)
	if err != nil {
		return vaultdoc.Payload{}, machine.ErrIntegrity
	}
	defer clear(plaintext)
	payload, err := vaultdoc.Decode(plaintext)
	if err != nil {
		return vaultdoc.Payload{}, machine.ErrIntegrity
	}
	return payload, nil
}

func resolvedConfigETag(configID []byte, revision uint64, items map[vaultItemIdentity]vaultMetadata) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("configra:resolved-config-etag:v1\x00"))
	_, _ = hash.Write(configID)
	writeUint64(hash, revision)
	identities := make([]vaultItemIdentity, 0, len(items))
	for identity := range items {
		identities = append(identities, identity)
	}
	slices.SortFunc(identities, func(left, right vaultItemIdentity) int {
		return strings.Compare(left.revisionKey(), right.revisionKey())
	})
	for _, identity := range identities {
		writeLengthPrefixed(hash, []byte(identity.namespaceKey))
		writeLengthPrefixed(hash, []byte(identity.itemKey))
		_, _ = hash.Write(items[identity].itemID)
		writeUint64(hash, items[identity].revision)
	}
	return `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`
}

func vaultFileETag(itemID []byte, revision uint64, fieldKey string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("configra:vault-file-etag:v1\x00"))
	_, _ = hash.Write(itemID)
	writeUint64(hash, revision)
	writeLengthPrefixed(hash, []byte(fieldKey))
	return `"` + base64.RawURLEncoding.EncodeToString(hash.Sum(nil)) + `"`
}

func writeUint64(destination io.Writer, value uint64) {
	_ = binary.Write(destination, binary.BigEndian, value)
}

func writeLengthPrefixed(destination io.Writer, value []byte) {
	_ = binary.Write(destination, binary.BigEndian, uint32(len(value)))
	_, _ = destination.Write(value)
}

var _ machine.Repository = (*Store)(nil)
