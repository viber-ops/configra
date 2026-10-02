//go:build integration

package mysqlstore

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/machine"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestProductionRolesSupportScopedWritesWithoutDDLOrDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn, admin, provider := pkiTestDatabase(t)

	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := randomID()
	prefix := "cfg_" + hex.EncodeToString(id[:4])
	roles := []string{"api", "management", "migrate", "backup", "doctor", "gc"}
	for _, role := range roles {
		name := prefix + "_" + role
		t.Cleanup(func() { admin.Exec("DROP ROLE IF EXISTS '" + name + "'") })
	}
	applyRoles := func(file string) {
		t.Helper()
		encoded, err := os.ReadFile("../../../deploy/mysql/" + file)
		if err != nil {
			t.Fatal(err)
		}
		script := strings.ReplaceAll(string(encoded), "`configra`", "`"+config.DBName+"`")
		for _, role := range roles {
			script = strings.ReplaceAll(script, "'configra_"+role+"'", "'"+prefix+"_"+role+"'")
		}
		for _, statement := range strings.Split(script, "\n") {
			if strings.TrimSpace(statement) != "" && !strings.HasPrefix(strings.TrimSpace(statement), "--") {
				if _, err := admin.ExecContext(ctx, statement); err != nil {
					t.Fatal("role template failed", err)
				}
			}
		}
	}
	accounts := map[string]string{}
	account := func(role string) string {
		t.Helper()
		if existing := accounts[role]; existing != "" {
			return existing
		}
		name := prefix + "_u_" + role
		for _, statement := range []string{"CREATE USER '" + name + "'@'%' IDENTIFIED BY 'disposable-test-password'", "GRANT '" + prefix + "_" + role + "' TO '" + name + "'@'%'", "SET DEFAULT ROLE '" + prefix + "_" + role + "' TO '" + name + "'@'%'"} {
			if _, err := admin.ExecContext(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { admin.Exec("DROP USER IF EXISTS '" + name + "'@'%'") })
		copy := *config
		copy.User = name
		copy.Passwd = "disposable-test-password"
		accounts[role] = copy.FormatDSN()
		return accounts[role]
	}
	applyRoles("migration-role.sql")
	initial, err := OpenManagement(ctx, account("migrate"), provider)
	if err != nil {
		t.Fatal("migration identity cannot initialize empty schema", err)
	}
	initial.Close()
	applyRoles("roles.sql")
	management, err := OpenManagement(ctx, account("management"), provider)
	if err != nil {
		t.Fatal("Management restart must not need DDL", err)
	}
	defer management.Close()
	actor := Actor{Type: "user", ID: "roles-admin"}
	if _, err := management.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "roles-environment", Actor: actor, Action: EnvironmentCreate, Key: "prod", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	authority, err := management.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "roles-authority", Actor: actor, DisplayName: "Roles", ValidDays: 2})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := management.CreateToken(ctx, TokenCreate{OperationID: "roles-write-token", Actor: actor, Kind: machine.TokenWriteScoped, DisplayName: "Writer", EnvironmentKeys: []string{"prod"}, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	api, err := OpenAPI(ctx, account("api"), provider)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	writer := Actor{Type: "token", ID: issued.PublicID}
	value := "private-roles-secret"
	if _, err := api.WriteScopedVault(ctx, ScopedVaultWrite{OperationID: "roles-vault-field", Actor: writer, EnvironmentKey: "prod", NamespaceKey: "ops", ItemKey: "app", Action: "put_field", Field: vaultdoc.Field{Key: "password", Name: "Password", Type: vaultdoc.Secret}, Value: vaultdoc.Value{Text: &value}}); err != nil {
		t.Fatal("scoped Vault grants", err)
	}
	if _, err := api.CommitConfig(ctx, ConfigCommit{OperationID: "roles-config-write", Actor: writer, EnvironmentKey: "prod", ConfigKey: "server", ConfigName: "Server", Format: configdoc.YAML, Content: []byte("password: '{vault.ops.app.password}'\n")}); err != nil {
		t.Fatal("scoped Config grants", err)
	}
	if _, err := api.PrepareRelease(ctx, ReleasePrepare{OperationID: "roles-release-prepare", Actor: writer, EnvironmentKey: "prod", ConfigKey: "server", ReleaseKey: "first", Spec: machine.ReleaseSpec{ConfigRevision: 1, ConfigPath: "server.yaml"}}); err != nil {
		t.Fatal("release preparation grants", err)
	}
	if _, err := api.ActivateRelease(ctx, ReleaseActivate{OperationID: "roles-release-activate", Actor: writer, EnvironmentKey: "prod", ConfigKey: "server", ReleaseKey: "first"}); err != nil {
		t.Fatal("release activation grants", err)
	}
	if _, err := api.ReadRelease(ctx, "prod", "server", "", ""); err != nil {
		t.Fatal("release read grants", err)
	}
	credential, err := api.IssueDeploymentCredential(ctx, DeploymentCredentialIssue{OperationID: "roles-deploy-issue", Actor: writer, EnvironmentKey: "prod", DisplayName: "Host", AuthorityID: authority.Authority.ID, ExpiresAt: time.Now().Add(30 * time.Minute)})
	if err != nil {
		t.Fatal("CA locking/issuance grants", err)
	}
	if _, err := api.RevokeDeploymentCredential(ctx, DeploymentCredentialRevoke{OperationID: "roles-deploy-revoke", Actor: writer, EnvironmentKey: "prod", PublicID: credential.Token.PublicID}); err != nil {
		t.Fatal("credential revocation grants", err)
	}
	if _, err := api.OperationalStats(ctx); err != nil {
		t.Fatal("sampler grants", err)
	}
	for _, statement := range []string{"CREATE TABLE should_not_create(id INT)", "DELETE FROM vault_item_revisions WHERE 1=0", "SELECT * FROM management_sessions", "INSERT INTO environments(id,resource_key,display_name) VALUES(UNHEX('00000000000000000000000000000011'),'forbidden','Forbidden')"} {
		if _, err := api.db.ExecContext(ctx, statement); err == nil {
			t.Fatal("API role exceeded its boundary")
		}
	}
	if _, err := management.ChangeSessions(ctx, SessionChange{OperationID: "roles-session-block", Actor: actor, Issuer: "https://idp.test", Subject: "operator", Action: "block"}); err != nil {
		t.Fatal("session policy grants", err)
	}
	if _, err := management.OperationalStats(ctx); err != nil {
		t.Fatal("Management sampler grants", err)
	}
	if _, err := management.db.ExecContext(ctx, "CREATE TABLE forbidden_ddl(id INT)"); err == nil {
		t.Fatal("service Management has DDL")
	}
	gc, err := OpenOutboxMaintenance(ctx, account("gc"), config.DBName)
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Close()
	if _, err := admin.ExecContext(ctx, "UPDATE outbox_events SET status='completed',completed_at=DATE_SUB(UTC_TIMESTAMP(6),INTERVAL 2 DAY)"); err != nil {
		t.Fatal(err)
	}
	pruned, err := gc.Prune(ctx, OutboxPrune{OperationID: "roles-prune-batch", ArchiveID: "roles-archive", Before: time.Now().Add(-time.Hour), Limit: 100, Apply: true}, func(data []byte) error {
		if len(data) == 0 {
			return fmt.Errorf("empty archive")
		}
		return nil
	})
	if err != nil || pruned.Pruned == 0 {
		t.Fatal("GC grants", err)
	}
	if _, err := gc.db.ExecContext(ctx, "SELECT * FROM vault_item_revisions"); err == nil {
		t.Fatal("GC can read Vault history")
	}

	// The same template also provisions the separate 3 -> 4 migration identity.
	for _, statement := range []string{"DROP TABLE config_release_states", "DROP TABLE config_release_sets", "DROP TABLE management_session_policies", "ALTER TABLE outbox_events DROP INDEX outbox_events_retention", "DELETE FROM schema_migrations WHERE version=4", "INSERT INTO schema_migrations(version) VALUES(3)"} {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if unexpected, err := OpenAPI(ctx, dsn, provider); err == nil {
		unexpected.Close()
		t.Fatal("API started against the old schema")
	}
	migrating, err := OpenManagement(ctx, account("migrate"), provider)
	if err != nil {
		t.Fatal("migration role cannot upgrade schema 3", err)
	}
	migrating.Close()
}
