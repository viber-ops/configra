package mysqlstore

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Concurrent creates probe for rows that do not exist yet with SELECT ... FOR
// UPDATE. Under REPEATABLE READ those probes take compatible gap locks, so two
// inserts into the same index gap can deadlock, and InnoDB rolls one of them
// back with error 1213. The victim's whole transaction, including its operation
// record, is gone, so the mutation can run again from the start without
// applying anything twice.
const deadlockAttempts = 4

func isDeadlock(err error) bool {
	var mysqlError *mysql.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1213
}

func retryDeadlock[T any](ctx context.Context, run func() (T, error)) (T, error) {
	for attempt := 1; ; attempt++ {
		result, err := run()
		if err == nil || attempt == deadlockAttempts || !isDeadlock(err) {
			return result, err
		}
		// Jitter keeps the colliding transactions from retrying in lockstep.
		delay := time.Duration(attempt)*5*time.Millisecond + rand.N(5*time.Millisecond)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, err
		case <-timer.C:
		}
	}
}
