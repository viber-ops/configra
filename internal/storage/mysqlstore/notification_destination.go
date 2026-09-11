package mysqlstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

type NotificationProvider string

const (
	NotificationGenericWebhook NotificationProvider = "generic_webhook"
	NotificationFeishuBot      NotificationProvider = "feishu_bot"
)

type NotificationDestination struct {
	Key          string               `json:"key"`
	DisplayName  string               `json:"display_name"`
	Provider     NotificationProvider `json:"provider"`
	SafeHost     string               `json:"safe_host"`
	MaskedSuffix string               `json:"masked_suffix"`
	Enabled      bool                 `json:"enabled"`
	EventTypes   []string             `json:"event_types"`
	Archived     bool                 `json:"archived"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type NotificationDestinationCommit struct {
	OperationID string
	Actor       Actor
	Key         string
	DisplayName string
	Provider    NotificationProvider
	URL         *string
	Secret      *string
	Enabled     bool
	EventTypes  []string
}

type NotificationDestinationResult struct {
	Outcome      Outcome              `json:"outcome"`
	Key          string               `json:"key"`
	DisplayName  string               `json:"display_name"`
	Provider     NotificationProvider `json:"provider"`
	SafeHost     string               `json:"safe_host"`
	MaskedSuffix string               `json:"masked_suffix"`
	Enabled      bool                 `json:"enabled"`
	EventTypes   []string             `json:"event_types"`
	Archived     bool                 `json:"archived"`
}

type notificationCredentials struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func (store *Store) CommitNotificationDestination(
	ctx context.Context,
	request NotificationDestinationCommit,
) (NotificationDestinationResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return NotificationDestinationResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	eventTypes, validationErr := validateNotificationDestinationCommit(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return NotificationDestinationResult{}, fmt.Errorf("begin Notification Destination commit: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return NotificationDestinationResult{}, err
	}
	if replayed {
		var previous NotificationDestinationResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return NotificationDestinationResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if validationErr != nil {
		return finishNotificationDestinationFailure(ctx, transaction, request, validationErr)
	}

	var (
		id                     []byte
		current                NotificationDestination
		algorithm, keyVersion  string
		nonce, ciphertext, dek []byte
	)
	err = transaction.QueryRowContext(ctx, `
		SELECT id, display_name, provider, safe_host, masked_suffix, enabled,
		       archived_at IS NOT NULL, algorithm, key_version, nonce, ciphertext, encrypted_dek
		FROM notification_destinations WHERE resource_key = ? FOR UPDATE
	`, request.Key).Scan(
		&id, &current.DisplayName, &current.Provider, &current.SafeHost, &current.MaskedSuffix,
		&current.Enabled, &current.Archived, &algorithm, &keyVersion, &nonce, &ciphertext, &dek,
	)
	found := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return NotificationDestinationResult{}, fmt.Errorf("lock Notification Destination: %w", err)
	}
	if found && (current.Archived || current.Provider != request.Provider) {
		return finishNotificationDestinationFailure(ctx, transaction, request, errors.New("Notification Destination is Archived or provider changed"))
	}
	if !found {
		if request.URL == nil {
			return finishNotificationDestinationFailure(ctx, transaction, request, errors.New("new Notification Destination requires URL"))
		}
		id, err = randomID()
		if err != nil {
			return NotificationDestinationResult{}, err
		}
	}

	credentials := notificationCredentials{}
	if found {
		plaintext, err := store.provider.DecryptSecret(notificationCredentialIdentity(id), vaultcrypto.EncryptedSecret{
			Algorithm: algorithm, KeyVersion: keyVersion, Nonce: nonce, Ciphertext: ciphertext, EncryptedDEK: dek,
		})
		if err != nil {
			return NotificationDestinationResult{}, fmt.Errorf("decrypt Notification Destination credentials: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(plaintext))
		decoder.DisallowUnknownFields()
		decodeErr := decoder.Decode(&credentials)
		clear(plaintext)
		if decodeErr != nil || credentials.URL == "" {
			return NotificationDestinationResult{}, fmt.Errorf("decode Notification Destination credentials: %w", vaultcrypto.ErrIntegrity)
		}
	}
	if request.URL != nil {
		credentials.URL = *request.URL
	}
	if request.Secret != nil {
		credentials.Secret = *request.Secret
	}
	parsed, err := parseNotificationURL(credentials.URL)
	if err != nil || len(credentials.Secret) > 16<<10 {
		return finishNotificationDestinationFailure(ctx, transaction, request, errors.New("invalid Notification Destination credentials"))
	}
	safeHost := strings.ToLower(parsed.Hostname())
	maskedSuffix := maskedURLSuffix(parsed)
	currentEvents, err := notificationEventTypes(ctx, transaction, id)
	if err != nil {
		return NotificationDestinationResult{}, err
	}
	result := NotificationDestinationResult{
		Outcome: OutcomeSuccess, Key: request.Key, DisplayName: request.DisplayName, Provider: request.Provider,
		SafeHost: safeHost, MaskedSuffix: maskedSuffix, Enabled: request.Enabled, EventTypes: eventTypes,
	}
	unchanged := found && current.DisplayName == request.DisplayName && current.Enabled == request.Enabled &&
		current.SafeHost == safeHost && current.MaskedSuffix == maskedSuffix && slices.Equal(currentEvents, eventTypes) &&
		request.URL == nil && request.Secret == nil
	if unchanged {
		result.Outcome = OutcomeNoChange
	} else {
		plaintext, err := json.Marshal(credentials)
		if err != nil {
			return NotificationDestinationResult{}, errors.New("encode Notification Destination credentials")
		}
		encrypted, err := store.provider.EncryptSecret(notificationCredentialIdentity(id), plaintext)
		clear(plaintext)
		if err != nil {
			return NotificationDestinationResult{}, fmt.Errorf("encrypt Notification Destination credentials: %w", err)
		}
		if found {
			_, err = transaction.ExecContext(ctx, `
				UPDATE notification_destinations
				SET display_name = ?, safe_host = ?, masked_suffix = ?, algorithm = ?, key_version = ?,
				    nonce = ?, ciphertext = ?, encrypted_dek = ?, enabled = ?
				WHERE id = ?
			`, request.DisplayName, safeHost, maskedSuffix, encrypted.Algorithm, encrypted.KeyVersion,
				encrypted.Nonce, encrypted.Ciphertext, encrypted.EncryptedDEK, request.Enabled, id)
		} else {
			_, err = transaction.ExecContext(ctx, `
				INSERT INTO notification_destinations
					(id, resource_key, display_name, provider, safe_host, masked_suffix,
					 algorithm, key_version, nonce, ciphertext, encrypted_dek, enabled)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, id, request.Key, request.DisplayName, request.Provider, safeHost, maskedSuffix,
				encrypted.Algorithm, encrypted.KeyVersion, encrypted.Nonce, encrypted.Ciphertext,
				encrypted.EncryptedDEK, request.Enabled)
		}
		if err != nil {
			return NotificationDestinationResult{}, fmt.Errorf("write Notification Destination: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `DELETE FROM notification_subscriptions WHERE destination_id = ?`, id); err != nil {
			return NotificationDestinationResult{}, fmt.Errorf("clear Notification subscriptions: %w", err)
		}
		for _, eventType := range eventTypes {
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO notification_subscriptions (destination_id, event_type) VALUES (?, ?)
			`, id, eventType); err != nil {
				return NotificationDestinationResult{}, fmt.Errorf("write Notification subscription: %w", err)
			}
		}
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination.updated", Action: "notification_destination.commit",
		ResourceType: "notification_destination", ResourceKey: request.Key,
	}, false); err != nil {
		return NotificationDestinationResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationDestinationResult{}, fmt.Errorf("commit Notification Destination: %w", err)
	}
	return result, nil
}

func (store *Store) ListNotificationDestinations(ctx context.Context, includeArchived bool) ([]NotificationDestination, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT destination.resource_key, destination.display_name, destination.provider,
		       destination.safe_host, destination.masked_suffix, destination.enabled,
		       destination.archived_at IS NOT NULL, destination.created_at, destination.updated_at,
		       subscription.event_type
		FROM notification_destinations AS destination
		LEFT JOIN notification_subscriptions AS subscription ON subscription.destination_id = destination.id
		WHERE destination.archived_at IS NULL OR ?
		ORDER BY destination.resource_key, subscription.event_type
	`, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("list Notification Destinations: %w", err)
	}
	defer rows.Close()
	result := make([]NotificationDestination, 0)
	for rows.Next() {
		var destination NotificationDestination
		var eventType sql.NullString
		if err := rows.Scan(
			&destination.Key, &destination.DisplayName, &destination.Provider, &destination.SafeHost,
			&destination.MaskedSuffix, &destination.Enabled, &destination.Archived,
			&destination.CreatedAt, &destination.UpdatedAt, &eventType,
		); err != nil {
			return nil, fmt.Errorf("scan Notification Destination: %w", err)
		}
		if len(result) == 0 || result[len(result)-1].Key != destination.Key {
			destination.EventTypes = make([]string, 0)
			destination.CreatedAt = destination.CreatedAt.UTC()
			destination.UpdatedAt = destination.UpdatedAt.UTC()
			result = append(result, destination)
		}
		if eventType.Valid {
			index := len(result) - 1
			result[index].EventTypes = append(result[index].EventTypes, eventType.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Notification Destinations: %w", err)
	}
	return result, nil
}

type NotificationDestinationLifecycleAction string

const (
	NotificationDestinationArchive   NotificationDestinationLifecycleAction = "archive"
	NotificationDestinationUnarchive NotificationDestinationLifecycleAction = "unarchive"
)

type NotificationDestinationLifecycle struct {
	OperationID string
	Actor       Actor
	Action      NotificationDestinationLifecycleAction
	Key         string
}

type NotificationDestinationLifecycleResult struct {
	Outcome  Outcome `json:"outcome"`
	Key      string  `json:"key"`
	Archived bool    `json:"archived"`
}

func (store *Store) ApplyNotificationDestinationLifecycle(
	ctx context.Context,
	request NotificationDestinationLifecycle,
) (NotificationDestinationLifecycleResult, error) {
	if !validOperationID(request.OperationID) || !validActor(request.Actor) {
		return NotificationDestinationLifecycleResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return NotificationDestinationLifecycleResult{}, fmt.Errorf("begin Notification Destination lifecycle: %w", err)
	}
	defer transaction.Rollback()
	replayed, replay, err := beginOperation(ctx, transaction, request.OperationID, digest, request.Actor)
	if err != nil {
		return NotificationDestinationLifecycleResult{}, err
	}
	if replayed {
		var previous NotificationDestinationLifecycleResult
		if err := json.Unmarshal(replay.Response, &previous); err != nil || previous.Outcome != replay.Outcome {
			return NotificationDestinationLifecycleResult{}, errors.New("existing Operation failed integrity validation")
		}
		return previous, outcomeError(previous.Outcome)
	}
	if !validResourceKey(request.Key) || (request.Action != NotificationDestinationArchive && request.Action != NotificationDestinationUnarchive) {
		return finishNotificationDestinationLifecycleFailure(ctx, transaction, request)
	}
	var id []byte
	var archivedAt sql.NullTime
	if err := transaction.QueryRowContext(ctx, `
		SELECT id, archived_at FROM notification_destinations WHERE resource_key = ? FOR UPDATE
	`, request.Key).Scan(&id, &archivedAt); errors.Is(err, sql.ErrNoRows) {
		return finishNotificationDestinationLifecycleFailure(ctx, transaction, request)
	} else if err != nil {
		return NotificationDestinationLifecycleResult{}, fmt.Errorf("lock Notification Destination lifecycle: %w", err)
	}
	result := NotificationDestinationLifecycleResult{Outcome: OutcomeNoChange, Key: request.Key, Archived: request.Action == NotificationDestinationArchive}
	if request.Action == NotificationDestinationArchive && !archivedAt.Valid {
		_, err = transaction.ExecContext(ctx, `UPDATE notification_destinations SET archived_at = UTC_TIMESTAMP(6) WHERE id = ?`, id)
		result.Outcome = OutcomeSuccess
	} else if request.Action == NotificationDestinationUnarchive && archivedAt.Valid {
		_, err = transaction.ExecContext(ctx, `UPDATE notification_destinations SET archived_at = NULL WHERE id = ?`, id)
		result.Outcome = OutcomeSuccess
	}
	if err != nil {
		return NotificationDestinationLifecycleResult{}, fmt.Errorf("update Notification Destination lifecycle: %w", err)
	}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination." + string(request.Action), Action: "notification_destination." + string(request.Action),
		ResourceType: "notification_destination", ResourceKey: request.Key,
	}, false); err != nil {
		return NotificationDestinationLifecycleResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationDestinationLifecycleResult{}, fmt.Errorf("commit Notification Destination lifecycle: %w", err)
	}
	return result, nil
}

