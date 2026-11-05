package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/viber-ops/configra/internal/machine"
)

type DeploymentCredentialIssue struct {
	OperationID    string
	Actor          Actor
	EnvironmentKey string
	DisplayName    string
	AuthorityID    string
	ExpiresAt      time.Time
}

type DeploymentCredentialResult struct {
	Outcome     Outcome                      `json:"outcome"`
	Token       TokenCreateResult            `json:"token"`
	Certificate ClientCertificateIssueResult `json:"certificate"`
}

func (store *Store) IssueDeploymentCredential(ctx context.Context, request DeploymentCredentialIssue) (DeploymentCredentialResult, error) {
	if !validOperationID(request.OperationID) || !validScopedActor(request.Actor) || request.Actor.Type != "token" {
		return DeploymentCredentialResult{}, ErrValidation
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return DeploymentCredentialResult{}, errors.New("begin deployment issuance")
	}
	defer tx.Rollback()
	if err := store.lockMasterKey(ctx, tx, false); err != nil {
		return DeploymentCredentialResult{}, err
	}
	token, err := authorizeScopedWrite(ctx, tx, request.Actor, request.EnvironmentKey, "", "")
	if err != nil {
		return DeploymentCredentialResult{}, err
	}
	replayed, replay, err := beginOperation(ctx, tx, request.OperationID, pkiDigest("deployment.issue", request), request.Actor)
	if err != nil {
		return DeploymentCredentialResult{}, err
	}
	if replayed {
		var previous DeploymentCredentialResult
		if json.Unmarshal(replay.Response, &previous) != nil || previous.Outcome != replay.Outcome {
			return previous, errors.New("invalid deployment replay")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if !validResourceKey(request.EnvironmentKey) || !time.Now().Before(request.ExpiresAt) || request.ExpiresAt.After(token.ExpiresAt) {
		return DeploymentCredentialResult{}, ErrValidation
	}
	certificate, err := store.issueClientCertificateRecord(ctx, tx, ClientCertificateIssue{
		Actor: request.Actor, AuthorityID: request.AuthorityID, DisplayName: request.DisplayName, notAfter: request.ExpiresAt,
	})
	if err != nil {
		return DeploymentCredentialResult{}, err
	}
	expiresAt := request.ExpiresAt.UTC()
	if certificate.Certificate.NotAfter.Before(expiresAt) {
		expiresAt = certificate.Certificate.NotAfter
	}
	fingerprint, _ := hex.DecodeString(certificate.Certificate.FingerprintSHA256)
	created, err := createTokenRecord(ctx, tx, TokenCreate{
		Actor: request.Actor, Kind: machine.TokenReadOnly, DisplayName: request.DisplayName, EnvironmentKeys: []string{request.EnvironmentKey},
		ConfigKeys: token.ConfigKeys, NamespaceKeys: token.NamespaceKeys, ExpiresAt: expiresAt,
		parentPublicID: token.PublicID, certificateFingerprint: fingerprint,
	})
	if err != nil {
		return DeploymentCredentialResult{}, err
	}
	result := DeploymentCredentialResult{Outcome: OutcomeSuccess, Token: created, Certificate: certificate}
	persisted := result
	persisted.Token.Token, persisted.Certificate.ExportBundle = "", ""
	if err := finishOperation(ctx, tx, request.OperationID, request.Actor, result.Outcome, 0, persisted, mutationEvent{
		Type: "deployment.issued", Action: "deployment.issue", EnvironmentKey: request.EnvironmentKey, ResourceType: "api_token", ResourceKey: created.PublicID,
	}, false); err != nil {
		return DeploymentCredentialResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeploymentCredentialResult{}, errors.New("commit deployment issuance")
	}
	return result, nil
}

type DeploymentCredentialRevoke struct {
	OperationID    string
	Actor          Actor
	EnvironmentKey string
	PublicID       string
}

func (store *Store) RevokeDeploymentCredential(ctx context.Context, request DeploymentCredentialRevoke) (TokenRevokeResult, error) {
	if !validOperationID(request.OperationID) || !validScopedActor(request.Actor) || request.Actor.Type != "token" || !validTokenPublicID(request.PublicID) {
		return TokenRevokeResult{}, ErrValidation
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return TokenRevokeResult{}, errors.New("begin deployment revocation")
	}
	defer tx.Rollback()
	if _, err := authorizeScopedWrite(ctx, tx, request.Actor, request.EnvironmentKey, "", ""); err != nil {
		return TokenRevokeResult{}, err
	}
	var revoked sql.NullTime
	var fingerprint []byte
	err = tx.QueryRowContext(ctx, `SELECT target.revoked_at, target.deployment_certificate_fingerprint
		FROM api_tokens AS target
		WHERE target.public_id = ? AND target.parent_public_id = ? AND target.kind = 'read-only'
		AND EXISTS (SELECT 1 FROM api_token_environments AS grant_record JOIN environments AS environment ON environment.id = grant_record.environment_id
		WHERE grant_record.token_id = target.id AND environment.resource_key = ?) FOR UPDATE`, request.PublicID, request.Actor.ID, request.EnvironmentKey).Scan(&revoked, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenRevokeResult{}, machine.ErrForbidden
	}
	if err != nil {
		return TokenRevokeResult{}, errors.New("read deployment credential")
	}
	replayed, replay, err := beginOperation(ctx, tx, request.OperationID, pkiDigest("deployment.revoke", request), request.Actor)
	if err != nil {
		return TokenRevokeResult{}, err
	}
	if replayed {
		var previous TokenRevokeResult
		if json.Unmarshal(replay.Response, &previous) != nil || previous.Outcome != replay.Outcome {
			return previous, errors.New("invalid deployment replay")
		}
		return previous, outcomeError(previous.Outcome)
	}
	result := TokenRevokeResult{Outcome: OutcomeNoChange, PublicID: request.PublicID, Revoked: true}
	if !revoked.Valid {
		if err := revokeTokenAndDeployments(ctx, tx, request.PublicID); err != nil {
			return result, err
		}
		result.Outcome = OutcomeSuccess
	}
	if err := finishOperation(ctx, tx, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "deployment.revoked", Action: "deployment.revoke", EnvironmentKey: request.EnvironmentKey, ResourceType: "api_token", ResourceKey: request.PublicID,
	}, false); err != nil {
		return TokenRevokeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TokenRevokeResult{}, errors.New("commit deployment revocation")
	}
	return result, nil
}

// Revoking a writer also revokes its children and their certificates atomically.
func revokeTokenAndDeployments(ctx context.Context, tx *sql.Tx, publicID string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE client_certificates AS certificate JOIN api_tokens AS token
		ON token.deployment_certificate_fingerprint = certificate.fingerprint_sha256
		SET certificate.revoked_at = COALESCE(certificate.revoked_at, UTC_TIMESTAMP(6))
		WHERE token.public_id = ? OR token.parent_public_id = ?`, publicID, publicID); err != nil {
		return errors.New("revoke deployment certificates")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = COALESCE(revoked_at, UTC_TIMESTAMP(6))
		WHERE public_id = ? OR parent_public_id = ?`, publicID, publicID); err != nil {
		return errors.New("revoke deployment Tokens")
	}
	return nil
}
