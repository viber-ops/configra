package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCollectorRejectsChangedPackageScopeAndPreservesExistingOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixture := t.TempDir()
	tool := filepath.Join(fixture, "collector")
	overrides := map[string]string{"GOTOOLCHAIN": "go1.26.7", "GOWORK": "off", "GOFLAGS": "-mod=readonly", "CGO_ENABLED": "0",
		"GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH, "GOAMD64": "v1", "GOARM64": "v8.0", "GOEXPERIMENT": ""}
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, changed := overrides[key]; !changed {
			env = append(env, entry)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	run := func(directory, command string, args ...string) ([]byte, error) {
		process := exec.CommandContext(ctx, command, args...)
		process.Dir, process.Env = directory, env
		return process.CombinedOutput()
	}
	if output, err := run(".", "go", "build", "-o", tool, "."); err != nil {
		t.Fatalf("build collector: %v\n%s", err, output)
	}
	commandDir := filepath.Join(fixture, "cmd", "configra")
	if err := os.MkdirAll(commandDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "go.mod"), []byte("module github.com/viber-ops/configra\n\ngo 1.25.13\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main(){fmt.Println(\"different application graph\")}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	subject := filepath.Join(fixture, "configra")
	if output, err := run(fixture, "go", "build", "-trimpath", "-buildvcs=false", "-o", subject, "./cmd/configra"); err != nil {
		t.Fatalf("build subject: %v\n%s", err, output)
	}
	destination := filepath.Join(fixture, "delivery")
	output, err := run(fixture, tool, "-binary", subject, "-out", destination)
	if err == nil || !strings.Contains(string(output), "package selection changed") {
		t.Fatalf("expected review rejection, got %v: %s", err, output)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("rejected collection created output: %v", err)
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep")
	if err := os.WriteFile(marker, []byte("existing material"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err = run(fixture, tool, "-binary", subject, "-out", destination)
	if err == nil || !strings.Contains(string(output), "must not already exist") {
		t.Fatalf("expected overwrite refusal, got %v: %s", err, output)
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "existing material" {
		t.Fatalf("existing output was altered: %v", err)
	}
}
