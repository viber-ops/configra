package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viber-ops/configra/internal/cli"
)

func TestCLIFlagsAreParsedByCobraWithoutLeakingInvalidValues(t *testing.T) {
	for _, args := range [][]string{
		{"--timeout", "private-argument-sentinel", "config", "get", "server"},
		{"private-argument-sentinel"},
		{"config", "put", "server", "--expected-revision", "private-argument-sentinel"},
	} {
		var stdout, stderr bytes.Buffer
		code := cli.Run(context.Background(), args, strings.NewReader(""), &stdout, &stderr, "test")
		if code != 2 || strings.Contains(stdout.String()+stderr.String(), "private-argument-sentinel") {
			t.Fatalf("usage code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run(context.Background(), []string{"--help"}, strings.NewReader(""), &stdout, &stderr, "test"); code != 0 || !strings.Contains(stdout.String(), "deployment-credential") {
		t.Fatalf("help: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLICommandDeadlineCancelsWaitingForSecretStdin(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	var stdout, stderr bytes.Buffer
	start := time.Now()
	code := cli.Run(context.Background(), []string{"config", "put", "server", "--file", "-", "--expected-revision", "0", "--operation-id", "stdin-timeout", "--timeout", "20ms"}, reader, &stdout, &stderr, "test")
	if code != 6 || !strings.Contains(stderr.String(), `"timeout"`) || time.Since(start) > time.Second {
		t.Fatalf("stdin deadline: %d %s", code, stderr.String())
	}
}

func TestCLIContextStoresPathsAndRejectsSensitiveInlineCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contexts.json")
	run := func(args ...string) (int, string) {
		var stdout, stderr bytes.Buffer
		code := cli.Run(context.Background(), append([]string{"--context-file", path}, args...), strings.NewReader(""), &stdout, &stderr, "test")
		return code, stdout.String() + stderr.String()
	}
	if code, output := run("context", "set", "testing", "--server", "https://configra.example.com", "--environment", "testing", "--token-file", "/run/secrets/token"); code != 0 {
		t.Fatalf("set context: %d %s", code, output)
	}
	if code, output := run("context", "use", "testing"); code != 0 {
		t.Fatalf("use context: %d %s", code, output)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private context permissions: %v", err)
	}
	if code, output := run("context", "show"); code != 0 || !strings.Contains(output, "https://configra.example.com") {
		t.Fatalf("show context: %d %s", code, output)
	}
	if code, output := run("context", "set", "testing", "--token", "private-argument-sentinel"); code != 2 || strings.Contains(output, "private-argument-sentinel") {
		t.Fatalf("inline token: %d %s", code, output)
	}
}
