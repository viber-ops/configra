package management

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteAdmissionRejectsExcessBeforeWorkAndLeavesReadsAvailable(t *testing.T) {
	limits := newWriteAdmission(WriteLimits{Concurrent: 1, RequestsPerSecond: 100, Burst: 100})
	entered, release := make(chan struct{}), make(chan struct{})
	handler := limits.wrap(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == "PUT" {
			close(entered)
			<-release
		}
		response.WriteHeader(204)
	}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "http://api/write", nil))
	}()
	defer func() { close(release); <-done }()
	<-entered
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest("PUT", "http://api/write", nil))
	if denied.Code != 429 || denied.Header().Get("Retry-After") == "" {
		t.Fatalf("overload: %d", denied.Code)
	}
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest("GET", "http://api/read", nil))
	if read.Code != 204 {
		t.Fatalf("write overload blocked reads: %d", read.Code)
	}
}
