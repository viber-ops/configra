package observability_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/observability"
)

func TestOperationalMetricsDescribeFailuresWithoutRequestValues(t *testing.T) {
	metrics := observability.New("api", nil)
	server := httptest.NewServer(metrics.Instrument(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(403) })))
	defer server.Close()
	response, err := http.Get(server.URL + "/private-resource-sentinel?token=private-token-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	metrics.Dropped("publish", 2)
	metrics.Worker("audit", false)
	scrape := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(scrape, httptest.NewRequest("GET", "http://metrics/metrics", nil))
	data, err := io.ReadAll(scrape.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{`configra_http_requests_total{code="403",method="get",service="api"} 1`, `configra_access_dropped_total{service="api",stage="publish"} 2`, `configra_worker_healthy{service="api",worker="audit"} 0`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing operational signal %s", expected)
		}
	}
	if strings.Contains(text, "private-resource") || strings.Contains(text, "private-token") {
		t.Fatal("request identity leaked into metrics")
	}
}
