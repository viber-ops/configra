package bootstrap

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/viber-ops/configra/internal/notification"
)

const maxConfigBytes = 1 << 20

type Mode string

const (
	Management Mode = "management"
	API        Mode = "api"
)

type Config struct {
	Version       int                  `yaml:"version"`
	Listen        string               `yaml:"listen"`
	TLS           TLSConfig            `yaml:"tls"`
	MySQL         MySQLConfig          `yaml:"mysql"`
	KeyProvider   KeyProviderConfig    `yaml:"key_provider"`
	OIDC          *OIDCConfig          `yaml:"oidc,omitempty"`
	NATS          *NATSConfig          `yaml:"nats,omitempty"`
	ClickHouse    *ClickHouseConfig    `yaml:"clickhouse,omitempty"`
	Notifications *NotificationsConfig `yaml:"notifications,omitempty"`
	Logging       LoggingConfig        `yaml:"logging,omitempty"`
}

type TLSConfig struct {
	CertificateFile string `yaml:"certificate_file"`
	PrivateKeyFile  string `yaml:"private_key_file"`
	ClientCAFile    string `yaml:"client_ca_file,omitempty"`
}

type MySQLConfig struct {
	DSNEnv string `yaml:"dsn_env"`
}

type KeyProviderConfig struct {
	MasterKeyFile string `yaml:"master_key_file"`
}

type OIDCConfig struct {
	Issuer                string   `yaml:"issuer"`
	ClientID              string   `yaml:"client_id"`
	ClientSecretEnv       string   `yaml:"client_secret_env"`
	RedirectURL           string   `yaml:"redirect_url"`
	Scopes                []string `yaml:"scopes,omitempty"`
	RoleSource            string   `yaml:"role_source"`
	RoleClaim             string   `yaml:"role_claim"`
	ViewerValues          []string `yaml:"viewer_values"`
	AdminValues           []string `yaml:"admin_values"`
	AllowInsecureLoopback bool     `yaml:"allow_insecure_loopback,omitempty"`
}

type LoggingConfig struct {
	Level string `yaml:"level,omitempty"`
}

type NATSConfig struct {
	URLs            []string `yaml:"urls"`
	CredentialsFile string   `yaml:"credentials_file,omitempty"`
	RootCAFile      string   `yaml:"root_ca_file,omitempty"`
}

type ClickHouseConfig struct {
	DSNEnv string `yaml:"dsn_env"`
}

type NotificationsConfig struct {
	AllowedInternalHosts []string `yaml:"allowed_internal_hosts,omitempty"`
	AllowedCIDRs         []string `yaml:"allowed_cidrs,omitempty"`
	RootCAFile           string   `yaml:"root_ca_file,omitempty"`
}

func Load(path string, mode Mode) (Config, error) {
	if mode != Management && mode != API {
		return Config{}, errors.New("invalid Server mode")
	}
	document, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read bootstrap Config: %w", err)
	}
	if len(document) == 0 || len(document) > maxConfigBytes {
		return Config{}, errors.New("bootstrap Config must contain between 1 byte and 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(document))
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode bootstrap Config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("bootstrap Config must contain exactly one YAML document")
	}
	if err := config.validate(mode); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config *Config) validate(mode Mode) error {
	if config.Version != 1 {
		return errors.New("bootstrap Config version must be 1")
	}
	if err := validateListen(config.Listen); err != nil {
		return err
	}
	if config.TLS.CertificateFile == "" || config.TLS.PrivateKeyFile == "" {
		return errors.New("TLS Certificate and Private Key files are required")
	}
	if config.TLS.ClientCAFile == "" {
		return errors.New("Client CA file is required")
	}
	if config.MySQL.DSNEnv == "" || !validEnvironmentVariable(config.MySQL.DSNEnv) {
		return errors.New("MySQL DSN environment variable name is invalid")
	}
	if config.KeyProvider.MasterKeyFile == "" {
		return errors.New("Master Key file is required")
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	switch config.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("logging level must be debug, info, warn, or error")
	}

	if mode == API {
		if config.OIDC != nil {
			return errors.New("OIDC configuration is not valid for API Server")
		}
		if config.ClickHouse != nil {
			return errors.New("ClickHouse configuration is not valid for API Server")
		}
		if config.Notifications != nil {
			return errors.New("Notifications configuration is not valid for API Server")
		}
		if config.NATS == nil {
			return errors.New("API Server NATS configuration is required")
		}
	} else {
		if config.OIDC == nil {
			return errors.New("Management Server OIDC configuration is required")
		}
		if config.NATS == nil || config.ClickHouse == nil {
			return errors.New("Management Server NATS and ClickHouse configuration is required")
		}
		if err := config.OIDC.validate(); err != nil {
			return err
		}
		if !validEnvironmentVariable(config.ClickHouse.DSNEnv) {
			return errors.New("ClickHouse DSN environment variable name is invalid")
		}
		if config.Notifications != nil {
			if err := notification.ValidateNetworkPolicy(
				config.Notifications.AllowedInternalHosts,
				config.Notifications.AllowedCIDRs,
			); err != nil {
				return err
			}
		}
	}
	if config.NATS != nil {
		if err := config.NATS.validate(); err != nil {
			return err
		}
	}

	dsn, ok := os.LookupEnv(config.MySQL.DSNEnv)
	if !ok || dsn == "" {
		return fmt.Errorf("environment variable %s is required", config.MySQL.DSNEnv)
	}
	if config.OIDC != nil {
		secret, ok := os.LookupEnv(config.OIDC.ClientSecretEnv)
		if !ok || secret == "" {
			return fmt.Errorf("environment variable %s is required", config.OIDC.ClientSecretEnv)
		}
	}
	if config.ClickHouse != nil {
		dsn, ok := os.LookupEnv(config.ClickHouse.DSNEnv)
		if !ok || dsn == "" {
			return fmt.Errorf("environment variable %s is required", config.ClickHouse.DSNEnv)
		}
	}
	return nil
}

