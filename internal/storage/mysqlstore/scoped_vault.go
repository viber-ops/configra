package mysqlstore

import (
	"context"
	"database/sql"
	"maps"
	"reflect"
	"slices"

	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

type ScopedVaultWrite struct {
	OperationID      string
	Actor            Actor
	EnvironmentKey   string
	NamespaceKey     string
	ItemKey          string
	ItemName         string
	ExpectedRevision uint64
	Action           string
	Fields           []vaultdoc.Field
	Values           map[string]vaultdoc.Value
	Field            vaultdoc.Field
	Value            vaultdoc.Value
}

func (store *Store) WriteScopedVault(ctx context.Context, request ScopedVaultWrite) (VaultCommitResult, error) {
	if request.Actor.Type != "token" || !validResourceKey(request.EnvironmentKey) {
		return VaultCommitResult{}, ErrValidation
	}
	digest := tokenRequestDigest(request)
	name := request.ItemName
	if name == "" {
		name = request.ItemKey
	}
	return store.CommitVault(ctx, VaultCommit{
		OperationID: request.OperationID, Actor: request.Actor, NamespaceKey: request.NamespaceKey, ItemKey: request.ItemKey,
		ItemName: name, ExpectedRevision: request.ExpectedRevision, scoped: &request, requestDigest: &digest, action: request.Action,
	})
}

// Merge only the requested Environment's values. A shared Variant is split
// before changing values, while unrelated Variants remain byte-for-byte equal.
func (store *Store) prepareScopedVault(ctx context.Context, tx *sql.Tx, commit *VaultCommit, found bool, itemID []byte, revision uint64) error {
	request := commit.scoped
	current := VaultItem{DisplayName: commit.ItemName}
	var err error
	if found {
		current, err = store.readVaultItem(ctx, tx, request.NamespaceKey, request.ItemKey, revision, true)
		if err != nil {
			return err
		}
		if request.ItemName == "" {
			commit.ItemName = current.DisplayName
		}
	}
	target := -1
	for index, variant := range current.Snapshot.Variants {
		if slices.Contains(variant.Environments, request.EnvironmentKey) {
			target = index
		}
	}
	fields := slices.Clone(current.Snapshot.Fields)
	values := map[string]vaultdoc.Value{}
	if target >= 0 {
		values = maps.Clone(current.Snapshot.Variants[target].Values)
	}
	switch request.Action {
	case "put_item":
		fields, values = slices.Clone(request.Fields), maps.Clone(request.Values)
	case "put_field":
		if !validResourceKey(request.Field.Key) {
			return ErrValidation
		}
		index := slices.IndexFunc(fields, func(field vaultdoc.Field) bool { return field.Key == request.Field.Key })
		if index < 0 {
			fields = append(fields, request.Field)
		} else {
			fields[index] = request.Field
		}
		values[request.Field.Key] = request.Value
	case "delete_field":
		if target < 0 {
			return ErrNotFound
		}
		if !validResourceKey(request.Field.Key) {
			return ErrValidation
		}
		fields = slices.DeleteFunc(fields, func(field vaultdoc.Field) bool { return field.Key == request.Field.Key })
		delete(values, request.Field.Key)
	case "delete_item":
		if target < 0 {
			return ErrNotFound
		}
		allowed, err := allVaultBindingsGranted(ctx, tx, request.Actor.ID, itemID, revision)
		if err != nil {
			return err
		}
		if !allowed {
			return machine.ErrForbidden
		}
		_, err = tx.ExecContext(ctx, `UPDATE vault_items SET archived_at = UTC_TIMESTAMP(6) WHERE id = ?`, itemID)
		return err
	default:
		return ErrValidation
	}
	if len(fields) == 0 {
		return ErrValidation
	}
	slices.SortFunc(fields, func(a, b vaultdoc.Field) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		return 0
	})
	schemaChanged := !reflect.DeepEqual(fields, current.Snapshot.Fields)
	if found && (schemaChanged || commit.ItemName != current.DisplayName) {
		allowed, err := allVaultBindingsGranted(ctx, tx, request.Actor.ID, itemID, revision)
		if err != nil {
			return err
		}
		if !allowed {
			return machine.ErrForbidden
		}
	}
	snapshot := cloneVaultSnapshot(current.Snapshot)
	snapshot.Fields = fields
	// Shared Field definitions apply to every Variant. Existing values survive;
	// newly added Fields use the supplied value in all authorized Variants.
	for index := range snapshot.Variants {
		variant := &snapshot.Variants[index]
		for key := range variant.Values {
			if !slices.ContainsFunc(fields, func(field vaultdoc.Field) bool { return field.Key == key }) {
				delete(variant.Values, key)
			}
		}
		for _, field := range fields {
			if _, exists := variant.Values[field.Key]; !exists {
				variant.Values[field.Key] = values[field.Key]
			}
		}
	}
	if target >= 0 && reflect.DeepEqual(snapshot.Variants[target].Values, values) {
		commit.Snapshot = snapshot
		return nil
	}
	variant := vaultdoc.Variant{Environments: []string{request.EnvironmentKey}, Values: values}
	if target >= 0 && len(snapshot.Variants[target].Environments) == 1 {
		variant.ID = snapshot.Variants[target].ID
		snapshot.Variants[target] = variant
	} else {
		if target >= 0 {
			snapshot.Variants[target].Environments = slices.DeleteFunc(snapshot.Variants[target].Environments, func(key string) bool { return key == request.EnvironmentKey })
		}
		snapshot.Variants = append(snapshot.Variants, variant)
	}
	commit.Snapshot = snapshot
	return nil
}

// This read returns exactly one Environment, never the full Item snapshot.
func (store *Store) ReadScopedVault(ctx context.Context, environment, namespace, item string) (VaultItem, error) {
	result, err := store.ReadVaultItem(ctx, namespace, item, 0, true)
	if err != nil {
		return VaultItem{}, err
	}
	for _, variant := range result.Snapshot.Variants {
		if slices.Contains(variant.Environments, environment) {
			variant.Environments = []string{environment}
			result.Snapshot.Variants = []vaultdoc.Variant{variant}
			return result, nil
		}
	}
	return VaultItem{}, ErrNotFound
}
