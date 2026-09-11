package management

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/logstore"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/notification"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
	managementweb "github.com/viber-ops/configra/web"
)

const maxManagementRequestBytes = 32 << 20

type ConfigRepository interface {
	AuthorityRepository
	ListEnvironments(context.Context, bool) ([]mysqlstore.Environment, error)
	ApplyEnvironmentChange(context.Context, mysqlstore.EnvironmentChange) (mysqlstore.EnvironmentChangeResult, error)
	ListConfigs(context.Context, bool) ([]mysqlstore.ConfigSummary, error)
	ListConfigRevisions(context.Context, string, string) ([]mysqlstore.ConfigRevision, error)
	ReadRawConfigRevision(context.Context, string, string, uint64) (mysqlstore.RawConfig, error)
	ValidateConfig(context.Context, mysqlstore.ConfigValidation) (mysqlstore.ConfigValidationResult, error)
	CommitConfig(context.Context, mysqlstore.ConfigCommit) (mysqlstore.ConfigCommitResult, error)
	MergeConfig(context.Context, mysqlstore.ConfigTransfer) (mysqlstore.ConfigCommitResult, error)
	ReplaceConfig(context.Context, mysqlstore.ConfigTransfer) (mysqlstore.ConfigCommitResult, error)
	RestoreConfig(context.Context, mysqlstore.ConfigRestore) (mysqlstore.ConfigCommitResult, error)
	CloneConfig(context.Context, mysqlstore.ConfigClone) (mysqlstore.ConfigCommitResult, error)
	ApplyConfigLifecycleChange(context.Context, mysqlstore.ConfigLifecycleChange) (mysqlstore.ConfigLifecycleResult, error)
	ReadRawConfig(context.Context, string, string) (mysqlstore.RawConfig, error)
	ReadResolvedConfig(context.Context, string, string, string) (machine.ResolvedConfig, error)
	ListVaultItems(context.Context, bool) ([]mysqlstore.VaultItemSummary, error)
	ListVaultUsages(context.Context, string, string) ([]mysqlstore.VaultUsage, error)
	ReadVaultItem(context.Context, string, string, uint64, bool) (mysqlstore.VaultItem, error)
	ListVaultRevisions(context.Context, string, string) ([]mysqlstore.VaultRevision, error)
	CommitVault(context.Context, mysqlstore.VaultCommit) (mysqlstore.VaultCommitResult, error)
	RestoreVault(context.Context, mysqlstore.VaultRestore) (mysqlstore.VaultCommitResult, error)
	ApplyVaultLifecycleChange(context.Context, mysqlstore.VaultLifecycleChange) (mysqlstore.VaultLifecycleResult, error)
	ListTokens(context.Context, bool) ([]mysqlstore.TokenSummary, error)
	CreateToken(context.Context, mysqlstore.TokenCreate) (mysqlstore.TokenCreateResult, error)
	SetTokenEnvironments(context.Context, mysqlstore.TokenEnvironmentChange) (mysqlstore.TokenEnvironmentResult, error)
	RevokeToken(context.Context, mysqlstore.TokenRevoke) (mysqlstore.TokenRevokeResult, error)
	ListClientCertificates(context.Context, bool) ([]mysqlstore.ClientCertificate, error)
	RegisterVerifiedClientCertificate(context.Context, mysqlstore.ClientCertificateRegister) (mysqlstore.ClientCertificateResult, error)
	RevokeClientCertificate(context.Context, mysqlstore.ClientCertificateRevoke) (mysqlstore.ClientCertificateResult, error)
	ListNotificationDestinations(context.Context, bool) ([]mysqlstore.NotificationDestination, error)
	CommitNotificationDestination(context.Context, mysqlstore.NotificationDestinationCommit) (mysqlstore.NotificationDestinationResult, error)
	ApplyNotificationDestinationLifecycle(context.Context, mysqlstore.NotificationDestinationLifecycle) (mysqlstore.NotificationDestinationLifecycleResult, error)
	ListNotificationDeliveries(context.Context, string, int) ([]mysqlstore.NotificationDelivery, error)
	QueueNotificationTest(context.Context, mysqlstore.NotificationTestRequest) (mysqlstore.NotificationQueueResult, error)
	RedeliverNotification(context.Context, mysqlstore.NotificationRedeliveryRequest) (mysqlstore.NotificationQueueResult, error)
}

type LogReader interface {
	ListAccess(context.Context, int) ([]logstore.AccessRecord, error)
	ListAudits(context.Context, logstore.AuditQuery) (logstore.AuditPage, error)
}

type server struct {
	configs   ConfigRepository
	publisher machine.AccessPublisher
	clientCAs *x509.CertPool
	logs      LogReader
}

