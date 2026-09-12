//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestValidateConfigFormatsAndReportsVaultWarningsWithoutWriting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := createIntegrationDatabase(t, ctx, "configra_config_validation")
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatalf("OpenManagement: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(ctx, "INSERT INTO environments (id, resource_key, display_name) VALUES (?, 'a', 'Environment A')", repeatedID(1)); err != nil {
		t.Fatalf("seed Environment: %v", err)
	}

	result, err := store.ValidateConfig(ctx, ConfigValidation{
		EnvironmentKey: "a", Format: configdoc.YAML,
		Content: []byte("database:\n    password: \"{vault.platform.redis.password}\"\n"),
	})
	if err != nil || result.Content != "database:\n  password: \"{vault.platform.redis.password}\"\n" ||
		len(result.Warnings) != 1 || result.Warnings[0].Code != "missing_vault_item" || result.Warnings[0].NamespaceKey != "platform" {
		t.Fatalf("ValidateConfig = %#v, %v", result, err)
	}
	var revisionCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM config_revisions").Scan(&revisionCount); err != nil || revisionCount != 0 {
		t.Fatalf("Config revisions after validation = %d, %v", revisionCount, err)
	}
	if _, err := store.ValidateConfig(ctx, ConfigValidation{
		EnvironmentKey: "missing", Format: configdoc.JSON, Content: []byte(`{"ok":true}`),
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("missing Environment = %v, want ErrValidation", err)
	}
}
