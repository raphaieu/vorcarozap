package monitoring_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestDetermineWindow(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)

	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	defaultDuration := 24 * time.Hour

	t.Run("First run without prior history", func(t *testing.T) {
		w, err := monitoring.DetermineWindow(ctx, queries, "nova query", now, defaultDuration)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		expectedStart := now.Add(-defaultDuration)
		if !w.Start.Equal(expectedStart) {
			t.Errorf("Start: esperado %v, obtido %v", expectedStart, w.Start)
		}
		if !w.End.Equal(now) {
			t.Errorf("End: esperado %v, obtido %v", now, w.End)
		}
	})

	t.Run("Subsequent run after completed run (recent)", func(t *testing.T) {
		query := "query frequente"
		completedWindowEnd := now.Add(-6 * time.Hour) // 6 horas atrás
		completedStr := completedWindowEnd.Format(time.RFC3339Nano)

		_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
			ID:                   uuid.NewString(),
			Status:               "completed",
			Query:                query,
			WindowStart:          sql.NullString{String: now.Add(-24 * time.Hour).Format(time.RFC3339Nano), Valid: true},
			WindowEnd:            sql.NullString{String: completedStr, Valid: true},
			DiscoveryProvider:    "openrouter",
			DiscoveryModel:       "openai/gpt-4.1-mini",
			VerificationProvider: "openrouter",
			VerificationModel:    "openai/gpt-4.1-mini",
			CreatedAt:            completedStr,
			CompletedAt:          sql.NullString{String: completedStr, Valid: true},
		})
		if err != nil {
			t.Fatalf("falha ao criar run concluído: %v", err)
		}

		w, err := monitoring.DetermineWindow(ctx, queries, query, now, defaultDuration)
		if err != nil {
			t.Fatalf("erro ao determinar janela: %v", err)
		}

		// A nova janela deve iniciar a partir do window_end do run anterior
		if !w.Start.Equal(completedWindowEnd) {
			t.Errorf("Start esperado %v (window_end do run anterior), obtido %v", completedWindowEnd, w.Start)
		}
		if !w.End.Equal(now) {
			t.Errorf("End esperado %v, obtido %v", now, w.End)
		}
	})

	t.Run("Partial and Failed runs do not advance window", func(t *testing.T) {
		query := "query com parcial e falha previa"

		// 1. Run completed 18h atrás
		cTime := now.Add(-18 * time.Hour)
		_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
			ID:                   uuid.NewString(),
			Status:               "completed",
			Query:                query,
			WindowStart:          sql.NullString{String: now.Add(-30 * time.Hour).Format(time.RFC3339Nano), Valid: true},
			WindowEnd:            sql.NullString{String: cTime.Format(time.RFC3339Nano), Valid: true},
			DiscoveryProvider:    "openrouter",
			DiscoveryModel:       "openai/gpt-4.1-mini",
			VerificationProvider: "openrouter",
			VerificationModel:    "openai/gpt-4.1-mini",
			CreatedAt:            cTime.Format(time.RFC3339Nano),
			CompletedAt:          sql.NullString{String: cTime.Format(time.RFC3339Nano), Valid: true},
		})
		if err != nil {
			t.Fatalf("falha ao criar run completed: %v", err)
		}

		// 2. Run partial 6h atrás (não deve avançar a janela!)
		pTime := now.Add(-6 * time.Hour)
		_, err = queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
			ID:                   uuid.NewString(),
			Status:               "partial",
			Query:                query,
			WindowStart:          sql.NullString{String: cTime.Format(time.RFC3339Nano), Valid: true},
			WindowEnd:            sql.NullString{String: pTime.Format(time.RFC3339Nano), Valid: true},
			DiscoveryProvider:    "openrouter",
			DiscoveryModel:       "openai/gpt-4.1-mini",
			VerificationProvider: "openrouter",
			VerificationModel:    "openai/gpt-4.1-mini",
			CreatedAt:            pTime.Format(time.RFC3339Nano),
			CompletedAt:          sql.NullString{String: pTime.Format(time.RFC3339Nano), Valid: true},
		})
		if err != nil {
			t.Fatalf("falha ao criar run partial: %v", err)
		}

		// 3. Run failed 2h atrás
		fTime := now.Add(-2 * time.Hour)
		_, err = queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
			ID:                   uuid.NewString(),
			Status:               "failed",
			Query:                query,
			WindowStart:          sql.NullString{String: cTime.Format(time.RFC3339Nano), Valid: true},
			WindowEnd:            sql.NullString{String: fTime.Format(time.RFC3339Nano), Valid: true},
			DiscoveryProvider:    "openrouter",
			DiscoveryModel:       "openai/gpt-4.1-mini",
			VerificationProvider: "openrouter",
			VerificationModel:    "openai/gpt-4.1-mini",
			CreatedAt:            fTime.Format(time.RFC3339Nano),
			CompletedAt:          sql.NullString{String: fTime.Format(time.RFC3339Nano), Valid: true},
		})
		if err != nil {
			t.Fatalf("falha ao criar run failed: %v", err)
		}

		w, err := monitoring.DetermineWindow(ctx, queries, query, now, defaultDuration)
		if err != nil {
			t.Fatalf("erro ao determinar janela: %v", err)
		}

		// Os runs partial e failed DEVEM ser ignorados; a janela parte do window_end do run completed (18h atrás)
		if !w.Start.Equal(cTime) {
			t.Errorf("Start esperado %v (run completed ignorando partial e failed), obtido %v", cTime, w.Start)
		}
	})

	t.Run("Old run (>24h) is capped to default window", func(t *testing.T) {
		query := "query antiga"
		oldTime := now.Add(-72 * time.Hour) // 3 dias atrás

		_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
			ID:                   uuid.NewString(),
			Status:               "completed",
			Query:                query,
			WindowStart:          sql.NullString{String: now.Add(-96 * time.Hour).Format(time.RFC3339Nano), Valid: true},
			WindowEnd:            sql.NullString{String: oldTime.Format(time.RFC3339Nano), Valid: true},
			DiscoveryProvider:    "openrouter",
			DiscoveryModel:       "openai/gpt-4.1-mini",
			VerificationProvider: "openrouter",
			VerificationModel:    "openai/gpt-4.1-mini",
			CreatedAt:            oldTime.Format(time.RFC3339Nano),
			CompletedAt:          sql.NullString{String: oldTime.Format(time.RFC3339Nano), Valid: true},
		})
		if err != nil {
			t.Fatalf("falha ao criar run antigo: %v", err)
		}

		w, err := monitoring.DetermineWindow(ctx, queries, query, now, defaultDuration)
		if err != nil {
			t.Fatalf("erro ao determinar janela: %v", err)
		}

		expectedStart := now.Add(-defaultDuration)
		if !w.Start.Equal(expectedStart) {
			t.Errorf("Start esperado %v (teto de 24h), obtido %v", expectedStart, w.Start)
		}
	})

	t.Run("Future timestamp anomaly falls back to default window", func(t *testing.T) {
		query := "query anomala"
		futureTime := now.Add(5 * time.Hour) // Anomalia de relógio

		_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
			ID:                   uuid.NewString(),
			Status:               "completed",
			Query:                query,
			WindowStart:          sql.NullString{String: now.Format(time.RFC3339Nano), Valid: true},
			WindowEnd:            sql.NullString{String: futureTime.Format(time.RFC3339Nano), Valid: true},
			DiscoveryProvider:    "openrouter",
			DiscoveryModel:       "openai/gpt-4.1-mini",
			VerificationProvider: "openrouter",
			VerificationModel:    "openai/gpt-4.1-mini",
			CreatedAt:            futureTime.Format(time.RFC3339Nano),
			CompletedAt:          sql.NullString{String: futureTime.Format(time.RFC3339Nano), Valid: true},
		})
		if err != nil {
			t.Fatalf("falha ao criar run anômalo: %v", err)
		}

		w, err := monitoring.DetermineWindow(ctx, queries, query, now, defaultDuration)
		if err != nil {
			t.Fatalf("erro ao determinar janela: %v", err)
		}

		expectedStart := now.Add(-defaultDuration)
		if !w.Start.Equal(expectedStart) {
			t.Errorf("Start esperado %v (fallback seguro contra futuro), obtido %v", expectedStart, w.Start)
		}
	})
}
