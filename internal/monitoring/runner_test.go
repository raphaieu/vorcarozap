package monitoring_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestRunnerCompleteFlow(t *testing.T) {
	db, ctx := setupTestDB(t)
	seedEntity(t, db, "ent-1", "Daniel Vorcaro", "person")
	seedCase(t, db, "case-1", "Operação Master")

	provider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			if input.WindowStart == "" || input.WindowEnd == "" {
				t.Errorf("esperava limites de janela temporal em DiscoverInput, obtido start=%q end=%q",
					input.WindowStart, input.WindowEnd)
			}
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:          "Daniel Vorcaro",
						CaseName:            "Operação Master",
						RelationshipType:    "investigado",
						Proposition:         "Investigado em inquérito sobre movimentações financeiras",
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com/materia-1",
						SourceTitle:         "Inquérito Policial",
						PublisherOrAuthor:   "STF",
						PublishedAt:         "2026-09-05",
						Excerpt:             "Conforme autos do STF...",
						Locator:             "Fls 12",
						TechnicalConfidence: 0.95,
					},
					{
						EntityName:          "Daniel Vorcaro",
						CaseName:            "Operação Master",
						RelationshipType:    "investigado",
						Proposition:         "Investigado em inquérito sobre movimentações financeiras", // Mesma proposição -> duplicata
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com/materia-1",
						Excerpt:             "Conforme autos do STF...",
						TechnicalConfidence: 0.95,
					},
				},
				Content:          `{"candidates":[...]}`,
				Model:            "openai/gpt-4.1-mini",
				PromptTokens:     120,
				CompletionTokens: 60,
				TotalTokens:      180,
				Cost:             0.0012,
				CostMicros:       1200,
				WebSearchCalls:   1,
			}, nil
		},
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
				Model:                    "openai/gpt-4.1-mini",
				PromptTokens:             80,
				CompletionTokens:         30,
				TotalTokens:              110,
				Cost:                     0.0008,
				CostMicros:               800,
			}, nil
		},
	}

	verifier := &mockSourceVerifier{}

	runner, err := monitoring.NewRunner(monitoring.RunnerConfig{
		DB:                     db,
		Provider:               provider,
		SourceVerifier:         verifier,
		DiscoveryModel:         "openai/gpt-4.1-mini",
		VerificationModel:      "openai/gpt-4.1-mini",
		MonitorWindow:          24 * time.Hour,
		MaxCandidatesPerRun:    10,
		MaxVerificationsPerRun: 10,
		MaxCostPerRunUSD:       0.25,
		MaxCostPerDayUSD:       1.00,
	})
	if err != nil {
		t.Fatalf("erro ao criar Runner: %v", err)
	}

	summary, err := runner.Run(ctx, "Daniel Vorcaro")
	if err != nil {
		t.Fatalf("falha ao executar runner: %v", err)
	}

	if summary.Status != "completed" {
		t.Errorf("status esperado 'completed', obtido %q", summary.Status)
	}
	if summary.TotalCandidates != 2 {
		t.Errorf("TotalCandidates esperado 2, obtido %d", summary.TotalCandidates)
	}
	if summary.UniqueCandidates != 1 {
		t.Errorf("UniqueCandidates esperado 1, obtido %d", summary.UniqueCandidates)
	}
	if summary.DuplicateCandidates != 1 {
		t.Errorf("DuplicateCandidates esperado 1, obtido %d", summary.DuplicateCandidates)
	}
	if summary.VerificationsExecuted != 1 {
		t.Errorf("VerificationsExecuted esperado 1, obtido %d", summary.VerificationsExecuted)
	}
	if summary.PublishedCount != 1 {
		t.Errorf("PublishedCount esperado 1, obtido %d", summary.PublishedCount)
	}
	if summary.DiscoveryCostUSD != 0.0012 {
		t.Errorf("DiscoveryCostUSD esperado 0.0012, obtido %v", summary.DiscoveryCostUSD)
	}
	if summary.VerificationCostUSD != 0.0008 {
		t.Errorf("VerificationCostUSD esperado 0.0008, obtido %v", summary.VerificationCostUSD)
	}
	if summary.TotalTokens != 290 {
		t.Errorf("TotalTokens esperado 290 (180+110), obtido %d", summary.TotalTokens)
	}
	if summary.PromptTokens != 200 { // 120 + 80
		t.Errorf("PromptTokens esperado 200, obtido %d", summary.PromptTokens)
	}
	if summary.CompletionTokens != 90 { // 60 + 30
		t.Errorf("CompletionTokens esperado 90, obtido %d", summary.CompletionTokens)
	}

	// Garante que o lock foi liberado após o término
	queries := sqlc.New(db)
	_, lockErr := queries.GetMonitoringLock(ctx, "monitoring")
	if !errors.Is(lockErr, sql.ErrNoRows) {
		t.Errorf("lock deveria ter sido liberado após o defer, erro: %v", lockErr)
	}
}

