package main

import (
	"os"
	"testing"
)

func TestVersionDoesNotStartServer(t *testing.T) {
	previous := os.Args
	os.Args = []string{"configra", "--version"}
	t.Cleanup(func() { os.Args = previous })
	if code := run(); code != 0 {
		t.Fatalf("version exit code = %d", code)
	}
}
