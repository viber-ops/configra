package source

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	configra "github.com/viber-ops/configra-go"
)

const MaxContentBytes = 3 << 20

type Object struct {
	Type        string `json:"type,omitempty" yaml:"type,omitempty"`
	Environment string `json:"environment" yaml:"environment"`
	Config      string `json:"config,omitempty" yaml:"config,omitempty"`
	Namespace   string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Item        string `json:"item,omitempty" yaml:"item,omitempty"`
	Field       string `json:"field,omitempty" yaml:"field,omitempty"`
	Path        string `json:"path" yaml:"path"`
}

type Material struct {
	Path      string
	Version   string
	Bytes     []byte
	Sensitive bool
	Format    string
}

type Fetcher interface {
	Read(context.Context, []Object, map[string][]byte) ([]Material, error)
}

type Reader struct {
	origin string
	roots  *x509.CertPool
}

func NewReader(origin string, rootCA []byte) (*Reader, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, errors.New("Configra URL must be one HTTPS origin")
	}
	var roots *x509.CertPool
	if len(rootCA) > 0 {
		roots = x509.NewCertPool()
		if len(rootCA) > 4<<20 || !roots.AppendCertsFromPEM(rootCA) {
			return nil, errors.New("invalid Configra server CA bundle")
		}
	}
	return &Reader{origin: origin, roots: roots}, nil
}

func (reader *Reader) Read(ctx context.Context, objects []Object, credentials map[string][]byte) ([]Material, error) {
	if err := ValidateObjects(objects); err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: reader.roots}
	certificate, privateKey := credentials["tls.crt"], credentials["tls.key"]
	if len(certificate) > 64<<10 || len(privateKey) > 64<<10 || len(credentials["token"]) > 512 {
		return nil, errors.New("client credentials exceed limits")
	}
	if len(certificate) != 0 || len(privateKey) != 0 {
		pair, err := tls.X509KeyPair(certificate, privateKey)
		if err != nil {
			return nil, errors.New("invalid client certificate and key")
		}
		tlsConfig.Certificates = []tls.Certificate{pair}
	}
	client, err := configra.NewClient(configra.ClientOptions{
		BaseURL: reader.origin, Token: strings.TrimSpace(string(credentials["token"])), TLSConfig: tlsConfig,
		Timeout: 15 * time.Second, MaxContentBytes: MaxContentBytes,
	})
	if err != nil {
		return nil, errors.New("invalid Configra client credentials")
	}
	defer client.CloseIdleConnections()
	var result []Material
	total := 0
	for _, object := range objects {
		material := Material{Path: object.Path}
		if object.Type == "file" {
			file, err := client.ReadFile(ctx, object.Environment, object.Namespace, object.Item, object.Field, "")
			if err != nil {
				return nil, errors.New("Configra file read failed")
			}
			material.Bytes, material.Version, material.Sensitive = file.Bytes, file.ETag, true
		} else {
			config, err := client.ReadResolvedConfig(ctx, object.Environment, object.Config, "")
			if err != nil {
				return nil, errors.New("Configra config read failed")
			}
			material.Bytes, material.Version, material.Format = []byte(config.Content), config.ETag, config.Format
			material.Sensitive = len(config.VaultRevisions) != 0
		}
		total += len(material.Bytes)
		if total > MaxContentBytes {
			return nil, errors.New("resolved objects exceed the 3 MiB mount limit")
		}
		result = append(result, material)
	}
	return result, nil
}

var resourceKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var fileElement = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func ValidateObjects(objects []Object) error {
	if len(objects) == 0 || len(objects) > 32 {
		return errors.New("between 1 and 32 objects are required")
	}
	seen := map[string]bool{}
	for _, object := range objects {
		if !resourceKey.MatchString(object.Environment) || !ValidPath(object.Path) || seen[object.Path] {
			return errors.New("invalid object Environment or duplicate/unsafe path")
		}
		seen[object.Path] = true
		switch object.Type {
		case "", "config":
			if !resourceKey.MatchString(object.Config) || object.Namespace != "" || object.Item != "" || object.Field != "" {
				return errors.New("invalid Config object")
			}
		case "file":
			if object.Config != "" || !resourceKey.MatchString(object.Namespace) || !resourceKey.MatchString(object.Item) || !resourceKey.MatchString(object.Field) {
				return errors.New("invalid File object")
			}
		default:
			return errors.New("object type must be config or file")
		}
	}
	return nil
}

func ValidPath(value string) bool {
	if value == "" || len(value) > 255 || path.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	for _, element := range strings.Split(value, "/") {
		if element == "." || strings.HasPrefix(element, "..") || !fileElement.MatchString(element) {
			return false
		}
	}
	return true
}