func NewHandler(configs ConfigRepository, publisher machine.AccessPublisher, clientCAs *x509.CertPool, logs ...LogReader) http.Handler {
	if configs == nil {
		panic("management ConfigRepository is required")
	}
	server := &server{configs: configs, publisher: publisher, clientCAs: clientCAs}
	if len(logs) > 0 {
		server.logs = logs[0]
	}
	mux := http.NewServeMux()
	interfaceHandler := managementweb.NewHandler()
	mux.Handle("GET /{$}", interfaceHandler)
	mux.Handle("GET /ui/", interfaceHandler)
	mux.HandleFunc("GET /v1/me", server.readMe)
	mux.HandleFunc("GET /v1/environments", server.listEnvironments)
	mux.HandleFunc("POST /v1/environments", server.createEnvironment)
	mux.HandleFunc("PATCH /v1/environments/{environment}", server.renameEnvironment)
	mux.HandleFunc("POST /v1/environments/{environment}/archive", server.archiveEnvironment)
	mux.HandleFunc("POST /v1/environments/{environment}/unarchive", server.unarchiveEnvironment)
	mux.HandleFunc("GET /v1/configs", server.listConfigs)
	mux.HandleFunc("POST /v1/configs/validate", server.validateConfig)
	mux.HandleFunc("POST /v1/configs/{config}/archive", server.archiveConfig)
	mux.HandleFunc("POST /v1/configs/{config}/unarchive", server.unarchiveConfig)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}", server.readRawConfig)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/resolved-preview", server.readResolvedConfigPreview)
	mux.HandleFunc("PUT /v1/environments/{environment}/configs/{config}", server.commitConfig)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/revisions", server.listConfigRevisions)
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/revisions/{revision}", server.readRawConfigRevision)
	mux.HandleFunc("POST /v1/environments/{environment}/configs/{config}/transfer-preview", server.previewConfigTransfer)
	mux.HandleFunc("POST /v1/environments/{environment}/configs/{config}/merge", server.mergeConfig)
	mux.HandleFunc("POST /v1/environments/{environment}/configs/{config}/replace", server.replaceConfig)
	mux.HandleFunc("POST /v1/environments/{environment}/configs/{config}/restore", server.restoreConfig)
	mux.HandleFunc("POST /v1/environments/{environment}/configs/{config}/clone", server.cloneConfig)
	mux.HandleFunc("GET /v1/vault-items", server.listVaultItems)
	mux.HandleFunc("GET /v1/vault-items/{namespace}/{item}", server.readVaultItem)
	mux.HandleFunc("GET /v1/vault-items/{namespace}/{item}/usages", server.listVaultUsages)
	mux.HandleFunc("GET /v1/vault-items/{namespace}/{item}/values", server.readVaultItemValues)
	mux.HandleFunc("GET /v1/vault-items/{namespace}/{item}/revisions", server.listVaultRevisions)
	mux.HandleFunc("GET /v1/vault-items/{namespace}/{item}/revisions/{revision}", server.readVaultItem)
	mux.HandleFunc("GET /v1/vault-items/{namespace}/{item}/revisions/{revision}/values", server.readVaultItemValues)
	mux.HandleFunc("PUT /v1/vault-items/{namespace}/{item}", server.commitVaultItem)
	mux.HandleFunc("POST /v1/vault-items/{namespace}/{item}/restore", server.restoreVaultItem)
	mux.HandleFunc("POST /v1/vault-items/{namespace}/{item}/archive", server.archiveVaultItem)
	mux.HandleFunc("POST /v1/vault-items/{namespace}/{item}/unarchive", server.unarchiveVaultItem)
	mux.HandleFunc("GET /v1/api-tokens", server.listTokens)
	mux.HandleFunc("POST /v1/api-tokens", server.createToken)
	mux.HandleFunc("PUT /v1/api-tokens/{public_id}/environments", server.setTokenEnvironments)
	mux.HandleFunc("POST /v1/api-tokens/{public_id}/revoke", server.revokeToken)
	mux.HandleFunc("GET /v1/client-certificates", server.listClientCertificates)
	mux.HandleFunc("GET /v1/certificate-authorities", server.listCertificateAuthorities)
	mux.HandleFunc("POST /v1/certificate-authorities", server.createCertificateAuthority)
	mux.HandleFunc("POST /v1/certificate-authorities/{authority}/revoke", server.revokeCertificateAuthority)
	mux.HandleFunc("POST /v1/client-certificates/issue", server.issueClientCertificate)
	mux.HandleFunc("POST /v1/client-certificates", server.registerClientCertificate)
	mux.HandleFunc("POST /v1/client-certificates/{fingerprint}/revoke", server.revokeClientCertificate)
	mux.HandleFunc("GET /v1/notification-destinations", server.listNotificationDestinations)
	mux.HandleFunc("PUT /v1/notification-destinations/{destination}", server.commitNotificationDestination)
	mux.HandleFunc("POST /v1/notification-destinations/{destination}/archive", server.archiveNotificationDestination)
	mux.HandleFunc("POST /v1/notification-destinations/{destination}/unarchive", server.unarchiveNotificationDestination)
	mux.HandleFunc("POST /v1/notification-destinations/{destination}/test", server.testNotificationDestination)
	mux.HandleFunc("GET /v1/notification-destinations/{destination}/deliveries", server.listNotificationDeliveries)
	mux.HandleFunc("POST /v1/notification-destinations/{destination}/deliveries/{delivery}/redeliver", server.redeliverNotification)
	mux.HandleFunc("GET /v1/access", server.listAccess)
	mux.HandleFunc("GET /v1/audit", server.listAudits)
	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeError(response, http.StatusForbidden, "csrf_rejected")
	}))
	return csrf.Handler(mux)
}

