// Package observability publishes value-free operational signals on a separate
// listener. Labels are fixed categories; request paths and identities are absent.
package observability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

type Metrics struct {
	registry                     *prometheus.Registry
	store                        *mysqlstore.Store
	requests                     *prometheus.CounterVec
	duration                     *prometheus.HistogramVec
	dropped                      *prometheus.CounterVec
	workers, workerLast          *prometheus.GaugeVec
	outbox, age                  *prometheus.GaugeVec
	credential, expiry, expiring *prometheus.GaugeVec
	rows, bytes                  *prometheus.GaugeVec
	database                     *prometheus.GaugeVec
	up, last                     prometheus.Gauge
	serverExpiry                 prometheus.Gauge
}

func New(service string, store *mysqlstore.Store) *Metrics {
	registry := prometheus.NewRegistry()
	register := prometheus.WrapRegistererWith(prometheus.Labels{"service": service}, registry)
	counter := func(name, help string, labels ...string) *prometheus.CounterVec {
		value := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "configra_" + name, Help: help}, labels)
		register.MustRegister(value)
		return value
	}
	gauge := func(name, help string, labels ...string) *prometheus.GaugeVec {
		value := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "configra_" + name, Help: help}, labels)
		register.MustRegister(value)
		return value
	}
	value := &Metrics{registry: registry, store: store}
	value.requests = counter("http_requests_total", "Completed HTTP requests.", "code", "method")
	value.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "configra_http_request_duration_seconds", Help: "HTTP request duration.", Buckets: prometheus.DefBuckets}, []string{"method"})
	register.MustRegister(value.duration)
	value.dropped = counter("access_dropped_total", "Known dropped Access Events; Core NATS is best effort.", "stage")
	value.workers = gauge("worker_healthy", "Last observed worker attempt succeeded.", "worker")
	value.workerLast = gauge("worker_last_attempt_timestamp_seconds", "Last completed worker attempt.", "worker")
	value.outbox = gauge("outbox_events", "Outstanding durable events.", "kind", "status")
	value.age = gauge("outbox_oldest_age_seconds", "Age of the oldest outstanding event.", "kind", "status")
	value.credential = gauge("credentials", "Unrevoked credential inventory including expired entries.", "kind", "state")
	value.expiry = gauge("credential_expiry_timestamp_seconds", "Earliest future expiry of an unrevoked credential; zero when absent.", "kind")
	value.expiring = gauge("credentials_expiring_14d", "Unrevoked credentials expiring within fourteen days.", "kind")
	value.rows = gauge("storage_estimated_rows", "Estimated InnoDB rows; not a completeness assertion.", "table")
	value.bytes = gauge("storage_bytes", "Allocated table and index bytes.", "table")
	value.database = gauge("database_pool", "SQL connection pool state and cumulative waits.", "stat")
	value.up = gauge("operational_sample_success", "Whether the last metadata sample succeeded.").WithLabelValues()
	value.last = gauge("operational_sample_timestamp_seconds", "Last successful metadata sample.").WithLabelValues()
	value.serverExpiry = gauge("server_certificate_expiry_timestamp_seconds", "Expiry of the certificate loaded by this process.").WithLabelValues()
	register.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return value
}

func (metrics *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{MaxRequestsInFlight: 2, Timeout: 5 * time.Second})
}
func (metrics *Metrics) Instrument(next http.Handler) http.Handler {
	return promhttp.InstrumentHandlerCounter(metrics.requests, promhttp.InstrumentHandlerDuration(metrics.duration, next))
}
func (metrics *Metrics) ServerCertificateExpiry(expiry time.Time) {
	metrics.serverExpiry.Set(float64(expiry.Unix()))
}
func (metrics *Metrics) Dropped(stage string, count uint64) {
	if stage == "publish" || stage == "consume" {
		metrics.dropped.WithLabelValues(stage).Add(float64(count))
	}
}
func (metrics *Metrics) Worker(worker string, ok bool) {
	if worker != "audit" && worker != "notification" && worker != "access" {
		return
	}
	value := 0.
	if ok {
		value = 1
	}
	metrics.workers.WithLabelValues(worker).Set(value)
	metrics.workerLast.WithLabelValues(worker).Set(float64(time.Now().Unix()))
}

func (metrics *Metrics) sample(ctx context.Context) {
	if metrics.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stats, err := metrics.store.OperationalStats(ctx)
	if err != nil {
		metrics.up.Set(0)
		return
	}
	metrics.up.Set(1)
	metrics.last.Set(float64(time.Now().Unix()))
	for _, kind := range []string{"audit", "notification"} {
		for _, status := range []string{"pending", "processing", "dead"} {
			metrics.outbox.WithLabelValues(kind, status).Set(0)
			metrics.age.WithLabelValues(kind, status).Set(0)
		}
	}
	for _, value := range stats.Outbox {
		metrics.outbox.WithLabelValues(value.Kind, value.Status).Set(float64(value.Count))
		metrics.age.WithLabelValues(value.Kind, value.Status).Set(value.OldestAgeSeconds)
	}
	for _, kind := range []string{"read-only", "write-scoped", "client-certificate", "authority"} {
		metrics.credential.WithLabelValues(kind, "unrevoked").Set(0)
		metrics.credential.WithLabelValues(kind, "expired").Set(0)
		metrics.expiry.WithLabelValues(kind).Set(0)
		metrics.expiring.WithLabelValues(kind).Set(0)
	}
	for _, value := range stats.Credentials {
		metrics.credential.WithLabelValues(value.Kind, "unrevoked").Set(float64(value.Count))
		metrics.credential.WithLabelValues(value.Kind, "expired").Set(float64(value.Expired))
		metrics.expiry.WithLabelValues(value.Kind).Set(value.NextExpiry)
		metrics.expiring.WithLabelValues(value.Kind).Set(float64(value.Expiring))
	}
	for _, value := range stats.Storage {
		metrics.rows.WithLabelValues(value.Table).Set(float64(value.EstimatedRows))
		metrics.bytes.WithLabelValues(value.Table).Set(float64(value.Bytes))
	}
	for name, value := range map[string]float64{"open": float64(stats.Database.OpenConnections), "in_use": float64(stats.Database.InUse), "idle": float64(stats.Database.Idle), "wait_count": float64(stats.Database.WaitCount), "wait_seconds": stats.Database.WaitDuration.Seconds(), "max_open": float64(stats.Database.MaxOpenConnections)} {
		metrics.database.WithLabelValues(name).Set(value)
	}
}

// Start opens only the separately configured internal address. Scraping never
// queries MySQL: bounded metadata sampling runs independently of API requests.
func (metrics *Metrics) Start(ctx context.Context, address string) (func(), error) {
	if address == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.New("listen on operational metrics address")
	}
	ctx, cancel := context.WithCancel(ctx)
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		metrics.sample(ctx)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				metrics.sample(ctx)
			}
		}
	}()
	go func() { _ = server.Serve(listener) }()
	return func() { cancel(); _ = server.Close(); <-done }, nil
}
