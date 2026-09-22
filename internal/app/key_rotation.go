package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func newKeyRotationCommand() *cobra.Command {
	var path, nextKeyPath, database string
	var offline bool
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "rotate-master-key",
		Short: "Rewrap all stored encrypted data during a verified maintenance window",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !offline || database == "" {
				return errors.New("rotation requires --confirm-database and --confirm-offline: stop all replicas/automatic restarts and verify separate database/key backups first")
			}
			if timeout <= 0 {
				return errors.New("rotation --timeout must be positive, for example 30m")
			}
			config, err := bootstrap.LoadForMaintenance(path)
			if err != nil {
				return err
			}
			parsed, err := mysql.ParseDSN(config.MySQLDSN())
			if err != nil || parsed.DBName != database {
				return errors.New("--confirm-database must exactly match the database named by the configured DSN; nothing was changed")
			}
			key, err := bootstrap.LoadMasterKey(nextKeyPath)
			if err != nil {
				return err
			}
			next, err := vaultcrypto.NewLocalKeyProvider(key)
			clear(key)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()
			store, err := openMaintenanceStore(ctx, config)
			if err != nil {
				return err
			}
			defer store.Close()
			result, err := store.RotateMasterKey(ctx, next)
			if err != nil {
				if !errors.Is(err, mysqlstore.ErrRotationOutcomeUnknown) && ctx.Err() != nil {
					return fmt.Errorf("rotation did not commit: %w", ctx.Err())
				}
				return err
			}
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return errors.New("rotation COMMITTED but its report could not be written; verify with the new key before restarting replicas")
			}
			return nil
		},
	}
	command.Flags().StringVar(&path, "config", "", "bootstrap YAML pointing to the current Master Key")
	command.Flags().StringVar(&nextKeyPath, "new-key-file", "", "separately backed-up base64-encoded 32-byte replacement key; file is not modified")
	command.Flags().StringVar(&database, "confirm-database", "", "exact target database name (prevents rotating a mistakenly configured DSN)")
	command.Flags().BoolVar(&offline, "confirm-offline", false, "confirm all replicas/automatic restarts are stopped and database/key backups have been verified")
	command.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "maximum database connection, lock and rotation time")
	for _, flag := range []string{"config", "new-key-file", "confirm-database"} {
		_ = command.MarkFlagRequired(flag)
	}
	return command
}
