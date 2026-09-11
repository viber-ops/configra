package vaultcrypto_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func TestLocalKeyProviderEncryptsAndDecryptsVaultSnapshot(t *testing.T) {
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey)
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	identity := vaultcrypto.SnapshotIdentity{ItemID: "item-redis", Revision: 7}
	plaintext := []byte(`{"variants":[{"environments":["a"],"values":{"password":"sentinel-secret"}}]}`)

	encrypted, err := provider.EncryptSnapshot(identity, plaintext)
	if err != nil {
		t.Fatalf("EncryptSnapshot: %v", err)
	}
	if bytes.Contains(encrypted.Ciphertext, []byte("sentinel-secret")) {
		t.Fatal("ciphertext contains plaintext secret")
	}
	decrypted, err := provider.DecryptSnapshot(identity, encrypted)
	if err != nil {
		t.Fatalf("DecryptSnapshot: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted snapshot = %q, want %q", decrypted, plaintext)
	}
}

func TestVaultSnapshotRejectsWrongIdentityAndTampering(t *testing.T) {
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	identity := vaultcrypto.SnapshotIdentity{ItemID: "item-redis", Revision: 7}
	encrypted, err := provider.EncryptSnapshot(identity, []byte("sentinel-secret"))
	if err != nil {
		t.Fatalf("EncryptSnapshot: %v", err)
	}

	tampered := encrypted
	tampered.Ciphertext = bytes.Clone(encrypted.Ciphertext)
	tampered.Ciphertext[0] ^= 1
	cases := []struct {
		name      string
		identity  vaultcrypto.SnapshotIdentity
		encrypted vaultcrypto.EncryptedSnapshot
	}{
		{name: "different item", identity: vaultcrypto.SnapshotIdentity{ItemID: "item-other", Revision: 7}, encrypted: encrypted},
		{name: "different revision", identity: vaultcrypto.SnapshotIdentity{ItemID: "item-redis", Revision: 8}, encrypted: encrypted},
		{name: "changed ciphertext", identity: identity, encrypted: tampered},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plaintext, decryptErr := provider.DecryptSnapshot(test.identity, test.encrypted)
			if !errors.Is(decryptErr, vaultcrypto.ErrIntegrity) {
				t.Fatalf("DecryptSnapshot error = %v, want ErrIntegrity", decryptErr)
			}
			if plaintext != nil {
				t.Fatalf("DecryptSnapshot returned partial plaintext %q", plaintext)
			}
		})
	}
}

func TestVaultSnapshotsUseFreshEncryptionMaterial(t *testing.T) {
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	identity := vaultcrypto.SnapshotIdentity{ItemID: "item-redis", Revision: 7}
	first, err := provider.EncryptSnapshot(identity, []byte("same-value"))
	if err != nil {
		t.Fatalf("first EncryptSnapshot: %v", err)
	}
	second, err := provider.EncryptSnapshot(identity, []byte("same-value"))
	if err != nil {
		t.Fatalf("second EncryptSnapshot: %v", err)
	}
	if bytes.Equal(first.Nonce, second.Nonce) ||
		bytes.Equal(first.Ciphertext, second.Ciphertext) ||
		bytes.Equal(first.EncryptedDEK, second.EncryptedDEK) {
		t.Fatal("two snapshots reused nonce, ciphertext, or wrapped DEK")
	}
}

func TestEncryptedSecretIsBoundToItsDomainIdentity(t *testing.T) {
	provider, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLocalKeyProvider: %v", err)
	}
	plaintext := []byte(`{"url":"https://hooks.example.test/token-sentinel","secret":"signing-sentinel"}`)
	encrypted, err := provider.EncryptSecret("notification-destination:mail", plaintext)
	if err != nil || bytes.Contains(encrypted.Ciphertext, []byte("sentinel")) {
		t.Fatalf("EncryptSecret = %#v, %v", encrypted, err)
	}
	decrypted, err := provider.DecryptSecret("notification-destination:mail", encrypted)
	if err != nil || !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("DecryptSecret = %q, %v", decrypted, err)
	}
	if leaked, err := provider.DecryptSecret("notification-destination:other", encrypted); !errors.Is(err, vaultcrypto.ErrIntegrity) || leaked != nil {
		t.Fatalf("wrong-identity DecryptSecret = %q, %v", leaked, err)
	}
	if leaked, err := provider.DecryptSnapshot(vaultcrypto.SnapshotIdentity{ItemID: "notification-destination:mail", Revision: 1}, encrypted); !errors.Is(err, vaultcrypto.ErrIntegrity) || leaked != nil {
		t.Fatalf("cross-domain DecryptSnapshot = %q, %v", leaked, err)
	}
}

func TestCryptoSentinelRejectsDifferentMasterKey(t *testing.T) {
	creator, err := vaultcrypto.NewLocalKeyProvider([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("creator: %v", err)
	}
	verifier, err := vaultcrypto.NewLocalKeyProvider([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	sentinel, err := creator.CreateSentinel()
	if err != nil {
		t.Fatalf("CreateSentinel: %v", err)
	}
	if err := creator.VerifySentinel(sentinel); err != nil {
		t.Fatalf("VerifySentinel with original key: %v", err)
	}
	if err := verifier.VerifySentinel(sentinel); !errors.Is(err, vaultcrypto.ErrIntegrity) {
		t.Fatalf("VerifySentinel with wrong key = %v, want ErrIntegrity", err)
	}
}
