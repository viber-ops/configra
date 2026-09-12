package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func TestGenericWebhookSignsExactBodyAndClassifiesResponses(t *testing.T) {
	fixed := time.Unix(1_725_000_000, 0).UTC()
	secret := "generic-signing-secret-sentinel"
	var redirected atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			response.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch {
		case request.URL.Path == "/success":
			if request.Header.Get("X-Configra-Timestamp") != "1725000000" ||
				request.Header.Get("X-Configra-Delivery") != "00112233445566778899aabbccddeeff" ||
				request.Header.Get("X-Configra-Event") != "config.created" ||
				request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("headers = %#v", request.Header)
			}
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write([]byte("v1\n1725000000\n00112233445566778899aabbccddeeff\nconfig.created\n"))
			_, _ = mac.Write(body)
			wantSignature := "v1=" + hex.EncodeToString(mac.Sum(nil))
			if request.Header.Get("X-Configra-Signature") != wantSignature {
				t.Errorf("signature = %q, want %q", request.Header.Get("X-Configra-Signature"), wantSignature)
			}
			if !json.Valid(body) || !strings.Contains(string(body), `"operation_id":"operation-create"`) ||
				strings.Contains(string(body), secret) {
				t.Errorf("body = %s", body)
			}
			response.WriteHeader(http.StatusNoContent)
		case request.URL.Path == "/retry":
			response.Header().Set("Retry-After", "2")
			response.WriteHeader(http.StatusServiceUnavailable)
		case request.URL.Path == "/dead":
			response.WriteHeader(http.StatusBadRequest)
		case strings.HasPrefix(request.URL.Path, "/redirect/"):
			status, _ := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/redirect/"))
			response.Header().Set("Location", serverURL(request)+"/redirected")
			response.WriteHeader(status)
		case request.URL.Path == "/redirected":
			redirected.Store(true)
			response.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	sender := newTestSender(t, server)
	sender.now = func() time.Time { return fixed }
	event := testEvent()
	target := mysqlstore.NotificationTarget{
		Provider: mysqlstore.NotificationGenericWebhook,
		Secret:   secret,
		Active:   true,
		Attempt:  1,
	}

	target.URL = server.URL + "/success"
	if result := sender.Send(context.Background(), target, event); result.Status != mysqlstore.NotificationDeliverySucceeded || result.HTTPStatus != 204 {
		t.Fatalf("success result = %#v", result)
	}
	target.URL = server.URL + "/retry"
	if result := sender.Send(context.Background(), target, event); result.Status != mysqlstore.NotificationDeliveryRetrying ||
		result.ErrorCode != "http_503" || !result.HasRetryAfter || result.RetryAfter != 2*time.Second {
		t.Fatalf("retry result = %#v", result)
	}
	target.URL = server.URL + "/dead"
	if result := sender.Send(context.Background(), target, event); result.Status != mysqlstore.NotificationDeliveryDead || result.ErrorCode != "http_400" {
		t.Fatalf("dead result = %#v", result)
	}
	for _, status := range []int{301, 302, 303, 307, 308} {
		target.URL = server.URL + "/redirect/" + strconv.Itoa(status)
		if result := sender.Send(context.Background(), target, event); result.Status != mysqlstore.NotificationDeliveryDead ||
			result.ErrorCode != "http_"+strconv.Itoa(status) || redirected.Load() {
			t.Fatalf("redirect %d result = %#v, followed = %v", status, result, redirected.Load())
		}
	}
}

func TestSenderBlocksLoopbackByDefaultBeforeRequest(t *testing.T) {
	var hits atomic.Uint32
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	sender, err := NewSender(Config{RootCAs: roots})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	result := sender.Send(context.Background(), mysqlstore.NotificationTarget{
		Provider: mysqlstore.NotificationGenericWebhook, URL: server.URL, Active: true, Attempt: 1,
	}, testEvent())
	if result.Status != mysqlstore.NotificationDeliveryDead || result.ErrorCode != "destination_blocked" || hits.Load() != 0 {
		t.Fatalf("blocked result = %#v, hits = %d", result, hits.Load())
	}
}

