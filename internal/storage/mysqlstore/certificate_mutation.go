package mysqlstore

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ClientCertificateRegister struct {
	OperationID string
	Actor       Actor
	DisplayName string
	Certificate *x509.Certificate
}

type ClientCertificateResult struct {
	Outcome           Outcome   `json:"outcome"`
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	DisplayName       string    `json:"display_name"`
	Subject           string    `json:"subject"`
	SerialHex         string    `json:"serial_hex"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	Revoked           bool      `json:"revoked"`
}

type ClientCertificateRevoke struct {
	OperationID       string
	Actor             Actor
	FingerprintSHA256 string
}

func (store *Store) RegisterVerifiedClientCertificate(
	ctx context.Context,
	request ClientCertificateRegister,
) (ClientCertificateResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ClientCertificateResult{}, ErrValidation
	}
	certificate, validationErr := validateVerifiedCertificate(request)
	digest := certificateRegisterDigest(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ClientCertificateResult{}, fmt.Errorf("begin Client Certificate registration: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return ClientCertificateResult{}, err
	}
	if replayed {
		var previous ClientCertificateResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return ClientCertificateResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return finishCertificateFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.register", "", validationErr)
	}
	authorityID, err := managedCertificateIssuer(ctx, transaction, certificate)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return finishCertificateFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.register", "", errors.New("Client Certificate Authority is revoked"))
		}
		return ClientCertificateResult{}, err
	}
	fingerprint := sha256.Sum256(certificate.Raw)
	fingerprintHex := hex.EncodeToString(fingerprint[:])
	subject := certificate.Subject.String()
	serialHex := strings.ToUpper(certificate.SerialNumber.Text(16))
	result := ClientCertificateResult{
		Outcome: OutcomeSuccess, FingerprintSHA256: fingerprintHex, DisplayName: request.DisplayName,
		Subject: subject, SerialHex: serialHex, NotBefore: certificate.NotBefore.UTC(), NotAfter: certificate.NotAfter.UTC(),
	}
	var existingID []byte
	var existingName string
	var revokedAt sql.NullTime
	err = transaction.QueryRowContext(ctx, `
		SELECT id, display_name, revoked_at FROM client_certificates
		WHERE fingerprint_sha256 = ? FOR UPDATE
	`, fingerprint[:]).Scan(&existingID, &existingName, &revokedAt)
	switch {
	case err == nil && revokedAt.Valid:
		return finishCertificateFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.register", fingerprintHex, errors.New("revoked Client Certificate cannot be restored"))
	case err == nil && existingName == request.DisplayName:
		result.Outcome = OutcomeNoChange
	case err == nil:
		if _, err := transaction.ExecContext(ctx, `UPDATE client_certificates SET display_name = ? WHERE id = ?`, request.DisplayName, existingID); err != nil {
			return ClientCertificateResult{}, fmt.Errorf("rename Client Certificate: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		id, err := randomID()
		if err != nil {
			return ClientCertificateResult{}, err
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO client_certificates
				(id, fingerprint_sha256, display_name, certificate_der, subject, serial_hex, not_before, not_after, authority_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, fingerprint[:], request.DisplayName, certificate.Raw, subject, serialHex,
			certificate.NotBefore.UTC(), certificate.NotAfter.UTC(), authorityID); err != nil {
			return ClientCertificateResult{}, fmt.Errorf("insert Client Certificate: %w", err)
		}
	case err != nil:
		return ClientCertificateResult{}, fmt.Errorf("lock Client Certificate: %w", err)
	}
	eventType := "client_certificate.registered"
	if result.Outcome == OutcomeNoChange {
		eventType = "client_certificate.no_change"
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: eventType, Action: "client_certificate.register", ResourceType: "client_certificate", ResourceKey: fingerprintHex,
	}, result.Outcome == OutcomeSuccess); err != nil {
		return ClientCertificateResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ClientCertificateResult{}, fmt.Errorf("commit Client Certificate registration: %w", err)
	}
	return result, nil
}

func (store *Store) RevokeClientCertificate(
	ctx context.Context,
	request ClientCertificateRevoke,
) (ClientCertificateResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ClientCertificateResult{}, ErrValidation
	}
	digest := certificateRevokeDigest(request)
	fingerprint, validationErr := parseFingerprint(request.FingerprintSHA256)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ClientCertificateResult{}, fmt.Errorf("begin Client Certificate revocation: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return ClientCertificateResult{}, err
	}
	if replayed {
		var previous ClientCertificateResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return ClientCertificateResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return finishCertificateFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.revoke", request.FingerprintSHA256, validationErr)
	}
	var id []byte
	var revokedAt sql.NullTime
	var result ClientCertificateResult
	err = transaction.QueryRowContext(ctx, `
		SELECT id, display_name, subject, serial_hex, not_before, not_after, revoked_at
		FROM client_certificates WHERE fingerprint_sha256 = ? FOR UPDATE
	`, fingerprint).Scan(&id, &result.DisplayName, &result.Subject, &result.SerialHex,
		&result.NotBefore, &result.NotAfter, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return finishCertificateFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.revoke", request.FingerprintSHA256, errors.New("Client Certificate does not exist"))
	}
	if err != nil {
		return ClientCertificateResult{}, fmt.Errorf("lock Client Certificate: %w", err)
	}
	result.FingerprintSHA256 = request.FingerprintSHA256
	result.Revoked = true
	result.Outcome = OutcomeNoChange
	if !revokedAt.Valid {
		if _, err := transaction.ExecContext(ctx, `UPDATE client_certificates SET revoked_at = UTC_TIMESTAMP(6) WHERE id = ?`, id); err != nil {
			return ClientCertificateResult{}, fmt.Errorf("revoke Client Certificate: %w", err)
		}
		result.Outcome = OutcomeSuccess
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "client_certificate.revoked", Action: "client_certificate.revoke", ResourceType: "client_certificate", ResourceKey: request.FingerprintSHA256,
	}, result.Outcome == OutcomeSuccess); err != nil {
		return ClientCertificateResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ClientCertificateResult{}, fmt.Errorf("commit Client Certificate revocation: %w", err)
	}
	return result, nil
}

func validateVerifiedCertificate(request ClientCertificateRegister) (*x509.Certificate, error) {
	if request.DisplayName == "" || len(request.DisplayName) > 255 || request.Certificate == nil {
		return nil, errors.New("Client Certificate and display name are required")
	}
	certificate, err := x509.ParseCertificate(request.Certificate.Raw)
	if err != nil || certificate.IsCA || certificate.SerialNumber == nil || certificate.SerialNumber.Sign() < 0 ||
		!certificate.NotBefore.Before(certificate.NotAfter) {
		return nil, errors.New("invalid verified Client Certificate")
	}
	if len(certificate.Subject.String()) > 1024 || len(certificate.SerialNumber.Text(16)) > 128 {
		return nil, errors.New("Client Certificate metadata is too large")
	}
	return certificate, nil
}

func certificateRegisterDigest(request ClientCertificateRegister) [sha256.Size]byte {
	var raw []byte
	if request.Certificate != nil {
		raw = request.Certificate.Raw
	}
	encoded, _ := json.Marshal(struct {
		OperationID string
		Actor       Actor
		DisplayName string
		Certificate []byte
	}{request.OperationID, request.Actor, request.DisplayName, raw})
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func certificateRevokeDigest(request ClientCertificateRevoke) [sha256.Size]byte {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	clear(encoded)
	return digest
}

func parseFingerprint(value string) ([]byte, error) {
	if len(value) != sha256.Size*2 {
		return nil, errors.New("invalid Client Certificate fingerprint")
	}
	fingerprint, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(fingerprint) != value {
		return nil, errors.New("invalid Client Certificate fingerprint")
	}
	return fingerprint, nil
}

func finishCertificateFailure(
	ctx context.Context,
	transaction *sql.Tx,
	operationID string,
	actor Actor,
	action string,
	resourceKey string,
	cause error,
) (ClientCertificateResult, error) {
	result := ClientCertificateResult{Outcome: OutcomeValidationFailed, FingerprintSHA256: resourceKey}
	if err := finishOperation(ctx, transaction, operationID, actor, result.Outcome, 0, result, mutationEvent{
		Type: "client_certificate.validation_failed", Action: action, ResourceType: "client_certificate", ResourceKey: resourceKey,
	}, false); err != nil {
		return ClientCertificateResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ClientCertificateResult{}, fmt.Errorf("commit Client Certificate validation Audit: %w", err)
	}
	return result, fmt.Errorf("%w: %v", ErrValidation, cause)
}
