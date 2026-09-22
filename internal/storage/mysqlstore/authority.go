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

	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

type CertificateAuthority struct {
	ID                     string    `json:"id"`
	DisplayName            string    `json:"display_name"`
	FingerprintSHA256      string    `json:"fingerprint_sha256"`
	CertificatePEM         string    `json:"certificate_pem"`
	NotBefore              time.Time `json:"not_before"`
	NotAfter               time.Time `json:"not_after"`
	Revoked                bool      `json:"revoked"`
	CreatedAt              time.Time `json:"created_at"`
	ClientCertificateCount uint64    `json:"client_certificate_count"`
}

type AuthorityCreate struct {
	OperationID string
	Actor       Actor
	DisplayName string
	ValidDays   int
}

type AuthorityRevoke struct {
	OperationID string
	Actor       Actor
	AuthorityID string
}

type ClientCertificateIssue struct {
	OperationID string
	Actor       Actor
	AuthorityID string
	DisplayName string
	ValidDays   int
}

type AuthorityResult struct {
	Outcome      Outcome              `json:"outcome"`
	Authority    CertificateAuthority `json:"authority"`
	ExportBundle string               `json:"export_bundle,omitempty"`
}

type ClientCertificateIssueResult struct {
	Outcome      Outcome                 `json:"outcome"`
	Certificate  ClientCertificateResult `json:"certificate"`
	AuthorityID  string                  `json:"authority_id"`
	ExportBundle string                  `json:"export_bundle,omitempty"`
}

type AuthorityQuery struct {
	InventoryQuery
	Usable bool
}

func (store *Store) ListCertificateAuthorities(ctx context.Context, query AuthorityQuery) (InventoryPage[CertificateAuthority], error) {
	page := InventoryPage[CertificateAuthority]{Items: make([]CertificateAuthority, 0)}
	if !query.valid() || (query.Usable && query.OnlyInactive) {
		return page, ErrValidation
	}
	where, arguments := query.filter("LOWER(HEX(authority.id))", "authority.revoked_at", "authority.display_name", "LOWER(HEX(authority.id))")
	if query.Usable {
		where += " AND authority.revoked_at IS NULL AND authority.not_before <= UTC_TIMESTAMP(6) AND authority.not_after > UTC_TIMESTAMP(6)"
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM certificate_authorities AS authority WHERE `+where, arguments...).Scan(&page.Total); err != nil {
		return page, errors.New("count certificate authorities")
	}
	rows, err := store.db.QueryContext(ctx, `SELECT authority.id, authority.display_name, authority.certificate_der,
		authority.revoked_at IS NOT NULL, authority.created_at,
		(SELECT COUNT(*) FROM client_certificates AS certificate WHERE certificate.authority_id = authority.id)
		FROM certificate_authorities AS authority WHERE `+where+`
		ORDER BY authority.created_at DESC, authority.id LIMIT ? OFFSET ?`, append(arguments, query.Limit, query.Offset)...)
	if err != nil {
		return page, errors.New("list certificate authorities")
	}
	defer rows.Close()
	for rows.Next() {
		var authority CertificateAuthority
		var id, der []byte
		if err := rows.Scan(&id, &authority.DisplayName, &der, &authority.Revoked, &authority.CreatedAt, &authority.ClientCertificateCount); err != nil {
			return page, errors.New("read certificate authority metadata")
		}
		certificate, err := x509.ParseCertificate(der)
		if err != nil || len(id) != 16 || !certificate.IsCA {
			return page, vaultcrypto.ErrIntegrity
		}
		authority.ID = hex.EncodeToString(id)
		setAuthorityCertificate(&authority, certificate)
		authority.CreatedAt = authority.CreatedAt.UTC()
		page.Items = append(page.Items, authority)
	}
	return page, rows.Err()
}

func (store *Store) ActiveCertificateAuthorities(ctx context.Context) ([][]byte, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT certificate_der FROM certificate_authorities
		WHERE revoked_at IS NULL AND not_before <= UTC_TIMESTAMP(6) AND not_after > UTC_TIMESTAMP(6) ORDER BY id`)
	if err != nil {
		return nil, errors.New("read managed Client CA trust")
	}
	defer rows.Close()
	var result [][]byte
	for rows.Next() {
		var der []byte
		if err := rows.Scan(&der); err != nil {
			return nil, errors.New("read managed Client CA certificate")
		}
		result = append(result, der)
	}
	return result, rows.Err()
}

