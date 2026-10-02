package management

import (
	"net/http"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func (server *server) changeSessions(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	if !principal.MFAVerified {
		writeError(response, http.StatusForbidden, "mfa_required")
		return
	}
	var body struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
		Action  string `json:"action"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.ChangeSessions(request.Context(), mysqlstore.SessionChange{OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "user", ID: principal.ActorID()}, Issuer: body.Issuer, Subject: body.Subject, Action: body.Action})
	writeMutationResult(response, result, err)
}

func (server *server) listSessionPolicies(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	query, _, ok := parseInventoryQuery(response, request, "")
	if !ok {
		return
	}
	items, err := server.configs.ListSessionPolicies(request.Context(), query)
	if err != nil {
		writeReadError(response, err)
		return
	}
	writeJSON(response, items)
}