func TestRunnerMaxCandidatesPerRun(t *testing.T) {
	db, ctx := setupTestDB(t)
	seedEntity(t, db, "ent-1", "Daniel Vorcaro", "person")
	seedCase(t, db, "case-1", "Operação Master")

	// Provedor retorna 4 candidatos únicos
	provider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:        "Daniel Vorcaro",
						CaseName:          "Operação Master",
						RelationshipType:  "investigado",
						Proposition:       "Fato 1",
						SuggestedGrade:    "A",
						SourceURL:         "https://noticias.exemplo.com/materia-1",
						SourceTitle:       "Notícia 1",
						PublisherOrAuthor: "STF",
						Excerpt:           "Trecho 1",
					},
					{
						EntityName:        "Daniel Vorcaro",
						CaseName:          "Operação Master",
						RelationshipType:  "investigado",
						Proposition:       "Fato 2",
						SuggestedGrade:    "A",
						SourceURL:         "https://noticias.exemplo.com/materia-2",
						SourceTitle:       "Notícia 2",
						PublisherOrAuthor: "STF",
						Excerpt:           "Trecho 2",
					},
					{
						EntityName:        "Daniel Vorcaro",
						CaseName:          "Operação Master",
						RelationshipType:  "investigado",
						Proposition:       "Fato 3",
						SuggestedGrade:    "A",
						SourceURL:         "https://noticias.exemplo.com/materia-3",
						SourceTitle:       "Notícia 3",
						PublisherOrAuthor: "STF",
						Excerpt:           "Trecho 3",
					},
					{
						EntityName:        "Daniel Vorcaro",
						CaseName:          "Operação Master",
						RelationshipType:  "investigado",
						Proposition:       "Fato 4",
						SuggestedGrade:    "A",
						SourceURL:         "https://noticias.exemplo.com/materia-4",
						SourceTitle:       "Notícia 4",
						PublisherOrAuthor: "STF",
						Excerpt:           "Trecho 4",
					},
				},
				Model:       "openai/gpt-4.1-mini",
				TotalTokens: 100,
				Cost:        0.01,
				CostMicros:  10000,
			}, nil
		},
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
				Model:                    "openai/gpt-4.1-mini",
				TotalTokens:              50,
				Cost:                     0.005,
				CostMicros:               5000,
			}, nil
		},
	}

	verifier := &mockSourceVerifier{}

	// MaxCandidatesPerRun = 2 (deve avaliar apenas 2 e marcar partial)
	runner, err := monitoring.NewRunner(monitoring.RunnerConfig{
		DB:                     db,
		Provider:               provider,
		SourceVerifier:         verifier,
		MaxCandidatesPerRun:    2,
		MaxVerificationsPerRun: 10,
	})
	if err != nil {
		t.Fatalf("erro ao criar Runner: %v", err)
	}

	summary, err := runner.Run(ctx, "teste teto de candidatos")
	if err != nil {
		t.Fatalf("falha ao executar runner: %v", err)
	}

	if summary.Status != "partial" {
		t.Errorf("status esperado 'partial' ao exceder teto de candidatos, obtido %q", summary.Status)
	}
	if summary.VerificationsExecuted != 2 {
		t.Errorf("esperava 2 verificações executadas, obtido %d", summary.VerificationsExecuted)
	}
	if !strings.Contains(summary.TechnicalSummary, "limite de candidatos por execução atingido (2 de 4 processados)") {
		t.Errorf("resumo técnico deveria mencionar teto de candidatos, obtido: %s", summary.TechnicalSummary)
	}

	// Todos os 4 candidatos devem estar no banco (nenhum descartado silenciosamente)
	queries := sqlc.New(db)
	candidates, err := queries.ListCandidatesByRunID(ctx, summary.RunID)
	if err != nil || len(candidates) != 4 {
		t.Fatalf("esperava todos os 4 candidatos salvos no banco, obtido %d", len(candidates))
	}

	// Os 2 primeiros foram publicados, os 2 restantes permaneceram em quarentena
	if candidates[0].EditorialStatus != "published" || candidates[1].EditorialStatus != "published" {
		t.Errorf("os 2 primeiros candidatos deveriam estar publicados")
	}
	if candidates[2].EditorialStatus != "quarantined" || candidates[3].EditorialStatus != "quarantined" {
		t.Errorf("os candidatos excedentes deveriam permanecer em quarentena auditável")
	}
}

