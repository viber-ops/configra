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
	EnvironmentCount uint64     `json:"environment_count"`
	AllowWithoutMTLS bool       `json:"allow_without_mtls"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	Revoked          bool       `json:"revoked"`
	CreatedAt        time.Time  `json:"created_at"`
}

func (store *Store) ListTokens(ctx context.Context, query InventoryQuery) (InventoryPage[TokenSummary], error) {
	page := InventoryPage[TokenSummary]{Items: make([]TokenSummary, 0)}
	if !query.valid() {
		return page, ErrValidation
	}
	where, arguments := query.filter("token.public_id", "token.revoked_at", "token.public_id", "token.display_name", "token.display_prefix")
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens AS token WHERE `+where, arguments...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count API Tokens: %w", err)
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT token.public_id, token.display_name, token.display_prefix, token.allow_without_mtls,
		       token.expires_at, token.revoked_at IS NOT NULL, token.created_at,
		       (SELECT COUNT(*) FROM api_token_environments AS grant_record WHERE grant_record.token_id = token.id),
		       preview.resource_key
		FROM (SELECT token.id, token.public_id, token.display_name, token.display_prefix, token.allow_without_mtls, token.expires_at, token.revoked_at, token.created_at
		      FROM api_tokens AS token WHERE `+where+` ORDER BY token.created_at DESC, token.public_id LIMIT ? OFFSET ?) AS token
		LEFT JOIN LATERAL (
			SELECT environment.resource_key FROM api_token_environments AS grant_record
			JOIN environments AS environment ON environment.id = grant_record.environment_id
			WHERE grant_record.token_id = token.id ORDER BY environment.resource_key LIMIT ?
		) AS preview ON TRUE
		ORDER BY token.created_at DESC, token.public_id, preview.resource_key
	`, append(arguments, query.Limit, query.Offset, SummaryEnvironmentLimit)...)
	if err != nil {
		return page, fmt.Errorf("list API Tokens: %w", err)
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
			&expiresAt, &token.Revoked, &token.CreatedAt, &token.EnvironmentCount, &environment,
		); err != nil {
			return page, fmt.Errorf("scan API Token: %w", err)
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
		return page, fmt.Errorf("iterate API Tokens: %w", err)
	}
	page.Items = result
	return page, nil
}

type ClientCertificate struct {
	AuthorityID       string    `json:"authority_id,omitempty"`
	AuthorityName     string    `json:"authority_name,omitempty"`
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	DisplayName       string    `json:"display_name"`
	Subject           string    `json:"subject"`
	SerialHex         string    `json:"serial_hex"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	Revoked           bool      `json:"revoked"`
	CreatedAt         time.Time `json:"created_at"`
}

func (store *Store) ListClientCertificates(ctx context.Context, query InventoryQuery) (InventoryPage[ClientCertificate], error) {
	page := InventoryPage[ClientCertificate]{Items: make([]ClientCertificate, 0)}
	if !query.valid() {
		return page, ErrValidation
	}
	where, arguments := query.filter("LOWER(HEX(certificate.fingerprint_sha256))", "certificate.revoked_at",
		"certificate.display_name", "certificate.subject", "certificate.serial_hex", "LOWER(HEX(certificate.fingerprint_sha256))", "authority.display_name")
	from := ` FROM client_certificates AS certificate LEFT JOIN certificate_authorities AS authority ON authority.id = certificate.authority_id WHERE ` + where
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from, arguments...).Scan(&page.Total); err != nil {
		return page, fmt.Errorf("count Client Certificates: %w", err)
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT LOWER(HEX(certificate.fingerprint_sha256)), certificate.display_name, certificate.subject, certificate.serial_hex,
		       certificate.not_before, certificate.not_after, certificate.revoked_at IS NOT NULL, certificate.created_at,
		       COALESCE(LOWER(HEX(certificate.authority_id)), ''), COALESCE(authority.display_name, '')
		`+from+` ORDER BY certificate.created_at DESC, certificate.fingerprint_sha256 LIMIT ? OFFSET ?
	`, append(arguments, query.Limit, query.Offset)...)
	if err != nil {
		return page, fmt.Errorf("list Client Certificates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var certificate ClientCertificate
		if err := rows.Scan(
			&certificate.FingerprintSHA256, &certificate.DisplayName, &certificate.Subject,
			&certificate.SerialHex, &certificate.NotBefore, &certificate.NotAfter,
			&certificate.Revoked, &certificate.CreatedAt, &certificate.AuthorityID, &certificate.AuthorityName,
		); err != nil {
			return page, fmt.Errorf("scan Client Certificate: %w", err)
		}
		certificate.NotBefore = certificate.NotBefore.UTC()
		certificate.NotAfter = certificate.NotAfter.UTC()
		certificate.CreatedAt = certificate.CreatedAt.UTC()
		page.Items = append(page.Items, certificate)
	}
	if err := rows.Err(); err != nil {
		return page, fmt.Errorf("iterate Client Certificates: %w", err)
	}
	return page, nil
}

func utcTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
