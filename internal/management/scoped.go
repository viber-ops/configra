package management

import (
	"context"
	"net/http"
	"time"

	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

// Only these routes are writable on the API listener. Human management routes
// and their OIDC/session middleware are never mounted here.
func NewMachineHandler(store *mysqlstore.Store, publisher machine.AccessPublisher) http.Handler {
	server := &server{configs: store, publisher: publisher}
	reads := machine.NewHandler(store, publisher)
	mux := http.NewServeMux()
	mux.Handle("GET /v1/environments/{environment}/configs/{config}", reads)
	mux.Handle("GET /v1/environments/{environment}/vault-items/{namespace}/{item}/fields/{field}/content", reads)
	protect := func(action, resourceType, parameter string, mutation bool, next http.HandlerFunc) http.HandlerFunc {
		return func(response http.ResponseWriter, request *http.Request) {
			request = request.WithContext(mysqlstore.WithAuditSourceIP(request.Context(), request.RemoteAddr))
			environment := request.PathValue("environment")
			token, _, status := machine.Authenticate(store, request, environment)
			if mutation && token.PublicID != "" {
				metadata := mysqlstore.RejectedMutation{RequestID: newErrorRequestID(), Actor: mysqlstore.Actor{Type: "token", ID: token.PublicID}, Action: action, ResourceType: resourceType}
				if auditResourceKey(environment) {
					metadata.EnvironmentKey = environment
				}
				if key := request.PathValue("field"); auditResourceKey(key) {
					metadata.FieldKey = key
				}
				if key := request.PathValue("namespace"); auditResourceKey(key) {
					metadata.NamespaceKey = key
				}
				if key := request.PathValue(parameter); validScopedAuditIdentity(resourceType, key) {
					metadata.ResourceKey = key
				}
				writer := &auditResponseWriter{ResponseWriter: response, requestID: metadata.RequestID}
				response.Header().Set("X-Request-ID", metadata.RequestID)
				writer.persist = func(code string) error {
					metadata.ErrorCode = code
					ctx, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), 3*time.Second)
					defer cancel()
					return store.RecordRejectedMutation(ctx, metadata)
				}
				response = writer
			}
			if status != 0 {
				writeError(response, status, scopedStatusCode(status))
				return
			}
			if token.Kind != machine.TokenWriteScoped || !auditResourceKey(environment) ||
				(request.PathValue("config") != "" && !token.AllowsConfig(request.PathValue("config"))) ||
				(request.PathValue("namespace") != "" && !token.AllowsNamespace(request.PathValue("namespace"))) {
				writeError(response, http.StatusForbidden, "scope_forbidden")
				return
			}
			for _, name := range []string{"config", "namespace", "item", "field"} {
				if key := request.PathValue(name); key != "" && !auditResourceKey(key) {
					writeError(response, http.StatusBadRequest, "invalid_resource_key")
					return
				}
			}
			request = request.WithContext(machine.WithToken(request.Context(), token))
			next(response, request)
		}
	}
	mux.HandleFunc("PUT /v1/environments/{environment}/configs/{config}", protect("config.commit", "config", "config", true, server.commitConfig))
	mux.HandleFunc("GET /v1/environments/{environment}/configs/{config}/raw", protect("config.read", "config", "config", false, func(response http.ResponseWriter, request *http.Request) {
		result, err := store.ReadRawConfig(request.Context(), request.PathValue("environment"), request.PathValue("config"))
		if err != nil {
			writeReadError(response, err)
			return
		}
		if writeJSONResponse(response, result) && publisher != nil {
			token, _ := machine.TokenFromContext(request.Context())
			publisher.TryPublish(machine.AccessEvent{Time: time.Now().UTC(), Principal: token.PublicID, Authentication: machine.AuthenticationMTLS, Environment: result.EnvironmentKey, ResourceType: "config", Resource: result.ConfigKey, ConfigRevision: result.Revision})
		}
	}))
	itemPath := "/v1/environments/{environment}/vault-items/{namespace}/{item}"
	mux.HandleFunc("GET "+itemPath, protect("vault.read", "vault_item", "item", false, func(response http.ResponseWriter, request *http.Request) {
		result, err := store.ReadScopedVault(request.Context(), request.PathValue("environment"), request.PathValue("namespace"), request.PathValue("item"))
		if err != nil {
			writeReadError(response, err)
			return
		}
		if writeJSONResponse(response, result) && publisher != nil {
			token, _ := machine.TokenFromContext(request.Context())
			publisher.TryPublish(machine.AccessEvent{Time: time.Now().UTC(), Principal: token.PublicID, Authentication: machine.AuthenticationMTLS, Environment: request.PathValue("environment"), Namespace: result.NamespaceKey, ResourceType: "vault_item", Resource: result.Key, VaultRevisions: map[string]uint64{result.NamespaceKey + "." + result.Key: result.Revision}})
		}
	}))
	for _, route := range []struct{ method, suffix, action string }{
		{"PUT", "", "put_item"}, {"DELETE", "", "delete_item"},
		{"PUT", "/fields/{field}", "put_field"}, {"DELETE", "/fields/{field}", "delete_field"},
	} {
		mux.HandleFunc(route.method+" "+itemPath+route.suffix, protect("vault."+route.action, "vault_item", "item", true, func(response http.ResponseWriter, request *http.Request) {
			var body struct {
				DisplayName      string                    `json:"display_name"`
				ExpectedRevision uint64                    `json:"expected_revision"`
				Fields           []vaultdoc.Field          `json:"fields"`
				Values           map[string]vaultdoc.Value `json:"values"`
				Name             string                    `json:"name"`
				Type             vaultdoc.FieldType        `json:"type"`
				Value            vaultdoc.Value            `json:"value"`
			}
			if !decodeJSON(response, request, &body) {
				return
			}
			token, _ := machine.TokenFromContext(request.Context())
			result, err := store.WriteScopedVault(request.Context(), mysqlstore.ScopedVaultWrite{
				OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "token", ID: token.PublicID},
				EnvironmentKey: request.PathValue("environment"), NamespaceKey: request.PathValue("namespace"), ItemKey: request.PathValue("item"),
				ItemName: body.DisplayName, ExpectedRevision: body.ExpectedRevision, Action: route.action, Fields: body.Fields, Values: body.Values,
				Field: vaultdoc.Field{Key: request.PathValue("field"), Name: body.Name, Type: body.Type}, Value: body.Value,
			})
			writeMutationResult(response, result, err)
		}))
	}
	mux.HandleFunc("POST /v1/environments/{environment}/deployment-credentials", protect("deployment.issue", "api_token", "public_id", true, func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			DisplayName string    `json:"display_name"`
			AuthorityID string    `json:"authority_id"`
			ExpiresAt   time.Time `json:"expires_at"`
		}
		if !decodeJSON(response, request, &body) {
			return
		}
		token, _ := machine.TokenFromContext(request.Context())
		result, err := store.IssueDeploymentCredential(request.Context(), mysqlstore.DeploymentCredentialIssue{
			OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "token", ID: token.PublicID},
			EnvironmentKey: request.PathValue("environment"), DisplayName: body.DisplayName, AuthorityID: body.AuthorityID, ExpiresAt: body.ExpiresAt,
		})
		writeMutationResult(response, result, err)
	}))
	mux.HandleFunc("POST /v1/environments/{environment}/deployment-credentials/{public_id}/revoke", protect("deployment.revoke", "api_token", "public_id", true, func(response http.ResponseWriter, request *http.Request) {
		token, _ := machine.TokenFromContext(request.Context())
		result, err := store.RevokeDeploymentCredential(request.Context(), mysqlstore.DeploymentCredentialRevoke{
			OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "token", ID: token.PublicID},
			EnvironmentKey: request.PathValue("environment"), PublicID: request.PathValue("public_id"),
		})
		writeMutationResult(response, result, err)
	}))
	mux.HandleFunc("/", protect("api.request", "machine_request", "", true, func(response http.ResponseWriter, _ *http.Request) {
		writeError(response, http.StatusForbidden, "scope_forbidden")
	}))
	return mux
}

func scopedStatusCode(status int) string {
	if status == http.StatusForbidden {
		return "scope_forbidden"
	}
	if status == http.StatusServiceUnavailable {
		return "service_unavailable"
	}
	return "unauthorized"
}

func validScopedAuditIdentity(kind, key string) bool {
	if kind == "api_token" {
		return len(key) == 16 && isLowerHex(key)
	}
	return auditResourceKey(key)
}

func isLowerHex(value string) bool {
	for _, c := range value {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func mutationActor(response http.ResponseWriter, request *http.Request) (mysqlstore.Actor, bool) {
	if token, ok := machine.TokenFromContext(request.Context()); ok && token.Kind == machine.TokenWriteScoped {
		return mysqlstore.Actor{Type: "token", ID: token.PublicID}, true
	}
	principal, ok := requireAdmin(response, request)
	return mysqlstore.Actor{Type: "user", ID: principal.ActorID()}, ok
}
