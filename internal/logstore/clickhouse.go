package logstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

type Store struct {
	connection driver.Conn
}

type AccessRecord struct {
	Time           time.Time              `json:"time"`
	Principal      string                 `json:"principal"`
	Authentication machine.Authentication `json:"authentication"`
	Environment    string                 `json:"environment"`
	Namespace      string                 `json:"namespace,omitempty"`
	ResourceType   string                 `json:"resource_type"`
	Resource       string                 `json:"resource"`
	ConfigRevision uint64                 `json:"config_revision"`
	VaultRevisions map[string]uint64      `json:"vault_revisions"`
}

type AuditRecord struct {
	ID           string    `json:"id"`
	Time         time.Time `json:"time"`
	EventType    string    `json:"event_type"`
	OperationID  string    `json:"operation_id"`
	ActorType    string    `json:"actor_type"`
	ActorID      string    `json:"actor_id"`
	Action       string    `json:"action"`
	Outcome      string    `json:"outcome"`
	Environment  string    `json:"environment"`
	Namespace    string    `json:"namespace,omitempty"`
	ResourceType string    `json:"resource_type"`
	Resource     string    `json:"resource"`
	Revision     uint64    `json:"revision"`
	Attempt      uint32    `json:"delivery_attempt"`
}

type AuditQuery struct {
	Limit  int
	Offset int
	Search string
}

type AuditPage struct {
	Items   []AuditRecord `json:"items"`
	HasMore bool          `json:"has_more"`
}

func New(dsn string) (*Store, error) {
	if dsn == "" {
		return nil, errors.New("ClickHouse DSN is required")
	}
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil || len(options.Addr) == 0 {
		return nil, errors.New("invalid ClickHouse DSN")
	}
	options.DialTimeout = 3 * time.Second
	options.ReadTimeout = 10 * time.Second
	options.MaxOpenConns = 4
	options.MaxIdleConns = 2
	options.ConnMaxLifetime = 30 * time.Minute
	if options.Compression == nil {
		options.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	}
	connection, err := clickhouse.Open(options)
	if err != nil {
		return nil, errors.New("initialize ClickHouse client")
	}
	return &Store{connection: connection}, nil
}

func (store *Store) Initialize(ctx context.Context) error {
	if err := store.connection.Ping(ctx); err != nil {
		return fmt.Errorf("ping ClickHouse: %w", err)
	}
	for _, statement := range clickHouseSchemaV1 {
		if err := store.connection.Exec(ctx, statement); err != nil {
			return fmt.Errorf("initialize ClickHouse log schema: %w", err)
		}
	}
	return nil
}

