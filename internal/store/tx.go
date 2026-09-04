package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// ExecTx executa uma função de forma transacional garantindo rollback em caso de falha
// e commit em caso de sucesso. As queries executadas dentro de fn utilizam a transação ativa.
func ExecTx(ctx context.Context, db *sql.DB, fn func(q *sqlc.Queries) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: falha ao iniciar transação: %w", err)
	}

	q := sqlc.New(tx)
	if err := fn(q); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("store: erro na transação (%v) e falha no rollback (%w)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: falha ao comitar transação: %w", err)
	}

	return nil
}
