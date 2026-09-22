package mysqlstore

import (
	"errors"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestStoredNotificationCredentialsFailClosedWithoutLeakingValues(t *testing.T) {
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{provider: provider}
	id := []byte("0123456789abcdef")
	valid := `{"url":"https://hooks.example.test/credential-sentinel","secret":"secret-sentinel"}`
	for _, document := range []string{
		valid, valid + `{}`, valid + ` trailing-secret-sentinel`, `null`, `{}`, `{"url":123}`,
		`{"url":"http://hooks.example.test/credential-sentinel"}`, `{"secret-sentinel":"unknown-field"}`,
		`{"url":"https://hooks.example.test","secret":"` + strings.Repeat("s", (16<<10)+1) + `"}`,
	} {
		encrypted, err := provider.EncryptSecret(notificationCredentialIdentity(id), []byte(document))
		if err != nil {
			t.Fatal(err)
		}
		credentials, err := store.decryptNotificationCredentials(id, encrypted)
		if document == valid {
			if err != nil || credentials.Secret != "secret-sentinel" {
				t.Fatalf("valid credentials: %v", err)
			}
		} else if !errors.Is(err, vaultcrypto.ErrIntegrity) || strings.Contains(err.Error(), "sentinel") || credentials != (notificationCredentials{}) {
			t.Fatal("invalid credentials were accepted or exposed")
		}
		if _, err := store.decryptNotificationCredentials([]byte("different-identity"), encrypted); !errors.Is(err, vaultcrypto.ErrIntegrity) {
			t.Fatal("accepted credentials copied from another identity")
		}
	}
}