func (store *Store) CreateCertificateAuthority(ctx context.Context, request AuthorityCreate) (AuthorityResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return AuthorityResult{}, ErrValidation
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthorityResult{}, errors.New("begin Authority creation")
	}
	defer transaction.Rollback()
	// The exclusive Sentinel lock also serializes the active-CA limit check.
	if err := store.lockMasterKey(ctx, transaction, true); err != nil {
		return AuthorityResult{}, err
	}
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, pkiDigest("authority.create", request), request.Actor)
	if err != nil {
		return AuthorityResult{}, err
	}
	if replayed {
		var previous AuthorityResult
		if json.Unmarshal(replay.Response, &previous) != nil || previous.Outcome != replay.Outcome {
			return AuthorityResult{}, vaultcrypto.ErrIntegrity
		}
		return previous, outcomeError(previous.Outcome)
	}
	var active int
	if err := transaction.QueryRowContext(ctx, "SELECT COUNT(*) FROM certificate_authorities WHERE revoked_at IS NULL AND not_after > UTC_TIMESTAMP(6)").Scan(&active); err != nil {
		return AuthorityResult{}, errors.New("count certificate authorities")
	}
	if active >= 32 {
		return AuthorityResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "certificate_authority.create", "", errors.New("active Authority limit reached"))
	}
	generated, err := clientcert.GenerateAuthority(request.DisplayName, request.ValidDays, time.Now())
	if err != nil {
		return AuthorityResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "certificate_authority.create", "", err)
	}
	defer clear(generated.PrivateKeyDER)
	id, err := randomID()
	if err != nil {
		return AuthorityResult{}, err
	}
	authority := CertificateAuthority{ID: hex.EncodeToString(id), DisplayName: request.DisplayName, CreatedAt: time.Now().UTC()}
	setAuthorityCertificate(&authority, generated.Certificate)
	encrypted, err := store.provider.EncryptSecret(authorityKeyIdentity(authority.ID, generated.Certificate.Raw), generated.PrivateKeyDER)
	if err != nil {
		return AuthorityResult{}, vaultcrypto.ErrIntegrity
	}
	bundle, err := clientcert.ExportBundle(generated, nil)
	if err != nil {
		return AuthorityResult{}, err
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO certificate_authorities
		(id, display_name, certificate_der, algorithm, key_version, nonce, ciphertext, encrypted_dek, not_before, not_after)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, authority.DisplayName, generated.Certificate.Raw,
		encrypted.Algorithm, encrypted.KeyVersion, encrypted.Nonce, encrypted.Ciphertext, encrypted.EncryptedDEK, authority.NotBefore, authority.NotAfter); err != nil {
		return AuthorityResult{}, errors.New("store encrypted Authority")
	}
	result := AuthorityResult{Outcome: OutcomeSuccess, Authority: authority}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "certificate_authority.created", Action: "certificate_authority.create", ResourceType: "certificate_authority", ResourceKey: authority.ID,
	}, false); err != nil {
		return AuthorityResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AuthorityResult{}, errors.New("commit Authority creation")
	}
	// Export material is intentionally attached only after the metadata transaction commits.
	result.ExportBundle = bundle
	return result, nil
}

