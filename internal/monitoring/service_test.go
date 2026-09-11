package monitoring_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/store"
)

type mockResearchProvider struct {
	discoverFn func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error)
	verifyFn   func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error)
}

func (m *mockResearchProvider) Discover(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx, input)
	}
	return nil, errors.New("mock: Discover não configurado")
}

func (m *mockResearchProvider) Verify(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
	if m.verifyFn != nil {
		return m.verifyFn(ctx, input)
	}
	return nil, research.ErrNotImplemented
}

func setupTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "monitoring_test.db")
	ctx := context.Background()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao executar migrations: %v", err)
	}

	return db, ctx
}

func TestExecuteRunSuccess(t *testing.T) {
	db, ctx := setupTestDB(t)

	mockProvider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:          "Daniel Vorcaro",
						Proposition:         "Aquisição de controle societário no Banco Master",
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com:443/materia-1#fragment",
						SourceTitle:         "Materia 1",
						PublisherOrAuthor:   "Veículo X",
						PublishedAt:         "2026-09-03",
						Excerpt:             "Conforme ata...",
						Locator:             "Pág 1",
						ContextLimits:       "Sem ressalvas",
						TechnicalConfidence: 0.95,
					},
					{
						EntityName:          "Banco Master",
						Proposition:         "Aumento de capital social aprovado",
						SuggestedGrade:      "B",
						SourceURL:           "https://noticias.exemplo.com/materia-2",
						SourceTitle:         "Materia 2",
						PublisherOrAuthor:   "Veículo Y",
						PublishedAt:         "2026-09-04",
						Excerpt:             "Assembleia aprovou aumento...",
						Locator:             "",
						ContextLimits:       "",
						TechnicalConfidence: 0.88,
					},
				},
				Content:          "{\"candidates\":[...]}",
				Model:            "openai/gpt-4.1-mini",
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
				WebSearchCalls:   2,
			}, nil
		},
	}

	svc, err := monitoring.NewService(monitoring.ServiceConfig{
		DB:             db,
		Provider:       mockProvider,
		DiscoveryModel: "openai/gpt-4.1-mini",
	})
	if err != nil {
		t.Fatalf("erro ao criar Service: %v", err)
	}

	res, err := svc.ExecuteRun(ctx, monitoring.RunInput{
		Query: "Daniel Vorcaro Banco Master",
	})
	if err != nil {
		t.Fatalf("ExecuteRun falhou: %v", err)
	}

	if res.Status != "completed" {
		t.Errorf("status esperado 'completed', obtido %q", res.Status)
	}
	if res.TotalCandidates != 2 {
		t.Errorf("TotalCandidates esperado 2, obtido %d", res.TotalCandidates)
	}
	if res.UniqueCandidates != 2 {
		t.Errorf("UniqueCandidates esperado 2, obtido %d", res.UniqueCandidates)
	}
	if res.DuplicateCandidates != 0 {
		t.Errorf("DuplicateCandidates esperado 0, obtido %d", res.DuplicateCandidates)
	}

	// Consulta o run no banco
	run, err := svc.GetRun(ctx, res.RunID)
	if err != nil {
		t.Fatalf("GetRun falhou: %v", err)
	}
	if run.Status != "completed" {
		t.Errorf("run.Status no banco esperado 'completed', obtido %q", run.Status)
	}
	if run.TotalTokens != 150 {
		t.Errorf("run.TotalTokens esperado 150, obtido %d", run.TotalTokens)
	}

	// Consulta os candidatos inseridos
	candidates, err := svc.ListCandidates(ctx, res.RunID)
	if err != nil {
		t.Fatalf("ListCandidates falhou: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("esperava 2 candidatos no banco, obtido %d", len(candidates))
	}

	for _, cand := range candidates {
		// Todo candidato DEVE iniciar em quarantined nesta fase
		if cand.EditorialStatus != "quarantined" {
			t.Errorf("candidato %s com status editorial inesperado %q; esperado 'quarantined'", cand.ID, cand.EditorialStatus)
		}
		if cand.IsDuplicate != 0 {
			t.Errorf("candidato %s marcado como duplicado indevidamente", cand.ID)
		}
		if cand.CanonicalUrl == "" {
			t.Errorf("candidato %s com canonical_url vazia", cand.ID)
		}
		// Valida remoção de porta 443 e fragmento na normalização
		if cand.EntityName == "Daniel Vorcaro" && cand.CanonicalUrl != "https://noticias.exemplo.com/materia-1" {
			t.Errorf("canonical_url normalizada inesperada: %q", cand.CanonicalUrl)
		}
	}
}

