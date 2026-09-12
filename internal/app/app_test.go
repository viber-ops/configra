package app

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/bootstrap"
)

func TestCommandHasOnlyManagementAndAPIServerModes(t *testing.T) {
	command := NewCommand()
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	names := make([]string, 0, len(command.Commands()))
	for _, child := range command.Commands() {
		if child.IsAvailableCommand() {
			names = append(names, child.Name())
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"api", "management"}) {
		t.Fatalf("commands = %v", names)
	}

	command.SetArgs([]string{"management"})
	err := command.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "config") {
		t.Fatalf("management without --config error = %v", err)
	}
}

func TestServeStopsCleanlyWhenItsContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		done <- serve(
			ctx,
			"127.0.0.1:0",
			&tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{{1}}}}},
			http.NotFoundHandler(),
			zap.NewNop(),
			bootstrap.API,
		)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not stop after Context cancellation")
	}
}

func TestHealthEndpointsExposeNoDependencyErrorDetails(t *testing.T) {
	dependencyErr := errors.New("mysql-error-secret-sentinel")
	pinged := 0
	handler := withHealth(http.NotFoundHandler(), func(context.Context) error {
		pinged++
		return dependencyErr
	})

	live := httptest.NewRecorder()
	handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusNoContent || pinged != 0 {
		t.Fatalf("live = %d, pings = %d", live.Code, pinged)
	}

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusServiceUnavailable || pinged != 1 || strings.Contains(ready.Body.String(), dependencyErr.Error()) {
		t.Fatalf("ready = %d %q, pings = %d", ready.Code, ready.Body.String(), pinged)
	}

	other := httptest.NewRecorder()
	handler.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if other.Code != http.StatusNotFound {
		t.Fatalf("other status = %d", other.Code)
	}
}
