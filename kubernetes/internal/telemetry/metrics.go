package telemetry

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/viber-ops/configra/kubernetes/internal/source"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Bounded labels deliberately exclude Binding names, Token IDs and paths.
type Metrics struct {
	fetches                  *prometheus.CounterVec
	latency                  prometheus.Histogram
	inflight                 prometheus.Gauge
	lastAttempt, lastSuccess prometheus.Gauge
	rpc                      *prometheus.CounterVec
}

func New(registry prometheus.Registerer) *Metrics {
	m := &Metrics{
		fetches:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "configra_kubernetes_fetches_total", Help: "Complete source batches by result."}, []string{"result"}),
		latency:     prometheus.NewHistogram(prometheus.HistogramOpts{Name: "configra_kubernetes_fetch_duration_seconds", Help: "Source batch read duration.", Buckets: []float64{.01, .05, .1, .5, 1, 5, 15, 30}}),
		inflight:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "configra_kubernetes_fetches_inflight", Help: "Source reads currently in progress."}),
		lastAttempt: prometheus.NewGauge(prometheus.GaugeOpts{Name: "configra_kubernetes_last_fetch_timestamp_seconds", Help: "Latest completed source read attempt."}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{Name: "configra_kubernetes_last_success_timestamp_seconds", Help: "Latest successful source batch; not application reload or every Binding freshness."}),
		rpc:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "configra_kubernetes_rpc_total", Help: "Provider RPC outcomes; no request values."}, []string{"method", "code"}),
	}
	registry.MustRegister(m.fetches, m.latency, m.inflight, m.lastAttempt, m.lastSuccess, m.rpc)
	return m
}

type observedSource struct {
	source.Fetcher
	metrics *Metrics
}

func (metrics *Metrics) Wrap(fetcher source.Fetcher) source.Fetcher {
	return &observedSource{fetcher, metrics}
}
func (reader *observedSource) Read(ctx context.Context, objects []source.Object, credentials map[string][]byte) (result []source.Material, err error) {
	start := time.Now()
	reader.metrics.inflight.Inc()
	defer func() {
		reader.metrics.inflight.Dec()
		reader.metrics.latency.Observe(time.Since(start).Seconds())
		reader.metrics.lastAttempt.SetToCurrentTime()
		outcome := "error"
		if err == nil {
			outcome = "success"
			reader.metrics.lastSuccess.SetToCurrentTime()
		}
		reader.metrics.fetches.WithLabelValues(outcome).Inc()
	}()
	return reader.Fetcher.Read(ctx, objects, credentials)
}
func (metrics *Metrics) Intercept(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	method := "other"
	switch info.FullMethod {
	case "/v1alpha1.CSIDriverProvider/Mount":
		method = "mount"
	case "/v1alpha1.CSIDriverProvider/Version":
		method = "version"
	}
	response, err := handler(ctx, request)
	metrics.rpc.WithLabelValues(method, status.Code(err).String()).Inc()
	return response, err
}
