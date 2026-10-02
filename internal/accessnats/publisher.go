package accessnats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/machine"
)

const Subject = "configra.v1.access"

type Config struct {
	URLs            []string
	CredentialsFile string
	RootCAFile      string
	OnDrop          func(uint64)
	OnConnection    func(bool)
}

type Publisher struct {
	connection *nats.Conn
	closed     atomic.Bool
	onDrop     func(uint64)
}

func Connect(config Config, logger *zap.Logger) (*Publisher, error) {
	connection, err := connect(config, "configra-api-v1", logger)
	if err != nil {
		return nil, err
	}
	return &Publisher{connection: connection, onDrop: config.OnDrop}, nil
}

func connect(config Config, name string, logger *zap.Logger, extra ...nats.Option) (*nats.Conn, error) {
	if len(config.URLs) == 0 || logger == nil {
		return nil, errors.New("NATS URLs and Zap logger are required")
	}
	options := []nats.Option{
		nats.Name(name),
		nats.Timeout(2 * time.Second),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectBufSize(8 << 20),
		nats.NoCallbacksAfterClientClose(),
		nats.DisconnectErrHandler(func(_ *nats.Conn, _ error) {
			if config.OnConnection != nil {
				config.OnConnection(false)
			}
			logger.Warn("NATS disconnected")
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			if config.OnConnection != nil {
				config.OnConnection(true)
			}
			logger.Info("NATS reconnected")
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			if config.OnConnection != nil {
				config.OnConnection(false)
			}
			logger.Error("NATS connection closed")
		}),
	}
	options = append(options, extra...)
	if config.CredentialsFile != "" {
		options = append(options, nats.UserCredentials(config.CredentialsFile))
	}
	if config.RootCAFile != "" {
		options = append(options, nats.RootCAs(config.RootCAFile))
	}
	connection, err := nats.Connect(strings.Join(config.URLs, ","), options...)
	if err != nil {
		return nil, errors.New("initialize NATS connection")
	}
	if config.OnConnection != nil {
		config.OnConnection(connection.IsConnected())
	}
	return connection, nil
}

func (publisher *Publisher) TryPublish(event machine.AccessEvent) bool {
	if publisher == nil || publisher.closed.Load() {
		if publisher != nil && publisher.onDrop != nil {
			publisher.onDrop(1)
		}
		return false
	}
	payload, err := marshalAccessEvent(event)
	if err != nil {
		if publisher.onDrop != nil {
			publisher.onDrop(1)
		}
		return false
	}
	ok := publisher.connection.Publish(Subject, payload) == nil
	if !ok && publisher.onDrop != nil {
		publisher.onDrop(1)
	}
	return ok
}

func (consumer *Consumer) TryPublish(event machine.AccessEvent) bool {
	if consumer == nil || consumer.connection.IsClosed() {
		if consumer != nil && consumer.onDrop != nil {
			consumer.onDrop(1)
		}
		return false
	}
	payload, err := marshalAccessEvent(event)
	if err != nil {
		if consumer.onDrop != nil {
			consumer.onDrop(1)
		}
		return false
	}
	ok := consumer.connection.Publish(Subject, payload) == nil
	if !ok && consumer.onDrop != nil {
		consumer.onDrop(1)
	}
	return ok
}

func (publisher *Publisher) Close(ctx context.Context) error {
	if publisher == nil || !publisher.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := publisher.connection.FlushWithContext(ctx)
	publisher.connection.Close()
	return err
}

func marshalAccessEvent(event machine.AccessEvent) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion  int                    `json:"schema_version"`
		Time           time.Time              `json:"time"`
		Principal      string                 `json:"principal"`
		Authentication machine.Authentication `json:"authentication"`
		Environment    string                 `json:"environment"`
		Namespace      string                 `json:"namespace,omitempty"`
		ResourceType   string                 `json:"resource_type"`
		Resource       string                 `json:"resource"`
		ConfigRevision uint64                 `json:"config_revision,omitempty"`
		VaultRevisions map[string]uint64      `json:"vault_revisions,omitempty"`
	}{
		SchemaVersion:  1,
		Time:           event.Time,
		Principal:      event.Principal,
		Authentication: event.Authentication,
		Environment:    event.Environment,
		Namespace:      event.Namespace,
		ResourceType:   event.ResourceType,
		Resource:       event.Resource,
		ConfigRevision: event.ConfigRevision,
		VaultRevisions: event.VaultRevisions,
	})
}

func decodeAccessEvent(payload []byte) (machine.AccessEvent, error) {
	if len(payload) == 0 || len(payload) > 64<<10 {
		return machine.AccessEvent{}, errors.New("invalid Access Event")
	}
	var envelope struct {
		SchemaVersion  int                    `json:"schema_version"`
		Time           time.Time              `json:"time"`
		Principal      string                 `json:"principal"`
		Authentication machine.Authentication `json:"authentication"`
		Environment    string                 `json:"environment"`
		Namespace      string                 `json:"namespace,omitempty"`
		ResourceType   string                 `json:"resource_type"`
		Resource       string                 `json:"resource"`
		ConfigRevision uint64                 `json:"config_revision,omitempty"`
		VaultRevisions map[string]uint64      `json:"vault_revisions,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return machine.AccessEvent{}, errors.New("invalid Access Event")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return machine.AccessEvent{}, errors.New("invalid Access Event")
	}
	if envelope.SchemaVersion != 1 || envelope.Time.IsZero() || envelope.Principal == "" || len(envelope.Principal) > 255 ||
		len(envelope.Environment) > 63 || len(envelope.Namespace) > 63 || envelope.Resource == "" || len(envelope.Resource) > 127 ||
		(envelope.Authentication != machine.AuthenticationMTLS && envelope.Authentication != machine.AuthenticationTokenOnly && envelope.Authentication != machine.AuthenticationOIDC) ||
		(envelope.Authentication != machine.AuthenticationOIDC && envelope.Environment == "") ||
		(envelope.ResourceType != "config" && envelope.ResourceType != "vault_file" && envelope.ResourceType != "vault_item" && envelope.ResourceType != "release") ||
		((envelope.ResourceType == "config" || envelope.ResourceType == "release") && envelope.ConfigRevision == 0) ||
		((envelope.ResourceType == "config" || envelope.ResourceType == "release") && envelope.Namespace != "") ||
		((envelope.ResourceType == "vault_file" || envelope.ResourceType == "vault_item") && envelope.Namespace == "") ||
		((envelope.ResourceType == "vault_file" || envelope.ResourceType == "vault_item") && len(envelope.VaultRevisions) != 1) {
		return machine.AccessEvent{}, errors.New("invalid Access Event")
	}
	for _, revision := range envelope.VaultRevisions {
		if revision == 0 {
			return machine.AccessEvent{}, errors.New("invalid Access Event")
		}
	}
	return machine.AccessEvent{
		Time:           envelope.Time.UTC(),
		Principal:      envelope.Principal,
		Authentication: envelope.Authentication,
		Environment:    envelope.Environment,
		Namespace:      envelope.Namespace,
		ResourceType:   envelope.ResourceType,
		Resource:       envelope.Resource,
		ConfigRevision: envelope.ConfigRevision,
		VaultRevisions: envelope.VaultRevisions,
	}, nil
}