func TestExecuteRunIntraRunDeduplication(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Mock retorna 2 candidatos idênticos na mesma resposta
	mockProvider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:          "Daniel Vorcaro",
						Proposition:         "Fato A",
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com/fato-a",
						Excerpt:             "Trecho comprobatório",
						TechnicalConfidence: 0.9,
					},
					{
						EntityName:          "  daniel   vorcaro  ", // Variação de espaços/case -> mesmo fingerprint
						Proposition:         "Fato A",
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com/fato-a#comentarios",
						Excerpt:             "Trecho comprobatório",
						TechnicalConfidence: 0.9,
					},
				},
				Model:       "openai/gpt-4.1-mini",
				TotalTokens: 100,
			}, nil
		},
	}

	svc, err := monitoring.NewService(monitoring.ServiceConfig{
		DB:       db,
		Provider: mockProvider,
	})
	if err != nil {
		t.Fatalf("erro ao criar Service: %v", err)
	}

	res, err := svc.ExecuteRun(ctx, monitoring.RunInput{Query: "pesquisa"})
	if err != nil {
		t.Fatalf("ExecuteRun falhou: %v", err)
	}

	if res.TotalCandidates != 2 || res.UniqueCandidates != 1 || res.DuplicateCandidates != 1 {
		t.Errorf("contagens inesperadas: total=%d, unique=%d, dup=%d",
			res.TotalCandidates, res.UniqueCandidates, res.DuplicateCandidates)
	}

	candidates, err := svc.ListCandidates(ctx, res.RunID)
	if err != nil {
		t.Fatalf("ListCandidates falhou: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("esperava 2 candidatos gravados para auditoria completa, obtido %d", len(candidates))
	}

	cand1 := candidates[0]
	cand2 := candidates[1]

	if cand1.IsDuplicate != 0 || cand1.CanonicalCandidateID.Valid {
		t.Errorf("primeiro candidato deveria ser o canônico (is_duplicate=0)")
	}

	if cand2.IsDuplicate != 1 {
		t.Errorf("segundo candidato deveria ser marcado como duplicado (is_duplicate=1)")
	}
	if cand2.DuplicateReason != "same_run_duplicate" {
		t.Errorf("duplicate_reason esperada 'same_run_duplicate', obtido %q", cand2.DuplicateReason)
	}
	if !cand2.CanonicalCandidateID.Valid || cand2.CanonicalCandidateID.String != cand1.ID {
		t.Errorf("segundo candidato deve apontar para o canônico %q, obtido %v", cand1.ID, cand2.CanonicalCandidateID)
	}
}

func TestExecuteRunCrossRunDeduplication(t *testing.T) {
	db, ctx := setupTestDB(t)

	mockProvider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:          "Daniel Vorcaro",
						Proposition:         "Proposição estável",
						SuggestedGrade:      "B",
						SourceURL:           "https://noticias.exemplo.com/materia-estavel",
						Excerpt:             "Trecho comprobatório...",
						TechnicalConfidence: 0.85,
					},
				},
				Model:       "openai/gpt-4.1-mini",
				TotalTokens: 50,
			}, nil
		},
	}

	svc, err := monitoring.NewService(monitoring.ServiceConfig{
		DB:       db,
		Provider: mockProvider,
	})
	if err != nil {
		t.Fatalf("erro ao criar Service: %v", err)
	}

	// 1. Executa o primeiro run
	res1, err := svc.ExecuteRun(ctx, monitoring.RunInput{Query: "run 1"})
	if err != nil {
		t.Fatalf("Run 1 falhou: %v", err)
	}
	if res1.UniqueCandidates != 1 || res1.DuplicateCandidates != 0 {
		t.Fatalf("Run 1 deveria ter 1 único e 0 duplicados")
	}

	candsRun1, err := svc.ListCandidates(ctx, res1.RunID)
	if err != nil || len(candsRun1) != 1 {
		t.Fatalf("esperava 1 candidato no Run 1")
	}
	originalCanonicID := candsRun1[0].ID

	// 2. Executa o segundo run (retorna a mesma proposição)
	res2, err := svc.ExecuteRun(ctx, monitoring.RunInput{Query: "run 2"})
	if err != nil {
		t.Fatalf("Run 2 falhou: %v", err)
	}
	if res2.TotalCandidates != 1 || res2.UniqueCandidates != 0 || res2.DuplicateCandidates != 1 {
		t.Errorf("Run 2 deveria detectar duplicata cross-run: total=%d, unique=%d, dup=%d",
			res2.TotalCandidates, res2.UniqueCandidates, res2.DuplicateCandidates)
	}

	candsRun2, err := svc.ListCandidates(ctx, res2.RunID)
	if err != nil || len(candsRun2) != 1 {
		t.Fatalf("esperava 1 candidato persistido no Run 2 para histórico de tokens/execução")
	}

	dupCand := candsRun2[0]
	if dupCand.IsDuplicate != 1 {
		t.Errorf("candidato no Run 2 deveria ter is_duplicate=1")
	}
	if dupCand.DuplicateReason != "existing_fingerprint" {
		t.Errorf("duplicate_reason esperada 'existing_fingerprint', obtido %q", dupCand.DuplicateReason)
	}
	if !dupCand.CanonicalCandidateID.Valid || dupCand.CanonicalCandidateID.String != originalCanonicID {
		t.Errorf("candidato duplicado no Run 2 deve apontar para o canônico %q do Run 1, obtido %v",
			originalCanonicID, dupCand.CanonicalCandidateID)
	}
}

