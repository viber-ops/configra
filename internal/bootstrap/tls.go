package bootstrap

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxCABundleBytes = 4 << 20

func LoadServerTLS(files TLSConfig, mode Mode) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(files.CertificateFile, files.PrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load Server TLS Certificate: %w", err)
	}
	config := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
	}
	if mode == Management {
		return config, nil
	}
	if mode != API {
		return nil, errors.New("invalid Server TLS mode")
	}
	clientCAs := x509.NewCertPool()
	if files.ClientCAFile != "" {
		clientCAs, err = LoadClientCAs(files.ClientCAFile)
		if err != nil {
			return nil, err
		}
	}
	config.ClientCAs = clientCAs
	config.ClientAuth = tls.VerifyClientCertIfGiven
	return config, nil
}

func LoadClientCAs(path string) (*x509.CertPool, error) {
	encodedCA, err := readCABundle(path, "Client CA")
	if err != nil {
		return nil, err
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(encodedCA) {
		return nil, errors.New("Client CA file contains no Certificate")
	}
	return clientCAs, nil
}

func LoadNotificationRootCAs(path string) (*x509.CertPool, error) {
	if path == "" {
		return nil, nil
	}
	encodedCA, err := readCABundle(path, "Notification Root CA")
	if err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(encodedCA) {
		return nil, errors.New("Notification Root CA file contains no Certificate")
	}
	return roots, nil
}

func readCABundle(path, label string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s file: %w", label, err)
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, maxCABundleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s file: %w", label, err)
	}
	if len(encoded) == 0 || len(encoded) > maxCABundleBytes {
		return nil, fmt.Errorf("%s file must contain between 1 byte and 4 MiB", label)
	}
	return encoded, nil
}
