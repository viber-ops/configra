package main

import (
	"os"
	"testing"
)

func TestMetadataFlagsDoNotRequireCluster(t *testing.T) {
	previous := os.Args
	t.Cleanup(func() { os.Args = previous })
	for _, flag := range []string{"--version", "version", "--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			os.Args = []string{"configra-kubernetes", flag}
			if err := run(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
