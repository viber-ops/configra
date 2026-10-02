package mysqlstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

type TokenCreate struct {
	Kind                   string   `json:",omitempty"`
	ConfigKeys             []string `json:",omitempty"`
	NamespaceKeys          []string `json:",omitempty"`
	parentPublicID         string
	certificateFingerprint []byte
	OperationID            string
	Actor                  Actor
	DisplayName            string
	EnvironmentKeys        []string
	AllowWithoutMTLS       bool
	ExpiresAt              time.Time
	NeverExpires           bool
}

type TokenCreateResult struct {
	Kind             string     `json:"kind"`
	ConfigKeys       []string   `json:"config_keys"`
	NamespaceKeys    []string   `json:"namespace_keys"`
	Outcome          Outcome    `json:"outcome"`
	PublicID         string     `json:"public_id"`
	DisplayPrefix    string     `json:"display_prefix"`
	Token            string     `json:"token,omitempty"`
	EnvironmentKeys  []string   `json:"environment_keys"`
	AllowWithoutMTLS bool       `json:"allow_without_mtls"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
}

type TokenEnvironmentChange struct {
	OperationID     string
	Actor           Actor
	PublicID        string
	EnvironmentKeys []string
	// Omit new fields from legacy PUT digests so existing operations still replay.
	Patch  bool     `json:",omitempty"`
	Add    []string `json:",omitempty"`
	Remove []string `json:",omitempty"`
}

type TokenEnvironmentResult struct {
	Outcome         Outcome  `json:"outcome"`
	PublicID        string   `json:"public_id"`
	EnvironmentKeys []string `json:"environment_keys"`
}

type TokenRevoke struct {
	OperationID string
	Actor       Actor
	PublicID    string
}

type TokenRevokeResult struct {
	Outcome  Outcome `json:"outcome"`
	PublicID string  `json:"public_id"`
	Revoked  bool    `json:"revoked"`
}

func (store *Store) CreateToken(ctx context.Context, request TokenCreate) (TokenCreateResult, error) {
	if request.Actor.Type == "token" {
		return TokenCreateResult{}, machine.ErrForbidden
	}
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return TokenCreateResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	validationErr := store.validateTokenCreate(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return TokenCreateResult{}, fmt.Errorf("begin API Token creation: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return TokenCreateResult{}, err
	}
	if replayed {
		var previous TokenCreateResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return TokenCreateResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return store.finishTokenCreateFailure(ctx, transaction, request, validationErr)
	}
	result, err := createTokenRecord(ctx, transaction, request)
	if errors.Is(err, ErrValidation) {
		return store.finishTokenCreateFailure(ctx, transaction, request, err)
	}
	if err != nil {
		return TokenCreateResult{}, err
	}
	persisted := result
	persisted.Token = ""
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, persisted, mutationEvent{
		Type: "token.created", Action: "token.create", ResourceType: "api_token", ResourceKey: result.PublicID,
	}, true); err != nil {
		return TokenCreateResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenCreateResult{}, fmt.Errorf("commit API Token creation: %w", err)
	}
	return result, nil
}

func (store *Store) SetTokenEnvironments(ctx context.Context, request TokenEnvironmentChange) (TokenEnvironmentResult, error) {
	if request.Actor.Type == "token" {
		return TokenEnvironmentResult{}, machine.ErrForbidden
	}
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return TokenEnvironmentResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	validationErr := error(nil)
	if !validTokenPublicID(request.PublicID) {
		validationErr = errors.New("invalid API Token public ID")
	}
	if request.Patch {
		seen := make(map[string]bool, len(request.Add)+len(request.Remove))
		for _, keys := range [][]string{request.Add, request.Remove} {
			for _, key := range keys {
				if !validResourceKey(key) || seen[key] {
					validationErr = errors.New("invalid, duplicate or overlapping Environment key")
				}
				seen[key] = true
			}
		}
		if len(request.EnvironmentKeys) != 0 {
			validationErr = errors.New("cannot combine replacement and incremental grants")
		}
	} else if len(request.Add)+len(request.Remove) != 0 {
		validationErr = errors.New("incremental grants require Patch")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("begin API Token Environment change: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return TokenEnvironmentResult{}, err
	}
	if replayed {
		var previous TokenEnvironmentResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return TokenEnvironmentResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return finishTokenEnvironmentFailure(ctx, transaction, request, validationErr)
	}
	var tokenID []byte
	var revokedAt sql.NullTime
	var immutableScope bool
	err = transaction.QueryRowContext(ctx, `
		SELECT id, revoked_at, kind = 'write-scoped' OR parent_public_id IS NOT NULL FROM api_tokens WHERE public_id = ? FOR UPDATE
	`, request.PublicID).Scan(&tokenID, &revokedAt, &immutableScope)
	if errors.Is(err, sql.ErrNoRows) || revokedAt.Valid || immutableScope {
		return finishTokenEnvironmentFailure(ctx, transaction, request, errors.New("API Token is missing or revoked"))
	}
	if err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("lock API Token: %w", err)
	}
	requested := request.EnvironmentKeys
	if request.Patch {
		requested = request.Add
	}
	keys, ids, err := activeEnvironmentIDs(ctx, transaction, requested)
	if errors.Is(err, ErrValidation) {
		return finishTokenEnvironmentFailure(ctx, transaction, request, err)
	}
	if err != nil {
		return TokenEnvironmentResult{}, err
	}
	result := TokenEnvironmentResult{Outcome: OutcomeNoChange, PublicID: request.PublicID}
	if request.Patch {
		// The token row lock serializes replacement, patch and revocation. Only
		// explicitly named grants are touched; no full grant set is read or returned.
		for _, key := range keys {
			changed, err := transaction.ExecContext(ctx, `INSERT INTO api_token_environments (token_id, environment_id)
				SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM api_token_environments WHERE token_id = ? AND environment_id = ?)`, tokenID, ids[key], tokenID, ids[key])
			if err != nil {
				return TokenEnvironmentResult{}, fmt.Errorf("add API Token Environment grant: %w", err)
			}
			if count, err := changed.RowsAffected(); err != nil {
				return TokenEnvironmentResult{}, err
			} else if count > 0 {
				result.Outcome = OutcomeSuccess
			}
		}
		for _, key := range request.Remove {
			changed, err := transaction.ExecContext(ctx, `DELETE grant_record FROM api_token_environments AS grant_record
				JOIN environments AS environment ON environment.id = grant_record.environment_id
				WHERE grant_record.token_id = ? AND environment.resource_key = ?`, tokenID, key)
			if err != nil {
				return TokenEnvironmentResult{}, fmt.Errorf("remove API Token Environment grant: %w", err)
			}
			if count, err := changed.RowsAffected(); err != nil {
				return TokenEnvironmentResult{}, err
			} else if count > 0 {
				result.Outcome = OutcomeSuccess
			}
		}
	} else {
		current, err := tokenEnvironmentKeys(ctx, transaction, tokenID)
		if err != nil {
			return TokenEnvironmentResult{}, err
		}
		result.EnvironmentKeys = keys
		if !slices.Equal(current, keys) {
			if _, err := transaction.ExecContext(ctx, `DELETE FROM api_token_environments WHERE token_id = ?`, tokenID); err != nil {
				return TokenEnvironmentResult{}, fmt.Errorf("clear API Token Environment grants: %w", err)
			}
			for _, key := range keys {
				if _, err := transaction.ExecContext(ctx, `INSERT INTO api_token_environments (token_id, environment_id) VALUES (?, ?)`, tokenID, ids[key]); err != nil {
					return TokenEnvironmentResult{}, fmt.Errorf("replace API Token Environment grant: %w", err)
				}
			}
			result.Outcome = OutcomeSuccess
		}
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "token.environments_updated", Action: request.action(), ResourceType: "api_token", ResourceKey: request.PublicID,
	}, result.Outcome == OutcomeSuccess); err != nil {
		return TokenEnvironmentResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("commit API Token Environment change: %w", err)
	}
	return result, nil
}

func (store *Store) RevokeToken(ctx context.Context, request TokenRevoke) (TokenRevokeResult, error) {
	if request.Actor.Type == "token" {
		return TokenRevokeResult{}, machine.ErrForbidden
	}
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return TokenRevokeResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return TokenRevokeResult{}, fmt.Errorf("begin API Token revocation: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return TokenRevokeResult{}, err
	}
	if replayed {
		var previous TokenRevokeResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return TokenRevokeResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if !validTokenPublicID(request.PublicID) {
		return finishTokenRevokeFailure(ctx, transaction, request, errors.New("invalid API Token public ID"))
	}
	var tokenID []byte
	var revokedAt sql.NullTime
	err = transaction.QueryRowContext(ctx, `
		SELECT id, revoked_at FROM api_tokens WHERE public_id = ? FOR UPDATE
	`, request.PublicID).Scan(&tokenID, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return finishTokenRevokeFailure(ctx, transaction, request, errors.New("API Token does not exist"))
	}
	if err != nil {
		return TokenRevokeResult{}, fmt.Errorf("lock API Token: %w", err)
	}
	result := TokenRevokeResult{Outcome: OutcomeNoChange, PublicID: request.PublicID, Revoked: true}
	if !revokedAt.Valid {
		if err := revokeTokenAndDeployments(ctx, transaction, request.PublicID); err != nil {
			return TokenRevokeResult{}, fmt.Errorf("revoke API Token: %w", err)
		}
		result.Outcome = OutcomeSuccess
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "token.revoked", Action: "token.revoke", ResourceType: "api_token", ResourceKey: request.PublicID,
	}, result.Outcome == OutcomeSuccess); err != nil {
		return TokenRevokeResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenRevokeResult{}, fmt.Errorf("commit API Token revocation: %w", err)
	}
	return result, nil
}

func (store *Store) validateTokenCreate(request TokenCreate) error {
	if request.Kind != "" && request.Kind != machine.TokenReadOnly && request.Kind != machine.TokenWriteScoped {
		return ErrValidation
	}
	if validateScopeKeys(request.ConfigKeys) != nil || validateScopeKeys(request.NamespaceKeys) != nil {
		return ErrValidation
	}
	if request.Kind == machine.TokenWriteScoped {
		maxTTL := store.WriteTokenMaxTTL
		if maxTTL <= 0 || maxTTL > 90*24*time.Hour {
			maxTTL = 90 * 24 * time.Hour
		}
		if request.Actor.Type != "user" || len(request.EnvironmentKeys) == 0 || request.AllowWithoutMTLS || request.NeverExpires || request.ExpiresAt.IsZero() || request.ExpiresAt.After(time.Now().Add(maxTTL)) {
			return ErrValidation
		}
	}
	if request.DisplayName == "" || len(request.DisplayName) > 255 {
		return errors.New("API Token display name is required and must not exceed 255 bytes")
	}
	if request.NeverExpires && !request.ExpiresAt.IsZero() {
		return errors.New("API Token cannot set both NeverExpires and ExpiresAt")
	}
	if !request.ExpiresAt.IsZero() && !time.Now().Before(request.ExpiresAt) {
		return errors.New("API Token expiry must be in the future")
	}
	return nil
}

func activeEnvironmentIDs(ctx context.Context, transaction *sql.Tx, requested []string) ([]string, map[string][]byte, error) {
	keys := slices.Clone(requested)
	slices.Sort(keys)
	for index, key := range keys {
		if !validResourceKey(key) || (index > 0 && key == keys[index-1]) {
			return nil, nil, fmt.Errorf("invalid or duplicate Environment %q: %w", key, ErrValidation)
		}
	}
	ids := make(map[string][]byte, len(keys))
	for _, key := range keys {
		var id []byte
		err := transaction.QueryRowContext(ctx, `
			SELECT id FROM environments WHERE resource_key = ? AND archived_at IS NULL
		`, key).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("Environment %s does not exist or is Archived: %w", key, ErrValidation)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read API Token Environment: %w", err)
		}
		ids[key] = id
	}
	return keys, ids, nil
}

func tokenEnvironmentKeys(ctx context.Context, transaction *sql.Tx, tokenID []byte) ([]string, error) {
	rows, err := transaction.QueryContext(ctx, `
		SELECT environment.resource_key
		FROM api_token_environments AS grant_record
		JOIN environments AS environment ON environment.id = grant_record.environment_id
		WHERE grant_record.token_id = ? ORDER BY environment.resource_key
	`, tokenID)
	if err != nil {
		return nil, fmt.Errorf("read API Token Environment grants: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan API Token Environment grant: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate API Token Environment grants: %w", err)
	}
	return keys, nil
}

func (store *Store) finishTokenCreateFailure(ctx context.Context, transaction *sql.Tx, request TokenCreate, cause error) (TokenCreateResult, error) {
	result := TokenCreateResult{Outcome: OutcomeValidationFailed}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "token.validation_failed", Action: "token.create", ResourceType: "api_token",
	}, false); err != nil {
		return TokenCreateResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenCreateResult{}, fmt.Errorf("commit API Token validation Audit: %w", err)
	}
	return result, withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
}

func finishTokenEnvironmentFailure(ctx context.Context, transaction *sql.Tx, request TokenEnvironmentChange, cause error) (TokenEnvironmentResult, error) {
	result := TokenEnvironmentResult{Outcome: OutcomeValidationFailed, PublicID: request.PublicID}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "token.validation_failed", Action: request.action(), ResourceType: "api_token", ResourceKey: request.PublicID,
	}, false); err != nil {
		return TokenEnvironmentResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("commit API Token Environment validation Audit: %w", err)
	}
	return result, withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
}

func (request TokenEnvironmentChange) action() string {
	if request.Patch {
		return "token.patch_environments"
	}
	return "token.set_environments"
}

func finishTokenRevokeFailure(ctx context.Context, transaction *sql.Tx, request TokenRevoke, cause error) (TokenRevokeResult, error) {
	result := TokenRevokeResult{Outcome: OutcomeValidationFailed, PublicID: request.PublicID}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "token.validation_failed", Action: "token.revoke", ResourceType: "api_token", ResourceKey: request.PublicID,
	}, false); err != nil {
		return TokenRevokeResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenRevokeResult{}, fmt.Errorf("commit API Token revocation validation Audit: %w", err)
	}
	return result, withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
}

func tokenRequestDigest(request any) [sha256.Size]byte {
	// Preserve legacy digests while distinguishing omitted scopes from explicit
	// empty (deny-all) scopes; encoding/json omitempty alone collapses them.
	if token, ok := request.(TokenCreate); ok && (token.ConfigKeys != nil || token.NamespaceKeys != nil) {
		request = struct {
			Request                           TokenCreate
			ConfigScopeSet, NamespaceScopeSet bool
		}{token, token.ConfigKeys != nil, token.NamespaceKeys != nil}
	}
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func validTokenPublicID(value string) bool {
	if len(value) < 6 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func createTokenRecord(ctx context.Context, transaction *sql.Tx, request TokenCreate) (TokenCreateResult, error) {
	environmentKeys, environmentIDs, err := activeEnvironmentIDs(ctx, transaction, request.EnvironmentKeys)
	if err != nil {
		return TokenCreateResult{}, err
	}
	kind := request.Kind
	if kind == "" {
		kind = machine.TokenReadOnly
	}
	tokenID, err := randomID()
	if err != nil {
		return TokenCreateResult{}, err
	}
	publicBytes := make([]byte, 8)
	secret := make([]byte, 32)
	if _, err := rand.Read(publicBytes); err != nil {
		return TokenCreateResult{}, fmt.Errorf("generate API Token public ID: %w", err)
	}
	if _, err := rand.Read(secret); err != nil {
		return TokenCreateResult{}, fmt.Errorf("generate API Token Secret: %w", err)
	}
	defer clear(secret)
	publicID := hex.EncodeToString(publicBytes)
	displayPrefix := "cfg_" + publicID
	secretDigest := sha256.Sum256(secret)
	plaintext := displayPrefix + "_" + base64.RawURLEncoding.EncodeToString(secret)
	expiresAt := request.ExpiresAt.UTC()
	if request.NeverExpires {
		expiresAt = time.Time{}
	} else if expiresAt.IsZero() {
		expiresAt = time.Now().UTC().Add(90 * 24 * time.Hour)
	}
	var databaseExpiry any
	var resultExpiry *time.Time
	if !expiresAt.IsZero() {
		databaseExpiry = expiresAt
		resultExpiry = &expiresAt
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO api_tokens
			(id, public_id, display_name, display_prefix, secret_digest, allow_without_mtls, expires_at,
 kind, config_keys, namespace_keys, parent_public_id, deployment_certificate_fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tokenID, publicID, request.DisplayName, displayPrefix, secretDigest[:], request.AllowWithoutMTLS, databaseExpiry, kind, scopeJSON(request.ConfigKeys), scopeJSON(request.NamespaceKeys), nullableString(request.parentPublicID), nullableBytes(request.certificateFingerprint)); err != nil {
		return TokenCreateResult{}, fmt.Errorf("insert API Token: %w", err)
	}
	for _, key := range environmentKeys {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO api_token_environments (token_id, environment_id) VALUES (?, ?)
		`, tokenID, environmentIDs[key]); err != nil {
			return TokenCreateResult{}, fmt.Errorf("grant API Token Environment: %w", err)
		}
	}
	result := TokenCreateResult{
		Outcome: OutcomeSuccess,
		Kind:    kind, ConfigKeys: request.ConfigKeys, NamespaceKeys: request.NamespaceKeys,
		PublicID:         publicID,
		DisplayPrefix:    displayPrefix,
		Token:            plaintext,
		EnvironmentKeys:  environmentKeys,
		AllowWithoutMTLS: request.AllowWithoutMTLS,
		ExpiresAt:        resultExpiry,
	}
	return result, nil
}

func validateScopeKeys(keys []string) error {
	if len(keys) > 256 {
		return ErrValidation
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !validResourceKey(key) || seen[key] {
			return ErrValidation
		}
		seen[key] = true
	}
	return nil
}

func scopeJSON(keys []string) any {
	if keys == nil {
		return nil
	}
	encoded, _ := json.Marshal(keys)
	return string(encoded)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