func TestRunnerPreservesCostAndTokensOnFailure(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Descoberta consome tokens e gera custo real, mas retorna candidato inválido gerando falha
	provider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:     "", // INVÁLIDO -> falha na validação
						Proposition:    "Fato",
						SuggestedGrade: "A",
						SourceURL:      "https://example.com/item",
					},
				},
				Model:            "openai/gpt-4.1-mini",
				PromptTokens:     300,
				CompletionTokens: 200,
				TotalTokens:      500,
				Cost:             0.05,
				CostMicros:       50000,
			}, nil
		},
	}
	verifier := &mockSourceVerifier{}

	runner, err := monitoring.NewRunner(monitoring.RunnerConfig{
		DB:             db,
		Provider:       provider,
		SourceVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("erro ao criar Runner: %v", err)
	}

	summary, err := runner.Run(ctx, "pesquisa que falhara pos descoberta")
	if err == nil {
		t.Fatal("esperava erro na execução")
	}

	// Summary retornado deve preservar o custo e tokens da descoberta
	if summary == nil {
		t.Fatal("summary não deve ser nil em falha")
	}
	if summary.Status != "failed" {
		t.Errorf("status esperado 'failed', obtido %q", summary.Status)
	}
	if summary.DiscoveryTokens != 500 || summary.TotalTokens != 500 {
		t.Errorf("tokens deveriam ser preservados na falha: DiscoveryTokens=%d TotalTokens=%d",
			summary.DiscoveryTokens, summary.TotalTokens)
	}
	if summary.DiscoveryCostMicros != 50000 || summary.TotalCostMicros != 50000 {
		t.Errorf("custo em micro-USD deveria ser preservado na falha: DiscoveryCostMicros=%d TotalCostMicros=%d",
			summary.DiscoveryCostMicros, summary.TotalCostMicros)
	}

	// No banco SQLite, a run com status 'failed' deve manter os tokens e custos
	queries := sqlc.New(db)
	runInDB, err := queries.GetMonitoringRunByID(ctx, summary.RunID)
	if err != nil {
		t.Fatalf("falha ao consultar run no banco: %v", err)
	}
	if runInDB.Status != "failed" {
		t.Errorf("status no banco: esperado 'failed', obtido %q", runInDB.Status)
	}
	if runInDB.TotalTokens != 500 || runInDB.TotalCostMicrousd != 50000 {
		t.Errorf("tokens/custos zerados indevidamente no banco: tokens=%d, costMicros=%d",
			runInDB.TotalTokens, runInDB.TotalCostMicrousd)
	}

	// A consulta diária DEVE incluir o custo da run com falha
	startOfDay := monitoring.StartOfUTCDay(time.Now()).Format(time.RFC3339Nano)
	dailyCost, err := queries.GetDailyMonitoringCostMicroUSD(ctx, startOfDay)
	if err != nil {
		t.Fatalf("falha ao consultar custo diário: %v", err)
	}
	if dailyCost < 50000 {
		t.Errorf("custo diário acumulado deve incluir runs falhas: obtido %d, esperado >= 50000", dailyCost)
	}
}