func (store *Store) AppendAccess(ctx context.Context, events []machine.AccessEvent) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := store.connection.PrepareBatch(ctx, `
		INSERT INTO access_events (
			schema_version, event_time, principal, authentication, environment,
			namespace, resource_type, resource, config_revision, vault_revisions
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare Access Event batch: %w", err)
	}
	defer batch.Close()
	for _, event := range events {
		vaultRevisions := event.VaultRevisions
		if vaultRevisions == nil {
			vaultRevisions = map[string]uint64{}
		}
		if err := batch.Append(
			uint16(1), event.Time.UTC(), event.Principal, string(event.Authentication), event.Environment,
			event.Namespace, event.ResourceType, event.Resource, event.ConfigRevision, vaultRevisions,
		); err != nil {
			return errors.New("encode Access Event batch")
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("insert Access Event batch: %w", err)
	}
	return nil
}

func (store *Store) AppendAudits(ctx context.Context, events []AuditRecord) error {
	if len(events) == 0 {
		return nil
	}
	batch, err := store.connection.PrepareBatch(ctx, `
		INSERT INTO audit_events (
			schema_version, event_id, event_time, event_type, operation_id,
			actor_type, actor_id, action, outcome, environment,
			namespace, resource_type, resource, revision, delivery_attempt
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare Audit Event batch: %w", err)
	}
	defer batch.Close()
	for _, event := range events {
		if err := batch.Append(
			uint16(1), event.ID, event.Time.UTC(), event.EventType, event.OperationID,
			event.ActorType, event.ActorID, event.Action, event.Outcome, event.Environment,
			event.Namespace, event.ResourceType, event.Resource, event.Revision, event.Attempt,
		); err != nil {
			return errors.New("encode Audit Event batch")
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("insert Audit Event batch: %w", err)
	}
	return nil
}

func (store *Store) ListAccess(ctx context.Context, limit int) ([]AccessRecord, error) {
	rows, err := store.connection.Query(ctx, `
		SELECT event_time, principal, authentication, environment,
		       namespace, resource_type, resource, config_revision, vault_revisions
		FROM access_events
		ORDER BY event_time DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list Access Events: %w", err)
	}
	defer rows.Close()
	records := make([]AccessRecord, 0, limit)
	for rows.Next() {
		var record AccessRecord
		var authentication string
		if err := rows.Scan(
			&record.Time, &record.Principal, &authentication, &record.Environment,
			&record.Namespace, &record.ResourceType, &record.Resource, &record.ConfigRevision, &record.VaultRevisions,
		); err != nil {
			return nil, fmt.Errorf("scan Access Event: %w", err)
		}
		record.Authentication = machine.Authentication(authentication)
		record.Time = record.Time.UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Access Events: %w", err)
	}
	return records, nil
}

func (store *Store) ListAudits(ctx context.Context, query AuditQuery) (AuditPage, error) {
	statement := `
		SELECT event_id, event_time, event_type, operation_id,
		       actor_type, actor_id, action, outcome, environment,
		       namespace, resource_type, resource, revision, delivery_attempt
		FROM audit_events
	`
	arguments := make([]any, 0, 3)
	if query.Search != "" {
		statement += `
		WHERE positionCaseInsensitiveUTF8(
			concat(event_id, ' ', operation_id, ' ', actor_id, ' ', action, ' ', outcome, ' ',
			       environment, ' ', namespace, ' ', resource_type, ' ', resource), ?
		) > 0
		`
		arguments = append(arguments, query.Search)
	}
	statement += `
		ORDER BY event_time DESC, event_id DESC
		LIMIT ? OFFSET ?
	`
	arguments = append(arguments, query.Limit+1, query.Offset)
	rows, err := store.connection.Query(ctx, statement, arguments...)
	if err != nil {
		return AuditPage{}, fmt.Errorf("list Audit Events: %w", err)
	}
	defer rows.Close()
	records := make([]AuditRecord, 0, query.Limit+1)
	for rows.Next() {
		var record AuditRecord
		if err := rows.Scan(
			&record.ID, &record.Time, &record.EventType, &record.OperationID,
			&record.ActorType, &record.ActorID, &record.Action, &record.Outcome, &record.Environment,
			&record.Namespace, &record.ResourceType, &record.Resource, &record.Revision, &record.Attempt,
		); err != nil {
			return AuditPage{}, fmt.Errorf("scan Audit Event: %w", err)
		}
		record.Time = record.Time.UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return AuditPage{}, fmt.Errorf("iterate Audit Events: %w", err)
	}
	page := AuditPage{Items: records, HasMore: len(records) > query.Limit}
	if page.HasMore {
		page.Items = page.Items[:query.Limit]
	}
	return page, nil
}

func (store *Store) Close() error {
	return store.connection.Close()
}

func DecodeAuditOutbox(event mysqlstore.OutboxEvent) (AuditRecord, error) {
	if event.Kind != mysqlstore.OutboxAudit || event.Type == "" || event.OperationID == "" ||
		event.Attempts == 0 || len(event.Payload) == 0 || len(event.Payload) > 64<<10 || zeroOutboxID(event.ID) {
		return AuditRecord{}, errors.New("invalid Audit Outbox metadata")
	}
	var payload struct {
		Time           time.Time          `json:"time"`
		OperationID    string             `json:"operation_id"`
		Actor          mysqlstore.Actor   `json:"actor"`
		Action         string             `json:"action"`
		Outcome        mysqlstore.Outcome `json:"outcome"`
		EnvironmentKey string             `json:"environment_key"`
		NamespaceKey   string             `json:"namespace_key,omitempty"`
		ResourceType   string             `json:"resource_type"`
		ResourceKey    string             `json:"resource_key"`
		Revision       uint64             `json:"revision,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return AuditRecord{}, errors.New("invalid Audit Outbox payload")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return AuditRecord{}, errors.New("invalid Audit Outbox payload")
	}
	if payload.Time.IsZero() || payload.OperationID != event.OperationID ||
		(payload.Actor.Type != "user" && payload.Actor.Type != "system") || payload.Actor.ID == "" || len(payload.Actor.ID) > 255 ||
		payload.Action == "" || len(payload.Action) > 128 || payload.ResourceType == "" || len(payload.ResourceType) > 64 ||
		len(payload.NamespaceKey) > 63 || (payload.ResourceType == "vault_item" && payload.NamespaceKey == "") ||
		payload.ResourceKey == "" || len(payload.ResourceKey) > 63 || !validAuditOutcome(payload.Outcome) {
		return AuditRecord{}, errors.New("invalid Audit Outbox payload")
	}
	return AuditRecord{
		ID:           hex.EncodeToString(event.ID[:]),
		Time:         payload.Time.UTC(),
		EventType:    event.Type,
		OperationID:  payload.OperationID,
		ActorType:    payload.Actor.Type,
		ActorID:      payload.Actor.ID,
		Action:       payload.Action,
		Outcome:      string(payload.Outcome),
		Environment:  payload.EnvironmentKey,
		Namespace:    payload.NamespaceKey,
		ResourceType: payload.ResourceType,
		Resource:     payload.ResourceKey,
		Revision:     payload.Revision,
		Attempt:      event.Attempts,
	}, nil
}

func validAuditOutcome(outcome mysqlstore.Outcome) bool {
	return outcome == mysqlstore.OutcomeSuccess || outcome == mysqlstore.OutcomeNoChange ||
		outcome == mysqlstore.OutcomeConflict || outcome == mysqlstore.OutcomeValidationFailed
}

func zeroOutboxID(id mysqlstore.OutboxID) bool {
	return id == mysqlstore.OutboxID{}
}

var clickHouseSchemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS access_events (
		schema_version UInt16,
		event_time DateTime64(9, 'UTC'),
		ingested_at DateTime64(6, 'UTC') DEFAULT now64(6),
		principal String,
		authentication LowCardinality(String),
		environment LowCardinality(String),
		namespace LowCardinality(String),
		resource_type LowCardinality(String),
		resource String,
		config_revision UInt64,
		vault_revisions Map(String, UInt64)
	) ENGINE = MergeTree
	PARTITION BY toYYYYMM(event_time)
	ORDER BY (environment, resource_type, resource, event_time)
	TTL event_time + INTERVAL 90 DAY DELETE`,

	`CREATE TABLE IF NOT EXISTS audit_events (
		schema_version UInt16,
		event_id FixedString(32),
		event_time DateTime64(9, 'UTC'),
		ingested_at DateTime64(6, 'UTC') DEFAULT now64(6),
		event_type LowCardinality(String),
		operation_id String,
		actor_type LowCardinality(String),
		actor_id String,
		action LowCardinality(String),
		outcome LowCardinality(String),
		environment LowCardinality(String),
		namespace LowCardinality(String),
		resource_type LowCardinality(String),
		resource String,
		revision UInt64,
		delivery_attempt UInt32
	) ENGINE = MergeTree
	PARTITION BY toYYYYMM(event_time)
	ORDER BY (event_time, event_id)`,
}
