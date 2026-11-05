package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/spf13/cobra"
	"github.com/viber-ops/configra/internal/bootstrap"
	"github.com/viber-ops/configra/internal/storage/mysqlstore"
)

func newSessionsCommand() *cobra.Command {
	var path, issuer, subject, action, operation string
	var timeout time.Duration
	command := &cobra.Command{Use: "sessions", Short: "Invalidate or block an OIDC identity using controlled maintenance database access", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if timeout <= 0 {
			return errors.New("session maintenance timeout must be positive")
		}
		ctx, cancel := context.WithTimeout(command.Context(), timeout)
		defer cancel()
		config, err := bootstrap.LoadForMaintenance(path)
		if err != nil {
			return err
		}
		store, err := openMaintenanceStore(ctx, config)
		if err != nil {
			return err
		}
		defer store.Close()
		result, err := store.ChangeSessions(ctx, mysqlstore.SessionChange{OperationID: operation, Actor: mysqlstore.Actor{Type: "system", ID: "maintenance-session-control"}, Issuer: issuer, Subject: subject, Action: action})
		if err != nil {
			return errors.New("session change failed; check identity, action, operation identity and database privileges")
		}
		if json.NewEncoder(command.OutOrStdout()).Encode(result) != nil {
			return errors.New("session change committed but receipt output failed")
		}
		return nil
	}}
	flags := command.Flags()
	flags.StringVar(&path, "config", "", "Maintenance bootstrap YAML")
	flags.StringVar(&issuer, "issuer", "", "Exact configured OIDC issuer")
	flags.StringVar(&subject, "subject", "", "Exact OIDC sub claim (not display name/email unless the IdP uses it as sub)")
	flags.StringVar(&action, "action", "invalidate", "invalidate, block or unblock; every action invalidates older cookies")
	flags.StringVar(&operation, "operation-id", "", "Stable operation identity; reuse on uncertain completion")
	flags.DurationVar(&timeout, "timeout", 30*time.Second, "Complete database operation deadline")
	for _, name := range []string{"config", "issuer", "subject", "operation-id"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}
