package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/viber-ops/configra/internal/configdoc"
)

type ConfigValidation struct {
	EnvironmentKey string
	Format         configdoc.Format
	Content        []byte
}

type ConfigValidationResult struct {
	Format   configdoc.Format `json:"format"`
	Content  string           `json:"content"`
	Warnings []ConfigWarning  `json:"warnings,omitempty"`
}

func (store *Store) ValidateConfig(ctx context.Context, request ConfigValidation) (ConfigValidationResult, error) {
	if !validResourceKey(request.EnvironmentKey) {
		return ConfigValidationResult{}, ErrValidation
	}
	document, err := configdoc.Canonicalize(request.Format, request.Content)
	if err != nil {
		return ConfigValidationResult{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	transaction, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ConfigValidationResult{}, fmt.Errorf("begin Config validation: %w", err)
	}
	defer transaction.Rollback()
	var environmentID []byte
	err = transaction.QueryRowContext(ctx, `
		SELECT id FROM environments WHERE resource_key = ? AND archived_at IS NULL
	`, request.EnvironmentKey).Scan(&environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigValidationResult{}, ErrValidation
	}
	if err != nil {
		return ConfigValidationResult{}, fmt.Errorf("read Config validation Environment: %w", err)
	}
	warnings, err := configWarnings(ctx, transaction, environmentID, document.References)
	if err != nil {
		return ConfigValidationResult{}, err
	}
	return ConfigValidationResult{Format: request.Format, Content: string(document.Content), Warnings: warnings}, nil
}