func (server *server) readMe(response http.ResponseWriter, request *http.Request) {
	principal, ok := requirePrincipal(response, request)
	if !ok {
		return
	}
	writeJSON(response, struct {
		Issuer  string         `json:"issuer"`
		Subject string         `json:"subject"`
		Email   string         `json:"email"`
		Role    humanauth.Role `json:"role"`
	}{principal.Issuer, principal.Subject, principal.Email, principal.Role})
}

func (server *server) listEnvironments(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	includeArchived, ok := parseIncludeArchived(response, request)
	if !ok {
		return
	}
	environments, err := server.configs.ListEnvironments(request.Context(), includeArchived)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.Environment `json:"items"`
	}{environments})
}

func (server *server) listConfigs(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	includeArchived, ok := parseIncludeArchived(response, request)
	if !ok {
		return
	}
	configs, err := server.configs.ListConfigs(request.Context(), includeArchived)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.ConfigSummary `json:"items"`
	}{configs})
}

func (server *server) listVaultItems(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	includeArchived, ok := parseIncludeArchived(response, request)
	if !ok {
		return
	}
	items, err := server.configs.ListVaultItems(request.Context(), includeArchived)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.VaultItemSummary `json:"items"`
	}{items})
}

func (server *server) listVaultUsages(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	items, err := server.configs.ListVaultUsages(request.Context(), request.PathValue("namespace"), request.PathValue("item"))
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.VaultUsage `json:"items"`
	}{items})
}

func (server *server) listVaultRevisions(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	revisions, err := server.configs.ListVaultRevisions(request.Context(), request.PathValue("namespace"), request.PathValue("item"))
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.VaultRevision `json:"items"`
	}{revisions})
}

func (server *server) readVaultItem(response http.ResponseWriter, request *http.Request) {
	server.readVault(response, request, false)
}

func (server *server) readVaultItemValues(response http.ResponseWriter, request *http.Request) {
	server.readVault(response, request, true)
}

func (server *server) readVault(response http.ResponseWriter, request *http.Request, includeValues bool) {
	var principal humanauth.Principal
	var ok bool
	if includeValues {
		principal, ok = requireAdmin(response, request)
	} else {
		principal, ok = requirePrincipal(response, request)
	}
	if !ok {
		return
	}
	revision := uint64(0)
	if raw := request.PathValue("revision"); raw != "" {
		var err error
		revision, err = strconv.ParseUint(raw, 10, 64)
		if err != nil || revision == 0 {
			writeError(response, http.StatusBadRequest, "invalid_revision")
			return
		}
	}
	item, err := server.configs.ReadVaultItem(request.Context(), request.PathValue("namespace"), request.PathValue("item"), revision, includeValues)
	if err != nil {
		writeReadError(response, err)
		return
	}
	if !includeValues {
		item = vaultItemWithoutValues(item)
	}
	if !writeJSONResponse(response, item) || !includeValues || server.publisher == nil {
		return
	}
	server.publisher.TryPublish(machine.AccessEvent{
		Time:           time.Now().UTC(),
		Principal:      principal.ActorID(),
		Authentication: machine.AuthenticationOIDC,
		Namespace:      item.NamespaceKey,
		ResourceType:   "vault_item",
		Resource:       item.Key,
		VaultRevisions: map[string]uint64{item.NamespaceKey + "." + item.Key: item.Revision},
	})
}

func vaultItemWithoutValues(item mysqlstore.VaultItem) mysqlstore.VaultItem {
	item.Snapshot.Fields = slices.Clone(item.Snapshot.Fields)
	variants := make([]vaultdoc.Variant, len(item.Snapshot.Variants))
	for index, variant := range item.Snapshot.Variants {
		variants[index] = variant
		variants[index].Environments = slices.Clone(variant.Environments)
		variants[index].Values = nil
	}
	item.Snapshot.Variants = variants
	return item
}

