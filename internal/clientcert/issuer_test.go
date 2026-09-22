package clientcert_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/clientcert"
)

func TestAuthorityIssuesClientOnlyCertificatesAndPrivateExport(t *testing.T) {
	now := time.Now().UTC()
	authority, err := clientcert.GenerateAuthority("Application clients", 30, now)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(authority.PrivateKeyDER)
	client, err := clientcert.GenerateClient(authority.Certificate, authority.PrivateKeyDER, "payments", 90, now)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(client.PrivateKeyDER)
	if client.Certificate.IsCA || client.Certificate.NotAfter.After(authority.Certificate.NotAfter) || client.Certificate.SerialNumber.Sign() <= 0 {
		t.Fatal("invalid client constraints")
	}
	roots := x509.NewCertPool()
	roots.AddCert(authority.Certificate)
	if _, err := client.Certificate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Certificate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err == nil {
		t.Fatal("client credential was accepted as a server certificate")
	}
	bundle, err := clientcert.ExportBundle(client, authority.Certificate)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := base64.StdEncoding.DecodeString(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(encoded)
	archive, err := zip.NewReader(bytes.NewReader(encoded), int64(len(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range archive.File {
		if file.Mode().Perm() != 0600 {
			t.Fatal("export file permissions are not private")
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name], err = io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(files) != 3 || len(files["ca.crt"]) == 0 {
		t.Fatal("incomplete credential bundle")
	}
	if _, err := tls.X509KeyPair(files["client.crt"], files["client.key"]); err != nil {
		t.Fatal("exported certificate and key do not form a pair")
	}
	key, _ := pem.Decode(files["client.key"])
	if key == nil || key.Type != "PRIVATE KEY" {
		t.Fatal("expected PKCS#8 private key")
	}
}

func TestAuthorityRejectsInvalidIssuanceAndMismatchedKey(t *testing.T) {
	now := time.Now().UTC()
	first, err := clientcert.GenerateAuthority("First", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := clientcert.GenerateAuthority("Second", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(first.PrivateKeyDER)
	defer clear(second.PrivateKeyDER)
	if _, err := clientcert.GenerateClient(first.Certificate, second.PrivateKeyDER, "client", 1, now); err == nil {
		t.Fatal("accepted an unrelated Authority key")
	}
	if _, err := clientcert.GenerateClient(first.Certificate, first.PrivateKeyDER, "client", 1, now.Add(48*time.Hour)); err == nil {
		t.Fatal("issued from an expired Authority")
	}
	if _, err := clientcert.ParseAuthorityKey(first.Certificate, first.PrivateKeyDER); err != nil {
		t.Fatalf("valid stored Authority pair: %v", err)
	}
	for _, key := range [][]byte{nil, []byte("private-key-sentinel"), second.PrivateKeyDER} {
		if _, err := clientcert.ParseAuthorityKey(first.Certificate, key); err == nil {
			t.Fatal("accepted invalid stored Authority key")
		}
	}
	if _, err := clientcert.ParseAuthorityKey(nil, first.PrivateKeyDER); err == nil {
		t.Fatal("accepted missing Authority certificate")
	}
	for _, days := range []int{-1, 366} {
		if _, err := clientcert.GenerateClient(first.Certificate, first.PrivateKeyDER, "client", days, now); err == nil {
			t.Fatal("accepted invalid client lifetime")
		}
	}
}

func TestManagedTrustRefreshUsesImmutableSnapshotsAndRetainsLastGood(t *testing.T) {
	first, err := clientcert.GenerateAuthority("First", 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := clientcert.GenerateAuthority("Second", 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer clear(first.PrivateKeyDER)
	defer clear(second.PrivateKeyDER)
	certificates := [][]byte{first.Certificate.Raw}
	var sourceErr error
	manager, err := clientcert.NewTrustManager(context.Background(), nil, func(context.Context) ([][]byte, error) { return certificates, sourceErr })
	if err != nil {
		t.Fatal(err)
	}
	config := manager.TLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	old, err := config.GetConfigForClient(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	certificates = append(certificates, second.Certificate.Raw)
	if err := manager.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, _ := config.GetConfigForClient(&tls.ClientHelloInfo{})
	if len(old.ClientCAs.Subjects()) != 1 || len(next.ClientCAs.Subjects()) != 2 || next.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Fatal("trust snapshot was mutated or not updated")
	}
	sourceErr = errors.New("offline")
	if manager.Refresh(context.Background()) == nil {
		t.Fatal("expected refresh failure")
	}
	retained, _ := config.GetConfigForClient(&tls.ClientHelloInfo{})
	if len(retained.ClientCAs.Subjects()) != 2 {
		t.Fatal("lost previously verified trust")
	}
}
