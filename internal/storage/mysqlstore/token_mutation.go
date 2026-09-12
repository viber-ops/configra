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
)

type TokenCreate struct {
	OperationID      string
	Actor            Actor
	DisplayName      string
	EnvironmentKeys  []string
	AllowWithoutMTLS bool
	ExpiresAt        time.Time
	NeverExpires     bool
}

type TokenCreateResult struct {
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
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return TokenCreateResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	validationErr := validateTokenCreate(request)
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
	environmentKeys, environmentIDs, err := activeEnvironmentIDs(ctx, transaction, request.EnvironmentKeys)
	if errors.Is(err, ErrValidation) {
		return store.finishTokenCreateFailure(ctx, transaction, request, err)
	}
	if err != nil {
		return TokenCreateResult{}, err
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
			(id, public_id, display_name, display_prefix, secret_digest, allow_without_mtls, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, tokenID, publicID, request.DisplayName, displayPrefix, secretDigest[:], request.AllowWithoutMTLS, databaseExpiry); err != nil {
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
		Outcome:          OutcomeSuccess,
		PublicID:         publicID,
		DisplayPrefix:    displayPrefix,
		Token:            plaintext,
		EnvironmentKeys:  environmentKeys,
		AllowWithoutMTLS: request.AllowWithoutMTLS,
		ExpiresAt:        resultExpiry,
	}
	persisted := result
	persisted.Token = ""
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, persisted, mutationEvent{
		Type: "token.created", Action: "token.create", ResourceType: "api_token", ResourceKey: publicID,
	}, true); err != nil {
		return TokenCreateResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenCreateResult{}, fmt.Errorf("commit API Token creation: %w", err)
	}
	return result, nil
}

func (store *Store) SetTokenEnvironments(ctx context.Context, request TokenEnvironmentChange) (TokenEnvironmentResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return TokenEnvironmentResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	validationErr := error(nil)
	if !validTokenPublicID(request.PublicID) {
		validationErr = errors.New("invalid API Token public ID")
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
	err = transaction.QueryRowContext(ctx, `
		SELECT id, revoked_at FROM api_tokens WHERE public_id = ? FOR UPDATE
	`, request.PublicID).Scan(&tokenID, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) || revokedAt.Valid {
		return finishTokenEnvironmentFailure(ctx, transaction, request, errors.New("API Token is missing or revoked"))
	}
	if err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("lock API Token: %w", err)
	}
	keys, ids, err := activeEnvironmentIDs(ctx, transaction, request.EnvironmentKeys)
	if errors.Is(err, ErrValidation) {
		return finishTokenEnvironmentFailure(ctx, transaction, request, err)
	}
	if err != nil {
		return TokenEnvironmentResult{}, err
	}
	current, err := tokenEnvironmentKeys(ctx, transaction, tokenID)
	if err != nil {
		return TokenEnvironmentResult{}, err
	}
	result := TokenEnvironmentResult{Outcome: OutcomeNoChange, PublicID: request.PublicID, EnvironmentKeys: keys}
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
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "token.environments_updated", Action: "token.set_environments", ResourceType: "api_token", ResourceKey: request.PublicID,
	}, result.Outcome == OutcomeSuccess); err != nil {
		return TokenEnvironmentResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("commit API Token Environment change: %w", err)
	}
	return result, nil
}

func (store *Store) RevokeToken(ctx context.Context, request TokenRevoke) (TokenRevokeResult, error) {
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
		if _, err := transaction.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = UTC_TIMESTAMP(6) WHERE id = ?`, tokenID); err != nil {
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

func validateTokenCreate(request TokenCreate) error {
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
		Type: "token.validation_failed", Action: "token.set_environments", ResourceType: "api_token", ResourceKey: request.PublicID,
	}, false); err != nil {
		return TokenEnvironmentResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return TokenEnvironmentResult{}, fmt.Errorf("commit API Token Environment validation Audit: %w", err)
	}
	return result, withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
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