func TestExecuteRunProviderError(t *testing.T) {
	db, ctx := setupTestDB(t)

	mockProvider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return nil, errors.New("openrouter: timeout de rede simulado")
		},
	}

	svc, err := monitoring.NewService(monitoring.ServiceConfig{
		DB:       db,
		Provider: mockProvider,
	})
	if err != nil {
		t.Fatalf("erro ao criar Service: %v", err)
	}

	res, err := svc.ExecuteRun(ctx, monitoring.RunInput{Query: "pesquisa com falha"})
	if err != nil {
		t.Fatalf("ExecuteRun não deveria retornar erro fatal Go para falha operacional do run: %v", err)
	}

	if res.Status != "failed" {
		t.Errorf("status do run esperado 'failed', obtido %q", res.Status)
	}
	if !strings.Contains(res.ErrorMessage, "timeout de rede simulado") {
		t.Errorf("mensagem de erro esperada contendo timeout, obtido %q", res.ErrorMessage)
	}

	run, err := svc.GetRun(ctx, res.RunID)
	if err != nil {
		t.Fatalf("GetRun falhou: %v", err)
	}
	if run.Status != "failed" {
		t.Errorf("status no banco esperado 'failed', obtido %q", run.Status)
	}
	if !run.CompletedAt.Valid || run.CompletedAt.String == "" {
		t.Errorf("run falhado deve ter completed_at preenchido")
	}
}

func TestExecuteRunDefensiveValidationFailure(t *testing.T) {
	tests := []struct {
		name      string
		candidate research.CandidateExtraction
	}{
		{
			name: "Entidade vazia",
			candidate: research.CandidateExtraction{
				EntityName:          "",
				Proposition:         "Proposição",
				SuggestedGrade:      "A",
				SourceURL:           "https://exemplo.com",
				TechnicalConfidence: 0.9,
			},
		},
		{
			name: "Proposição vazia",
			candidate: research.CandidateExtraction{
				EntityName:          "Nome",
				Proposition:         "   ",
				SuggestedGrade:      "A",
				SourceURL:           "https://exemplo.com",
				TechnicalConfidence: 0.9,
			},
		},
		{
			name: "Grau inválido",
			candidate: research.CandidateExtraction{
				EntityName:          "Nome",
				Proposition:         "Proposição",
				SuggestedGrade:      "Z",
				SourceURL:           "https://exemplo.com",
				TechnicalConfidence: 0.9,
			},
		},
		{
			name: "Confiança negativa",
			candidate: research.CandidateExtraction{
				EntityName:          "Nome",
				Proposition:         "Proposição",
				SuggestedGrade:      "A",
				SourceURL:           "https://exemplo.com",
				TechnicalConfidence: -0.5,
			},
		},
		{
			name: "Confiança acima de 1.0",
			candidate: research.CandidateExtraction{
				EntityName:          "Nome",
				Proposition:         "Proposição",
				SuggestedGrade:      "A",
				SourceURL:           "https://exemplo.com",
				TechnicalConfidence: 1.2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ctx := setupTestDB(t)

			mockProvider := &mockResearchProvider{
				discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
					return &research.DiscoverResult{
						Candidates: []research.CandidateExtraction{tt.candidate},
						Model:      "openai/gpt-4.1-mini",
					}, nil
				},
			}

			svc, err := monitoring.NewService(monitoring.ServiceConfig{
				DB:       db,
				Provider: mockProvider,
			})
			if err != nil {
				t.Fatalf("erro ao criar Service: %v", err)
			}

			res, err := svc.ExecuteRun(ctx, monitoring.RunInput{Query: "teste"})
			if err != nil {
				t.Fatalf("ExecuteRun não deveria retornar erro fatal Go ao registrar falha de validação: %v", err)
			}

			if res.Status != "failed" {
				t.Errorf("status do run esperado 'failed', obtido %q", res.Status)
			}

			run, err := svc.GetRun(ctx, res.RunID)
			if err != nil {
				t.Fatalf("GetRun falhou: %v", err)
			}
			if run.Status != "failed" {
				t.Errorf("status no banco esperado 'failed', obtido %q", run.Status)
			}
			if !run.CompletedAt.Valid || run.CompletedAt.String == "" {
				t.Errorf("run falhado deve possuir completed_at preenchido")
			}

			// Nenhum candidato deve ser gravado
			cands, err := svc.ListCandidates(ctx, res.RunID)
			if err != nil {
				t.Fatalf("ListCandidates falhou: %v", err)
			}
			if len(cands) != 0 {
				t.Errorf("nenhum candidato deveria ter sido persistido, obtido %d", len(cands))
			}
		})
	}
}

