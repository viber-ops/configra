//go:build integration

package mysqlstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/clientcert"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestAuthoritySigningKeyCannotBeReboundToAnotherCertificate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn, database, provider := pkiTestDatabase(t)
	store, err := OpenManagement(ctx, dsn, provider)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	actor := Actor{Type: "user", ID: "admin"}
	created, err := store.CreateCertificateAuthority(ctx, AuthorityCreate{OperationID: "create-ca", Actor: actor, DisplayName: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	substitute, err := clientcert.GenerateAuthority("Substitute", 365, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(substitute.PrivateKeyDER)
	if _, err := database.ExecContext(ctx, "UPDATE certificate_authorities SET certificate_der = ? WHERE id = UNHEX(?)", substitute.Certificate.Raw, created.Authority.ID); err != nil {
		t.Fatal(err)
	}
	result, err := store.IssueClientCertificate(ctx, ClientCertificateIssue{OperationID: "issue-after-tamper", Actor: actor, AuthorityID: created.Authority.ID, DisplayName: "workload"})
	if !errors.Is(err, vaultcrypto.ErrIntegrity) || result.ExportBundle != "" {
		t.Fatal("tampered Authority certificate was accepted or private data escaped")
	}
}
