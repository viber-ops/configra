package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func newPruneCommand() *cobra.Command {
	var path, database, before, archive, operation string
	var limit int
	var apply bool
	var timeout time.Duration
	command := &cobra.Command{Use: "prune-outbox", Short: "Preview or archive and prune one completed delivery batch; keep all Operations and resource history", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		cutoff, err := time.Parse(time.RFC3339, before)
		if err != nil || timeout <= 0 || (apply && (archive == "" || operation == "")) {
			return errors.New("provide an explicit past RFC3339 --before, positive timeout, and --archive/--operation-id with --apply")
		}
		ctx, cancel := context.WithTimeout(command.Context(), timeout)
		defer cancel()
		config, err := bootstrap.LoadForMetadataMaintenance(path)
		if err != nil {
			return err
		}
		maintenance, err := mysqlstore.OpenOutboxMaintenance(ctx, config.MySQLDSN(), database)
		if err != nil {
			return err
		}
		defer maintenance.Close()
		absolute, err := filepath.Abs(archive)
		if err != nil {
			return errors.New("invalid archive destination")
		}
		identity := sha256.Sum256([]byte(absolute))
		result, err := maintenance.Prune(ctx, mysqlstore.OutboxPrune{OperationID: operation, Before: cutoff, Limit: limit, ArchiveID: hex.EncodeToString(identity[:]), Apply: apply}, func(data []byte) error { return saveOutboxArchive(absolute, data) })
		if err != nil {
			return err
		}
		if json.NewEncoder(command.OutOrStdout()).Encode(result) != nil {
			return errors.New("cannot write maintenance receipt; replay the same operation if apply was requested")
		}
		return nil
	}}
	flags := command.Flags()
	flags.StringVar(&path, "config", "", "Database-only bootstrap YAML")
	flags.StringVar(&database, "confirm-database", "", "Exact target database name")
	flags.StringVar(&before, "before", "", "Fixed RFC3339 retention cutoff; retain it across retries")
	flags.IntVar(&limit, "limit", 100, "Maximum events in one transaction (1..1000)")
	flags.BoolVar(&apply, "apply", false, "Archive durably and delete eligible records; default is preview only")
	flags.StringVar(&archive, "archive", "", "Private archive file; existing identical bytes are reusable after an uncertain commit")
	flags.StringVar(&operation, "operation-id", "", "Stable operation identity for this batch")
	flags.DurationVar(&timeout, "timeout", time.Minute, "Bounded maintenance deadline")
	for _, name := range []string{"config", "confirm-database", "before"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

func saveOutboxArchive(path string, data []byte) error {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return errors.New("archive requires a directory not writable by group or others")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("existing archive must be a private regular file")
		}
		file, err := os.Open(path)
		if err != nil {
			return errors.New("cannot verify existing archive")
		}
		defer file.Close()
		existing, err := io.ReadAll(io.LimitReader(file, int64(len(data))+1))
		if err != nil || !bytes.Equal(existing, data) {
			return errors.New("existing archive differs; no overwrite is permitted")
		}
		return syncArchiveDirectory(parent)
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect archive")
	}
	file, err := os.CreateTemp(parent, ".outbox-archive-*")
	if err != nil {
		return errors.New("cannot create archive")
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("cannot persist archive")
	}
	// Link is an atomic no-overwrite publication, including against symlink paths.
	if os.Link(file.Name(), path) != nil {
		return errors.New("cannot publish archive without overwrite")
	}
	return syncArchiveDirectory(parent)
}

func syncArchiveDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return errors.New("cannot sync archive directory")
	}
	defer directory.Close()
	if directory.Sync() != nil {
		return errors.New("cannot sync archive directory")
	}
	return nil
}
