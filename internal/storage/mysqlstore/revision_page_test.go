package mysqlstore

import (
	"context"
	"errors"
	"testing"
)

func TestRevisionListsRejectUnboundedQueriesWithoutDatabase(t *testing.T) {
	store := &Store{}
	for _, limit := range []int{-1, 0, MaxRevisionPageSize + 1} {
		if _, err := store.ListConfigRevisions(context.Background(), "a", "payment", RevisionQuery{Limit: limit}); !errors.Is(err, ErrValidation) {
			t.Errorf("Config limit %d: %v", limit, err)
		}
		if _, err := store.ListVaultRevisions(context.Background(), "platform", "redis", RevisionQuery{Limit: limit}); !errors.Is(err, ErrValidation) {
			t.Errorf("Vault limit %d: %v", limit, err)
		}
	}
}
