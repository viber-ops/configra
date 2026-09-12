package bootstrap_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/bootstrap"
)

func TestLoadServerTLSUsesOptionalVerifiedClientCertificatesForAPI(t *testing.T) {
	certificateFile, keyFile, caFile := writeTLSFiles(t)
	config, err := bootstrap.LoadServerTLS(bootstrap.TLSConfig{
		CertificateFile: certificateFile,
		PrivateKeyFile:  keyFile,
		ClientCAFile:    caFile,
	}, bootstrap.API)
	if err != nil {
		t.Fatalf("LoadServerTLS: %v", err)
	}
	if config.MinVersion != tls.VersionTLS12 || config.ClientAuth != tls.VerifyClientCertIfGiven ||
		len(config.Certificates) != 1 || config.ClientCAs == nil || len(config.ClientCAs.Subjects()) != 1 {
		t.Fatalf("API TLS Config = %#v", config)
	}
}

func TestLoadServerTLSDoesNotRequestClientCertificateForManagement(t *testing.T) {
	certificateFile, keyFile, caFile := writeTLSFiles(t)
	config, err := bootstrap.LoadServerTLS(bootstrap.TLSConfig{
		CertificateFile: certificateFile,
		PrivateKeyFile:  keyFile,
		ClientCAFile:    caFile,
	}, bootstrap.Management)
	if err != nil {
		t.Fatalf("LoadServerTLS: %v", err)
	}
	if config.ClientAuth != tls.NoClientCert || config.ClientCAs != nil {
		t.Fatalf("Management TLS Config = %#v", config)
	}
	roots, err := bootstrap.LoadClientCAs(caFile)
	if err != nil || len(roots.Subjects()) != 1 {
		t.Fatalf("LoadClientCAs = %#v, %v", roots, err)
	}
}

func TestLoadServerTLSRejectsInvalidMaterialWithoutEchoingIt(t *testing.T) {
	privateKeySentinel := "private-key-material-sentinel"
	certificateFile := writeFile(t, "bad-cert.pem", "bad-certificate")
	keyFile := writeFile(t, "bad-key.pem", privateKeySentinel)
	_, err := bootstrap.LoadServerTLS(bootstrap.TLSConfig{
		CertificateFile: certificateFile,
		PrivateKeyFile:  keyFile,
	}, bootstrap.Management)
	if err == nil || strings.Contains(err.Error(), privateKeySentinel) {
		t.Fatalf("invalid key error = %v", err)
	}

	validCertificate, validKey, _ := writeTLSFiles(t)
	_, err = bootstrap.LoadServerTLS(bootstrap.TLSConfig{
		CertificateFile: validCertificate,
		PrivateKeyFile:  validKey,
		ClientCAFile:    writeFile(t, "bad-ca.pem", "ca-material-sentinel"),
	}, bootstrap.API)
	if err == nil || strings.Contains(err.Error(), "ca-material-sentinel") {
		t.Fatalf("invalid CA error = %v", err)
	}
}

func TestLoadNotificationRootCAsExtendsSystemTrustWithoutLeakingInvalidPEM(t *testing.T) {
	_, _, caFile := writeTLSFiles(t)
	roots, err := bootstrap.LoadNotificationRootCAs(caFile)
	if err != nil || roots == nil || len(roots.Subjects()) == 0 {
		t.Fatalf("LoadNotificationRootCAs = %#v, %v", roots, err)
	}
	if roots, err := bootstrap.LoadNotificationRootCAs(""); err != nil || roots != nil {
		t.Fatalf("empty LoadNotificationRootCAs = %#v, %v", roots, err)
	}
	const sentinel = "notification-ca-material-sentinel"
	if _, err := bootstrap.LoadNotificationRootCAs(writeFile(t, "bad-notification-ca.pem", sentinel)); err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("invalid Notification CA error = %v", err)
	}
}

func writeTLSFiles(t *testing.T) (string, string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	now := time.Now().UTC()
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "configra.test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	encodedKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	directory := t.TempDir()
	certificateFile := filepath.Join(directory, "tls.crt")
	keyFile := filepath.Join(directory, "tls.key")
	caFile := filepath.Join(directory, "client-ca.pem")
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey})
	for path, content := range map[string][]byte{certificateFile: certificatePEM, keyFile: keyPEM, caFile: certificatePEM} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write TLS file: %v", err)
		}
	}
	return certificateFile, keyFile, caFile
}
