package machine

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (server *server) readRelease(response http.ResponseWriter, request *http.Request) {
	environment, config, key := request.PathValue("environment"), request.PathValue("config"), request.PathValue("release")
	if !validResourceKey(environment) || !validResourceKey(config) || (key != "" && !validResourceKey(key)) {
		writeError(response, 400, "invalid_resource_key")
		return
	}
	token, authentication, status := server.authenticate(request, environment)
	if status != 0 {
		writeError(response, status, statusCode(status))
		return
	}
	if !token.AllowsConfig(config) {
		writeError(response, 403, "scope_forbidden")
		return
	}
	ctx := WithToken(request.Context(), token)
	var value any
	var err error
	var bundle ReleaseBundle
	if strings.HasSuffix(request.Pattern, "/release-state") {
		value, err = server.repository.ReadReleaseState(ctx, environment, config)
	} else {
		bundle, err = server.repository.ReadRelease(ctx, environment, config, key, request.Header.Get("If-None-Match"))
		value = bundle
	}
	response.Header().Set("Cache-Control", "no-store")
	if bundle.ETag != "" {
		response.Header().Set("ETag", bundle.ETag)
	}
	switch {
	case errors.Is(err, ErrNotModified):
		response.WriteHeader(http.StatusNotModified)
		return
	case errors.Is(err, ErrForbidden):
		writeError(response, 403, "scope_forbidden")
		return
	case errors.Is(err, ErrNotFound):
		writeError(response, 404, "not_found")
		return
	case errors.Is(err, ErrUnresolved):
		writeError(response, 422, "unresolved_vault_reference")
		return
	case errors.Is(err, ErrIntegrity):
		writeError(response, 500, "crypto_integrity_failure")
		return
	case err != nil:
		writeError(response, 503, "service_unavailable")
		return
	}
	response.Header().Set("Content-Type", "application/json")
	if json.NewEncoder(response).Encode(value) == nil && bundle.Config.ConfigRevision > 0 {
		server.publisher.TryPublish(AccessEvent{Time: server.now().UTC(), Principal: token.PublicID, Authentication: authentication, Environment: environment, ResourceType: "release", Resource: config + "." + bundle.Manifest.ReleaseKey, ConfigRevision: bundle.Config.ConfigRevision, VaultRevisions: bundle.Manifest.VaultRevisions})
	}
}
