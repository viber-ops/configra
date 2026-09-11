package management

import (
	"context"
	"crypto/x509"
	"net/http"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

type AuthorityRepository interface {
	ListCertificateAuthorities(context.Context, bool) ([]mysqlstore.CertificateAuthority, error)
	ActiveCertificateAuthorities(context.Context) ([][]byte, error)
	CreateCertificateAuthority(context.Context, mysqlstore.AuthorityCreate) (mysqlstore.AuthorityResult, error)
	RevokeCertificateAuthority(context.Context, mysqlstore.AuthorityRevoke) (mysqlstore.AuthorityResult, error)
	IssueClientCertificate(context.Context, mysqlstore.ClientCertificateIssue) (mysqlstore.ClientCertificateIssueResult, error)
}

func (server *server) listCertificateAuthorities(response http.ResponseWriter, request *http.Request) {
	if _, ok := requireAdmin(response, request); !ok {
		return
	}
	includeRevoked, ok := parseBooleanQuery(response, request, "include_revoked")
	if !ok {
		return
	}
	items, err := server.configs.ListCertificateAuthorities(request.Context(), includeRevoked)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "service_unavailable")
		return
	}
	writeJSON(response, struct {
		Items []mysqlstore.CertificateAuthority `json:"items"`
	}{items})
}

func (server *server) createCertificateAuthority(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		ValidDays   int    `json:"valid_days"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.CreateCertificateAuthority(request.Context(), mysqlstore.AuthorityCreate{
		OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		DisplayName: body.DisplayName, ValidDays: body.ValidDays,
	})
	writeMutationResult(response, result, err)
}

func (server *server) revokeCertificateAuthority(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	result, err := server.configs.RevokeCertificateAuthority(request.Context(), mysqlstore.AuthorityRevoke{
		OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		AuthorityID: request.PathValue("authority"),
	})
	writeMutationResult(response, result, err)
}

func (server *server) issueClientCertificate(response http.ResponseWriter, request *http.Request) {
	principal, ok := requireAdmin(response, request)
	if !ok {
		return
	}
	var body struct {
		AuthorityID string `json:"authority_id"`
		DisplayName string `json:"display_name"`
		ValidDays   int    `json:"valid_days"`
	}
	if !decodeJSON(response, request, &body) {
		return
	}
	result, err := server.configs.IssueClientCertificate(request.Context(), mysqlstore.ClientCertificateIssue{
		OperationID: request.Header.Get("Idempotency-Key"), Actor: mysqlstore.Actor{Type: "user", ID: principal.ActorID()},
		AuthorityID: body.AuthorityID, DisplayName: body.DisplayName, ValidDays: body.ValidDays,
	})
	writeMutationResult(response, result, err)
}

func (server *server) clientTrust(ctx context.Context) (*x509.CertPool, error) {
	roots := x509.NewCertPool()
	if server.clientCAs != nil {
		roots = server.clientCAs.Clone()
	}
	certificates, err := server.configs.ActiveCertificateAuthorities(ctx)
	if err != nil {
		return nil, err
	}
	for _, der := range certificates {
		certificate, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		roots.AddCert(certificate)
	}
	return roots, nil
}
