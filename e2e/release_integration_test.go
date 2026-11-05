//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	configra "github.com/viber-ops/configra-go"
)

// Uses the executable, real HTTPS/mTLS, MySQL and the existing audit pipeline.
func exerciseReleaseCLI(t *testing.T, ctx context.Context, client *configra.Client, directory string, run func(int, ...string) []byte) {
	t.Helper()
	raw, err := client.ReadRawConfig(ctx, "testing", "server")
	if err != nil {
		t.Fatal(err)
	}
	item, err := client.ReadVaultItem(ctx, "testing", "deployment", "secrets")
	if err != nil {
		t.Fatal(err)
	}
	spec := configra.ReleaseSpec{ConfigRevision: raw.Revision, ConfigPath: "server.yaml"}
	for i := 1; i <= 17; i++ {
		key := fmt.Sprintf("file_%02d", i)
		spec.Files = append(spec.Files, configra.ReleaseFileRef{Namespace: "deployment", Item: "secrets", Field: key, Path: "secrets/" + key, Revision: item.Revision})
	}
	manifestPath := filepath.Join(directory, "release-spec.json")
	prepare := func(key, operation string) {
		t.Helper()
		encoded, _ := json.Marshal(spec)
		if os.WriteFile(manifestPath, encoded, 0600) != nil {
			t.Fatal("write fixture manifest")
		}
		run(0, "release", "prepare", "server", key, "--file", manifestPath, "--operation-id", operation)
	}
	prepare("first", "cli-release-prepare-one")
	prepare("first", "cli-release-prepare-one")
	first, err := client.ReadRelease(ctx, "testing", "server", "first", "")
	if err != nil || len(first.Files) != 17 {
		t.Fatal("candidate incomplete", err)
	}
	bad := filepath.Join(directory, "reject-release")
	if os.WriteFile(bad, []byte("#!/bin/sh\nprintf 'private-value-validator'\nprintf 'private-value-validator-error' >&2\nexit 1\n"), 0700) != nil {
		t.Fatal("write validator")
	}
	run(5, "release", "activate", "server", "first", "--expected-generation", "0", "--operation-id", "cli-release-reject", "--validator", bad)
	state, err := client.ReadReleaseState(ctx, "testing", "server")
	if err != nil || state.Generation != 0 {
		t.Fatal("rejected candidate activated", err)
	}
	good := filepath.Join(directory, "accept-release")
	if os.WriteFile(good, []byte("#!/bin/sh\ntest -s \"$CONFIGRA_CONFIG_FILE\" && test -s \"$CONFIGRA_RELEASE_DIR/secrets/file_17\"\n"), 0700) != nil {
		t.Fatal("write validator")
	}
	run(0, "release", "activate", "server", "first", "--expected-generation", "0", "--operation-id", "cli-release-activate-one", "--validator", good)
	output := filepath.Join(directory, "release-export")
	run(0, "release", "get", "server", "--output-dir", output)
	for _, name := range []string{"server.yaml", "secrets/file_17"} {
		info, err := os.Stat(filepath.Join(output, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("insecure/incomplete release export")
		}
	}
	config, err := client.WriteConfig(ctx, "testing", "server", configra.ConfigWrite{RevisionWrite: configra.RevisionWrite{OperationID: "release-draft-config", ExpectedRevision: raw.Revision}, Name: "Server", Format: "yaml", Content: "version: private-value-next\n"})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := client.WriteFile(ctx, "testing", "deployment", "secrets", "file_17", configra.FileWrite{RevisionWrite: configra.RevisionWrite{OperationID: "release-draft-file", ExpectedRevision: item.Revision}, Name: "File 17", File: configra.VaultFile{Filename: "new.key", ContentType: "application/octet-stream", Bytes: []byte("private-value-next-file")}})
	if err != nil {
		t.Fatal(err)
	}
	active, err := client.ReadRelease(ctx, "testing", "server", "", "")
	if err != nil || active.Config.Content != first.Config.Content || !bytes.Equal(active.Files[16].Bytes, first.Files[16].Bytes) {
		t.Fatal("draft edits changed active release", err)
	}
	_, err = client.ReadRelease(ctx, "foreign", "server", "first", active.ETag)
	var api *configra.APIError
	if !errors.As(err, &api) || api.StatusCode != 403 {
		t.Fatal("release bypassed environment scope", err)
	}
	spec.ConfigRevision = config.Revision
	for i := range spec.Files {
		spec.Files[i].Revision = changed.Revision
	}
	prepare("second", "cli-release-prepare-two")
	run(4, "release", "activate", "server", "second", "--expected-generation", "0", "--operation-id", "cli-release-stale", "--validator", good)
	run(0, "release", "activate", "server", "second", "--expected-generation", "1", "--operation-id", "cli-release-activate-two", "--validator", good)
	second, err := client.ReadRelease(ctx, "testing", "server", "", "")
	if err != nil || second.Generation != 2 || string(second.Files[16].Bytes) != "private-value-next-file" {
		t.Fatal("activation missed File generation", err)
	}
	run(0, "release", "rollback", "server", "first", "--expected-generation", "2", "--operation-id", "cli-release-rollback-one", "--validator", good)
	run(0, "release", "rollback", "server", "first", "--expected-generation", "2", "--operation-id", "cli-release-rollback-one", "--validator", good)
	rolled, err := client.ReadRelease(ctx, "testing", "server", "", active.ETag)
	if err != nil || rolled.Generation != 3 || rolled.Config.Content != first.Config.Content || !bytes.Equal(rolled.Files[16].Bytes, first.Files[16].Bytes) {
		t.Fatal("rollback mixed content or accepted ABA ETag", err)
	}
}
