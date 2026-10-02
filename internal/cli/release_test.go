package cli_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/cli"
)

func TestReleaseValidatorDeadlineAndMutationNeverActivate(t *testing.T) {
	var activations atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			activations.Add(1)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("ETag", `"fixture-release"`)
		io.WriteString(w, `{"manifest":{"environment":"prod","config_key":"server","release_key":"first","config_revision":1,"config_path":"server.yaml","files":[],"vault_revisions":{}},"generation":0,"digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","config":{"format":"yaml","content":"version: 1\n","config_revision":1,"vault_revisions":{}},"files":[]}`)
	}))
	defer server.Close()
	directory := t.TempDir()
	token := filepath.Join(directory, "token")
	ca := filepath.Join(directory, "ca.pem")
	if os.WriteFile(token, []byte("cfg_fixture1_"+base64.RawURLEncoding.EncodeToString(make([]byte, 32))), 0600) != nil {
		t.Fatal("token fixture")
	}
	if os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600) != nil {
		t.Fatal("trust fixture")
	}
	for _, test := range []struct {
		name, script, timeout string
		code                  int
	}{{"modified", "printf 'private-validator-value' > \"$CONFIGRA_CONFIG_FILE\"\nprintf 'private-validator-output'\n", "30s", 5}, {"canceled", "printf 'private-validator-output'\nexec sleep 10\n", "250ms", 6}} {
		t.Run(test.name, func(t *testing.T) {
			validator := filepath.Join(directory, test.name)
			if os.WriteFile(validator, []byte("#!/bin/sh\n"+test.script), 0700) != nil {
				t.Fatal("validator fixture")
			}
			args := []string{"--context-file", filepath.Join(directory, "contexts.json"), "--server", server.URL, "--environment", "prod", "--token-file", token, "--client-cert", "", "--client-key", "", "--server-ca", ca, "--timeout", test.timeout, "release", "activate", "server", "first", "--expected-generation", "0", "--operation-id", "fixture-operation", "--validator", "/bin/sh", "--validator-arg", validator}
			var stdout, stderr bytes.Buffer
			began := time.Now()
			code := cli.Run(context.Background(), args, strings.NewReader(""), &stdout, &stderr, "fixture")
			if code != test.code || activations.Load() != 0 || strings.Contains(stdout.String()+stderr.String(), "private-validator") || (test.name == "canceled" && time.Since(began) > 2*time.Second) {
				t.Fatalf("validation boundary: code=%d, %s", code, stderr.String())
			}
		})
	}
}
