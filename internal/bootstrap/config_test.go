package bootstrap_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viber-ops/configra/internal/bootstrap"
)

func TestLoadManagementConfigResolvesOnlyNamedSecretEnvironmentVariables(t *testing.T) {
	t.Setenv("CONFIGRA_TEST_MYSQL_DSN", "mysql-secret-sentinel")
	t.Setenv("CONFIGRA_TEST_OIDC_SECRET", "oidc-secret-sentinel")
	t.Setenv("CONFIGRA_TEST_CLICKHOUSE_DSN", "clickhouse-secret-sentinel")
	path := writeFile(t, "config.yaml", `
version: 1
listen: 127.0.0.1:8443
tls:
  certificate_file: /run/configra/tls.crt
  private_key_file: /run/configra/tls.key
  client_ca_file: /run/configra/client-ca.pem
mysql:
  dsn_env: CONFIGRA_TEST_MYSQL_DSN
key_provider:
  master_key_file: /run/configra/master-key
clickhouse:
  dsn_env: CONFIGRA_TEST_CLICKHOUSE_DSN
nats:
  urls: [nats://127.0.0.1:4222]
notifications:
  allowed_internal_hosts: [hooks.corp.internal]
  allowed_cidrs: [10.20.0.0/16]
  root_ca_file: /run/configra/notification-ca.pem
oidc:
  issuer: https://identity.example.com
  client_id: configra
  client_secret_env: CONFIGRA_TEST_OIDC_SECRET
  redirect_url: https://configra.example.com/auth/callback
  role_source: userinfo
  role_claim: groups
  viewer_values: [developers]
  admin_values: [configra-admins]
logging:
  level: warn
`)

	config, err := bootstrap.Load(path, bootstrap.Management)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.Version != 1 || config.Listen != "127.0.0.1:8443" || config.Logging.Level != "warn" ||
		config.TLS.ClientCAFile != "/run/configra/client-ca.pem" || config.Notifications == nil ||
		len(config.Notifications.AllowedInternalHosts) != 1 || len(config.Notifications.AllowedCIDRs) != 1 ||
		config.Notifications.RootCAFile != "/run/configra/notification-ca.pem" {
		t.Fatalf("Config = %#v", config)
	}
	if config.MySQLDSN() != "mysql-secret-sentinel" || config.ClickHouseDSN() != "clickhouse-secret-sentinel" ||
		config.OIDCClientSecret() != "oidc-secret-sentinel" {
		t.Fatal("named secret environment variables were not resolved")
	}
}

func TestDeploymentExamplesMatchTheBootstrapSchema(t *testing.T) {
	t.Setenv("CONFIGRA_MYSQL_DSN", "mysql-secret-sentinel")
	t.Setenv("CONFIGRA_CLICKHOUSE_DSN", "clickhouse-secret-sentinel")
	t.Setenv("CONFIGRA_OIDC_CLIENT_SECRET", "oidc-secret-sentinel")
	for name, mode := range map[string]bootstrap.Mode{
		"management.example.yaml": bootstrap.Management,
		"api.example.yaml":        bootstrap.API,
	} {
		if _, err := bootstrap.Load(filepath.Join("..", "..", "deploy", name), mode); err != nil {
			t.Fatalf("Load %s: %v", name, err)
		}
	}
}

func TestLoadAPIConfigAllowsManagedClientCAsAndRejectsUnusedOIDC(t *testing.T) {
	t.Setenv("CONFIGRA_TEST_MYSQL_DSN", "mysql-secret-sentinel")
	valid := `
version: 1
listen: :9443
tls:
  certificate_file: /run/configra/tls.crt
  private_key_file: /run/configra/tls.key
  client_ca_file: /run/configra/client-ca.pem
mysql:
  dsn_env: CONFIGRA_TEST_MYSQL_DSN
key_provider:
  master_key_file: /run/configra/master-key
nats:
  urls: [nats://127.0.0.1:4222]
`
	config, err := bootstrap.Load(writeFile(t, "api.yaml", valid), bootstrap.API)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.TLS.ClientCAFile != "/run/configra/client-ca.pem" || config.Logging.Level != "info" ||
		config.NATS == nil || len(config.NATS.URLs) != 1 {
		t.Fatalf("API Config = %#v", config)
	}

	withoutCA := strings.Replace(valid, "  client_ca_file: /run/configra/client-ca.pem\n", "", 1)
	if _, err := bootstrap.Load(writeFile(t, "without-ca.yaml", withoutCA), bootstrap.API); err != nil {
		t.Fatalf("API Config using managed Client CAs failed: %v", err)
	}
	withOIDC := valid + `
oidc:
  issuer: https://identity.example.com
  client_id: unused
  client_secret_env: UNUSED
  redirect_url: https://configra.example.com/auth/callback
  role_source: id_token
  role_claim: groups
  viewer_values: [viewer]
  admin_values: [admin]
`
	if _, err := bootstrap.Load(writeFile(t, "api-oidc.yaml", withOIDC), bootstrap.API); err == nil {
		t.Fatal("API Config with unused OIDC section succeeded")
	}
	withoutNATS := strings.Replace(valid, "nats:\n  urls: [nats://127.0.0.1:4222]\n", "", 1)
	if _, err := bootstrap.Load(writeFile(t, "without-nats.yaml", withoutNATS), bootstrap.API); err == nil {
		t.Fatal("API Config without NATS succeeded")
	}
	credentialInURL := strings.Replace(valid, "nats://127.0.0.1:4222", "nats://user:secret-value-sentinel@127.0.0.1:4222", 1)
	if _, err := bootstrap.Load(writeFile(t, "nats-url-secret.yaml", credentialInURL), bootstrap.API); err == nil || strings.Contains(err.Error(), "secret-value-sentinel") {
		t.Fatalf("credential-bearing NATS URL error = %v", err)
	}
	withNotifications := valid + "notifications:\n  allowed_cidrs: [10.20.0.0/16]\n"
	if _, err := bootstrap.Load(writeFile(t, "api-notifications.yaml", withNotifications), bootstrap.API); err == nil {
		t.Fatal("API Config with unused Notifications section succeeded")
	}
}