func TestFeishuUsesOfficialSignatureAndValidatesProviderResponse(t *testing.T) {
	const (
		secret    = "demo"
		timestamp = int64(1_599_360_473)
		wantSign  = "l1N0gAcBjdwBvGm1xMjOF0XSyaLRpR7tuO5dHfhAYc8="
	)
	var responseBody atomic.Value
	responseBody.Store(`{"code":0,"msg":"success"}`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			Timestamp string `json:"timestamp"`
			Sign      string `json:"sign"`
			Message   string `json:"msg_type"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode Feishu body: %v", err)
		}
		if body.Timestamp != "1599360473" || body.Sign != wantSign || body.Message != "text" {
			t.Errorf("Feishu body = %#v", body)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, responseBody.Load().(string))
	}))
	defer server.Close()
	sender := newTestSender(t, server)
	sender.now = func() time.Time { return time.Unix(timestamp, 0).UTC() }
	parsedServerURL, _ := url.Parse(server.URL)
	sender.feishuHost = parsedServerURL.Hostname()
	sender.feishuPort = parsedServerURL.Port()
	target := mysqlstore.NotificationTarget{
		Provider: mysqlstore.NotificationFeishuBot,
		URL:      server.URL + "/open-apis/bot/v2/hook/test-token",
		Secret:   secret,
		Active:   true,
		Attempt:  1,
	}
	if result := sender.Send(context.Background(), target, testEvent()); result.Status != mysqlstore.NotificationDeliverySucceeded {
		t.Fatalf("Feishu success = %#v", result)
	}
	responseBody.Store(`{"code":11232,"msg":"rate limited"}`)
	if result := sender.Send(context.Background(), target, testEvent()); result.Status != mysqlstore.NotificationDeliveryRetrying || result.ErrorCode != "feishu_11232" {
		t.Fatalf("Feishu rate limit = %#v", result)
	}
	responseBody.Store(`not-json`)
	if result := sender.Send(context.Background(), target, testEvent()); result.Status != mysqlstore.NotificationDeliveryDead || result.ErrorCode != "invalid_provider_response" {
		t.Fatalf("Feishu invalid response = %#v", result)
	}
}

func TestSignatureFixedVectors(t *testing.T) {
	if got := genericSignature(
		"test-secret", "1700000000", "del_01", "vault.updated", []byte(`{"revision":21,"vault":"redis"}`),
	); got != "v1=5686b164694544f1dfe884eeb0c401b9593efb86096e4504cda40455bb2d036f" {
		t.Fatalf("Generic signature = %q", got)
	}
	if got := feishuSignature("demo", "1599360473"); got != "l1N0gAcBjdwBvGm1xMjOF0XSyaLRpR7tuO5dHfhAYc8=" {
		t.Fatalf("Feishu signature = %q", got)
	}
}

func TestAddressPolicyDefaultAndExplicitInternalExceptions(t *testing.T) {
	policy, err := newAddressPolicy([]string{"database.internal"}, []string{"10.20.0.0/16", "fd12:3456::/32"})
	if err != nil {
		t.Fatalf("newAddressPolicy: %v", err)
	}
	tests := []struct {
		name    string
		host    string
		address string
		port    uint16
		want    bool
	}{
		{"public HTTPS", "example.com", "8.8.8.8", 443, true},
		{"public custom port", "example.com", "8.8.8.8", 8443, false},
		{"private default", "other.internal", "10.21.1.2", 443, false},
		{"private CIDR", "other.internal", "10.20.1.2", 8443, true},
		{"private exact host", "database.internal", "192.168.1.2", 8443, true},
		{"ULA CIDR", "other.internal", "fd12:3456::1", 443, true},
		{"documentation", "example.test", "192.0.2.1", 443, false},
		{"loopback remains denied", "database.internal", "127.0.0.1", 443, false},
		{"metadata remains denied", "database.internal", "169.254.169.254", 443, false},
		{"IPv6 loopback remains denied", "database.internal", "::1", 443, false},
		{"NAT64 remains denied", "database.internal", "64:ff9b::a00:1", 443, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.allows(test.host, netip.MustParseAddr(test.address), test.port); got != test.want {
				t.Fatalf("allows(%q, %q, %d) = %v, want %v", test.host, test.address, test.port, got, test.want)
			}
		})
	}
	if _, err := newAddressPolicy(nil, []string{"127.0.0.0/8"}); err == nil {
		t.Fatal("newAddressPolicy accepted hard-denied loopback CIDR")
	}
}

func TestDestinationURLProviderBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		provider mysqlstore.NotificationProvider
		url      string
		valid    bool
	}{
		{"Generic", mysqlstore.NotificationGenericWebhook, "https://hooks.example.com/path?token=x", true},
		{"Generic custom port", mysqlstore.NotificationGenericWebhook, "https://hooks.internal:8443/path", true},
		{"plain HTTP", mysqlstore.NotificationGenericWebhook, "http://hooks.example.com/path", false},
		{"userinfo", mysqlstore.NotificationGenericWebhook, "https://user:password@hooks.example.com/path", false},
		{"fragment", mysqlstore.NotificationGenericWebhook, "https://hooks.example.com/path#token", false},
		{"opaque", mysqlstore.NotificationGenericWebhook, "https:opaque", false},
		{"Unicode host", mysqlstore.NotificationGenericWebhook, "https://例子.example/path", false},
		{"IPv6 zone", mysqlstore.NotificationGenericWebhook, "https://[fe80::1%25en0]/path", false},
		{"zero port", mysqlstore.NotificationGenericWebhook, "https://hooks.example.com:0/path", false},
		{"Feishu", mysqlstore.NotificationFeishuBot, "https://open.feishu.cn/open-apis/bot/v2/hook/token", true},
		{"Feishu wrong host", mysqlstore.NotificationFeishuBot, "https://evil.example.com/open-apis/bot/v2/hook/token", false},
		{"Feishu wrong path", mysqlstore.NotificationFeishuBot, "https://open.feishu.cn/not-a-hook/token", false},
		{"Feishu query", mysqlstore.NotificationFeishuBot, "https://open.feishu.cn/open-apis/bot/v2/hook/token?x=1", false},
		{"Feishu port", mysqlstore.NotificationFeishuBot, "https://open.feishu.cn:8443/open-apis/bot/v2/hook/token", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDestinationURL(test.provider, test.url)
			if (err == nil) != test.valid || (err != nil && strings.Contains(err.Error(), test.url)) {
				t.Fatalf("ValidateDestinationURL error = %v, valid = %v", err, test.valid)
			}
		})
	}
}

func TestRetryAfterFormatsInvalidValuesAndCap(t *testing.T) {
	now := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name   string
		values []string
		want   time.Duration
		valid  bool
	}{
		{"seconds", []string{"120"}, 2 * time.Minute, true},
		{"IMF fixdate", []string{now.Add(5 * time.Minute).Format(http.TimeFormat)}, 5 * time.Minute, true},
		{"RFC850", []string{now.Add(5 * time.Minute).Format(time.RFC850)}, 5 * time.Minute, true},
		{"asctime", []string{now.Add(5 * time.Minute).Format(time.ANSIC)}, 5 * time.Minute, true},
		{"past", []string{now.Add(-time.Hour).Format(http.TimeFormat)}, 0, true},
		{"cap", []string{"999999999999999999"}, maxRetryAfter, true},
		{"negative", []string{"-1"}, 0, false},
		{"malformed", []string{"later"}, 0, false},
		{"multiple", []string{"1", "2"}, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			for _, value := range test.values {
				header.Add("Retry-After", value)
			}
			got, valid := parseRetryAfter(header, now)
			if got != test.want || valid != test.valid {
				t.Fatalf("parseRetryAfter = %v/%v, want %v/%v", got, valid, test.want, test.valid)
			}
		})
	}
}

func newTestSender(t *testing.T, server *httptest.Server) *Sender {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	sender, err := NewSender(Config{RootCAs: roots})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	sender.client.Transport = server.Client().Transport
	return sender
}

func testEvent() Event {
	return Event{
		SchemaVersion: 1,
		DeliveryID:    "00112233445566778899aabbccddeeff",
		Type:          "config.created",
		Time:          time.Unix(1_725_000_000, 0).UTC(),
		OperationID:   "operation-create",
		Actor:         mysqlstore.Actor{Type: "user", ID: "admin@example.com"},
		Action:        "config.commit",
		Outcome:       mysqlstore.OutcomeSuccess,
		Environment:   "production",
		ResourceType:  "config",
		Resource:      "payment",
		Revision:      1,
	}
}

func serverURL(request *http.Request) string {
	return "https://" + request.Host
}