func (store *Store) IssueClientCertificate(ctx context.Context, request ClientCertificateIssue) (ClientCertificateIssueResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return ClientCertificateIssueResult{}, ErrValidation
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return ClientCertificateIssueResult{}, errors.New("begin Client Certificate issuance")
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, pkiDigest("certificate.issue", request), request.Actor)
	if err != nil {
		return ClientCertificateIssueResult{}, err
	}
	if replayed {
		var previous ClientCertificateIssueResult
		if json.Unmarshal(replay.Response, &previous) != nil || previous.Outcome != replay.Outcome {
			return ClientCertificateIssueResult{}, vaultcrypto.ErrIntegrity
		}
		return previous, outcomeError(previous.Outcome)
	}
	authorityID, err := parseAuthorityID(request.AuthorityID)
	if err != nil {
		return ClientCertificateIssueResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.issue", request.AuthorityID, err)
	}
	var der []byte
	var revoked sql.NullTime
	var encrypted vaultcrypto.EncryptedSecret
	err = transaction.QueryRowContext(ctx, `SELECT certificate_der, revoked_at, algorithm, key_version, nonce, ciphertext, encrypted_dek
		FROM certificate_authorities WHERE id = ? FOR UPDATE`, authorityID).Scan(&der, &revoked,
		&encrypted.Algorithm, &encrypted.KeyVersion, &encrypted.Nonce, &encrypted.Ciphertext, &encrypted.EncryptedDEK)
	if errors.Is(err, sql.ErrNoRows) || revoked.Valid {
		return ClientCertificateIssueResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.issue", request.AuthorityID, errors.New("Authority is unavailable"))
	}
	if err != nil {
		return ClientCertificateIssueResult{}, errors.New("read Authority for issuance")
	}
	authority, err := x509.ParseCertificate(der)
	if err != nil {
		return ClientCertificateIssueResult{}, vaultcrypto.ErrIntegrity
	}
	private, err := store.provider.DecryptSecret(authorityKeyIdentity(request.AuthorityID, der), encrypted)
	if err != nil {
		return ClientCertificateIssueResult{}, vaultcrypto.ErrIntegrity
	}
	defer clear(private)
	generated, err := clientcert.GenerateClient(authority, private, request.DisplayName, request.ValidDays, time.Now())
	if err != nil {
		return ClientCertificateIssueResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "client_certificate.issue", request.AuthorityID, err)
	}
	defer clear(generated.PrivateKeyDER)
	bundle, err := clientcert.ExportBundle(generated, authority)
	if err != nil {
		return ClientCertificateIssueResult{}, err
	}
	fingerprint := sha256.Sum256(generated.Certificate.Raw)
	certificate := ClientCertificateResult{Outcome: OutcomeSuccess, FingerprintSHA256: hex.EncodeToString(fingerprint[:]),
		DisplayName: request.DisplayName, Subject: generated.Certificate.Subject.String(), SerialHex: strings.ToUpper(generated.Certificate.SerialNumber.Text(16)),
		NotBefore: generated.Certificate.NotBefore.UTC(), NotAfter: generated.Certificate.NotAfter.UTC()}
	id, err := randomID()
	if err != nil {
		return ClientCertificateIssueResult{}, err
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO client_certificates
		(id, fingerprint_sha256, display_name, certificate_der, subject, serial_hex, not_before, not_after, authority_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, fingerprint[:], certificate.DisplayName, generated.Certificate.Raw,
		certificate.Subject, certificate.SerialHex, certificate.NotBefore, certificate.NotAfter, authorityID); err != nil {
		return ClientCertificateIssueResult{}, errors.New("register issued Client Certificate")
	}
	result := ClientCertificateIssueResult{Outcome: OutcomeSuccess, Certificate: certificate, AuthorityID: request.AuthorityID}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "client_certificate.issued", Action: "client_certificate.issue", ResourceType: "client_certificate", ResourceKey: certificate.FingerprintSHA256,
	}, false); err != nil {
		return ClientCertificateIssueResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ClientCertificateIssueResult{}, errors.New("commit Client Certificate issuance")
	}
	result.ExportBundle = bundle
	return result, nil
}

func (store *Store) RevokeCertificateAuthority(ctx context.Context, request AuthorityRevoke) (AuthorityResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return AuthorityResult{}, ErrValidation
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthorityResult{}, errors.New("begin Authority revocation")
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, pkiDigest("authority.revoke", request), request.Actor)
	if err != nil {
		return AuthorityResult{}, err
	}
	if replayed {
		var previous AuthorityResult
		if json.Unmarshal(replay.Response, &previous) != nil || previous.Outcome != replay.Outcome {
			return AuthorityResult{}, vaultcrypto.ErrIntegrity
		}
		return previous, outcomeError(previous.Outcome)
	}
	id, err := parseAuthorityID(request.AuthorityID)
	if err != nil {
		return AuthorityResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "certificate_authority.revoke", request.AuthorityID, err)
	}
	var authority CertificateAuthority
	var der []byte
	var revoked sql.NullTime
	err = transaction.QueryRowContext(ctx, `SELECT display_name, certificate_der, revoked_at, created_at FROM certificate_authorities WHERE id = ? FOR UPDATE`, id).
		Scan(&authority.DisplayName, &der, &revoked, &authority.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthorityResult{}, finishPKIFailure(ctx, transaction, request.OperationID, request.Actor, "certificate_authority.revoke", request.AuthorityID, errors.New("Authority does not exist"))
	}
	if err != nil {
		return AuthorityResult{}, errors.New("lock Authority for revocation")
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return AuthorityResult{}, vaultcrypto.ErrIntegrity
	}
	authority.ID, authority.Revoked = request.AuthorityID, true
	setAuthorityCertificate(&authority, certificate)
	result := AuthorityResult{Outcome: OutcomeNoChange, Authority: authority}
	if !revoked.Valid {
		if _, err := transaction.ExecContext(ctx, "UPDATE certificate_authorities SET revoked_at = UTC_TIMESTAMP(6) WHERE id = ?", id); err != nil {
			return AuthorityResult{}, errors.New("revoke Authority")
		}
		if _, err := transaction.ExecContext(ctx, "UPDATE client_certificates SET revoked_at = UTC_TIMESTAMP(6) WHERE authority_id = ? AND revoked_at IS NULL", id); err != nil {
			return AuthorityResult{}, errors.New("revoke Authority Client Certificates")
		}
		result.Outcome = OutcomeSuccess
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "certificate_authority.revoked", Action: "certificate_authority.revoke", ResourceType: "certificate_authority", ResourceKey: request.AuthorityID,
	}, false); err != nil {
		return AuthorityResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return AuthorityResult{}, errors.New("commit Authority revocation")
	}
	return result, nil
}

