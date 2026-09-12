package clientcert_test

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/viber-ops/configra/internal/clientcert"
)

func TestManagedTrustPreservesHTTP2Negotiation(t *testing.T) {
	template := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := template.TLS.Certificates[0]
	template.Close()
	manager, err := clientcert.NewTrustManager(context.Background(), nil, func(context.Context) ([][]byte, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { io.WriteString(response, "ok") }))
	server.EnableHTTP2 = true
	server.TLS = manager.TLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}})
	server.StartTLS()
	defer server.Close()
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.ProtoMajor != 2 {
		t.Fatalf("managed trust negotiated %s instead of HTTP/2", response.Proto)
	}
}
