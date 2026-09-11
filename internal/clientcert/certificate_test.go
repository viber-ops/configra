package clientcert_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/clientcert"
)

func TestParseAndVerifyAcceptsConfiguredClientCAAndFutureRotationCertificate(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	ca, caKey := testCertificate(t, nil, nil, x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Configra Client CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	})
	leaf, _ := testCertificate(t, ca, caKey, x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "datacenter-a"},
		NotBefore: now.Add(time.Hour), NotAfter: now.AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	input := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	verified, err := clientcert.ParseAndVerify(input, roots, now)
	if err != nil {
		t.Fatalf("ParseAndVerify: %v", err)
	}
	if verified.Subject.CommonName != "datacenter-a" || verified.SerialNumber.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("verified Certificate = %#v", verified.Subject)
	}
}

func TestParseAndVerifyRejectsUntrustedExpiredWrongUsageOrPrivateKeyInput(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	ca, caKey := testCertificate(t, nil, nil, x509.Certificate{
		SerialNumber: big.NewInt(10), Subject: pkix.Name{CommonName: "Configra Client CA"},
		NotBefore: now.AddDate(-1, 0, 0), NotAfter: now.AddDate(10, 0, 0), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	})
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	leaf := func(serial int64, notAfter time.Time, usages []x509.ExtKeyUsage) *x509.Certificate {
		certificate, _ := testCertificate(t, ca, caKey, x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "client"},
			NotBefore: now.AddDate(-1, 0, 0), NotAfter: notAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: usages,
		})
		return certificate
	}
	valid := leaf(11, now.AddDate(1, 0, 0), []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
	expired := leaf(12, now.Add(-time.Hour), []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
	wrongUsage := leaf(13, now.AddDate(1, 0, 0), []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	tests := []struct {
		name  string
		input []byte
		roots *x509.CertPool
	}{
		{"untrusted", valid.Raw, x509.NewCertPool()},
		{"expired", expired.Raw, roots},
		{"wrong usage", wrongUsage.Raw, roots},
		{"private key appended", append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: valid.Raw}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("sentinel-private-key")})...), roots},
		{"garbage", []byte("not a certificate"), roots},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := clientcert.ParseAndVerify(test.input, test.roots, now); err == nil {
				t.Fatal("ParseAndVerify accepted invalid Certificate input")
			}
		})
	}
}

func testCertificate(
	t *testing.T,
	parent *x509.Certificate,
	parentKey ed25519.PrivateKey,
	template x509.Certificate,
) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if parent == nil {
		parent = &template
		parentKey = privateKey
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, parent, publicKey, parentKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return certificate, privateKey
}