func TestRunnerIntegerBudgetAggregationAfterRestart(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_restart_budget.db")
	ctx := context.Background()

	// 1. Abre banco, aplica migrations e insere 10 runs com 123 micro-USD cada
	db1, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir db1: %v", err)
	}
	if err := store.Migrate(ctx, db1); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	for i := 0; i < 10; i++ {
		_, err = db1.ExecContext(ctx, `
			INSERT INTO monitoring_runs (
				id, status, query, discovery_model, total_cost_microusd, total_cost, created_at
			) VALUES (?, 'completed', 'restart query', 'openai/gpt-4.1-mini', 123, 0.000123, ?)
		`, uuid.NewString(), nowStr)
		if err != nil {
			t.Fatalf("falha ao inserir run %d: %v", i, err)
		}
	}
	_ = db1.Close()

	// 2. Simula reinício de processo abrindo nova conexão
	db2, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao reabrir banco: %v", err)
	}
	defer db2.Close()

	queries2 := sqlc.New(db2)
	startOfDay := monitoring.StartOfUTCDay(now).Format(time.RFC3339Nano)
	dailyCostMicros, err := queries2.GetDailyMonitoringCostMicroUSD(ctx, startOfDay)
	if err != nil {
		t.Fatalf("falha ao consultar custo diário após reinício: %v", err)
	}

	// 10 * 123 = 1230 micro-USD exatos, sem perda de precisão
	if dailyCostMicros != 1230 {
		t.Errorf("custo diário acumulado pós-restart = %d; esperado 1230 micro-USD", dailyCostMicros)
	}
}

