package management

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

type mutationAuditRoute struct{ action, resourceType, resourceParameter string }

var mutationAuditRoutes = map[string]mutationAuditRoute{
	"POST /v1/environments":                                                            {"environment.create", "environment", "environment"},
	"PATCH /v1/environments/{environment}":                                             {"environment.rename", "environment", "environment"},
	"POST /v1/environments/{environment}/archive":                                      {"environment.archive", "environment", "environment"},
	"POST /v1/environments/{environment}/unarchive":                                    {"environment.unarchive", "environment", "environment"},
	"PUT /v1/environments/{environment}/configs/{config}":                              {"config.commit", "config", "config"},
	"POST /v1/environments/{environment}/configs/{config}/merge":                       {"config.merge", "config", "config"},
	"POST /v1/environments/{environment}/configs/{config}/replace":                     {"config.replace", "config", "config"},
	"POST /v1/environments/{environment}/configs/{config}/restore":                     {"config.restore", "config", "config"},
	"POST /v1/environments/{environment}/configs/{config}/clone":                       {"config.clone", "config", "config"},
	"POST /v1/configs/{config}/archive":                                                {"config.archive", "config", "config"},
	"POST /v1/configs/{config}/unarchive":                                              {"config.unarchive", "config", "config"},
	"PUT /v1/vault-items/{namespace}/{item}":                                           {"vault.commit", "vault_item", "item"},
	"POST /v1/vault-items/{namespace}/{item}/restore":                                  {"vault.restore", "vault_item", "item"},
	"POST /v1/vault-items/{namespace}/{item}/archive":                                  {"vault.archive", "vault_item", "item"},
	"POST /v1/vault-items/{namespace}/{item}/unarchive":                                {"vault.unarchive", "vault_item", "item"},
	"POST /v1/api-tokens":                                                              {"token.create", "api_token", "public_id"},
	"PUT /v1/api-tokens/{public_id}/environments":                                      {"token.set_environments", "api_token", "public_id"},
	"POST /v1/api-tokens/{public_id}/revoke":                                           {"token.revoke", "api_token", "public_id"},
	"POST /v1/client-certificates":                                                     {"client_certificate.register", "client_certificate", "fingerprint"},
	"POST /v1/client-certificates/issue":                                               {"client_certificate.issue", "client_certificate", "fingerprint"},
	"POST /v1/client-certificates/{fingerprint}/revoke":                                {"client_certificate.revoke", "client_certificate", "fingerprint"},
	"POST /v1/certificate-authorities":                                                 {"certificate_authority.create", "certificate_authority", "authority"},
	"POST /v1/certificate-authorities/{authority}/revoke":                              {"certificate_authority.revoke", "certificate_authority", "authority"},
	"PUT /v1/notification-destinations/{destination}":                                  {"notification_destination.commit", "notification_destination", "destination"},
	"POST /v1/notification-destinations/{destination}/archive":                         {"notification_destination.archive", "notification_destination", "destination"},
	"POST /v1/notification-destinations/{destination}/unarchive":                       {"notification_destination.unarchive", "notification_destination", "destination"},
	"POST /v1/notification-destinations/{destination}/test":                            {"notification_destination.test", "notification_destination", "destination"},
	"POST /v1/notification-destinations/{destination}/deliveries/{delivery}/redeliver": {"notification_destination.redeliver", "notification_destination", "destination"},
}

