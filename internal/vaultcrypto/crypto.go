package vaultcrypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	AlgorithmAES256GCM = "AES-256-GCM"
	LocalKeyVersion    = "local-v1"
	dekSize            = 32
)

var (
	ErrIntegrity       = errors.New("vault cryptographic integrity failure")
	ErrInvalidIdentity = errors.New("invalid vault snapshot identity")
)

type SnapshotIdentity struct {
	ItemID   string
	Revision uint64
}

type EncryptedSnapshot struct {
	Algorithm    string
	KeyVersion   string
	Nonce        []byte
	Ciphertext   []byte
	EncryptedDEK []byte
}

type EncryptedSecret = EncryptedSnapshot

type EncryptedSentinel struct {
	Algorithm  string
	KeyVersion string
	Nonce      []byte
	Ciphertext []byte
}

type LocalKeyProvider struct {
	master cipher.AEAD
}

func NewLocalKeyProvider(masterKey []byte) (*LocalKeyProvider, error) {
	if len(masterKey) != dekSize {
		return nil, fmt.Errorf("master key must contain exactly %d bytes", dekSize)
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("initialize master key: %w", err)
	}
	master, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize master key AEAD: %w", err)
	}
	return &LocalKeyProvider{master: master}, nil
}

func (provider *LocalKeyProvider) EncryptSnapshot(identity SnapshotIdentity, plaintext []byte) (EncryptedSnapshot, error) {
	if err := validateIdentity(identity); err != nil {
		return EncryptedSnapshot{}, err
	}
	return provider.encrypt(plaintext, snapshotAAD(identity, AlgorithmAES256GCM, LocalKeyVersion))
}

func (provider *LocalKeyProvider) EncryptSecret(identity string, plaintext []byte) (EncryptedSecret, error) {
	if identity == "" || len(identity) > 255 {
		return EncryptedSecret{}, ErrInvalidIdentity
	}
	return provider.encrypt(plaintext, secretAAD(identity, AlgorithmAES256GCM, LocalKeyVersion))
}

func (provider *LocalKeyProvider) encrypt(plaintext, aad []byte) (EncryptedSnapshot, error) {
	dek, err := randomBytes(dekSize)
	if err != nil {
		return EncryptedSnapshot{}, fmt.Errorf("generate data encryption key: %w", err)
	}
	defer clear(dek)

	block, err := aes.NewCipher(dek)
	if err != nil {
		return EncryptedSnapshot{}, fmt.Errorf("initialize data encryption key: %w", err)
	}
	dataAEAD, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedSnapshot{}, fmt.Errorf("initialize data AEAD: %w", err)
	}
	nonce, err := randomBytes(dataAEAD.NonceSize())
	if err != nil {
		return EncryptedSnapshot{}, fmt.Errorf("generate snapshot nonce: %w", err)
	}
	ciphertext := dataAEAD.Seal(nil, nonce, plaintext, aad)

	wrapNonce, err := randomBytes(provider.master.NonceSize())
	if err != nil {
		return EncryptedSnapshot{}, fmt.Errorf("generate key-wrapping nonce: %w", err)
	}
	wrappedDEK := provider.master.Seal(nil, wrapNonce, dek, append(aad, []byte("\x00wrapped-dek")...))
	encryptedDEK := append(wrapNonce, wrappedDEK...)

	return EncryptedSnapshot{
		Algorithm:    AlgorithmAES256GCM,
		KeyVersion:   LocalKeyVersion,
		Nonce:        nonce,
		Ciphertext:   ciphertext,
		EncryptedDEK: encryptedDEK,
	}, nil
}

func (provider *LocalKeyProvider) DecryptSnapshot(identity SnapshotIdentity, encrypted EncryptedSnapshot) ([]byte, error) {
	if validateIdentity(identity) != nil {
		return nil, ErrIntegrity
	}
	return provider.decrypt(encrypted, snapshotAAD(identity, encrypted.Algorithm, encrypted.KeyVersion))
}

