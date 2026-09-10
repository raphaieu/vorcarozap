package sourcecheck

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// PersistCheckResult atualiza o registro da fonte no banco de dados após a conclusão da chamada de rede.
// A atualização ocorre sob escopo transacional isolado e rápido, garantindo que nenhuma transação SQLite
// permaneça aberta durante operações de I/O de rede.
func PersistCheckResult(ctx context.Context, db *sql.DB, sourceID string, currentStatus domain.SourceAccessStatus, res CheckResult) (*sqlc.Source, error) {
	if db == nil {
		return nil, fmt.Errorf("sourcecheck: db não pode ser nulo")
	}

	newStatus := ResolveStatusUpdate(currentStatus, res)
	checkedAtStr := res.CheckedAt.Format(time.RFC3339Nano)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	var httpStatus sql.NullInt64
	if res.HTTPStatus > 0 {
		httpStatus = sql.NullInt64{
			Int64: int64(res.HTTPStatus),
			Valid: true,
		}
	}

	var normErr sql.NullString
	if res.NormalizedErrorCode != "" {
		normErr = sql.NullString{
			String: res.NormalizedErrorCode,
			Valid:  true,
		}
	}

	q := sqlc.New(db)
	updated, err := q.UpdateSourceAccessStatus(ctx, sqlc.UpdateSourceAccessStatusParams{
		SourceAccessStatus:    string(newStatus),
		SourceAccessCheckedAt: sql.NullString{String: checkedAtStr, Valid: true},
		HttpStatus:            httpStatus,
		NormalizedErrorCode:   normErr,
		UpdatedAt:             nowStr,
		ID:                    sourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("sourcecheck: falha ao atualizar status da fonte %s: %w", sourceID, err)
	}

	return &updated, nil
}

// CheckAndPersist coordena a execução segura do GET e a persistência do resultado,
// assegurando que a chamada de rede seja realizada totalmente fora do banco.
func CheckAndPersist(ctx context.Context, v *Verifier, db *sql.DB, sourceID string, rawURL string, currentStatus domain.SourceAccessStatus) (CheckResult, error) {
	if v == nil {
		return CheckResult{}, fmt.Errorf("sourcecheck: verifier não pode ser nulo")
	}

	// 1. Chamada de rede sem transações abertas
	res := v.Check(ctx, rawURL)

	// 2. Persistência transacional rápida do resultado
	if db != nil && sourceID != "" {
		if _, err := PersistCheckResult(ctx, db, sourceID, currentStatus, res); err != nil {
			return res, err
		}
	}

	return res, nil
}
