package clientcert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync/atomic"
	"time"
)

// TrustManager refreshes public trust independently of individual TLS handshakes.
// Credential revocation remains a per-request authorization decision.
type TrustManager struct {
	static  *x509.CertPool
	current atomic.Pointer[x509.CertPool]
	source  func(context.Context) ([][]byte, error)
}

func NewTrustManager(ctx context.Context, static *x509.CertPool, source func(context.Context) ([][]byte, error)) (*TrustManager, error) {
	if source == nil {
		return nil, errors.New("Client CA source is required")
	}
	if static == nil {
		static = x509.NewCertPool()
	}
	manager := &TrustManager{static: static.Clone(), source: source}
	if err := manager.Refresh(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func (manager *TrustManager) Refresh(ctx context.Context) error {
	certificates, err := manager.source(ctx)
	if err != nil {
		return errors.New("refresh managed Client CA trust")
	}
	roots := manager.static.Clone()
	for _, der := range certificates {
		certificate, err := x509.ParseCertificate(der)
		if err != nil || !certificate.IsCA {
			return errors.New("invalid managed Client CA certificate")
		}
		roots.AddCert(certificate)
	}
	manager.current.Store(roots)
	return nil
}

func (manager *TrustManager) TLSConfig(base *tls.Config) *tls.Config {
	config := base.Clone()
	// net/http clones TLS configuration before adding its ALPN protocols. The
	// per-handshake clone must carry them too, rather than downgrading to HTTP/1.
	if len(config.NextProtos) == 0 {
		config.NextProtos = []string{"h2", "http/1.1"}
	}
	config.ClientAuth = tls.VerifyClientCertIfGiven
	config.ClientCAs = manager.current.Load()
	config.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		connection := config.Clone()
		connection.GetConfigForClient = nil
		connection.ClientCAs = manager.current.Load()
		return connection, nil
	}
	return config
}

func (manager *TrustManager) Run(ctx context.Context, failed func()) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshContext, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := manager.Refresh(refreshContext)
			cancel()
			if err != nil && ctx.Err() == nil && failed != nil {
				failed()
			}
		}
	}
}