func (config *NATSConfig) validate() error {
	if len(config.URLs) == 0 {
		return errors.New("NATS URLs are required")
	}
	seen := make(map[string]struct{}, len(config.URLs))
	for _, value := range config.URLs {
		parsed, err := url.Parse(value)
		if err != nil || parsed.User != nil || parsed.Host == "" ||
			(parsed.Scheme != "nats" && parsed.Scheme != "tls") ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("NATS URL is invalid or contains credentials")
		}
		if _, exists := seen[value]; exists {
			return errors.New("NATS URLs must be unique")
		}
		seen[value] = struct{}{}
		if config.RootCAFile != "" && parsed.Scheme != "tls" {
			return errors.New("NATS Root CA requires tls URLs")
		}
	}
	return nil
}

func (config *OIDCConfig) validate() error {
	if config.Issuer == "" || config.ClientID == "" || config.RedirectURL == "" || config.RoleClaim == "" ||
		len(config.ViewerValues) == 0 || len(config.AdminValues) == 0 {
		return errors.New("OIDC Issuer, Client ID, Redirect URL, and Role mapping are required")
	}
	if !validEnvironmentVariable(config.ClientSecretEnv) {
		return errors.New("OIDC Client Secret environment variable name is invalid")
	}
	if config.RoleSource != "id_token" && config.RoleSource != "userinfo" {
		return errors.New("OIDC Role Source must be id_token or userinfo")
	}
	return nil
}

func (config Config) MySQLDSN() string {
	return os.Getenv(config.MySQL.DSNEnv)
}

func (config Config) OIDCClientSecret() string {
	if config.OIDC == nil {
		return ""
	}
	return os.Getenv(config.OIDC.ClientSecretEnv)
}

func (config Config) ClickHouseDSN() string {
	if config.ClickHouse == nil {
		return ""
	}
	return os.Getenv(config.ClickHouse.DSNEnv)
}

func LoadMasterKey(path string) ([]byte, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Master Key file: %w", err)
	}
	defer clear(encoded)
	if len(encoded) == 0 || len(encoded) > 1024 {
		return nil, errors.New("Master Key file has invalid size")
	}
	encoded = bytes.TrimSpace(encoded)
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	size, err := base64.StdEncoding.Strict().Decode(decoded, encoded)
	if err != nil || size != 32 {
		clear(decoded)
		return nil, errors.New("Master Key file must contain one base64-encoded 256-bit key")
	}
	return decoded[:size], nil
}

func validateListen(value string) error {
	_, portText, err := net.SplitHostPort(value)
	if err != nil {
		return errors.New("listen must be a host:port address")
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return errors.New("listen port must be between 1 and 65535")
	}
	return nil
}

func validEnvironmentVariable(value string) bool {
	if value == "" || !(value[0] == '_' || value[0] >= 'A' && value[0] <= 'Z') {
		return false
	}
	for _, character := range value[1:] {
		if character != '_' && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') {
			return false
		}
	}
	return !strings.ContainsAny(value, "\x00\r\n")
}