func (server *server) listConfigRevisions(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	revisions, err := server.configs.ListConfigRevisions(
		request.Context(), request.PathValue("environment"), request.PathValue("config"),
	)
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.ConfigRevision `json:"items"`
	}{revisions})
}

func (server *server) readRawConfigRevision(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	revision, err := strconv.ParseUint(request.PathValue("revision"), 10, 64)
	if err != nil || revision == 0 {
		writeError(response, http.StatusBadRequest, "invalid_revision")
		return
	}
	config, err := server.configs.ReadRawConfigRevision(
		request.Context(), request.PathValue("environment"), request.PathValue("config"), revision,
	)
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, config)
}

func (server *server) createEnvironment(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		Key         string `json:"key"`
		DisplayName string `json:"display_name"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	server.changeEnvironment(response, request, principal, mysqlstore.EnvironmentCreate, body.Key, body.DisplayName)
}

func (server *server) renameEnvironment(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	server.changeEnvironment(response, request, principal, mysqlstore.EnvironmentRename, request.PathValue("environment"), body.DisplayName)
}

func (server *server) archiveEnvironment(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if ok {
		server.changeEnvironment(response, request, principal, mysqlstore.EnvironmentArchive, request.PathValue("environment"), "")
	}
}

func (server *server) unarchiveEnvironment(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if ok {
		server.changeEnvironment(response, request, principal, mysqlstore.EnvironmentUnarchive, request.PathValue("environment"), "")
	}
}

func (server *server) changeEnvironment(
	response http.ResponseWriter,
	request *http.Request,
	principal humanauth.Principal,
	action mysqlstore.EnvironmentAction,
	key string,
	displayName string,
) {
	result, err := server.configs.ApplyEnvironmentChange(request.Context(), mysqlstore.EnvironmentChange{
		OperationID: request.Header.Get("Idempotency-Key"),
		Actor:       mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		Action:      action,
		Key:         key,
		DisplayName: displayName,
	})
	writeMutationResult(response, result, err)
}

func (server *server) readRawConfig(response http.ResponseWriter, request *http.Request) {
	if _, ok := humanauth.PrincipalFromContext(request.Context()); !ok {
		writeError(response, http.StatusUnauthorized, "unauthenticated")
		return
	}
	config, err := server.configs.ReadRawConfig(
		request.Context(), request.PathValue("environment"), request.PathValue("config"),
	)
	if err != nil {
		if errors.Is(err, mysqlstore.ErrNotFound) {
			writeError(response, http.StatusNotFound, "not_found")
		} else {
			writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		}
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(response).Encode(config)
}

func (server *server) commitConfig(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		Name             string `json:"name"`
		ExpectedRevision uint64 `json:"expected_revision"`
		Format           string `json:"format"`
		Content          string `json:"content"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.CommitConfig(request.Context(), mysqlstore.ConfigCommit{
		OperationID:      request.Header.Get("Idempotency-Key"),
		Actor:            mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		EnvironmentKey:   request.PathValue("environment"),
		ConfigKey:        request.PathValue("config"),
		ConfigName:       body.Name,
		ExpectedRevision: body.ExpectedRevision,
		Format:           configdoc.Format(body.Format),
		Content:          []byte(body.Content),
	})
	writeMutationResult(response, result, err)
}

