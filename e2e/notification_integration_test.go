//go:build integration

package e2e_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/humanauth"
	"github.com/viber-ops/configra/internal/logworker"
	"github.com/viber-ops/configra/internal/management"
	"github.com/viber-ops/configra/internal/notification"
)

func TestManagementQueuesAndWorkerDeliversSignedNotificationsWithDurableRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var (
		retryOnce atomic.Bool
		mu        sync.Mutex
		requests  []capturedNotificationRequest
	)
	endpoint := newPrivateTLSServer(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 128<<10))
		if err != nil {
			t.Errorf("read Notification body: %v", err)
		}
		mu.Lock()
		requests = append(requests, capturedNotificationRequest{
			Body: body, Signature: request.Header.Get("X-Configra-Signature"),
			Delivery: request.Header.Get("X-Configra-Delivery"),
		})
		mu.Unlock()
		if retryOnce.CompareAndSwap(true, false) {
			response.Header().Set("Retry-After", "0")
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	readinessClient := &http.Client{
		Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: endpoint.Roots}},
		Timeout:   3 * time.Second,
	}
	readinessRequest, _ := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.URL, nil)
	readinessResponse, err := readinessClient.Do(readinessRequest)
	if err != nil {
		t.Skipf("private TLS endpoint %s is not locally reachable: %v", endpoint.Address, err)
	}
	_ = readinessResponse.Body.Close()
	readinessClient.CloseIdleConnections()
	store := newStore(t, ctx)
	managementAPI := management.NewHandler(store, nil, nil)
	admin := humanauth.Principal{Subject: "notification-e2e-admin", Role: humanauth.RoleAdmin}
	managementRequest := func(method, path, operationID, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "https://management.test"+path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if operationID != "" {
			request.Header.Set("Idempotency-Key", operationID)
		}
		request = request.WithContext(humanauth.WithPrincipal(request.Context(), admin))
		response := httptest.NewRecorder()
		managementAPI.ServeHTTP(response, request)
		return response
	}

	const secret = "notification-e2e-secret-sentinel"
	urlWithCredential := endpoint.URL + "/generic/url-token-sentinel"
	commitBody, _ := json.Marshal(map[string]any{
		"display_name": "Operations", "provider": "generic_webhook", "url": urlWithCredential,
		"secret": secret, "enabled": true, "event_types": []string{"config.created"},
	})
	commit := managementRequest(http.MethodPut, "/v1/notification-destinations/operations", "notification-e2e-destination", string(commitBody))
	if commit.Code != http.StatusOK || strings.Contains(commit.Body.String(), secret) || strings.Contains(commit.Body.String(), "url-token-sentinel") {
		t.Fatalf("Destination commit = %d %q", commit.Code, commit.Body.String())
	}

	sender, err := notification.NewSender(notification.Config{
		AllowedCIDRs: []string{endpoint.Address.String() + "/32"}, RootCAs: endpoint.Roots,
	})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	defer sender.CloseIdleConnections()
	queued := managementRequest(http.MethodPost, "/v1/notification-destinations/operations/test", "notification-e2e-test-one", "")
	if queued.Code != http.StatusOK {
		t.Fatalf("queue Notification test = %d %q", queued.Code, queued.Body.String())
	}
	if delivered, err := logworker.DeliverNotificationsOnce(ctx, store, sender); err != nil || delivered != 1 {
		history := managementRequest(http.MethodGet, "/v1/notification-destinations/operations/deliveries?limit=10", "", "")
		mu.Lock()
		requestCount := len(requests)
		mu.Unlock()
		t.Fatalf("DeliverNotificationsOnce = %d, %v; address = %s; history = %s; received = %d", delivered, err, endpoint.Address, history.Body.String(), requestCount)
	}

	retryOnce.Store(true)
	queued = managementRequest(http.MethodPost, "/v1/notification-destinations/operations/test", "notification-e2e-test-two", "")
	if queued.Code != http.StatusOK {
		t.Fatalf("queue retrying Notification test = %d %q", queued.Code, queued.Body.String())
	}
	if delivered, err := logworker.DeliverNotificationsOnce(ctx, store, sender); err != nil || delivered != 0 {
		t.Fatalf("first retryable delivery = %d, %v", delivered, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		delivered, err := logworker.DeliverNotificationsOnce(ctx, store, sender)
		if err != nil {
			t.Fatalf("retry Notification delivery: %v", err)
		}
		if delivered == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Notification retry did not become due")
		}
		time.Sleep(20 * time.Millisecond)
	}

	history := managementRequest(http.MethodGet, "/v1/notification-destinations/operations/deliveries?limit=10", "", "")
	if history.Code != http.StatusOK || strings.Count(history.Body.String(), `"status":`) != 3 ||
		strings.Contains(history.Body.String(), secret) || strings.Contains(history.Body.String(), "url-token-sentinel") {
		t.Fatalf("Notification history = %d %q", history.Code, history.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("received Notification requests = %d", len(requests))
	}
	for _, request := range requests {
		if request.Signature == "" || request.Delivery == "" || strings.Contains(string(request.Body), secret) ||
			strings.Contains(string(request.Body), "url-token-sentinel") {
			t.Fatalf("unsafe Notification request = %#v", request)
		}
	}
	if requests[1].Delivery != requests[2].Delivery {
		t.Fatalf("retry Delivery IDs changed: %q != %q", requests[1].Delivery, requests[2].Delivery)
	}
}

type capturedNotificationRequest struct {
	Body      []byte
	Signature string
	Delivery  string
}

type privateTLSServer struct {
	URL     string
	Address netip.Addr
	Roots   *x509.CertPool
}

func newPrivateTLSServer(t *testing.T, handler http.Handler) privateTLSServer {
	t.Helper()
	address := privateInterfaceAddress(t)
	listener, err := net.Listen("tcp4", net.JoinHostPort(address.String(), "0"))
	if err != nil {
		t.Skipf("listen on private interface %s: %v", address, err)
	}
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Notification CA key: %v", err)
	}
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Configra E2E Notification CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatalf("create Notification CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse Notification CA: %v", err)
	}
	leafPublic, leafPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Notification server key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: address.String()},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IP(address.AsSlice())},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, leafPublic, caPrivate)
	if err != nil {
		t.Fatalf("create Notification server Certificate: %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second}
	tlsListener := tls.NewListener(listener, &tls.Config{
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafPrivate,
		}},
	})
	go func() { _ = server.Serve(tlsListener) }()
	t.Cleanup(func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	})
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return privateTLSServer{
		URL: "https://" + listener.Addr().String(), Address: address, Roots: roots,
	}
}

func privateInterfaceAddress(t *testing.T) netip.Addr {
	t.Helper()
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("list network interfaces: %v", err)
	}
	for _, candidate := range addresses {
		prefix, err := netip.ParsePrefix(candidate.String())
		if err == nil && prefix.Addr().Is4() && prefix.Addr().IsPrivate() && !prefix.Addr().IsLoopback() {
			return prefix.Addr()
		}
	}
	t.Skip("no non-loopback private IPv4 interface is available")
	return netip.Addr{}
}
