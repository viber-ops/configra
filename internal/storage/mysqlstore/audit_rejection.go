package mysqlstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// RejectedMutation contains only server-selected metadata. Request IDs are not
// OperationIDs: a rejected request must not reserve or overwrite a caller's key.
type RejectedMutation struct {
	RequestID      string
	Actor          Actor
	Action         string
	ErrorCode      string
	EnvironmentKey string
	NamespaceKey   string
	ResourceType   string
	ResourceKey    string
}

func (store *Store) RecordRejectedMutation(ctx context.Context, rejected RejectedMutation) error {
	id, err := hex.DecodeString(rejected.RequestID)
	if err != nil || len(id) != 12 || len(rejected.RequestID) != 24 || !validActor(rejected.Actor) ||
		!auditLabel(rejected.Action, 128) || !auditLabel(rejected.ErrorCode, 64) || !auditLabel(rejected.ResourceType, 64) {
		return errors.New("invalid rejected Mutation metadata")
	}
	if rejected.EnvironmentKey != "" && !validResourceKey(rejected.EnvironmentKey) {
		return ErrValidation
	}
	if rejected.NamespaceKey != "" && !validResourceKey(rejected.NamespaceKey) {
		return ErrValidation
	}
	if rejected.ResourceKey != "" && !validAuditResourceIdentity(rejected.ResourceType, rejected.ResourceKey) {
		return ErrValidation
	}
	payload := struct {
		Time           time.Time `json:"time"`
		OperationID    string    `json:"operation_id"`
		RequestID      string    `json:"request_id"`
		Actor          Actor     `json:"actor"`
		Action         string    `json:"action"`
		Outcome        Outcome   `json:"outcome"`
		ErrorCode      string    `json:"error_code"`
		EnvironmentKey string    `json:"environment_key"`
		NamespaceKey   string    `json:"namespace_key,omitempty"`
		ResourceType   string    `json:"resource_type"`
		ResourceKey    string    `json:"resource_key"`
	}{Time: time.Now().UTC(), RequestID: rejected.RequestID, Actor: rejected.Actor,
		Action: rejected.Action, Outcome: OutcomeValidationFailed, ErrorCode: rejected.ErrorCode,
		EnvironmentKey: rejected.EnvironmentKey, NamespaceKey: rejected.NamespaceKey,
		ResourceType: rejected.ResourceType, ResourceKey: rejected.ResourceKey}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID, err := randomID()
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO outbox_events
		(id, kind, event_type, operation_id, payload) VALUES (?, 'audit', 'management.request_rejected', NULL, ?)`, eventID, string(encoded))
	return err
}

func validAuditResourceIdentity(kind, key string) bool {
	if kind == "client_certificate" || kind == "certificate_authority" || kind == "api_token" {
		length := map[string]int{"client_certificate": 64, "certificate_authority": 32, "api_token": 16}[kind]
		_, err := hex.DecodeString(key)
		return err == nil && len(key) == length && key == strings.ToLower(key)
	}
	return validResourceKey(key)
}

func auditLabel(value string, limit int) bool {
	if value == "" || len(value) > limit {
		return false
	}
	for _, c := range value {
		if (c < 'a' || c > 'z') && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

// auditedError is attached only after the state and its Audit Outbox transaction
// committed (or when replaying that outcome). It preserves errors.Is/As behavior.
type auditedError struct{ cause error }

func (err *auditedError) Error() string { return err.cause.Error() }
func (err *auditedError) Unwrap() error { return err.cause }
func withCommittedAudit(err error) error {
	if err == nil {
		return nil
	}
	return &auditedError{cause: err}
}
func AuditWasCommitted(err error) bool {
	var committed *auditedError
	return errors.As(err, &committed)
}