func (server *server) validateConfig(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	var body struct {
		Environment string `json:"environment"`
		Format      string `json:"format"`
		Content     string `json:"content"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.ValidateConfig(request.Context(), mysqlstore.ConfigValidation{
		EnvironmentKey: body.Environment,
		Format:         configdoc.Format(body.Format),
		Content:        []byte(body.Content),
	})
	writeMutationResult(response, result, err)
}

func (server *server) readResolvedConfigPreview(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	environment := request.PathValue("environment")
	config := request.PathValue("config")
	resolved, err := server.configs.ReadResolvedConfig(request.Context(), environment, config, "")
	if err != nil {
		writeReadError(response, err)
		return
	}
	if !writeJSONResponse(response, resolved) || server.publisher == nil {
		return
	}
	server.publisher.TryPublish(machine.AccessEvent{
		Time:           time.Now().UTC(),
		Principal:      principal.ActorID(),
		Authentication: machine.AuthenticationOIDC,
		Environment:    environment,
		ResourceType:   "config",
		Resource:       config,
		ConfigRevision: resolved.ConfigRevision,
		VaultRevisions: maps.Clone(resolved.VaultRevisions),
	})
}

func (server *server) mergeConfig(response http.ResponseWriter, request *http.Request) {
	server.transferConfig(response, request, true)
}

func (server *server) previewConfigTransfer(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	var body struct {
		Mode              string `json:"mode"`
		SourceEnvironment string `json:"source_environment"`
		SourceConfig      string `json:"source_config"`
		SourceRevision    uint64 `json:"source_revision"`
		TargetRevision    uint64 `json:"target_revision"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	if (body.Mode != "merge" && body.Mode != "replace") || body.SourceEnvironment == "" || body.SourceConfig == "" || body.SourceRevision == 0 || body.TargetRevision == 0 {
		writeError(response, http.StatusUnprocessableEntity, "validation_failed")
		return
	}
	source, err := server.configs.ReadRawConfigRevision(request.Context(), body.SourceEnvironment, body.SourceConfig, body.SourceRevision)
	if err != nil {
		writeReadError(response, err)
		return
	}
	target, err := server.configs.ReadRawConfigRevision(request.Context(), request.PathValue("environment"), request.PathValue("config"), body.TargetRevision)
	if err != nil {
		writeReadError(response, err)
		return
	}
	currentTarget, err := server.configs.ReadRawConfig(request.Context(), request.PathValue("environment"), request.PathValue("config"))
	if err != nil {
		writeReadError(response, err)
		return
	}
	if body.Mode == "merge" && source.Format != target.Format {
		writeError(response, http.StatusUnprocessableEntity, "format_mismatch")
		return
	}
	canonicalSource, err := configdoc.Canonicalize(configdoc.Format(source.Format), []byte(source.Content))
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "validation_failed")
		return
	}
	content := string(canonicalSource.Content)
	if body.Mode == "merge" {
		merged, err := configdoc.Merge(configdoc.Format(source.Format), canonicalSource.Content, []byte(target.Content))
		if err != nil {
			writeError(response, http.StatusUnprocessableEntity, "validation_failed")
			return
		}
		content = string(merged.Content)
	}
	writeJSON(response, struct {
		Mode                   string `json:"mode"`
		Format                 string `json:"format"`
		Content                string `json:"content"`
		SourceRevision         uint64 `json:"source_revision"`
		TargetRevision         uint64 `json:"target_revision"`
		ExpectedTargetRevision uint64 `json:"expected_target_revision"`
	}{body.Mode, source.Format, content, source.Revision, target.Revision, currentTarget.Revision})
}

func (server *server) replaceConfig(response http.ResponseWriter, request *http.Request) {
	server.transferConfig(response, request, false)
}

func (server *server) transferConfig(response http.ResponseWriter, request *http.Request, merge bool) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		SourceEnvironment      string `json:"source_environment"`
		SourceConfig           string `json:"source_config"`
		SourceRevision         uint64 `json:"source_revision"`
		TargetRevision         uint64 `json:"target_revision"`
		ExpectedTargetRevision uint64 `json:"expected_target_revision"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	transfer := mysqlstore.ConfigTransfer{
		OperationID:            request.Header.Get("Idempotency-Key"),
		Actor:                  mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		SourceEnvironmentKey:   body.SourceEnvironment,
		SourceConfigKey:        body.SourceConfig,
		SourceRevision:         body.SourceRevision,
		TargetEnvironmentKey:   request.PathValue("environment"),
		TargetConfigKey:        request.PathValue("config"),
		TargetRevision:         body.TargetRevision,
		ExpectedTargetRevision: body.ExpectedTargetRevision,
	}
	var result mysqlstore.ConfigCommitResult
	var err error
	if merge {
		result, err = server.configs.MergeConfig(request.Context(), transfer)
	} else {
		result, err = server.configs.ReplaceConfig(request.Context(), transfer)
	}
	writeMutationResult(response, result, err)
}

func (server *server) restoreConfig(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		SourceRevision   uint64 `json:"source_revision"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.RestoreConfig(request.Context(), mysqlstore.ConfigRestore{
		OperationID:      request.Header.Get("Idempotency-Key"),
		Actor:            mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		EnvironmentKey:   request.PathValue("environment"),
		ConfigKey:        request.PathValue("config"),
		SourceRevision:   body.SourceRevision,
		ExpectedRevision: body.ExpectedRevision,
	})
	writeMutationResult(response, result, err)
}

