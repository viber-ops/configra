package mysqlstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type DeploymentSummary struct {
	PublicID               string    `json:"public_id"`
	ParentPublicID         string    `json:"parent_public_id"`
	DisplayName            string    `json:"display_name"`
	ExpiresAt              time.Time `json:"expires_at"`
	Revoked                bool      `json:"revoked"`
	CertificateFingerprint string    `json:"certificate_fingerprint"`
	ConfigKeys             []string  `json:"config_keys"`
	NamespaceKeys          []string  `json:"namespace_keys"`
}

func (store *Store) ListDeploymentCredentials(ctx context.Context, actor Actor, environment, after string) ([]DeploymentSummary, error) {
	if actor.Type != "token" || (after != "" && !validTokenPublicID(after)) {
		return nil, ErrValidation
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.New("begin deployment inventory")
	}
	defer tx.Rollback()
	if _, err := authorizeScopedWrite(ctx, tx, actor, environment, "", ""); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT token.public_id,token.parent_public_id,token.display_name,token.expires_at,token.revoked_at IS NOT NULL,LOWER(HEX(token.deployment_certificate_fingerprint)),token.config_keys,token.namespace_keys
		FROM api_tokens AS token WHERE token.parent_public_id=? AND token.kind='read-only' AND token.public_id>?
		AND EXISTS(SELECT 1 FROM api_token_environments AS granted JOIN environments AS environment ON environment.id=granted.environment_id WHERE granted.token_id=token.id AND environment.resource_key=?)
		ORDER BY token.public_id LIMIT 100`, actor.ID, after, environment)
	if err != nil {
		return nil, errors.New("read deployment inventory")
	}
	result := make([]DeploymentSummary, 0)
	for rows.Next() {
		var entry DeploymentSummary
		var configs, namespaces []byte
		if rows.Scan(&entry.PublicID, &entry.ParentPublicID, &entry.DisplayName, &entry.ExpiresAt, &entry.Revoked, &entry.CertificateFingerprint, &configs, &namespaces) != nil {
			rows.Close()
			return nil, errors.New("read deployment metadata")
		}
		if (configs != nil && json.Unmarshal(configs, &entry.ConfigKeys) != nil) || (namespaces != nil && json.Unmarshal(namespaces, &entry.NamespaceKeys) != nil) {
			rows.Close()
			return nil, errors.New("invalid deployment scopes")
		}
		result = append(result, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, errors.New("read deployment inventory")
	}
	if tx.Commit() != nil {
		return nil, errors.New("complete deployment inventory")
	}
	return result, nil
}
