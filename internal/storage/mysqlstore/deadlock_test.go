package mysqlstore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestRetryDeadlockRerunsOnlyDeadlockVictims(t *testing.T) {
	deadlock := fmt.Errorf("create Vault Item: %w", &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"})

	calls := 0
	result, err := retryDeadlock(context.Background(), func() (int, error) {
		calls++
		if calls < 3 {
			return 0, deadlock
		}
		return 7, nil
	})
	if err != nil || result != 7 || calls != 3 {
		t.Fatalf("retried deadlock: result=%d err=%v calls=%d", result, err, calls)
	}

	calls = 0
	if _, err := retryDeadlock(context.Background(), func() (int, error) { calls++; return 0, deadlock }); !isDeadlock(err) || calls != deadlockAttempts {
		t.Fatalf("persistent deadlock: err=%v calls=%d", err, calls)
	}

	calls = 0
	other := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
	if _, err := retryDeadlock(context.Background(), func() (int, error) { calls++; return 0, other }); !errors.Is(err, other) || calls != 1 {
		t.Fatalf("other errors are not retried: err=%v calls=%d", err, calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	if _, err := retryDeadlock(ctx, func() (int, error) { calls++; return 0, deadlock }); !isDeadlock(err) || calls != 1 {
		t.Fatalf("cancelled context stops retrying: err=%v calls=%d", err, calls)
	}
}
