package mysqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type TokenSummary struct {
	PublicID         string     `json:"public_id"`
	DisplayName      string     `json:"display_name"`
	DisplayPrefix    string     `json:"display_prefix"`
	EnvironmentKeys  []string   `json:"environment_keys"`
	AllowWithoutMTLS bool       `json:"allow_without_mtls"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	Revoked          bool       `json:"revoked"`
	CreatedAt        time.Time  `json:"created_at"`
}

func (store *Store) ListTokens(ctx context.Context, includeRevoked bool) ([]TokenSummary, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT token.public_id, token.display_name, token.display_prefix, token.allow_without_mtls,
		       token.expires_at, token.revoked_at IS NOT NULL, token.created_at,
		       environment.resource_key
		FROM api_tokens AS token
		LEFT JOIN api_token_environments AS grant_record ON grant_record.token_id = token.id
		LEFT JOIN environments AS environment ON environment.id = grant_record.environment_id
		WHERE token.revoked_at IS NULL OR ?
		ORDER BY token.created_at DESC, token.public_id, environment.resource_key
	`, includeRevoked)
	if err != nil {
		return nil, fmt.Errorf("list API Tokens: %w", err)
	}
	defer rows.Close()
	result := make([]TokenSummary, 0)
	for rows.Next() {
		var (
			token       TokenSummary
			expiresAt   sql.NullTime
			environment sql.NullString
		)
		if err := rows.Scan(
			&token.PublicID, &token.DisplayName, &token.DisplayPrefix, &token.AllowWithoutMTLS,
			&expiresAt, &token.Revoked, &token.CreatedAt, &environment,
		); err != nil {
			return nil, fmt.Errorf("scan API Token: %w", err)
		}
		if len(result) == 0 || result[len(result)-1].PublicID != token.PublicID {
			token.EnvironmentKeys = make([]string, 0)
			token.CreatedAt = token.CreatedAt.UTC()
			token.ExpiresAt = utcTime(expiresAt)
			result = append(result, token)
		}
		if environment.Valid {
			index := len(result) - 1
			result[index].EnvironmentKeys = append(result[index].EnvironmentKeys, environment.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate API Tokens: %w", err)
	}
	return result, nil
}

type ClientCertificate struct {
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	DisplayName       string    `json:"display_name"`
	Subject           string    `json:"subject"`
	SerialHex         string    `json:"serial_hex"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	Revoked           bool      `json:"revoked"`
	CreatedAt         time.Time `json:"created_at"`
}

func (store *Store) ListClientCertificates(ctx context.Context, includeRevoked bool) ([]ClientCertificate, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT LOWER(HEX(fingerprint_sha256)), display_name, subject, serial_hex,
		       not_before, not_after, revoked_at IS NOT NULL, created_at
		FROM client_certificates
		WHERE revoked_at IS NULL OR ?
		ORDER BY created_at DESC, fingerprint_sha256
	`, includeRevoked)
	if err != nil {
		return nil, fmt.Errorf("list Client Certificates: %w", err)
	}
	defer rows.Close()
	result := make([]ClientCertificate, 0)
	for rows.Next() {
		var certificate ClientCertificate
		if err := rows.Scan(
			&certificate.FingerprintSHA256, &certificate.DisplayName, &certificate.Subject,
			&certificate.SerialHex, &certificate.NotBefore, &certificate.NotAfter,
			&certificate.Revoked, &certificate.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan Client Certificate: %w", err)
		}
		certificate.NotBefore = certificate.NotBefore.UTC()
		certificate.NotAfter = certificate.NotAfter.UTC()
		certificate.CreatedAt = certificate.CreatedAt.UTC()
		result = append(result, certificate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Client Certificates: %w", err)
	}
	return result, nil
}

func utcTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
