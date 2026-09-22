package clientcert

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"sort"
	"strings"
	"time"
)

type GeneratedCertificate struct {
	Certificate   *x509.Certificate
	PrivateKeyDER []byte
}

func GenerateAuthority(name string, days int, now time.Time) (GeneratedCertificate, error) {
	if days == 0 {
		days = 5 * 365
	}
	if !validCertificateName(name) || days < 1 || days > 10*365 {
		return GeneratedCertificate{}, errors.New("invalid Authority name or lifetime")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return GeneratedCertificate{}, errors.New("generate Authority key")
	}
	serial, err := newSerial()
	if err != nil {
		return GeneratedCertificate{}, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name, Organization: []string{"Configra"}},
		NotBefore: now.UTC().Add(-5 * time.Minute), NotAfter: now.UTC().Add(time.Duration(days) * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	return createCertificate(template, template, key, key)
}

func GenerateClient(authority *x509.Certificate, privateKeyDER []byte, name string, days int, now time.Time) (GeneratedCertificate, error) {
	if days == 0 {
		days = 90
	}
	if !validCertificateName(name) || days < 1 || days > 365 || authority == nil || !authority.IsCA ||
		now.Before(authority.NotBefore) || !now.Before(authority.NotAfter) {
		return GeneratedCertificate{}, errors.New("invalid Client Certificate name, lifetime, or Authority")
	}
	signer, err := ParseAuthorityKey(authority, privateKeyDER)
	if err != nil {
		return GeneratedCertificate{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return GeneratedCertificate{}, errors.New("generate Client Certificate key")
	}
	serial, err := newSerial()
	if err != nil {
		return GeneratedCertificate{}, err
	}
	notAfter := now.UTC().Add(time.Duration(days) * 24 * time.Hour)
	if notAfter.After(authority.NotAfter) {
		notAfter = authority.NotAfter
	}
	notBefore := now.UTC().Add(-5 * time.Minute)
	if notBefore.Before(authority.NotBefore) {
		notBefore = authority.NotBefore
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name, Organization: []string{"Configra"}},
		NotBefore: notBefore, NotAfter: notAfter, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	return createCertificate(template, authority, key, signer)
}

// ParseAuthorityKey verifies the stored pair without requiring a currently valid
// certificate. Recovery must also verify expired and revoked authorities.
func ParseAuthorityKey(authority *x509.Certificate, privateKeyDER []byte) (crypto.Signer, error) {
	if authority == nil || !authority.IsCA || !authority.BasicConstraintsValid || authority.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, errors.New("invalid Authority certificate")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(privateKeyDER)
	if err != nil {
		return nil, errors.New("invalid Authority signing key")
	}
	signer, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, errors.New("invalid Authority signing key")
	}
	public, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil || !bytes.Equal(public, authority.RawSubjectPublicKeyInfo) {
		return nil, errors.New("Authority key does not match certificate")
	}
	return signer, nil
}

func CertificatePEM(certificate *x509.Certificate) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))
}

// ExportBundle creates a memory-only archive with fixed, non-user-controlled paths.
func ExportBundle(generated GeneratedCertificate, authority *x509.Certificate) (string, error) {
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: generated.PrivateKeyDER})
	defer clear(privatePEM)
	files := map[string][]byte{}
	if generated.Certificate.IsCA {
		files["ca.crt"] = []byte(CertificatePEM(generated.Certificate))
		files["ca.key"] = privatePEM
	} else {
		files["client.crt"] = []byte(CertificatePEM(generated.Certificate))
		files["client.key"] = privatePEM
		files["ca.crt"] = []byte(CertificatePEM(authority))
	}
	var buffer bytes.Buffer
	defer func() { clear(buffer.Bytes()) }()
	archive := zip.NewWriter(&buffer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0600)
		file, err := archive.CreateHeader(header)
		if err != nil {
			return "", errors.New("create credential export")
		}
		if _, err := file.Write(files[name]); err != nil {
			return "", errors.New("write credential export")
		}
	}
	if err := archive.Close(); err != nil {
		return "", errors.New("finish credential export")
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func createCertificate(template, parent *x509.Certificate, key *ecdsa.PrivateKey, signer crypto.Signer) (GeneratedCertificate, error) {
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		return GeneratedCertificate{}, errors.New("sign certificate")
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return GeneratedCertificate{}, errors.New("parse generated certificate")
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return GeneratedCertificate{}, errors.New("encode generated private key")
	}
	return GeneratedCertificate{Certificate: certificate, PrivateKeyDER: privateDER}, nil
}

func newSerial() (*big.Int, error) {
	maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 159), big.NewInt(1))
	value, err := rand.Int(rand.Reader, maximum)
	if err != nil {
		return nil, errors.New("generate certificate serial")
	}
	return value.Add(value, big.NewInt(1)), nil
}

func validCertificateName(name string) bool {
	return strings.TrimSpace(name) == name && name != "" && len(name) <= 128 && !strings.ContainsAny(name, "\x00\r\n")
}