func (server *server) cloneConfig(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		TargetEnvironment string `json:"target_environment"`
		TargetConfig      string `json:"target_config"`
		TargetName        string `json:"target_name"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.CloneConfig(request.Context(), mysqlstore.ConfigClone{
		OperationID:          request.Header.Get("Idempotency-Key"),
		Actor:                mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		SourceEnvironmentKey: request.PathValue("environment"),
		SourceConfigKey:      request.PathValue("config"),
		TargetEnvironmentKey: body.TargetEnvironment,
		TargetConfigKey:      body.TargetConfig,
		TargetConfigName:     body.TargetName,
	})
	writeMutationResult(response, result, err)
}

func (server *server) archiveConfig(response http.ResponseWriter, request *http.Request) {
	server.changeConfigLifecycle(response, request, mysqlstore.ConfigArchive)
}

func (server *server) unarchiveConfig(response http.ResponseWriter, request *http.Request) {
	server.changeConfigLifecycle(response, request, mysqlstore.ConfigUnarchive)
}

func (server *server) changeConfigLifecycle(response http.ResponseWriter, request *http.Request, action mysqlstore.ConfigLifecycleAction) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.ApplyConfigLifecycleChange(request.Context(), mysqlstore.ConfigLifecycleChange{
		OperationID: request.Header.Get("Idempotency-Key"),
		Actor:       mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		Action:      action,
		Key:         request.PathValue("config"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) commitVaultItem(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		DisplayName      string            `json:"display_name"`
		ExpectedRevision uint64            `json:"expected_revision"`
		Snapshot         vaultdoc.Snapshot `json:"snapshot"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.CommitVault(request.Context(), mysqlstore.VaultCommit{
		OperationID:      request.Header.Get("Idempotency-Key"),
		Actor:            mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		NamespaceKey:     request.PathValue("namespace"),
		ItemKey:          request.PathValue("item"),
		ItemName:         body.DisplayName,
		ExpectedRevision: body.ExpectedRevision,
		Snapshot:         body.Snapshot,
	})
	writeMutationResult(response, result, err)
}

func (server *server) restoreVaultItem(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		SourceRevision   uint64 `json:"source_revision"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.RestoreVault(request.Context(), mysqlstore.VaultRestore{
		OperationID:      request.Header.Get("Idempotency-Key"),
		Actor:            mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		NamespaceKey:     request.PathValue("namespace"),
		ItemKey:          request.PathValue("item"),
		SourceRevision:   body.SourceRevision,
		ExpectedRevision: body.ExpectedRevision,
	})
	writeMutationResult(response, result, err)
}

func (server *server) archiveVaultItem(response http.ResponseWriter, request *http.Request) {
	server.changeVaultLifecycle(response, request, mysqlstore.VaultArchive)
}

func (server *server) unarchiveVaultItem(response http.ResponseWriter, request *http.Request) {
	server.changeVaultLifecycle(response, request, mysqlstore.VaultUnarchive)
}

func (server *server) changeVaultLifecycle(response http.ResponseWriter, request *http.Request, action mysqlstore.VaultLifecycleAction) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.ApplyVaultLifecycleChange(request.Context(), mysqlstore.VaultLifecycleChange{
		OperationID:  request.Header.Get("Idempotency-Key"),
		Actor:        mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		Action:       action,
		NamespaceKey: request.PathValue("namespace"),
		Key:          request.PathValue("item"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) listAccess(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	limit, ok := parseLimit(response, request, 100, 500)
	if !ok {
		return
	}
	if server.logs == nil {
		writeError(response, http.StatusServiceUnavailable, "log_store_unavailable")
		return
	}
	records, err := server.logs.ListAccess(request.Context(), limit)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "log_store_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []logstore.AccessRecord `json:"items"`
	}{records})
}

func (server *server) listAudits(response http.ResponseWriter, request *http.Request) {
	if _, ok := requirePrincipal(response, request); !ok {
		return
	}
	query, ok := parseAuditQuery(response, request)
	if !ok {
		return
	}
	if server.logs == nil {
		writeError(response, http.StatusServiceUnavailable, "log_store_unavailable")
		return
	}
	page, err := server.logs.ListAudits(request.Context(), query)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "log_store_unavailable")
		return
	}
	writeJSON(response, page)
}

func (server *server) listTokens(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	includeRevoked, ok := parseBooleanQuery(response, request, "include_revoked")
	if !ok {
		return
	}
	tokens, err := server.configs.ListTokens(request.Context(), includeRevoked)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.TokenSummary `json:"items"`
	}{tokens})
}

func (server *server) createToken(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		DisplayName      string     `json:"display_name"`
		EnvironmentKeys  []string   `json:"environment_keys"`
		AllowWithoutMTLS bool       `json:"allow_without_mtls"`
		ExpiresAt        *time.Time `json:"expires_at"`
		NeverExpires     bool       `json:"never_expires"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	var expiresAt time.Time
	if body.ExpiresAt != nil {
		expiresAt = body.ExpiresAt.UTC()
	}
	result, err := server.configs.CreateToken(request.Context(), mysqlstore.TokenCreate{
		OperationID:      request.Header.Get("Idempotency-Key"),
		Actor:            mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		DisplayName:      body.DisplayName,
		EnvironmentKeys:  body.EnvironmentKeys,
		AllowWithoutMTLS: body.AllowWithoutMTLS,
		ExpiresAt:        expiresAt,
		NeverExpires:     body.NeverExpires,
	})
	writeMutationResult(response, result, err)
}