func setAuthorityCertificate(authority *CertificateAuthority, certificate *x509.Certificate) {
	fingerprint := sha256.Sum256(certificate.Raw)
	authority.FingerprintSHA256 = hex.EncodeToString(fingerprint[:])
	authority.CertificatePEM = clientcert.CertificatePEM(certificate)
	authority.NotBefore, authority.NotAfter = certificate.NotBefore.UTC(), certificate.NotAfter.UTC()
}

func authorityKeyIdentity(id string, der []byte) string {
	fingerprint := sha256.Sum256(der)
	return "ca:" + id + ":" + hex.EncodeToString(fingerprint[:])
}

func parseAuthorityID(value string) ([]byte, error) {
	id, err := hex.DecodeString(value)
	if err != nil || len(id) != 16 || hex.EncodeToString(id) != value {
		return nil, errors.New("invalid Authority ID")
	}
	return id, nil
}

// Associate imported certificates signed with an exported managed CA key as well,
// so import cannot bypass subsequent Authority revocation.
func managedCertificateIssuer(ctx context.Context, transaction *sql.Tx, leaf *x509.Certificate) ([]byte, error) {
	rows, err := transaction.QueryContext(ctx, "SELECT id, certificate_der FROM certificate_authorities ORDER BY id")
	if err != nil {
		return nil, errors.New("read certificate issuer")
	}
	var matched []byte
	for rows.Next() {
		var id, der []byte
		if err := rows.Scan(&id, &der); err != nil {
			rows.Close()
			return nil, errors.New("read certificate issuer")
		}
		authority, err := x509.ParseCertificate(der)
		if err != nil {
			rows.Close()
			return nil, vaultcrypto.ErrIntegrity
		}
		if leaf.CheckSignatureFrom(authority) == nil {
			matched = append([]byte(nil), id...)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, errors.New("read certificate issuer")
	}
	if matched == nil {
		return nil, nil
	}
	var revoked sql.NullTime
	if err := transaction.QueryRowContext(ctx, "SELECT revoked_at FROM certificate_authorities WHERE id = ? FOR UPDATE", matched).Scan(&revoked); err != nil {
		return nil, errors.New("lock certificate issuer")
	}
	if revoked.Valid {
		return nil, ErrValidation
	}
	return matched, nil
}

func pkiDigest(action string, request any) [sha256.Size]byte {
	encoded, _ := json.Marshal(struct {
		Action  string
		Request any
	}{action, request})
	return sha256.Sum256(encoded)
}

func finishPKIFailure(ctx context.Context, transaction *sql.Tx, operationID string, actor Actor, action, resource string, cause error) error {
	if _, err := parseAuthorityID(resource); err != nil {
		resource = ""
	}
	result := struct {
		Outcome Outcome `json:"outcome"`
	}{OutcomeValidationFailed}
	if err := finishOperation(ctx, transaction, operationID, actor, result.Outcome, 0, result, mutationEvent{
		Type: "certificate.validation_failed", Action: action, ResourceType: "certificate_authority", ResourceKey: resource,
	}, false); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return errors.New("commit certificate validation Audit")
	}
	return withCommittedAudit(fmt.Errorf("%w: %v", ErrValidation, cause))
}
