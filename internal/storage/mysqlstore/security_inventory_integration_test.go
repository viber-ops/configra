//go:build integration

package mysqlstore

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/configdoc"
	"github.com/viber-ops/configra/internal/vaultcrypto"
	"github.com/viber-ops/configra/internal/vaultdoc"
)

func TestAuthorityInventoryUsableExcludesExpiredAndFutureCertificates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	// Seed consistent, signed certificates at three dates without changing host
	// time. Normal creation cannot create expired or future-dated authorities.
	for _, fixture := range []struct {
		name   string
		offset time.Duration
	}{{"expired", -48 * time.Hour}, {"current", 0}, {"future", 48 * time.Hour}} {
		generated, err := clientcert.GenerateAuthority(fixture.name, 1, time.Now().Add(fixture.offset))
		if err != nil {
			t.Fatal(err)
		}
		id, err := randomID()
		if err != nil {
			clear(generated.PrivateKeyDER)
			t.Fatal(err)
		}
		encrypted, err := provider.EncryptSecret(authorityKeyIdentity(hex.EncodeToString(id), generated.Certificate.Raw), generated.PrivateKeyDER)
		clear(generated.PrivateKeyDER)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.db.ExecContext(ctx, `INSERT INTO certificate_authorities
			(id, display_name, certificate_der, algorithm, key_version, nonce, ciphertext, encrypted_dek, not_before, not_after)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, fixture.name, generated.Certificate.Raw,
			encrypted.Algorithm, encrypted.KeyVersion, encrypted.Nonce, encrypted.Ciphertext, encrypted.EncryptedDEK, generated.Certificate.NotBefore, generated.Certificate.NotAfter)
		if err != nil {
			t.Fatal(err)
		}
	}
	all, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: InventoryQuery{Limit: 50}})
	if err != nil || all.Total != 3 || len(all.Items) != 3 {
		t.Fatal("non-revoked history disappeared", err)
	}
	usable, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: InventoryQuery{Limit: 50}, Usable: true})
	if err != nil || usable.Total != 1 || len(usable.Items) != 1 || usable.Items[0].DisplayName != "current" {
		t.Fatal("expired/future issuer offered", err)
	}
	trust, err := store.ActiveCertificateAuthorities(ctx)
	if err != nil || len(trust) != 1 {
		t.Fatal("invalid issuer dates entered trust set", err)
	}
}

func TestSecurityInventoriesAndCompleteVaultImpactAreBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dsn, _, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	actor := Actor{Type: "user", ID: "security-inventory-review"}
	if _, err := store.ApplyEnvironmentChange(ctx, EnvironmentChange{OperationID: "security-env", Actor: actor, Action: EnvironmentCreate, Key: "production", DisplayName: "Production"}); err != nil {
		t.Fatal(err)
	}
	secret := "inventory-secret-must-not-be-listed"
	if _, err := store.CommitVault(ctx, VaultCommit{OperationID: "security-vault", Actor: actor, NamespaceKey: "platform", ItemKey: "db", ItemName: "Database", Snapshot: vaultdoc.Snapshot{
		Fields:   []vaultdoc.Field{{Key: "password", Name: "Password", Type: vaultdoc.Secret}, {Key: "username", Name: "Username", Type: vaultdoc.Text}},
		Variants: []vaultdoc.Variant{{Environments: []string{"production"}, Values: map[string]vaultdoc.Value{"password": {Text: &secret}, "username": {Text: &secret}}}},
	}}); err != nil {
		t.Fatal(err)
	}
	const count = 1005
	var lastAuthority, lastCertificate string
	events := make([]string, count)
	for index := range events {
		events[index] = fmt.Sprintf("event.%04d", index)
	}
	url := "https://hooks.example.test/url-secret-sentinel"
	for index := 0; index < count; index++ {
		key := fmt.Sprintf("entry_%04d", index)
		authority, err := store.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "ca-" + key, Actor: actor, DisplayName: key, ValidDays: 365})
		if err != nil {
			t.Fatal(err)
		}
		certificate, err := store.IssueClientCertificate(ctx, ClientCertificateIssue{OperationID: "cert-" + key, Actor: actor, AuthorityID: authority.Authority.ID, DisplayName: key, ValidDays: 30})
		if err != nil {
			t.Fatal(err)
		}
		if index < count-32 {
			if _, err := store.RevokeCertificateAuthority(ctx, AuthorityRevoke{OperationID: "revoke-" + key, Actor: actor, AuthorityID: authority.Authority.ID}); err != nil {
				t.Fatal(err)
			}
		}
		lastAuthority, lastCertificate = authority.Authority.ID, certificate.Certificate.FingerprintSHA256
		subscriptions := events[:1]
		if index == 0 {
			subscriptions = events
		}
		if _, err := store.CommitNotificationDestination(ctx, NotificationDestinationCommit{OperationID: "notify-" + key, Actor: actor, Key: key, DisplayName: key, Provider: NotificationGenericWebhook, URL: &url, Secret: &secret, Enabled: true, EventTypes: subscriptions}); err != nil {
			t.Fatal(err)
		}
		field := "password"
		if index == count-1 {
			field = "username"
		}
		if _, err := store.CommitConfig(ctx, ConfigCommit{OperationID: "config-" + key, Actor: actor, EnvironmentKey: "production", ConfigKey: key, ConfigName: key, Format: configdoc.YAML, Content: []byte("value: '{vault.platform.db." + field + "}'\n")}); err != nil {
			t.Fatal(err)
		}
	}
	maxBytes := 0
	for _, limit := range []int{1, 50, 100} {
		for _, offset := range []int{0, 500, 1000, count} {
			query := InventoryQuery{Limit: limit, Offset: offset, IncludeInactive: true}
			authorities, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: query})
			if err != nil {
				t.Fatal(err)
			}
			certificates, err := store.ListClientCertificates(ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			destinations, err := store.ListNotificationDestinations(ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			query.IncludeInactive = false
			usages, err := store.ListVaultUsages(ctx, "platform", "db", VaultUsageQuery{InventoryQuery: query})
			if err != nil {
				t.Fatal(err)
			}
			subscriptions, err := store.ListNotificationEventTypes(ctx, "entry_0000", query)
			if err != nil {
				t.Fatal(err)
			}
			want := min(limit, count-offset)
			if authorities.Total != count || certificates.Total != count || destinations.Total != count || usages.Total != count || subscriptions.Total != count ||
				len(authorities.Items) != want || len(certificates.Items) != want || len(destinations.Items) != want || len(usages.Items) != want || len(subscriptions.Items) != want {
				t.Fatal("truncated total or unbounded inventory page")
			}
			for _, authority := range authorities.Items {
				if authority.ClientCertificateCount != 1 {
					t.Fatal("lost certificate count")
				}
			}
			for _, certificate := range certificates.Items {
				if certificate.AuthorityName != certificate.DisplayName || certificate.AuthorityID == "" {
					t.Fatal("off-page issuer label lost")
				}
			}
			for _, destination := range destinations.Items {
				want := uint64(1)
				if destination.Key == "entry_0000" {
					want = count
				}
				if destination.EventTypeCount != want || len(destination.EventTypes) > 3 {
					t.Fatal("unbounded or incorrect subscription summary")
				}
			}
			encoded, err := json.Marshal([]any{authorities, certificates, destinations, usages, subscriptions})
			if err != nil {
				t.Fatal(err)
			}
			maxBytes = max(maxBytes, len(encoded))
			if len(encoded) > 5000*limit+1024 {
				t.Fatalf("oversize page: %d bytes", len(encoded))
			}
			for _, forbidden := range []string{secret, url, "PRIVATE KEY", "export_bundle", "ciphertext", "encrypted_dek"} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("inventory exposed %q", forbidden)
				}
			}
		}
	}
	for _, search := range []string{"ENTRY_1004", "%", "' OR 1=1 --"} {
		query := InventoryQuery{Limit: 1, Search: search, IncludeInactive: true}
		ca, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: query})
		if err != nil {
			t.Fatal(err)
		}
		cert, err := store.ListClientCertificates(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		notify, err := store.ListNotificationDestinations(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		query.IncludeInactive = false
		usage, err := store.ListVaultUsages(ctx, "platform", "db", VaultUsageQuery{InventoryQuery: query})
		if err != nil {
			t.Fatal(err)
		}
		want := uint64(0)
		if search == "ENTRY_1004" {
			want = 1
		}
		if ca.Total != want || cert.Total != want || notify.Total != want || usage.Total != want {
			t.Fatalf("literal search %q lost scope", search)
		}
	}
	ca, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: InventoryQuery{Limit: 1, Key: lastAuthority}})
	if err != nil || ca.Total != 1 || ca.Items[0].ID != lastAuthority {
		t.Fatal("exact Authority identity", err)
	}
	cert, err := store.ListClientCertificates(ctx, InventoryQuery{Limit: 1, Key: lastCertificate})
	if err != nil || cert.Total != 1 || cert.Items[0].FingerprintSHA256 != lastCertificate {
		t.Fatal("exact certificate identity", err)
	}
	usable, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: InventoryQuery{Limit: 50}, Usable: true})
	if err != nil || usable.Total != 32 || len(usable.Items) != 32 {
		t.Fatal("usable Authorities hidden by history", err)
	}
	trust, err := store.ActiveCertificateAuthorities(ctx)
	if err != nil || len(trust) != 32 {
		t.Fatal("management pagination truncated TLS trust", err)
	}
	revoked, err := store.ListCertificateAuthorities(ctx, AuthorityQuery{InventoryQuery: InventoryQuery{Limit: 1, OnlyInactive: true}})
	if err != nil || revoked.Total != count-32 {
		t.Fatal("revoked Authority filter", err)
	}
	revokedCertificates, err := store.ListClientCertificates(ctx, InventoryQuery{Limit: 1, OnlyInactive: true})
	if err != nil || revokedCertificates.Total != count-32 {
		t.Fatal("revoked certificate filter", err)
	}
	if _, err := store.ApplyNotificationDestinationLifecycle(ctx, NotificationDestinationLifecycle{OperationID: "security-archive", Actor: actor, Action: NotificationDestinationArchive, Key: "entry_0000"}); err != nil {
		t.Fatal(err)
	}
	archived, err := store.ListNotificationDestinations(ctx, InventoryQuery{Limit: 1, OnlyInactive: true})
	if err != nil || archived.Total != 1 {
		t.Fatal("archived Destination filter", err)
	}
	subscriptions, err := store.ListNotificationEventTypes(ctx, "entry_0000", InventoryQuery{Limit: 1, Search: "EVENT.1004"})
	if err != nil || subscriptions.Total != 1 || subscriptions.Items[0] != "event.1004" {
		t.Fatal("archived subscription search", err)
	}
	if _, err := store.ListNotificationEventTypes(ctx, "missing", InventoryQuery{Limit: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}

	query := VaultUsageQuery{InventoryQuery: InventoryQuery{Limit: 1}, Impact: true, Fields: []string{"password"}, Environments: []string{"production"}}
	impact, err := store.ListVaultUsages(ctx, "platform", "db", query)
	if err != nil || impact.Total != 1 || len(impact.Items) != 1 || impact.Items[0].ConfigKey != "entry_1004" {
		t.Fatalf("missed off-page field impact: %#v, %v", impact, err)
	}
	query.Fields = []string{"username", "password"}
	impact, err = store.ListVaultUsages(ctx, "platform", "db", query)
	if err != nil || impact.Total != 0 || len(impact.Items) != 0 {
		t.Fatal("unchanged schema impact", err)
	}
	query.Environments = []string{}
	impact, err = store.ListVaultUsages(ctx, "platform", "db", query)
	if err != nil || impact.Total != count || len(impact.Items) != 1 {
		t.Fatal("environment removal missed references", err)
	}
	query.Offset = 1000
	impact, err = store.ListVaultUsages(ctx, "platform", "db", query)
	if err != nil || impact.Total != count || len(impact.Items) != 1 {
		t.Fatal("impact pagination changed total", err)
	}
	if _, err := store.ListVaultUsages(ctx, "elsewhere", "db", query); !errors.Is(err, ErrNotFound) {
		t.Fatal("namespace isolation", err)
	}
	if _, err := store.ApplyConfigLifecycleChange(ctx, ConfigLifecycleChange{OperationID: "archive-config", Actor: actor, Action: ConfigArchive, Key: "entry_1004"}); err != nil {
		t.Fatal(err)
	}
	query.Offset, query.Fields, query.Environments = 0, []string{"password"}, []string{"production"}
	impact, err = store.ListVaultUsages(ctx, "platform", "db", query)
	if err != nil || impact.Total != 0 {
		t.Fatal("archived reference counted as current", err)
	}
	verified, err := store.VerifyEncryptedState(ctx)
	if err != nil || verified != (EncryptionVerification{VaultRevisions: 1, CertificateAuthorities: count, NotificationDestinations: count}) {
		t.Fatalf("recovery verification skipped records beyond an inventory page: %#v, %v", verified, err)
	}
	next, err := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	if rotated, err := store.RotateMasterKey(ctx, next); err != nil || rotated.EncryptionVerification != verified {
		t.Fatalf("rotation skipped records beyond an inventory page: %#v, %v", rotated, err)
	}
	fresh, err := OpenAPI(ctx, dsn, next)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if rotated, err := fresh.VerifyEncryptedState(ctx); err != nil || rotated != verified {
		t.Fatalf("rotated full-inventory verification: %#v, %v", rotated, err)
	}
	t.Logf("MySQL 8.0.22: %d Authorities/certificates/destinations/subscriptions/references; max combined five-page response %d bytes", count, maxBytes)
}
