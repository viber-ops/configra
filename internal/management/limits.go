package management

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type WriteLimits struct {
	Concurrent                int
	RequestsPerSecond         float64
	Burst                     int
	PerTokenRequestsPerSecond float64
}

type tokenLimit struct {
	limiter *rate.Limiter
	last    time.Time
}
type writeAdmission struct {
	slots      chan struct{}
	global     *rate.Limiter
	perToken   rate.Limit
	burst      int
	retryAfter string
	mu         sync.Mutex
	tokens     map[string]*tokenLimit
}

func newWriteAdmission(config WriteLimits) *writeAdmission {
	if config.Concurrent < 1 || config.Concurrent > 8 {
		config.Concurrent = 4
	}
	if config.RequestsPerSecond <= 0 || math.IsNaN(config.RequestsPerSecond) || math.IsInf(config.RequestsPerSecond, 0) {
		config.RequestsPerSecond = 50
	}
	if config.PerTokenRequestsPerSecond <= 0 || math.IsNaN(config.PerTokenRequestsPerSecond) || math.IsInf(config.PerTokenRequestsPerSecond, 0) {
		config.PerTokenRequestsPerSecond = config.RequestsPerSecond / 2
	}
	if config.Burst < 1 || config.Burst > 10000 {
		config.Burst = 100
	}
	return &writeAdmission{slots: make(chan struct{}, config.Concurrent), global: rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.Burst), perToken: rate.Limit(config.PerTokenRequestsPerSecond), burst: max(1, config.Burst/2), retryAfter: strconv.Itoa(max(1, int(math.Ceil(1/min(config.RequestsPerSecond, config.PerTokenRequestsPerSecond))))), tokens: map[string]*tokenLimit{}}
}

func (admission *writeAdmission) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case "POST", "PUT", "PATCH", "DELETE":
		default:
			next.ServeHTTP(response, request)
			return
		}
		// Reject before authentication/admission; no mutation or Audit is accepted.
		// The separate authenticated per-Token rejection below is durably audited.
		if !admission.global.Allow() {
			admission.reject(response)
			return
		}
		select {
		case admission.slots <- struct{}{}:
			defer func() { <-admission.slots }()
		default:
			admission.reject(response)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (admission *writeAdmission) tokenAllowed(id string) bool {
	admission.mu.Lock()
	defer admission.mu.Unlock()
	now := time.Now()
	entry := admission.tokens[id]
	if entry == nil {
		// ponytail: at most 4096 live caller buckets; use an eviction queue if
		// sustained thousands-of-writers workloads make this bounded scan material.
		if len(admission.tokens) >= 4096 {
			for key, value := range admission.tokens {
				if now.Sub(value.last) > 10*time.Minute {
					delete(admission.tokens, key)
				}
			}
		}
		if len(admission.tokens) >= 4096 {
			return false
		}
		entry = &tokenLimit{limiter: rate.NewLimiter(admission.perToken, admission.burst)}
		admission.tokens[id] = entry
	}
	entry.last = now
	return entry.limiter.AllowN(now, 1)
}

func (admission *writeAdmission) reject(response http.ResponseWriter) {
	response.Header().Set("Retry-After", admission.retryAfter)
	writeError(response, http.StatusTooManyRequests, "write_rate_limited")
}