type NotificationDestinationID [16]byte

type NotificationTarget struct {
	OutboxID       OutboxID
	DestinationID  NotificationDestinationID
	DestinationKey string
	Provider       NotificationProvider
	URL            string
	Secret         string
	ErrorCode      string
	Attempt        uint32
	Active         bool
}

func (store *Store) ClaimNotificationTargets(ctx context.Context, outboxID OutboxID, limit int) ([]NotificationTarget, error) {
	if outboxID == (OutboxID{}) || limit < 1 || limit > 1000 {
		return nil, errors.New("invalid Notification target claim")
	}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin Notification target claim: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `
		UPDATE notification_targets
		SET status = 'dead', last_error_code = 'attempt_limit_reached'
		WHERE outbox_event_id = ? AND status = 'pending' AND attempts >= 8
	`, outboxID[:]); err != nil {
		return nil, fmt.Errorf("expire Notification targets at attempt limit: %w", err)
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT target.destination_id, destination.resource_key, destination.provider,
		       destination.enabled AND destination.archived_at IS NULL,
		       destination.algorithm, destination.key_version, destination.nonce,
		       destination.ciphertext, destination.encrypted_dek, target.attempts
		FROM notification_targets AS target
		JOIN notification_destinations AS destination ON destination.id = target.destination_id
		JOIN outbox_events AS event ON event.id = target.outbox_event_id
		WHERE target.outbox_event_id = ? AND event.kind = 'notification' AND event.status = 'processing'
		  AND target.status = 'pending' AND target.next_attempt_at <= UTC_TIMESTAMP(6) AND target.attempts < 8
		ORDER BY target.next_attempt_at, destination.resource_key
		LIMIT ?
		FOR UPDATE
	`, outboxID[:], limit)
	if err != nil {
		return nil, fmt.Errorf("select Notification targets: %w", err)
	}
	targets := make([]NotificationTarget, 0)
	for rows.Next() {
		var (
			target                 NotificationTarget
			destinationID          []byte
			algorithm, keyVersion  string
			nonce, ciphertext, dek []byte
			attempts               uint32
		)
		if err := rows.Scan(
			&destinationID, &target.DestinationKey, &target.Provider, &target.Active,
			&algorithm, &keyVersion, &nonce, &ciphertext, &dek, &attempts,
		); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan Notification target: %w", err)
		}
		if len(destinationID) != len(target.DestinationID) {
			_ = rows.Close()
			return nil, errors.New("Notification target failed integrity validation")
		}
		copy(target.OutboxID[:], outboxID[:])
		copy(target.DestinationID[:], destinationID)
		target.Attempt = attempts + 1
		if target.Active {
			plaintext, err := store.provider.DecryptSecret(notificationCredentialIdentity(destinationID), vaultcrypto.EncryptedSecret{
				Algorithm: algorithm, KeyVersion: keyVersion, Nonce: nonce, Ciphertext: ciphertext, EncryptedDEK: dek,
			})
			if err != nil {
				target.ErrorCode = "credential_integrity_failure"
			} else {
				var credentials notificationCredentials
				decoder := json.NewDecoder(bytes.NewReader(plaintext))
				decoder.DisallowUnknownFields()
				decodeErr := decoder.Decode(&credentials)
				clear(plaintext)
				if decodeErr != nil || credentials.URL == "" {
					target.ErrorCode = "credential_integrity_failure"
				} else {
					target.URL = credentials.URL
					target.Secret = credentials.Secret
				}
			}
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate Notification targets: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close Notification targets: %w", err)
	}
	for _, target := range targets {
		result, err := transaction.ExecContext(ctx, `
			UPDATE notification_targets SET attempts = ?
			WHERE outbox_event_id = ? AND destination_id = ? AND status = 'pending' AND attempts = ?
		`, target.Attempt, target.OutboxID[:], target.DestinationID[:], target.Attempt-1)
		if err != nil {
			return nil, fmt.Errorf("claim Notification target: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("inspect claimed Notification target: %w", err)
		}
		if changed != 1 {
			return nil, errors.New("Notification target claim lost")
		}
	}
	if err := transaction.Commit(); err != nil {
		return nil, fmt.Errorf("commit Notification target claim: %w", err)
	}
	return targets, nil
}

func validateNotificationDestinationCommit(request NotificationDestinationCommit) ([]string, error) {
	if !validResourceKey(request.Key) || request.DisplayName == "" || len(request.DisplayName) > 255 ||
		(request.Provider != NotificationGenericWebhook && request.Provider != NotificationFeishuBot) {
		return nil, errors.New("invalid Notification Destination metadata")
	}
	eventTypes := slices.Clone(request.EventTypes)
	slices.Sort(eventTypes)
	for index, eventType := range eventTypes {
		if !validNotificationEventType(eventType) || (index > 0 && eventType == eventTypes[index-1]) {
			return nil, errors.New("invalid or duplicate Notification Event type")
		}
	}
	if request.URL != nil {
		parsed, err := parseNotificationURL(*request.URL)
		if err != nil || (request.Provider == NotificationFeishuBot && !validFeishuNotificationURL(parsed)) {
			return nil, errors.New("invalid Notification Destination URL")
		}
	}
	if request.Secret != nil && len(*request.Secret) > 16<<10 {
		return nil, errors.New("Notification Secret exceeds 16 KiB")
	}
	return eventTypes, nil
}

func validFeishuNotificationURL(parsed *url.URL) bool {
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	token := strings.TrimPrefix(parsed.EscapedPath(), "/open-apis/bot/v2/hook/")
	return strings.EqualFold(parsed.Hostname(), "open.feishu.cn") && port == "443" && parsed.RawQuery == "" &&
		token != "" && token != parsed.EscapedPath() && !strings.Contains(token, "/")
}

func parseNotificationURL(value string) (*url.URL, error) {
	if value == "" || len(value) > 8<<10 {
		return nil, errors.New("Notification URL is required and must not exceed 8 KiB")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, errors.New("Notification URL must be an absolute HTTPS URL without userinfo or fragment")
	}
	return parsed, nil
}

func maskedURLSuffix(parsed *url.URL) string {
	value := parsed.EscapedPath()
	if parsed.RawQuery != "" {
		value += "?" + parsed.RawQuery
	}
	characters := []rune(value)
	if len(characters) > 4 {
		characters = characters[len(characters)-4:]
	}
	return "••••" + string(characters)
}

func validNotificationEventType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') &&
			character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func notificationCredentialIdentity(id []byte) string {
	return "notification-destination:" + hex.EncodeToString(id)
}

func notificationEventTypes(ctx context.Context, transaction *sql.Tx, id []byte) ([]string, error) {
	if len(id) == 0 {
		return nil, nil
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT event_type FROM notification_subscriptions WHERE destination_id = ? ORDER BY event_type
	`, id)
	if err != nil {
		return nil, fmt.Errorf("read Notification subscriptions: %w", err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, fmt.Errorf("scan Notification subscription: %w", err)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Notification subscriptions: %w", err)
	}
	return result, nil
}

func finishNotificationDestinationFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request NotificationDestinationCommit,
	cause error,
) (NotificationDestinationResult, error) {
	result := NotificationDestinationResult{Outcome: OutcomeValidationFailed, Key: request.Key}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination.validation_failed", Action: "notification_destination.commit",
		ResourceType: "notification_destination", ResourceKey: request.Key,
	}, false); err != nil {
		return NotificationDestinationResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationDestinationResult{}, fmt.Errorf("commit Notification Destination validation Audit: %w", err)
	}
	return result, fmt.Errorf("%w: %v", ErrValidation, cause)
}

func finishNotificationDestinationLifecycleFailure(
	ctx context.Context,
	transaction *sql.Tx,
	request NotificationDestinationLifecycle,
) (NotificationDestinationLifecycleResult, error) {
	result := NotificationDestinationLifecycleResult{Outcome: OutcomeValidationFailed, Key: request.Key}
	if err := finishOperation(ctx, transaction, request.OperationID, request.Actor, result.Outcome, 0, result, mutationEvent{
		Type: "notification_destination.validation_failed", Action: "notification_destination." + string(request.Action),
		ResourceType: "notification_destination", ResourceKey: request.Key,
	}, false); err != nil {
		return NotificationDestinationLifecycleResult{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NotificationDestinationLifecycleResult{}, fmt.Errorf("commit Notification Destination lifecycle Audit: %w", err)
	}
	return result, ErrValidation
}
