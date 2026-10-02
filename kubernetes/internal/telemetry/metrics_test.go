package telemetry_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/viber-ops/configra/kubernetes/internal/source"
	"github.com/viber-ops/configra/kubernetes/internal/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type upstream struct{ err error }

func (u *upstream) Read(context.Context, []source.Object, map[string][]byte) ([]source.Material, error) {
	return nil, u.err
}
func TestMetricsReportFailuresWithoutRequestLabelsOrValues(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := telemetry.New(registry)
	raw := &upstream{}
	reader := metrics.Wrap(raw)
	_, _ = reader.Read(context.Background(), nil, map[string][]byte{"token": []byte("private-token")})
	raw.err = errors.New("private-value")
	_, _ = reader.Read(context.Background(), nil, nil)
	_, _ = metrics.Intercept(context.Background(), "private-argument", &grpc.UnaryServerInfo{FullMethod: "/v1alpha1.CSIDriverProvider/Mount"}, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.Unavailable, "private-response")
	})
	recorder := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, required := range []string{`configra_kubernetes_fetches_total{result="error"} 1`, `configra_kubernetes_fetches_total{result="success"} 1`, `configra_kubernetes_rpc_total{code="Unavailable",method="mount"} 1`, `configra_kubernetes_fetches_inflight 0`} {
		if !strings.Contains(body, required) {
			t.Fatal("missing outcome metric")
		}
	}
	if strings.Contains(body, "private-") {
		t.Fatal("telemetry exposed request or error details")
	}
}