// Rejections are recorded before their response is sent. No body, query string,
// caller operation key, or raw URL is captured. The trusted route identifies the
// action; only validated route parameters can identify a resource.
func (server *server) auditRejections(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		principal, authenticated := humanauth.PrincipalFromContext(request.Context())
		if !authenticated || !strings.HasPrefix(request.URL.Path, "/v1/") ||
			(request.Method != http.MethodPost && request.Method != http.MethodPut && request.Method != http.MethodPatch && request.Method != http.MethodDelete) {
			next.ServeHTTP(response, request)
			return
		}
		_, pattern := mux.Handler(request)
		if pattern == "POST /v1/configs/validate" || pattern == "POST /v1/environments/{environment}/configs/{config}/transfer-preview" {
			next.ServeHTTP(response, request)
			return
		}
		metadata := rejectedMetadata(pattern, request)
		metadata.Actor = mysqlstore.Actor{Type: "user", ID: principal.ActorID()}
		metadata.RequestID = newErrorRequestID()
		response.Header().Set("X-Request-ID", metadata.RequestID)
		writer := &auditResponseWriter{ResponseWriter: response, requestID: metadata.RequestID}
		writer.persist = func(code string) error {
			metadata.ErrorCode = code
			ctx, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), 3*time.Second)
			defer cancel()
			return server.configs.RecordRejectedMutation(ctx, metadata)
		}
		next.ServeHTTP(writer, request)
	})
}

func rejectedMetadata(pattern string, request *http.Request) mysqlstore.RejectedMutation {
	route, known := mutationAuditRoutes[pattern]
	if !known {
		return mysqlstore.RejectedMutation{Action: "management.request", ResourceType: "management_request"}
	}
	metadata := mysqlstore.RejectedMutation{Action: route.action, ResourceType: route.resourceType}
	_, pathPattern, _ := strings.Cut(pattern, " ")
	parts, values := strings.Split(pathPattern, "/"), strings.Split(request.URL.EscapedPath(), "/")
	if len(parts) != len(values) {
		return metadata
	}
	for index, part := range parts {
		if !strings.HasPrefix(part, "{") || !strings.HasSuffix(part, "}") {
			continue
		}
		name := strings.Trim(part, "{}")
		value, err := url.PathUnescape(values[index])
		if err != nil {
			continue
		}
		if name == "environment" && auditResourceKey(value) {
			metadata.EnvironmentKey = value
		}
		if name == "namespace" && auditResourceKey(value) {
			metadata.NamespaceKey = value
		}
		if name != route.resourceParameter {
			continue
		}
		valid := auditResourceKey(value)
		if name == "fingerprint" || name == "authority" || name == "public_id" {
			length := map[string]int{"fingerprint": 64, "authority": 32, "public_id": 16}[name]
			_, err := hex.DecodeString(value)
			valid = err == nil && len(value) == length && value == strings.ToLower(value)
		}
		if valid {
			metadata.ResourceKey = value
		}
	}
	return metadata
}

func auditResourceKey(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, c := range value {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

type auditResponseWriter struct {
	http.ResponseWriter
	requestID string
	errorCode string
	committed bool
	written   bool
	failed    bool
	persist   func(string) error
}

func (writer *auditResponseWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }
func (writer *auditResponseWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		writer.ResponseWriter.WriteHeader(status)
		return
	}
	if writer.written {
		return
	}
	writer.written = true
	if status >= 400 && !writer.committed {
		code := writer.errorCode
		if code == "" {
			switch status {
			case 404:
				code = "not_found"
			case 405:
				code = "method_not_allowed"
			default:
				code = "request_rejected"
			}
		}
		if err := writer.persist(code); err != nil {
			writer.failed = true
			writer.ResponseWriter.Header().Set("Cache-Control", "no-store")
			writer.ResponseWriter.Header().Set("Content-Type", "application/json")
			writer.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer.ResponseWriter).Encode(map[string]any{"error": map[string]string{"code": "audit_unavailable", "request_id": writer.requestID}})
			return
		}
	}
	writer.ResponseWriter.WriteHeader(status)
}
func (writer *auditResponseWriter) Write(data []byte) (int, error) {
	if !writer.written {
		writer.WriteHeader(http.StatusOK)
	}
	if writer.failed {
		return len(data), nil
	}
	return writer.ResponseWriter.Write(data)
}

func newErrorRequestID() string {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	return hex.EncodeToString(id)
}
