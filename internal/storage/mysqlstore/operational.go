package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type OutboxHealth struct {
	Kind, Status     string
	Count            uint64
	OldestAgeSeconds float64
}
type CredentialHealth struct {
	Kind                     string
	Count, Expired, Expiring uint64
	NextExpiry               float64
}
type StorageUsage struct {
	Table                string
	EstimatedRows, Bytes uint64
}
type OperationalStats struct {
	Database    sql.DBStats
	Outbox      []OutboxHealth
	Credentials []CredentialHealth
	Storage     []StorageUsage
}

// OperationalStats returns bounded metadata only, never event payloads or values.
// Table row counts are InnoDB estimates; expiry/outbox counts are exact.
func (store *Store) OperationalStats(ctx context.Context) (OperationalStats, error) {
	result := OperationalStats{Database: store.db.Stats()}
	rows, err := store.db.QueryContext(ctx, `SELECT kind,status,COUNT(*),MIN(created_at) FROM outbox_events WHERE status <> 'completed' GROUP BY kind,status`)
	if err != nil {
		return result, errors.New("read outbox health")
	}
	for rows.Next() {
		var value OutboxHealth
		var oldest time.Time
		if err := rows.Scan(&value.Kind, &value.Status, &value.Count, &oldest); err != nil {
			rows.Close()
			return result, errors.New("read outbox health")
		}
		value.OldestAgeSeconds = max(0, time.Since(oldest).Seconds())
		result.Outbox = append(result.Outbox, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, errors.New("read outbox health")
	}
	rows, err = store.db.QueryContext(ctx, `SELECT kind,COUNT(*),COALESCE(SUM(expiry<=UTC_TIMESTAMP(6)),0),
		COALESCE(SUM(expiry>UTC_TIMESTAMP(6) AND expiry<=DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 14 DAY)),0),
		COALESCE(MIN(CASE WHEN expiry>UTC_TIMESTAMP(6) THEN UNIX_TIMESTAMP(expiry) END),0)
		FROM (
		 SELECT kind,expires_at AS expiry FROM api_tokens WHERE revoked_at IS NULL
		 UNION ALL SELECT 'client-certificate',certificate.not_after FROM client_certificates AS certificate LEFT JOIN certificate_authorities AS authority ON authority.id=certificate.authority_id
		 WHERE certificate.revoked_at IS NULL AND (certificate.authority_id IS NULL OR (authority.revoked_at IS NULL AND authority.not_after>UTC_TIMESTAMP(6)))
		 UNION ALL SELECT 'authority',not_after FROM certificate_authorities WHERE revoked_at IS NULL
		) AS credentials GROUP BY kind`)
	if err != nil {
		return result, errors.New("read credential health")
	}
	for rows.Next() {
		var value CredentialHealth
		if rows.Scan(&value.Kind, &value.Count, &value.Expired, &value.Expiring, &value.NextExpiry) != nil {
			rows.Close()
			return result, errors.New("read credential health")
		}
		result.Credentials = append(result.Credentials, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, errors.New("read credential health")
	}
	rows, err = store.db.QueryContext(ctx, `SELECT table_name,COALESCE(table_rows,0),COALESCE(data_length,0)+COALESCE(index_length,0) FROM information_schema.tables
		WHERE table_schema=DATABASE() AND table_name IN ('configs','config_revisions','vault_items','vault_item_revisions','operations','outbox_events','notification_deliveries','api_tokens','client_certificates','certificate_authorities','management_sessions','management_session_policies','config_release_sets','config_release_states')`)
	if err != nil {
		return result, errors.New("read storage health")
	}
	for rows.Next() {
		var value StorageUsage
		if rows.Scan(&value.Table, &value.EstimatedRows, &value.Bytes) != nil {
			rows.Close()
			return result, errors.New("read storage health")
		}
		result.Storage = append(result.Storage, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, errors.New("read storage health")
	}
	return result, nil
}
