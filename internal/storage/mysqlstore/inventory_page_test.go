package mysqlstore

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInventoryStorageRejectsUnboundedQueries(t *testing.T) {
	store := &Store{}
	for _, query := range []InventoryQuery{{}, {Limit: -1}, {Limit: 101}, {Limit: 1, Offset: -1}, {Limit: 1, Search: strings.Repeat("x", 257)}, {Limit: 1, Key: strings.Repeat("x", 129)}} {
		if _, err := store.ListEnvironments(context.Background(), EnvironmentQuery{InventoryQuery: query}); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListConfigs(context.Background(), ConfigQuery{InventoryQuery: query}); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListVaultItems(context.Background(), VaultQuery{InventoryQuery: query}); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListTokens(context.Background(), query); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListClientCertificates(context.Background(), query); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListCertificateAuthorities(context.Background(), AuthorityQuery{InventoryQuery: query}); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListNotificationDestinations(context.Background(), query); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListNotificationEventTypes(context.Background(), "ops", query); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
		if _, err := store.ListVaultUsages(context.Background(), "platform", "db", VaultUsageQuery{InventoryQuery: query}); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
	}
}

func TestVaultImpactRequiresExplicitValidSets(t *testing.T) {
	store := &Store{}
	for _, query := range []VaultUsageQuery{
		{Impact: true}, {Impact: true, Fields: []string{}, Environments: nil},
		{Impact: true, Fields: []string{"password", "password"}, Environments: []string{}},
		{Impact: true, Fields: []string{"Password"}, Environments: []string{}},
		{Impact: true, Fields: []string{}, Environments: []string{"a", "a"}},
		{Fields: []string{}},
	} {
		query.InventoryQuery = InventoryQuery{Limit: 50}
		if _, err := store.ListVaultUsages(context.Background(), "platform", "db", query); !errors.Is(err, ErrValidation) {
			t.Fatal(err)
		}
	}
	if _, err := store.ListVaultUsages(context.Background(), "platform", "db", VaultUsageQuery{
		InventoryQuery: InventoryQuery{Limit: 50, Search: "hide-impact"}, Impact: true, Fields: []string{}, Environments: []string{},
	}); !errors.Is(err, ErrValidation) {
		t.Fatal("impact search must not hide references", err)
	}
}

func TestGrantPatchDoesNotChangeLegacyPutOperationDigest(t *testing.T) {
	legacy := struct {
		OperationID     string
		Actor           Actor
		PublicID        string
		EnvironmentKeys []string
	}{"existing-operation", Actor{Type: "user", ID: "admin"}, "0123456789abcdef", []string{"a"}}
	current := TokenEnvironmentChange{OperationID: legacy.OperationID, Actor: legacy.Actor, PublicID: legacy.PublicID, EnvironmentKeys: legacy.EnvironmentKeys}
	if tokenRequestDigest(legacy) != tokenRequestDigest(current) {
		t.Fatal("legacy PUT replay digest changed")
	}
	current.Patch, current.Add, current.EnvironmentKeys = true, []string{"a"}, nil
	if tokenRequestDigest(legacy) == tokenRequestDigest(current) {
		t.Fatal("PATCH and PUT must not share operation identity")
	}
}