func TestExecuteRunInvalidURLs(t *testing.T) {
	invalidURLs := []string{
		"ftp://servidor.com/arquivo",
		"javascript:alert(1)",
		"exemplo.com/sem-esquema",
		"",
		"   ",
		"http:///sem-host",
	}

	for _, rawURL := range invalidURLs {
		t.Run("URL: "+rawURL, func(t *testing.T) {
			db, ctx := setupTestDB(t)

			mockProvider := &mockResearchProvider{
				discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
					return &research.DiscoverResult{
						Candidates: []research.CandidateExtraction{
							{
								EntityName:          "Daniel Vorcaro",
								Proposition:         "Proposição",
								SuggestedGrade:      "A",
								SourceURL:           rawURL,
								TechnicalConfidence: 0.9,
							},
						},
						Model: "openai/gpt-4.1-mini",
					}, nil
				},
			}

			svc, err := monitoring.NewService(monitoring.ServiceConfig{
				DB:       db,
				Provider: mockProvider,
			})
			if err != nil {
				t.Fatalf("erro ao criar Service: %v", err)
			}

			res, err := svc.ExecuteRun(ctx, monitoring.RunInput{Query: "teste url"})
			if err != nil {
				t.Fatalf("ExecuteRun não deveria retornar erro fatal Go ao registrar falha de URL: %v", err)
			}

			if res.Status != "failed" {
				t.Errorf("status esperado 'failed' para URL inválida %q, obtido %q", rawURL, res.Status)
			}

			run, err := svc.GetRun(ctx, res.RunID)
			if err != nil {
				t.Fatalf("GetRun falhou: %v", err)
			}
			if run.Status != "failed" {
				t.Errorf("status no banco esperado 'failed', obtido %q", run.Status)
			}

			// Nenhum candidato deve ser gravado
			cands, err := svc.ListCandidates(ctx, res.RunID)
			if err != nil {
				t.Fatalf("ListCandidates falhou: %v", err)
			}
			if len(cands) != 0 {
				t.Errorf("nenhum candidato deveria ter sido persistido para URL inválida, obtido %d", len(cands))
			}
		})
	}
}

func TestCandidatesDoNotAffectPublicClaimsViewOrMetrics(t *testing.T) {
	db, ctx := setupTestDB(t)

	mockProvider := &mockResearchProvider{
		discoverFn: func(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
			return &research.DiscoverResult{
				Candidates: []research.CandidateExtraction{
					{
						EntityName:          "Daniel Vorcaro",
						Proposition:         "Proposição em quarentena",
						SuggestedGrade:      "A",
						SourceURL:           "https://noticias.exemplo.com/materia-quarentena",
						TechnicalConfidence: 0.99,
					},
				},
				Model: "openai/gpt-4.1-mini",
			}, nil
		},
	}

	svc, err := monitoring.NewService(monitoring.ServiceConfig{
		DB:       db,
		Provider: mockProvider,
	})
	if err != nil {
		t.Fatalf("erro ao criar Service: %v", err)
	}

	_, err = svc.ExecuteRun(ctx, monitoring.RunInput{Query: "teste"})
	if err != nil {
		t.Fatalf("ExecuteRun falhou: %v", err)
	}

	// 1. Garante que public_claims_view tem 0 registros
	var publicClaimsCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM public_claims_view").Scan(&publicClaimsCount)
	if err != nil {
		t.Fatalf("falha ao consultar public_claims_view: %v", err)
	}
	if publicClaimsCount != 0 {
		t.Errorf("public_claims_view deveria estar vazia, obtido %d registros", publicClaimsCount)
	}

	// 2. Garante que nenhuma tabela editorial (claims, entities, sources) foi criada/mutada
	var claimsCount, entitiesCount, sourcesCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM claims").Scan(&claimsCount)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM entities").Scan(&entitiesCount)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM sources").Scan(&sourcesCount)

	if claimsCount != 0 || entitiesCount != 0 || sourcesCount != 0 {
		t.Errorf("nenhuma tabela editorial deveria ser modificada na VZ-011: claims=%d, entities=%d, sources=%d",
			claimsCount, entitiesCount, sourcesCount)
	}
}
