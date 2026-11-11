package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode"

	"github.com/viber-ops/configra/internal/vaultdoc"
)

// configractl repeats the server's edge-whitespace rule so it does not link vaultdoc; the two must not drift (#62).
func TestEdgeSpaceMatchesTheServerRule(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if edgeSpace(r) != vaultdoc.IsEdgeSpace(r) {
			t.Fatalf("U+%04X: configractl %v, server %v", r, edgeSpace(r), vaultdoc.IsEdgeSpace(r))
		}
	}
}

func TestSecretFieldPutWarnsAboutEdgeWhitespaceWithoutTheValue(t *testing.T) {
	for _, test := range []struct {
		kind, value string
		warn        bool
	}{
		{"secret", "s3cr3t-value\n", true},
		{"secret", "　s3cr3t-value", true},
		{"secret", "s3cr3t value", false},
		{"text", " http://service.example.com:8918\n", false},
	} {
		var stdout, stderr bytes.Buffer
		Run(context.Background(), []string{"--context-file", t.TempDir() + "/missing.json", "vault", "field", "put", "ops", "app", "password", "--type", test.kind, "--file", "-", "--expected-revision", "0", "--operation-id", "edge-warning"}, strings.NewReader(test.value), &stdout, &stderr, "test")
		if got := strings.HasPrefix(stderr.String(), `{"warning":"secret_edge_whitespace"}`+"\n"); got != test.warn {
			t.Errorf("%s %q: warning = %v; stderr %q", test.kind, test.value, got, stderr.String())
		}
		if strings.Contains(stderr.String()+stdout.String(), "s3cr3t") {
			t.Errorf("%s %q: output repeats the value", test.kind, test.value)
		}
	}
}
