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
	"testing/synctest"
	"time"

	"go.uber.org/zap"

	"github.com/viber-ops/configra/internal/bootstrap"
)

func TestCommandHasTwoServerModesAndMaintenanceCommands(t *testing.T) {
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
	if !slices.Equal(names, []string{"api", "doctor", "management", "rotate-master-key"}) {
		t.Fatalf("commands = %v", names)
	}

	command.SetArgs([]string{"management"})
	err := command.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "config") {
		t.Fatalf("management without --config error = %v", err)
	}
}

func TestKeyRotationRequiresTargetAndOfflineConfirmation(t *testing.T) {
	for _, args := range [][]string{
		{"rotate-master-key"},
		{"rotate-master-key", "--config=missing.yaml", "--new-key-file=new-key", "--confirm-offline"},
		{"rotate-master-key", "--config=missing.yaml", "--new-key-file=new-key", "--confirm-database=test"},
		{"rotate-master-key", "--config=missing.yaml", "--new-key-file=new-key", "--confirm-database=", "--confirm-offline"},
		{"rotate-master-key", "--config=missing.yaml", "--new-key-file=new-key", "--confirm-database=test", "--confirm-offline", "--timeout=0s"},
	} {
		command := NewCommand()
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(args)
		if err := command.ExecuteContext(context.Background()); err == nil || strings.Contains(err.Error(), "read bootstrap") {
			t.Fatalf("rotation confirmation was not checked before accessing configuration: %v", err)
		}
	}
}

func TestDoctorRequiresExplicitScanAndPositiveTimeout(t *testing.T) {
	for _, args := range [][]string{
		{"doctor", "--verify-vault"},
		{"doctor", "--config=missing.yaml"},
		{"doctor", "--config=missing.yaml", "--verify-vault=false"},
		{"doctor", "--config=missing.yaml", "--verify-vault", "--timeout=0s"},
		{"doctor", "--config=missing.yaml", "--verify-vault", "--timeout=-1s"},
		{"doctor", "--config=missing.yaml", "--verify-vault", "unexpected"},
	} {
		command := NewCommand()
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(args)
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatalf("accepted %v", args)
		}
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

func TestRequestDeadlineBoundsWorkAndPreservesEarlierCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, parentLimit := range []time.Duration{time.Minute, time.Second} {
			ctx, cancel := context.WithTimeout(context.Background(), parentLimit)
			started := time.Now()
			var workContext context.Context
			handler := withRequestDeadline(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				workContext = request.Context()
				<-workContext.Done()
				if !errors.Is(workContext.Err(), context.DeadlineExceeded) {
					t.Errorf("work cancellation = %v", workContext.Err())
				}
				response.WriteHeader(http.StatusServiceUnavailable)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/configs", nil).WithContext(ctx))
			cancel()
			if elapsed := time.Since(started); elapsed != min(parentLimit, 20*time.Second) || response.Code != http.StatusServiceUnavailable {
				t.Fatalf("work ran for %s and returned %d", elapsed, response.Code)
			}
		}
	})
}