func (server *server) setTokenEnvironments(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		EnvironmentKeys []string `json:"environment_keys"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.SetTokenEnvironments(request.Context(), mysqlstore.TokenEnvironmentChange{
		OperationID:     request.Header.Get("Idempotency-Key"),
		Actor:           mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		PublicID:        request.PathValue("public_id"),
		EnvironmentKeys: body.EnvironmentKeys,
	})
	writeMutationResult(response, result, err)
}

func (server *server) revokeToken(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.RevokeToken(request.Context(), mysqlstore.TokenRevoke{
		OperationID: request.Header.Get("Idempotency-Key"),
		Actor:       mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		PublicID:    request.PathValue("public_id"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) listClientCertificates(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	includeRevoked, ok := parseBooleanQuery(response, request, "include_revoked")
	if !ok {
		return
	}
	certificates, err := server.configs.ListClientCertificates(request.Context(), includeRevoked)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.ClientCertificate `json:"items"`
	}{certificates})
}

func (server *server) registerClientCertificate(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		DisplayName    string `json:"display_name"`
		CertificatePEM string `json:"certificate_pem"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	roots, err := server.clientTrust(request.Context())
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	if len(roots.Subjects()) == 0 {
		writeError(response, http.StatusServiceUnavailable, "client_ca_unavailable")
		return
	}
	certificate, err := clientcert.ParseAndVerify([]byte(body.CertificatePEM), roots, time.Now())
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "validation_failed")
		return
	}
	result, err := server.configs.RegisterVerifiedClientCertificate(request.Context(), mysqlstore.ClientCertificateRegister{
		OperationID: request.Header.Get("Idempotency-Key"),
		Actor:       mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		DisplayName: body.DisplayName,
		Certificate: certificate,
	})
	writeMutationResult(response, result, err)
}

