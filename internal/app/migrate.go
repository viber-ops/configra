package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func newMigrateCommand() *cobra.Command {
	var path, database string
	var timeout time.Duration
	command := &cobra.Command{Use: "migrate", Short: "Initialize or upgrade the named database with a temporary migration identity", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if timeout <= 0 {
			return errors.New("migration timeout must be positive")
		}
		config, err := bootstrap.LoadForMaintenance(path)
		if err != nil {
			return err
		}
		parsed, err := mysql.ParseDSN(config.MySQLDSN())
		if err != nil || database == "" || parsed.DBName != database {
			return errors.New("migration database confirmation does not match the configured DSN")
		}
		key, err := bootstrap.LoadMasterKey(config.KeyProvider.MasterKeyFile)
		if err != nil {
			return err
		}
		provider, err := vaultcrypto.NewLocalKeyProvider(key)
		clear(key)
		if err != nil {
			return errors.New("invalid migration Master Key")
		}
		ctx, cancel := context.WithTimeout(command.Context(), timeout)
		defer cancel()
		store, err := mysqlstore.OpenManagement(ctx, config.MySQLDSN(), provider)
		if err != nil {
			return errors.New("migration failed; keep services stopped and check database privileges, matching Master Key and schema version")
		}
		defer store.Close()
		version, err := store.SchemaVersion(ctx)
		if err != nil {
			return errors.New("migration completed but schema verification failed")
		}
		if json.NewEncoder(command.OutOrStdout()).Encode(map[string]any{"schema_version": version, "database": database}) != nil {
			return errors.New("migration completed but receipt output failed")
		}
		return nil
	}}
	command.Flags().StringVar(&path, "config", "", "Storage-only bootstrap YAML with a temporary migration DSN")
	command.Flags().StringVar(&database, "confirm-database", "", "Exact database name to initialize or upgrade")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum migration duration")
	_ = command.MarkFlagRequired("config")
	_ = command.MarkFlagRequired("confirm-database")
	return command
}