func TestRunnerLeaseLossSafety(t *testing.T) {
	db, ctx := setupTestDB(t)
	seedEntity(t, db, "ent-1", "Daniel Vorcaro", "person")
	seedCase(t, db, "case-1", "Operação Master")

	var pipelineCancel context.CancelFunc

	provider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:        "Daniel Vorcaro",
						CaseName:          "Operação Master",
						RelationshipType:  "investigado",
						Proposition:       "Fato 1",
						SuggestedGrade:    "A",
						SourceURL:         "https://noticias.exemplo.com/materia-1",
						SourceTitle:       "Notícia 1",
						PublisherOrAuthor: "STF",
						Excerpt:           "Trecho 1",
					},
					{
						EntityName:        "Daniel Vorcaro",
						CaseName:          "Operação Master",
						RelationshipType:  "investigado",
						Proposition:       "Fato 2",
						SuggestedGrade:    "A",
						SourceURL:         "https://noticias.exemplo.com/materia-2",
						SourceTitle:       "Notícia 2",
						PublisherOrAuthor: "STF",
						Excerpt:           "Trecho 2",
					},
				},
				Model:       "openai/gpt-4.1-mini",
				TotalTokens: 100,
				Cost:        0.01,
				CostMicros:  10000,
			}, nil
		},
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			// Simula cancelamento abrupto do pipeline (perda de lease) na primeira verificação
			if pipelineCancel != nil {
				pipelineCancel()
			}
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
				Model:                    "openai/gpt-4.1-mini",
				TotalTokens:              50,
				Cost:                     0.005,
				CostMicros:               5000,
			}, nil
		},
	}

	verifier := &mockSourceVerifier{}

	runner, err := monitoring.NewRunner(monitoring.RunnerConfig{
		DB:             db,
		Provider:       provider,
		SourceVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("erro ao criar Runner: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	pipelineCancel = cancel

	summary, err := runner.Run(runCtx, "pesquisa com perda de lease")
	if err != nil {
		t.Fatalf("falha ao executar runner: %v", err)
	}

	if summary.Status != "partial" {
		t.Errorf("status esperado 'partial' após cancelamento/perda de lease, obtido %q", summary.Status)
	}

	// No banco SQLite, a run DEVE ter sido finalizada como partial e não ter ficado presa em running
	queries := sqlc.New(db)
	runInDB, err := queries.GetMonitoringRunByID(ctx, summary.RunID)
	if err != nil {
		t.Fatalf("falha ao consultar run: %v", err)
	}
	if runInDB.Status != "partial" {
		t.Errorf("run no banco deveria estar em 'partial', obtido %q", runInDB.Status)
	}
	if !runInDB.CompletedAt.Valid || runInDB.CompletedAt.String == "" {
		t.Errorf("run no banco deveria ter CompletedAt preenchido")
	}
}

func TestRunnerUTCDayRolloverBudget(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Simula agora como 2026-09-12 00:05:00 UTC
	now := time.Date(2026, 9, 12, 0, 5, 0, 0, time.UTC)
	nowFunc := func() time.Time { return now }

	// Insere uma run de ontem (2026-09-11 23:55:00 UTC) com gasto de $1.00 USD
	yesterday := time.Date(2026, 9, 11, 23, 55, 0, 0, time.UTC).Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (
			id, status, query, discovery_model, total_cost_microusd, total_cost, created_at
		) VALUES (?, 'completed', 'query de ontem', 'openai/gpt-4.1-mini', 1000000, 1.00, ?)
	`, uuid.NewString(), yesterday)
	if err != nil {
		t.Fatalf("falha ao inserir run de ontem: %v", err)
	}

	provider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{},
				Model:      "openai/gpt-4.1-mini",
			}, nil
		},
	}
	verifier := &mockSourceVerifier{}

	runner, err := monitoring.NewRunner(monitoring.RunnerConfig{
		DB:               db,
		Provider:         provider,
		SourceVerifier:   verifier,
		MaxCostPerDayUSD: 1.00,
		NowFunc:          nowFunc,
	})
	if err != nil {
		t.Fatalf("erro ao criar Runner: %v", err)
	}

	// A execução de hoje DEVE passar porque o teto diário de hoje (2026-09-12) está limpo (gasto de ontem não conta no dia de hoje)
	summary, err := runner.Run(ctx, "query de hoje")
	if err != nil {
		t.Fatalf("esperava sucesso na virada do dia UTC, obtido erro: %v", err)
	}
	if summary.Status != "completed" {
		t.Errorf("status esperado 'completed', obtido %q", summary.Status)
	}
}

func TestRunnerLeaseLossBetweenDiscoverAndIngestion(t *testing.T) {
	db, ctx := setupTestDB(t)
	seedEntity(t, db, "ent-1", "Daniel Vorcaro", "person")

	var pipelineCancel context.CancelFunc

	provider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			// Simula cancelamento do contexto do pipeline imediatamente antes do retorno de Discover
			if pipelineCancel != nil {
				pipelineCancel()
			}
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:          "Daniel Vorcaro",
						Proposition:         "Fato descoberto",
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com/materia-1",
						SourceTitle:         "Notícia 1",
						PublisherOrAuthor:   "STF",
						Excerpt:             "Trecho",
						TechnicalConfidence: 0.95,
					},
				},
				Model:            "openai/gpt-4.1-mini",
				PromptTokens:     350,
				CompletionTokens: 150,
				TotalTokens:      500,
				CostMicros:       25000,
				Cost:             0.025,
				WebSearchCalls:   2,
			}, nil
		},
	}

	verifier := &mockSourceVerifier{}

	runner, err := monitoring.NewRunner(monitoring.RunnerConfig{
		DB:             db,
		Provider:       provider,
		SourceVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("erro ao criar Runner: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	pipelineCancel = cancel

	summary, err := runner.Run(runCtx, "pesquisa cancelada pos descoberta")
	if err == nil {
		t.Fatal("esperava erro decorrente do cancelamento/perda de lease")
	}

	if summary == nil {
		t.Fatal("summary não deve ser nil")
	}
	if summary.Status != "failed" {
		t.Errorf("status esperado 'failed', obtido %q", summary.Status)
	}
	if summary.TotalTokens != 500 || summary.DiscoveryCostMicros != 25000 {
		t.Errorf("tokens/custos deveriam ser preservados no summary: TotalTokens=%d, DiscoveryCostMicros=%d",
			summary.TotalTokens, summary.DiscoveryCostMicros)
	}

	// No banco SQLite, a run DEVE estar persistida com status 'failed' e custos preservados
	queries := sqlc.New(db)
	runInDB, err := queries.GetMonitoringRunByID(ctx, summary.RunID)
	if err != nil {
		t.Fatalf("falha ao consultar run no banco: %v", err)
	}
	if runInDB.Status != "failed" {
		t.Errorf("status no banco: esperado 'failed', obtido %q", runInDB.Status)
	}
	if runInDB.TotalTokens != 500 || runInDB.TotalCostMicrousd != 25000 {
		t.Errorf("dados no banco não foram preservados: tokens=%d, costMicros=%d",
			runInDB.TotalTokens, runInDB.TotalCostMicrousd)
	}

	// A consulta de custo diário DEVE incluir os 25000 micro-USD consumidos
	startOfDay := monitoring.StartOfUTCDay(time.Now()).Format(time.RFC3339Nano)
	dailyCost, err := queries.GetDailyMonitoringCostMicroUSD(ctx, startOfDay)
	if err != nil {
		t.Fatalf("falha ao consultar custo diário: %v", err)
	}
	if dailyCost < 25000 {
		t.Errorf("custo diário acumulado deve incluir a run falha: obtido %d, esperado >= 25000", dailyCost)
	}
}
