//go:build integration

package mysqlstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestJSONParametersPreserveMutationReplayAndRejectionAudit(t *testing.T) {
	for _, interpolate := range []bool{false, true} {
		for _, sqlMode := range []string{"", "NO_BACKSLASH_ESCAPES"} {
			t.Run(fmt.Sprintf("interpolate=%t/sqlMode=%s", interpolate, sqlMode), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				dsn, _, provider := pkiTestDatabase(t)
				config, err := mysql.ParseDSN(dsn)
				if err != nil {
					t.Fatal("invalid fixture DSN")
				}
				config.InterpolateParams = interpolate
				if sqlMode != "" {
					if config.Params == nil {
						config.Params = make(map[string]string)
					}
					config.Params["sql_mode"] = "'" + sqlMode + "'"
				}
				store, err := OpenManagement(ctx, config.FormatDSN(), provider)
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				actor := Actor{Type: "user", ID: "operator'\\中文"}
				t.Run("mutation and replay", func(t *testing.T) {
					request := EnvironmentChange{OperationID: "json-parameter-env", Actor: actor, Action: EnvironmentCreate,
						Key: "prod", DisplayName: "Production"}
					first, err := store.ApplyEnvironmentChange(ctx, request)
					if err != nil {
						t.Fatal(err)
					}
					replayed, err := store.ApplyEnvironmentChange(ctx, request)
					if err != nil || replayed != first {
						t.Fatal("JSON Operation response must replay unchanged", err)
					}
				})
				t.Run("rejection receipt", func(t *testing.T) {
					if err := store.RecordRejectedMutation(ctx, RejectedMutation{RequestID: strings.Repeat("a", 24), Actor: actor,
						Action: "config.commit", ErrorCode: "invalid_request", ResourceType: "config"}); err != nil {
						t.Fatal(err)
					}
				})
				events, err := store.ClaimOutbox(ctx, "audit", 10, time.Minute)
				if err != nil || len(events) != 2 {
					t.Fatalf("expected exactly two durable Audit events, got %d: %v", len(events), err)
				}
				for _, event := range events {
					var payload struct{ Actor Actor }
					if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Actor != actor {
						t.Fatal("JSON Audit parameters did not preserve the actor")
					}
				}
			})
		}
	}
}