func TestLoadConfigFailsClosedWithoutLeakingSecretValues(t *testing.T) {
	t.Setenv("CONFIGRA_TEST_MYSQL_DSN", "dsn-value-sentinel")
	t.Setenv("CONFIGRA_TEST_OIDC_SECRET", "oidc-value-sentinel")
	t.Setenv("CONFIGRA_TEST_CLICKHOUSE_DSN", "clickhouse-value-sentinel")
	base := `
version: 1
listen: :8443
tls:
  certificate_file: cert.pem
  private_key_file: key.pem
  client_ca_file: client-ca.pem
mysql:
  dsn_env: CONFIGRA_TEST_MYSQL_DSN
key_provider:
  master_key_file: master-key
clickhouse:
  dsn_env: CONFIGRA_TEST_CLICKHOUSE_DSN
nats:
  urls: [nats://127.0.0.1:4222]
oidc:
  issuer: https://identity.example.com
  client_id: configra
  client_secret_env: CONFIGRA_TEST_OIDC_SECRET
  redirect_url: https://configra.example.com/auth/callback
  role_source: id_token
  role_claim: groups
  viewer_values: [viewer]
  admin_values: [admin]
`
	tests := map[string]string{
		"unknown field":       base + "surprise: true\n",
		"multiple documents":  base + "---\nversion: 1\n",
		"unsupported version": strings.Replace(base, "version: 1", "version: 2", 1),
		"invalid listen":      strings.Replace(base, "listen: :8443", "listen: missing-port", 1),
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := bootstrap.Load(writeFile(t, name+".yaml", document), bootstrap.Management)
			if err == nil {
				t.Fatal("Load succeeded")
			}
			if strings.Contains(err.Error(), "dsn-value-sentinel") || strings.Contains(err.Error(), "oidc-value-sentinel") ||
				strings.Contains(err.Error(), "clickhouse-value-sentinel") {
				t.Fatalf("error leaked a secret: %v", err)
			}
		})
	}
	withoutCA := strings.Replace(base, "  client_ca_file: client-ca.pem\n", "", 1)
	if _, err := bootstrap.Load(writeFile(t, "without-client-ca.yaml", withoutCA), bootstrap.Management); err != nil {
		t.Fatalf("Management Config using managed Client CAs failed: %v", err)
	}

	missingEnv := strings.Replace(base, "CONFIGRA_TEST_MYSQL_DSN", "CONFIGRA_MISSING_DSN", 1)
	if _, err := bootstrap.Load(writeFile(t, "missing-env.yaml", missingEnv), bootstrap.Management); err == nil || !strings.Contains(err.Error(), "CONFIGRA_MISSING_DSN") {
		t.Fatalf("missing env error = %v", err)
	}
	for name, notificationConfig := range map[string]string{
		"invalid-notification-cidr":  "notifications:\n  allowed_cidrs: [not-a-cidr]\n",
		"loopback-notification-cidr": "notifications:\n  allowed_cidrs: [127.0.0.0/8]\n",
		"wildcard-notification-host": "notifications:\n  allowed_internal_hosts: ['*.internal']\n",
	} {
		if _, err := bootstrap.Load(writeFile(t, name+".yaml", base+notificationConfig), bootstrap.Management); err == nil {
			t.Fatalf("%s succeeded", name)
		}
	}
}

func TestLoadMasterKeyReadsOneBase64EncodedAES256Key(t *testing.T) {
	want := bytes.Repeat([]byte{0x5a}, 32)
	path := writeFile(t, "master-key", base64.StdEncoding.EncodeToString(want)+"\n")
	got, err := bootstrap.LoadMasterKey(path)
	if err != nil {
		t.Fatalf("LoadMasterKey: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Master Key = %x", got)
	}

	for name, content := range map[string]string{
		"not base64":   "master-key-value-sentinel",
		"wrong length": base64.StdEncoding.EncodeToString([]byte("short")),
		"too large":    strings.Repeat("A", 1025),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := bootstrap.LoadMasterKey(writeFile(t, name, content))
			if err == nil {
				t.Fatal("LoadMasterKey succeeded")
			}
			if strings.Contains(err.Error(), content) || strings.Contains(err.Error(), "master-key-value-sentinel") {
				t.Fatalf("error leaked Master Key material: %v", err)
			}
		})
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}