func (provider *LocalKeyProvider) DecryptSecret(identity string, encrypted EncryptedSecret) ([]byte, error) {
	if identity == "" || len(identity) > 255 {
		return nil, ErrIntegrity
	}
	return provider.decrypt(encrypted, secretAAD(identity, encrypted.Algorithm, encrypted.KeyVersion))
}

func (provider *LocalKeyProvider) decrypt(encrypted EncryptedSnapshot, aad []byte) ([]byte, error) {
	if encrypted.Algorithm != AlgorithmAES256GCM ||
		encrypted.KeyVersion != LocalKeyVersion ||
		len(encrypted.EncryptedDEK) <= provider.master.NonceSize() {
		return nil, ErrIntegrity
	}
	wrapNonce := encrypted.EncryptedDEK[:provider.master.NonceSize()]
	wrappedDEK := encrypted.EncryptedDEK[provider.master.NonceSize():]
	dek, err := provider.master.Open(nil, wrapNonce, wrappedDEK, append(aad, []byte("\x00wrapped-dek")...))
	if err != nil || len(dek) != dekSize {
		clear(dek)
		return nil, ErrIntegrity
	}
	defer clear(dek)

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, ErrIntegrity
	}
	dataAEAD, err := cipher.NewGCM(block)
	if err != nil || len(encrypted.Nonce) != dataAEAD.NonceSize() {
		return nil, ErrIntegrity
	}
	plaintext, err := dataAEAD.Open(nil, encrypted.Nonce, encrypted.Ciphertext, aad)
	if err != nil {
		return nil, ErrIntegrity
	}
	return plaintext, nil
}

var sentinelPlaintext = []byte("configra-crypto-sentinel-v1")

func (provider *LocalKeyProvider) CreateSentinel() (EncryptedSentinel, error) {
	nonce, err := randomBytes(provider.master.NonceSize())
	if err != nil {
		return EncryptedSentinel{}, fmt.Errorf("generate sentinel nonce: %w", err)
	}
	return EncryptedSentinel{
		Algorithm:  AlgorithmAES256GCM,
		KeyVersion: LocalKeyVersion,
		Nonce:      nonce,
		Ciphertext: provider.master.Seal(nil, nonce, sentinelPlaintext, sentinelAAD()),
	}, nil
}

func (provider *LocalKeyProvider) VerifySentinel(sentinel EncryptedSentinel) error {
	if sentinel.Algorithm != AlgorithmAES256GCM ||
		sentinel.KeyVersion != LocalKeyVersion ||
		len(sentinel.Nonce) != provider.master.NonceSize() {
		return ErrIntegrity
	}
	plaintext, err := provider.master.Open(nil, sentinel.Nonce, sentinel.Ciphertext, sentinelAAD())
	if err != nil {
		return ErrIntegrity
	}
	defer clear(plaintext)
	if subtle.ConstantTimeCompare(plaintext, sentinelPlaintext) != 1 {
		return ErrIntegrity
	}
	return nil
}

func validateIdentity(identity SnapshotIdentity) error {
	if identity.ItemID == "" || identity.Revision == 0 {
		return ErrInvalidIdentity
	}
	return nil
}

func snapshotAAD(identity SnapshotIdentity, algorithm, keyVersion string) []byte {
	var aad bytes.Buffer
	aad.WriteString("configra:vault-snapshot:v1\x00")
	writeString(&aad, identity.ItemID)
	_ = binary.Write(&aad, binary.BigEndian, identity.Revision)
	writeString(&aad, algorithm)
	writeString(&aad, keyVersion)
	return aad.Bytes()
}

func secretAAD(identity, algorithm, keyVersion string) []byte {
	var aad bytes.Buffer
	aad.WriteString("configra:encrypted-secret:v1\x00")
	writeString(&aad, identity)
	writeString(&aad, algorithm)
	writeString(&aad, keyVersion)
	return aad.Bytes()
}

func sentinelAAD() []byte {
	return []byte("configra:crypto-sentinel:v1")
}

func writeString(destination io.Writer, value string) {
	_ = binary.Write(destination, binary.BigEndian, uint32(len(value)))
	_, _ = io.WriteString(destination, value)
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return nil, err
	}
	return value, nil
}