func (server *server) revokeClientCertificate(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.RevokeClientCertificate(request.Context(), mysqlstore.ClientCertificateRevoke{
		OperationID:       request.Header.Get("Idempotency-Key"),
		Actor:             mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		FingerprintSHA256: request.PathValue("fingerprint"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) listNotificationDestinations(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	includeArchived, ok := parseIncludeArchived(response, request)
	if !ok {
		return
	}
	destinations, err := server.configs.ListNotificationDestinations(request.Context(), includeArchived)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.NotificationDestination `json:"items"`
	}{destinations})
}

func (server *server) commitNotificationDestination(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		DisplayName string                          `json:"display_name"`
		Provider    mysqlstore.NotificationProvider `json:"provider"`
		URL         *string                         `json:"url"`
		Secret      *string                         `json:"secret"`
		Enabled     bool                            `json:"enabled"`
		EventTypes  []string                        `json:"event_types"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	if body.URL != nil && notification.ValidateDestinationURL(body.Provider, *body.URL) != nil {
		writeError(response, http.StatusUnprocessableEntity, "validation_failed")
		return
	}
	result, err := server.configs.CommitNotificationDestination(request.Context(), mysqlstore.NotificationDestinationCommit{
		OperationID: request.Header.Get("Idempotency-Key"),
		Actor:       mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		Key:         request.PathValue("destination"),
		DisplayName: body.DisplayName,
		Provider:    body.Provider,
		URL:         body.URL,
		Secret:      body.Secret,
		Enabled:     body.Enabled,
		EventTypes:  body.EventTypes,
	})
	writeMutationResult(response, result, err)
}

func (server *server) archiveNotificationDestination(response http.ResponseWriter, request *http.Request) {
	server.changeNotificationDestinationLifecycle(response, request, mysqlstore.NotificationDestinationArchive)
}

func (server *server) unarchiveNotificationDestination(response http.ResponseWriter, request *http.Request) {
	server.changeNotificationDestinationLifecycle(response, request, mysqlstore.NotificationDestinationUnarchive)
}

func (server *server) changeNotificationDestinationLifecycle(
	response http.ResponseWriter,
	request *http.Request,
	action mysqlstore.NotificationDestinationLifecycleAction,
) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.ApplyNotificationDestinationLifecycle(request.Context(), mysqlstore.NotificationDestinationLifecycle{
		OperationID: request.Header.Get("Idempotency-Key"),
		Actor:       mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		Action:      action,
		Key:         request.PathValue("destination"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) testNotificationDestination(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.QueueNotificationTest(request.Context(), mysqlstore.NotificationTestRequest{
		OperationID:    request.Header.Get("Idempotency-Key"),
		Actor:          mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		DestinationKey: request.PathValue("destination"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) listNotificationDeliveries(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	limit, ok := parseLimit(response, request, 100, 1000)
	if !ok {
		return
	}
	deliveries, err := server.configs.ListNotificationDeliveries(
		request.Context(), request.PathValue("destination"), limit,
	)
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.NotificationDelivery `json:"items"`
	}{deliveries})
}

func (server *server) redeliverNotification(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.RedeliverNotification(request.Context(), mysqlstore.NotificationRedeliveryRequest{
		OperationID:    request.Header.Get("Idempotency-Key"),
		Actor:          mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		DestinationKey: request.PathValue("destination"),
		DeliveryID:     request.PathValue("delivery"),
	})
	writeMutationResult(response, result, err)
}

func requireAdmin(response http.ResponseWriter, request *http.Request) (humanauth.Principal, bool) {
	principal, ok := requirePrincipal(response, request)
	if !ok {
		return humanauth.Principal{}, false
	}
	if principal.Role != humanauth.RoleAdmin {
		writeError(response, http.StatusForbidden, "forbidden")
		return humanauth.Principal{}, false
	}
	return principal, true
}

func requirePrincipal(response http.ResponseWriter, request *http.Request) (humanauth.Principal, bool) {
	principal, ok := humanauth.PrincipalFromContext(request.Context())
	if !ok {
		writeError(response, http.StatusUnauthorized, "unauthenticated")
		return humanauth.Principal{}, false
	}
	return principal, true
}

func decodeJSON(response http.ResponseWriter, request *http.Request, destination any) bool {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return false
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxManagementRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(response, http.StatusRequestEntityTooLarge, "request_too_large")
			return false
		}
		writeError(response, http.StatusBadRequest, "invalid_request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func parseIncludeArchived(response http.ResponseWriter, request *http.Request) (bool, bool) {
	return parseBooleanQuery(response, request, "include_archived")
}

func parseBooleanQuery(response http.ResponseWriter, request *http.Request, name string) (bool, bool) {
	query := request.URL.Query()
	if len(query) > 1 || (len(query) == 1 && !query.Has(name)) || len(query[name]) > 1 {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return false, false
	}
	switch query.Get(name) {
	case "", "false":
		return false, true
	case "true":
		return true, true
	default:
		writeError(response, http.StatusBadRequest, "invalid_request")
		return false, false
	}
}

func parseLimit(response http.ResponseWriter, request *http.Request, fallback, maximum int) (int, bool) {
	query := request.URL.Query()
	if len(query) == 0 {
		return fallback, true
	}
	if len(query) != 1 || !query.Has("limit") || len(query["limit"]) != 1 {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit < 1 || limit > maximum {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return 0, false
	}
	return limit, true
}

func parseAuditQuery(response http.ResponseWriter, request *http.Request) (logstore.AuditQuery, bool) {
	values := request.URL.Query()
	for name, entries := range values {
		if (name != "limit" && name != "offset" && name != "q") || len(entries) != 1 {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return logstore.AuditQuery{}, false
		}
	}
	query := logstore.AuditQuery{Limit: 100, Search: strings.TrimSpace(values.Get("q"))}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return logstore.AuditQuery{}, false
		}
		query.Limit = limit
	}
	if raw := values.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return logstore.AuditQuery{}, false
		}
		query.Offset = offset
	}
	if len(query.Search) > 256 {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return logstore.AuditQuery{}, false
	}
	return query, true
}

func writeReadError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mysqlstore.ErrNotFound), errors.Is(err, machine.ErrNotFound):
		writeError(response, http.StatusNotFound, "not_found")
	case errors.Is(err, machine.ErrUnresolved):
		writeError(response, http.StatusUnprocessableEntity, "unresolved_vault_reference")
	case errors.Is(err, vaultcrypto.ErrIntegrity), errors.Is(err, machine.ErrIntegrity):
		writeError(response, http.StatusInternalServerError, "crypto_integrity_failure")
	default:
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
	}
}

func writeMutationResult(response http.ResponseWriter, result any, err error) {
	if err != nil {
		switch {
		case errors.Is(err, mysqlstore.ErrOperationReuse):
			writeError(response, http.StatusConflict, "operation_id_reused")
		case errors.Is(err, mysqlstore.ErrConflict):
			writeError(response, http.StatusConflict, "revision_conflict")
		case errors.Is(err, mysqlstore.ErrValidation):
			writeError(response, http.StatusUnprocessableEntity, "validation_failed")
		default:
			writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		}
		return
	}
	writeJSON(response, result)
}

func writeJSON(response http.ResponseWriter, value any) {
	_ = writeJSONResponse(response, value)
}

func writeJSONResponse(response http.ResponseWriter, value any) bool {
	payload, err := json.Marshal(value)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "internal_error")
		return false
	}
	payload = append(payload, '\n')
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	written, err := response.Write(payload)
	return err == nil && written == len(payload)
}

func writeError(response http.ResponseWriter, status int, code string) {
	requestID := make([]byte, 12)
	_, _ = rand.Read(requestID)
	body := struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}{}
	body.Error.Code = code
	body.Error.RequestID = hex.EncodeToString(requestID)
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}
