package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
	"github.com/viber-ops/configra/internal/vaultcrypto"
)

func newDoctorCommand() *cobra.Command {
	var path string
	var verify bool
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Read-only verification of all encrypted Vault revisions, CA keys and notification credentials",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !verify {
				return errors.New("doctor requires --verify-vault to scan all encrypted records (including CA keys and notification credentials)")
			}
			if timeout <= 0 {
				return errors.New("doctor --timeout must be positive, for example 10m")
			}
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()
			report, err := verifyRecovery(ctx, path)
			if err != nil {
				if ctx.Err() != nil {
					return fmt.Errorf("doctor incomplete (no success report): %w", ctx.Err())
				}
				return err
			}
			if err := json.NewEncoder(command.OutOrStdout()).Encode(report); err != nil {
				return errors.New("doctor could not write its success report")
			}
			return nil
		},
	}
	command.Flags().StringVar(&path, "config", "", "server or storage-only bootstrap YAML; DSN must point to the database to check")
	command.Flags().BoolVar(&verify, "verify-vault", false, "verify every encrypted record, including historical and inactive records")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time for database connection and full scan")
	_ = command.MarkFlagRequired("config")
	return command
}

func verifyRecovery(ctx context.Context, path string) (mysqlstore.EncryptionVerification, error) {
	var empty mysqlstore.EncryptionVerification
	config, err := bootstrap.LoadForMaintenance(path)
	if err != nil {
		return empty, err
	}
	store, err := openMaintenanceStore(ctx, config)
	if err != nil {
		return empty, err
	}
	defer store.Close()
	return store.VerifyEncryptedState(ctx)
}

func openMaintenanceStore(ctx context.Context, config bootstrap.Config) (*mysqlstore.Store, error) {
	masterKey, err := bootstrap.LoadMasterKey(config.KeyProvider.MasterKeyFile)
	if err != nil {
		return nil, err
	}
	provider, err := vaultcrypto.NewLocalKeyProvider(masterKey)
	clear(masterKey)
	if err != nil {
		return nil, err
	}
	// OpenAPI only verifies an existing schema. A recovery check must never create
	// an empty installation or run migrations on the supplied database.
	store, err := mysqlstore.OpenAPI(ctx, config.MySQLDSN(), provider)
	if err != nil {
		if errors.Is(err, vaultcrypto.ErrIntegrity) {
			return nil, errors.New("crypto Sentinel failed; check the Master Key and database backup; do not initialize or overwrite the database")
		}
		// DSN parser/driver errors may quote credentials or connection parameters.
		return nil, errors.New("cannot open initialized database; check DSN, connectivity, SELECT permissions and matching Configra schema version; no initialization or migration was attempted")
	}
	return store, nil
}
