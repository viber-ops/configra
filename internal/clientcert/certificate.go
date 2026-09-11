package clientcert

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

const maxCertificateInputBytes = 1 << 20

func ParseAndVerify(input []byte, roots *x509.CertPool, now time.Time) (*x509.Certificate, error) {
	if roots == nil || len(input) == 0 || len(input) > maxCertificateInputBytes {
		return nil, errors.New("Client Certificate and CA roots are required")
	}
	certificates, err := parseCertificates(input)
	if err != nil {
		return nil, err
	}
	leaf := certificates[0]
	if leaf.IsCA {
		return nil, errors.New("Client Certificate cannot be a CA")
	}
	now = now.UTC()
	if !now.Before(leaf.NotAfter) {
		return nil, errors.New("Client Certificate is expired")
	}
	verifyAt := now
	if verifyAt.Before(leaf.NotBefore) {
		verifyAt = leaf.NotBefore
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range certificates[1:] {
		intermediates.AddCert(certificate)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CurrentTime:   verifyAt,
	}); err != nil {
		return nil, fmt.Errorf("verify Client Certificate: %w", err)
	}
	return leaf, nil
}

func parseCertificates(input []byte) ([]*x509.Certificate, error) {
	if !bytes.Contains(input, []byte("-----BEGIN")) {
		certificate, err := x509.ParseCertificate(input)
		if err != nil {
			return nil, fmt.Errorf("parse Client Certificate DER: %w", err)
		}
		return []*x509.Certificate{certificate}, nil
	}
	remaining := input
	var certificates []*x509.Certificate
	for len(bytes.TrimSpace(remaining)) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("Client Certificate PEM may contain only CERTIFICATE blocks")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse Client Certificate PEM: %w", err)
		}
		certificates = append(certificates, certificate)
		if len(certificates) > 10 {
			return nil, errors.New("Client Certificate chain exceeds 10 certificates")
		}
		remaining = rest
	}
	if len(certificates) == 0 {
		return nil, errors.New("Client Certificate PEM is empty")
	}
	return certificates, nil
}
