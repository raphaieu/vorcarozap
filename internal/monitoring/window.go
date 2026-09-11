package monitoring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// Erros sentinela para determinação da janela temporal.
var (
	ErrInvalidWindowDuration = errors.New("monitoring: duração da janela de monitoramento inválida")
)

// Window representa os limites temporais em UTC de uma execução de monitoramento.
type Window struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// StartISO retorna o início da janela formatado em ISO-8601 UTC.
func (w Window) StartISO() string {
	return w.Start.Format(time.RFC3339Nano)
}

// EndISO retorna o término da janela formatado em ISO-8601 UTC.
func (w Window) EndISO() string {
	return w.End.Format(time.RFC3339Nano)
}

// DetermineWindow calcula deterministicamente a janela incremental:
// 1. window_end é fixado no instante atual em UTC;
// 2. default_start = window_end - windowDuration;
// 3. Consulta a última execução com status exclusivamente 'completed' para a mesma query;
// 4. Utiliza o window_end persistido dessa última execução concluída como watermark;
// 5. Se o watermark existir e for posterior a default_start (e não estiver no futuro), window_start = watermark;
// 6. Garante estritamente que window_start < window_end e não seja uma data futura.
func DetermineWindow(ctx context.Context, queries *sqlc.Queries, query string, now time.Time, windowDuration time.Duration) (Window, error) {
	if windowDuration <= 0 {
		return Window{}, ErrInvalidWindowDuration
	}

	windowEnd := now.UTC()
	defaultStart := windowEnd.Add(-windowDuration)
	windowStart := defaultStart

	if queries != nil && query != "" {
		lastRun, err := queries.GetLastSuccessfulMonitoringRunByQuery(ctx, query)
		if err == nil {
			var lastWatermark time.Time
			var parseErr error

			// Utiliza exclusivamente o window_end persistido da última execução 'completed'
			if lastRun.WindowEnd.Valid && lastRun.WindowEnd.String != "" {
				lastWatermark, parseErr = time.Parse(time.RFC3339Nano, lastRun.WindowEnd.String)
				if parseErr != nil {
					lastWatermark, parseErr = time.Parse(time.RFC3339, lastRun.WindowEnd.String)
				}
			}

			if parseErr == nil && !lastWatermark.IsZero() {
				lastWatermark = lastWatermark.UTC()
				// Proteção contra anomalias de relógio (timestamp no futuro ou posterior ao agora)
				if !lastWatermark.After(windowEnd) && lastWatermark.After(defaultStart) {
					windowStart = lastWatermark
				}
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Window{}, fmt.Errorf("monitoring: falha ao consultar histórico da janela no banco: %w", err)
		}
	}

	// Invariante de segurança: window_start deve ser estritamente anterior a window_end
	if !windowStart.Before(windowEnd) {
		windowStart = defaultStart
	}

	return Window{
		Start: windowStart,
		End:   windowEnd,
	}, nil
}
