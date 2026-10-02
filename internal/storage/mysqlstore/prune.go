package mysqlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// OutboxMaintenance exposes metadata maintenance only. Its least-privilege
// database account needs no Master Key or access to decrypted content.
type OutboxMaintenance struct{ db *sql.DB }
type OutboxPrune struct {
	OperationID string
	Before      time.Time
	Limit       int
	ArchiveID   string
	Apply       bool
}
type OutboxPruneResult struct {
	Outcome       Outcome `json:"outcome"`
	Candidates    int     `json:"candidates"`
	Pruned        int     `json:"pruned"`
	ArchiveSHA256 string  `json:"archive_sha256,omitempty"`
}
type archivedEvent struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Type        string          `json:"event_type"`
	OperationID string          `json:"operation_id,omitempty"`
	Payload     json.RawMessage `json:"payload"`
	Attempts    uint32          `json:"attempts"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt time.Time       `json:"completed_at"`
}

func OpenOutboxMaintenance(ctx context.Context, dsn, expectedDatabase string) (*OutboxMaintenance, error) {
	if expectedDatabase == "" {
		return nil, ErrValidation
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, errors.New("open metadata database")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var name string
	var version uint64
	if db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name) != nil || name != expectedDatabase || db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version) != nil || version != schemaVersion {
		db.Close()
		return nil, errors.New("metadata database name or schema verification failed")
	}
	return &OutboxMaintenance{db: db}, nil
}
func (maintenance *OutboxMaintenance) Close() error { return maintenance.db.Close() }

// Prune archives one bounded batch before deleting only completed, fully
// delivered events. Operations, all Revisions and failed/pending deliveries stay.
// The archive callback must durably save exactly its bytes before returning nil.
func (maintenance *OutboxMaintenance) Prune(ctx context.Context, request OutboxPrune, archive func([]byte) error) (OutboxPruneResult, error) {
	if request.Before.IsZero() || !request.Before.Before(time.Now()) || request.Limit < 1 || request.Limit > 1000 || (request.Apply && (!validOperationID(request.OperationID) || !validOperationID(request.ArchiveID) || archive == nil)) {
		return OutboxPruneResult{}, ErrValidation
	}
	tx, err := maintenance.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return OutboxPruneResult{}, errors.New("begin outbox maintenance")
	}
	defer tx.Rollback()
	actor := Actor{Type: "system", ID: "maintenance-outbox-prune"}
	if request.Apply {
		replayed, replay, err := beginOperation(ctx, tx, request.OperationID, tokenRequestDigest(request), actor)
		if err != nil {
			return OutboxPruneResult{}, err
		}
		if replayed {
			var result OutboxPruneResult
			if json.Unmarshal(replay.Response, &result) != nil {
				return result, errors.New("invalid maintenance replay")
			}
			return result, outcomeError(result.Outcome)
		}
	}
	query := `SELECT event.id,event.kind,event.event_type,COALESCE(event.operation_id,''),event.payload,event.attempts,event.created_at,event.completed_at
		FROM outbox_events AS event WHERE event.status='completed' AND event.completed_at<?
		AND NOT EXISTS(SELECT 1 FROM notification_targets AS target WHERE target.outbox_event_id=event.id AND target.status<>'succeeded')
		ORDER BY event.completed_at,event.id LIMIT ?`
	if request.Apply {
		query += " FOR UPDATE SKIP LOCKED"
	}
	rows, err := tx.QueryContext(ctx, query, request.Before.UTC(), request.Limit)
	if err != nil {
		return OutboxPruneResult{}, errors.New("select completed outbox batch")
	}
	var records []archivedEvent
	var ids [][]byte
	for rows.Next() {
		var event archivedEvent
		var id []byte
		if rows.Scan(&id, &event.Kind, &event.Type, &event.OperationID, &event.Payload, &event.Attempts, &event.CreatedAt, &event.CompletedAt) != nil || len(id) != 16 {
			rows.Close()
			return OutboxPruneResult{}, errors.New("read completed outbox batch")
		}
		event.ID = hex.EncodeToString(id)
		records = append(records, event)
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return OutboxPruneResult{}, errors.New("read completed outbox batch")
	}
	result := OutboxPruneResult{Outcome: OutcomeNoChange, Candidates: len(records)}
	if !request.Apply {
		return result, nil
	}
	event := mutationEvent{Type: "outbox.pruned", Action: "outbox.prune", ResourceType: "maintenance", ResourceKey: "completed_outbox"}
	if len(records) > 0 {
		arguments := make([]any, len(ids))
		for index, id := range ids {
			arguments[index] = id
		}
		where := " WHERE outbox_event_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")"
		loadRelated := func(query, order string) ([]json.RawMessage, error) {
			rows, err := tx.QueryContext(ctx, query+where+" ORDER BY "+order+" LIMIT 10001", arguments...)
			if err != nil {
				return nil, errors.New("read delivery archive")
			}
			defer rows.Close()
			result := make([]json.RawMessage, 0)
			for rows.Next() {
				var value []byte
				if rows.Scan(&value) != nil || len(result) >= 10000 {
					return nil, errors.New("delivery archive too large; reduce batch limit")
				}
				result = append(result, value)
			}
			if rows.Err() != nil {
				return nil, errors.New("read delivery archive")
			}
			return result, nil
		}
		targets, err := loadRelated(`SELECT JSON_OBJECT('outbox_event_id',LOWER(HEX(outbox_event_id)),'destination_id',LOWER(HEX(destination_id)),'status',status,'attempts',attempts,'next_attempt_at',next_attempt_at,'last_error_code',last_error_code,'created_at',created_at,'updated_at',updated_at) FROM notification_targets`, "outbox_event_id,destination_id")
		if err != nil {
			return result, err
		}
		deliveries, err := loadRelated(`SELECT JSON_OBJECT('id',LOWER(HEX(id)),'outbox_event_id',LOWER(HEX(outbox_event_id)),'destination_id',LOWER(HEX(destination_id)),'attempt',attempt,'status',status,'http_status',http_status,'latency_ms',latency_ms,'provider_error_code',provider_error_code,'created_at',created_at) FROM notification_deliveries`, "outbox_event_id,destination_id,attempt")
		if err != nil {
			return result, err
		}
		data, err := json.Marshal(struct {
			Version    int               `json:"version"`
			Events     []archivedEvent   `json:"events"`
			Targets    []json.RawMessage `json:"notification_targets"`
			Deliveries []json.RawMessage `json:"notification_deliveries"`
		}{1, records, targets, deliveries})
		if err != nil {
			return result, errors.New("encode outbox archive")
		}
		if len(data) > 32<<20 {
			return result, errors.New("outbox archive exceeds 32 MiB; reduce batch limit")
		}
		if err := archive(data); err != nil {
			return result, errors.New("durable outbox archive failed; nothing pruned")
		}
		digest := sha256.Sum256(data)
		result.ArchiveSHA256 = hex.EncodeToString(digest[:])
		for _, id := range ids {
			for _, statement := range []string{`DELETE FROM notification_deliveries WHERE outbox_event_id=?`, `DELETE FROM notification_targets WHERE outbox_event_id=?`, `DELETE FROM outbox_events WHERE id=? AND status='completed'`} {
				if _, err := tx.ExecContext(ctx, statement, id); err != nil {
					return OutboxPruneResult{}, errors.New("prune archived outbox batch")
				}
			}
		}
		result.Pruned = len(records)
		result.Outcome = OutcomeSuccess
		event.ResourceType = "outbox_archive"
		event.ResourceKey = result.ArchiveSHA256
	}
	if err := finishOperation(ctx, tx, request.OperationID, actor, result.Outcome, 0, result, event, false); err != nil {
		return OutboxPruneResult{}, err
	}
	if tx.Commit() != nil {
		return OutboxPruneResult{}, errors.New("outbox maintenance commit uncertain; replay the same operation")
	}
	return result, nil
}
