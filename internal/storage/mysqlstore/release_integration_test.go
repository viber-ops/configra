//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestReleaseSetsPinWholeDeploymentAndRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	admin := Actor{Type: "user", ID: "release-admin"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "release-env", Actor: admin, Action: EnvironmentCreate, Key: "prod", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	issued, err := store.CreateToken(ctx, TokenCreate{OperationID: "release-writer", Actor: admin, Kind: machine.TokenWriteScoped, DisplayName: "Publisher", EnvironmentKeys: []string{"prod"}, ConfigKeys: []string{"server"}, NamespaceKeys: []string{"ops"}, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	actor := Actor{Type: "token", ID: issued.PublicID}
	token, err := store.TokenForEnvironment(ctx, issued.PublicID, "prod")
	if err != nil {
		t.Fatal(err)
	}
	readCtx := machine.WithToken(ctx, token)
	var members []machine.ReleaseFileRef
	writeDraft := func(revision uint64, secret string) {
		t.Helper()
		snapshot := testVaultSnapshot("", []string{"prod"}, "service", secret)
		for i := 0; i < 17; i++ {
			key := fmt.Sprintf("file_%02d", i)
			snapshot.Fields = append(snapshot.Fields, vaultdoc.Field{Key: key, Name: key, Type: vaultdoc.File})
			snapshot.Variants[0].Values[key] = vaultdoc.Value{File: &vaultdoc.FileValue{Filename: key, ContentType: "application/octet-stream", Bytes: []byte(secret + key)}}
		}
		if _, err := store.CommitVault(ctx, VaultCommit{OperationID: fmt.Sprintf("release-vault-%d", revision), Actor: admin, NamespaceKey: "ops", ItemKey: "app", ItemName: "App", ExpectedRevision: revision - 1, Snapshot: snapshot}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: fmt.Sprintf("release-config-%d", revision), Actor: actor, EnvironmentKey: "prod", ConfigKey: "server", ConfigName: "Server", ExpectedRevision: revision - 1, Format: configdoc.YAML, Content: []byte(fmt.Sprintf("version: %d\npassword: \"{vault.ops.app.password}\"\n", revision))}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 17; i++ {
		key := fmt.Sprintf("file_%02d", i)
		members = append(members, machine.ReleaseFileRef{Namespace: "ops", Item: "app", Field: key, Path: "secrets/" + key, Revision: 1})
	}
	writeDraft(1, "release-secret-one")
	prepare := ReleasePrepare{OperationID: "release-prepare-one", Actor: actor, EnvironmentKey: "prod", ConfigKey: "server", ReleaseKey: "first", Spec: machine.ReleaseSpec{ConfigRevision: 1, ConfigPath: "server.yaml", Files: members}}
	if _, err := store.PrepareRelease(readCtx, prepare); err != nil {
		t.Fatal(err)
	}
	activate := ReleaseActivate{OperationID: "release-activate-one", Actor: actor, EnvironmentKey: "prod", ConfigKey: "server", ReleaseKey: "first"}
	state, err := store.ActivateRelease(readCtx, activate)
	if err != nil || state.Generation != 1 {
		t.Fatalf("activation: %#v %v", state, err)
	}
	first, err := store.ReadRelease(readCtx, "prod", "server", "", "")
	if err != nil || len(first.Files) != 17 || !strings.Contains(first.Config.Content, "release-secret-one") {
		t.Fatal("incomplete first release", err)
	}
	writeDraft(2, "release-secret-two")
	unchanged, err := store.ReadRelease(readCtx, "prod", "server", "", first.ETag)
	if !errors.Is(err, machine.ErrNotModified) || unchanged.ETag != first.ETag {
		t.Fatal("draft writes changed active release", err)
	}
	for i := range prepare.Spec.Files {
		prepare.Spec.Files[i].Revision = 2
	}
	prepare.OperationID, prepare.ReleaseKey, prepare.Spec.ConfigRevision = "release-prepare-two", "second", 2
	if _, err := store.PrepareRelease(readCtx, prepare); err != nil {
		t.Fatal(err)
	}
	activate.OperationID, activate.ReleaseKey = "release-activate-stale", "second"
	if _, err := store.ActivateRelease(readCtx, activate); !errors.Is(err, ErrConflict) {
		t.Fatal("stale generation accepted", err)
	}
	activate.OperationID, activate.ExpectedGeneration = "release-activate-two", 1
	if result, err := store.ActivateRelease(readCtx, activate); err != nil || result.Generation != 2 {
		t.Fatal("second activation failed", err)
	}
	second, err := store.ReadRelease(readCtx, "prod", "server", "", "")
	if err != nil || !strings.Contains(second.Config.Content, "release-secret-two") || string(second.Files[16].Bytes) != "release-secret-twofile_16" {
		t.Fatal("release mixed generations", err)
	}
	activate.OperationID, activate.ReleaseKey, activate.ExpectedGeneration = "release-rollback-one", "first", 2
	if result, err := store.ActivateRelease(readCtx, activate); err != nil || result.Generation != 3 {
		t.Fatal("rollback failed", err)
	}
	rolled, err := store.ReadRelease(readCtx, "prod", "server", "", first.ETag)
	if err != nil || rolled.ETag == first.ETag || rolled.Config.Content != first.Config.Content || string(rolled.Files[16].Bytes) != string(first.Files[16].Bytes) {
		t.Fatal("rollback did not restore whole bundle / ABA tag", err)
	}
	if replay, err := store.ActivateRelease(readCtx, activate); err != nil || replay.Generation != 3 {
		t.Fatal("activation replay changed generation", err)
	}
	denied := token
	denied.NamespaceKeys = []string{}
	if _, err := store.ReadRelease(machine.WithToken(ctx, denied), "prod", "server", "", rolled.ETag); !errors.Is(err, machine.ErrForbidden) {
		t.Fatal("conditional read bypassed namespace scope", err)
	}
	if _, err := store.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "release-remove-field", Actor: actor, EnvironmentKey: "prod", NamespaceKey: "ops", ItemKey: "app", ExpectedRevision: 2, Action: "delete_field", Field: vaultdoc.Field{Key: "file_16"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadRelease(readCtx, "prod", "server", "", rolled.ETag); !errors.Is(err, machine.ErrUnresolved) {
		t.Fatal("old release bypassed field removal", err)
	}
}
