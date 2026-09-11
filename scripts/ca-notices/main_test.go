package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCACollectorRejectsUnreviewedSourceAndPreservesExistingOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fixture := t.TempDir()
	tool := filepath.Join(fixture, "ca-notices")
	build := exec.CommandContext(ctx, "go", "build", "-o", tool, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build collector: %v\n%s", err, out)
	}
	source := filepath.Join(fixture, "source.tar.xz")
	if err := os.WriteFile(source, []byte("not the reviewed source"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(fixture, "materials")
	run := func() ([]byte, error) {
		return exec.CommandContext(ctx, tool, "-rootfs", fixture, "-source", source, "-out", output).CombinedOutput()
	}
	out, err := run()
	if err == nil || !strings.Contains(string(out), "source archive") || !strings.Contains(string(out), "reviewed SHA-256") {
		t.Fatalf("expected source rejection: %v %s", err, out)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("rejected source created output: %v", err)
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(output, "keep")
	if err := os.WriteFile(marker, []byte("existing material"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run()
	if err == nil || !strings.Contains(string(out), "must not already exist") {
		t.Fatalf("expected overwrite refusal: %v %s", err, out)
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "existing material" {
		t.Fatalf("existing output changed: %v", err)
	}
}
