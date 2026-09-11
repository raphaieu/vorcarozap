package monitoring_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/metrics"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/sourcecheck"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

type mockSourceVerifier struct {
	checkFn func(ctx context.Context, rawURL string) sourcecheck.CheckResult
}

func (m *mockSourceVerifier) Check(ctx context.Context, rawURL string) sourcecheck.CheckResult {
	if m.checkFn != nil {
		return m.checkFn(ctx, rawURL)
	}
	return sourcecheck.CheckResult{
		URL:                 rawURL,
		SanitizedURL:        rawURL,
		Status:              domain.SourceAccessReachable,
		TechnicalReason:     sourcecheck.ReasonOK,
		HTTPStatus:          200,
		NormalizedErrorCode: "200_ok",
		CheckedAt:           time.Now().UTC(),
	}
}

func seedEntity(t *testing.T, db *sql.DB, id, name, entityType string) {
	t.Helper()
	queries := sqlc.New(db)
	norm := normalize.Name(name)
	slug := normalize.Slug(name)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := queries.CreateEntity(context.Background(), sqlc.CreateEntityParams{
		ID:                 id,
		Type:               entityType,
		Name:               name,
		NormalizedName:     norm,
		Slug:               slug,
		Category:           "pessoa",
		RoleOrContext:      "Contexto",
		Reach:              "nacional",
		Summary:            "Resumo",
		Relevance:          3,
		RelevanceRationale: "Relevante",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade %q: %v", name, err)
	}
}

func seedCase(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	queries := sqlc.New(db)
	slug := normalize.Slug(name)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := queries.CreateCase(context.Background(), sqlc.CreateCaseParams{
		ID:          id,
		Name:        name,
		Slug:        slug,
		Description: "Descrição do caso",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso %q: %v", name, err)
	}
}

func seedCandidate(t *testing.T, db *sql.DB, cand sqlc.CreateMonitoringCandidateParams) sqlc.MonitoringCandidate {
	t.Helper()
	queries := sqlc.New(db)
	res, err := queries.CreateMonitoringCandidate(context.Background(), cand)
	if err != nil {
		t.Fatalf("falha ao criar candidato %s: %v", cand.ID, err)
	}
	return res
}

func createTestMonitoringRun(t *testing.T, db *sql.DB) string {
	t.Helper()
	queries := sqlc.New(db)
	runID := "run-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := queries.CreateMonitoringRun(context.Background(), sqlc.CreateMonitoringRunParams{
		ID:                   runID,
		Status:               "completed",
		Query:                "consulta teste",
		DiscoveryProvider:    "openrouter",
		DiscoveryModel:       "openai/gpt-4.1-mini",
		VerificationProvider: "openrouter",
		VerificationModel:    "openai/gpt-4.1-mini",
		PromptTokens:         100,
		CompletionTokens:     50,
		TotalTokens:          150,
		EstimatedCost:        sql.NullFloat64{Float64: 0.001, Valid: true},
		WebSearchCalls:       1,
		SummaryCounts:        "{}",
		TechnicalSummary:     "{}",
		CreatedAt:            now,
		CompletedAt:          sql.NullString{String: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("falha ao criar monitoring run: %v", err)
	}
	return runID
}

func TestEvaluateCandidate_Success_GradeA_Publish(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-grade-a-01",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro adquiriu o controle societário do Banco Master conforme documentação societária",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-controle-master",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-controle-master",
		SourceTitle:                "Aquisição de Controle no Banco Master",
		PublisherOrAuthor:          "Jornal Econômico",
		PublishedAt:                sql.NullString{String: "2026-09-01", Valid: true},
		Excerpt:                    "Daniel Vorcaro adquiriu o controle societário do Banco Master em assembleia extraordinária.",
		Locator:                    "Pág. 3",
		ContextLimits:              "Operação aprovada sem restrições",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
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
			}, nil
		},
	}

	mockVerifier := &mockSourceVerifier{}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:                db,
		Provider:          mockProvider,
		SourceVerifier:    mockVerifier,
		VerificationModel: "openai/gpt-4.1-mini",
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate retornou erro inesperado: %v", err)
	}

	if !res.Published {
		t.Fatalf("candidato Grau A deveria ter sido publicado, res: %+v", res)
	}
	if res.EditorialStatus != "published" {
		t.Errorf("status retornado esperado 'published', obtido %q", res.EditorialStatus)
	}
	if res.PublishedClaimID == "" {
		t.Fatalf("published_claim_id não preenchido no resultado")
	}

	queries := sqlc.New(db)
	candDB, err := queries.GetMonitoringCandidateByID(ctx, candID)
	if err != nil {
		t.Fatalf("falha ao buscar candidato atualizado no banco: %v", err)
	}
	if candDB.EditorialStatus != "published" {
		t.Errorf("editorial_status no banco esperado 'published', obtido %q", candDB.EditorialStatus)
	}
	if !candDB.PublishedClaimID.Valid || candDB.PublishedClaimID.String != res.PublishedClaimID {
		t.Errorf("published_claim_id inconsistente no banco: %v", candDB.PublishedClaimID)
	}
	if candDB.StructuralGatePassed.Int64 != 1 || candDB.SemanticGatePassed.Int64 != 1 {
		t.Errorf("gates deveriam estar marcados como 1 (aprovados)")
	}

	// 1. Valida persistência da avaliação semântica
	evals, err := queries.ListSemanticEvaluationsByCandidateID(ctx, candID)
	if err != nil {
		t.Fatalf("falha ao listar semantic_evaluations: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("esperava 1 semantic_evaluation gravada, obtido %d", len(evals))
	}
	if evals[0].RecommendedAction != "publish" || evals[0].IdentityMatch != 1 || evals[0].ClaimSupported != 1 {
		t.Errorf("dados de semantic_evaluations incorretos: %+v", evals[0])
	}

	// 2. Valida materialização de sources
	source, err := queries.GetSourceByCanonicalURL(ctx, "https://noticias.exemplo.com/materia-controle-master")
	if err != nil {
		t.Fatalf("source não materializada: %v", err)
	}
	if source.SourceAccessStatus != string(domain.SourceAccessReachable) {
		t.Errorf("source.SourceAccessStatus esperado 'reachable', obtido %q", source.SourceAccessStatus)
	}

	// 3. Valida materialização de claims
	claim, err := queries.GetClaimByID(ctx, res.PublishedClaimID)
	if err != nil {
		t.Fatalf("claim não encontrado: %v", err)
	}
	if claim.Status != "published" {
		t.Errorf("claim.Status esperado 'published', obtido %q", claim.Status)
	}
	if claim.Disposition != "supports_link" {
		t.Errorf("claim.Disposition para Grau A esperado 'supports_link', obtido %q", claim.Disposition)
	}
	if claim.Grade != "A" {
		t.Errorf("claim.Grade esperado 'A', obtido %q", claim.Grade)
	}
	if claim.MetricEligible != 1 {
		t.Errorf("claim.MetricEligible esperado 1 para Grau A publicado, obtido %d", claim.MetricEligible)
	}

	// 4. Valida materialização de relationships
	rel, err := queries.GetRelationshipByID(ctx, claim.RelationshipID)
	if err != nil {
		t.Fatalf("falha ao buscar relacionamento do claim: %v", err)
	}
	if rel.SubjectEntityID != subjID {
		t.Errorf("subject_entity_id incorreto no relacionamento: %s != %s", rel.SubjectEntityID, subjID)
	}
	if !rel.TargetEntityID.Valid || rel.TargetEntityID.String != targetID {
		t.Errorf("target_entity_id incorreto no relacionamento: %v", rel.TargetEntityID)
	}

	// 5. Valida active supports (evidence e evidence_sources)
	supports, err := queries.ListActiveSupportsByClaimID(ctx, claim.ID)
	if err != nil {
		t.Fatalf("falha ao listar active supports por claim: %v", err)
	}
	if len(supports) != 1 {
		t.Fatalf("esperava 1 active support materializado, obtido %d", len(supports))
	}
	if supports[0].SourceCanonicalUrl != "https://noticias.exemplo.com/materia-controle-master" {
		t.Errorf("canonical_url do suporte incorreta: %q", supports[0].SourceCanonicalUrl)
	}

	// 6. Valida que o claim aparece em public_claims_view
	var viewCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM public_claims_view WHERE claim_id = ?", claim.ID).Scan(&viewCount)
	if err != nil {
		t.Fatalf("falha ao consultar public_claims_view: %v", err)
	}
	if viewCount != 1 {
		t.Errorf("claim publicado deveria estar visível em public_claims_view, count=%d", viewCount)
	}

	// 7. Valida que as métricas contam este claim
	pubMetrics, err := metrics.GetPublicMetrics(ctx, queries, "", 5)
	if err != nil {
		t.Fatalf("falha ao calcular métricas públicas: %v", err)
	}
	if pubMetrics.Overview.TotalClaims != 1 {
		t.Errorf("TotalClaims em métricas esperado 1, obtido %d", pubMetrics.Overview.TotalClaims)
	}
	if pubMetrics.Network.EligibleClaims != 1 {
		t.Errorf("EligibleClaims em rede elegível esperado 1, obtido %d", pubMetrics.Network.EligibleClaims)
	}
}

func TestEvaluateCandidate_Success_GradeB_CaseTarget(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	caseID := "case-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedCase(t, db, caseID, "Operação Greenfield")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                   candID,
		MonitoringRunID:      runID,
		Fingerprint:          "fp-grade-b-case",
		FingerprintVersion:   1,
		EntityName:           "Daniel Vorcaro",
		NormalizedEntityName: normalize.Name("Daniel Vorcaro"),
		CaseName:             "Operação Greenfield",
		NormalizedCaseName:   normalize.Name("Operação Greenfield"),
		RelationshipType:     "investigated_in",
		Proposition:          "Daniel Vorcaro prestou esclarecimentos no âmbito da Operação Greenfield",
		SuggestedGrade:       "B",
		SourceUrl:            "https://noticias.exemplo.com/materia-greenfield",
		CanonicalUrl:         "https://noticias.exemplo.com/materia-greenfield",
		SourceTitle:          "Esclarecimentos na Greenfield",
		PublisherOrAuthor:    "Agência de Notícias",
		PublishedAt:          sql.NullString{String: "2026-09-02", Valid: true},
		Excerpt:              "Conforme relatório preliminar dos investigadores...",
		ContextLimits:        "Fase de instrução",
		TechnicalConfidence:  0.88,
		RawPayload:           "{}",
		EditorialStatus:      "quarantined",
		IsDuplicate:          0,
		CreatedAt:            now,
		UpdatedAt:            now,
	})

	mockProvider := &mockResearchProvider{
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
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if !res.Published {
		t.Fatalf("candidato com caso alvo deveria ser publicado: %+v", res)
	}

	queries := sqlc.New(db)
	claim, err := queries.GetClaimByID(ctx, res.PublishedClaimID)
	if err != nil {
		t.Fatalf("falha ao buscar claim: %v", err)
	}
	if claim.Disposition != "supports_link" {
		t.Errorf("claim.Disposition para Grau B esperado 'supports_link', obtido %q", claim.Disposition)
	}
	if claim.Grade != "B" {
		t.Errorf("claim.Grade esperado 'B', obtido %q", claim.Grade)
	}

	// Verifica relationship vinculada ao caso
	rel, err := queries.GetRelationshipByID(ctx, claim.RelationshipID)
	if err != nil {
		t.Fatalf("falha ao obter relacionamento: %v", err)
	}
	if !rel.CaseID.Valid || rel.CaseID.String != caseID {
		t.Errorf("relacionamento deveria apontar para caseID %q, obtido %v", caseID, rel.CaseID)
	}
	if rel.TargetEntityID.Valid {
		t.Errorf("relacionamento com caso não deve possuir target_entity_id: %v", rel.TargetEntityID)
	}
}

func TestEvaluateCandidate_Success_GradeC_WithLimitsAndConservativeLanguage(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-grade-c-valid",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "associated_with",
		Proposition:                "Alega-se possível atuação de Daniel Vorcaro em tratativas preliminares com o Banco Master segundo relatos não confirmados",
		SuggestedGrade:             "C",
		SourceUrl:                  "https://noticias.exemplo.com/materia-rumores",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-rumores",
		SourceTitle:                "Rumores sobre tratativas",
		PublisherOrAuthor:          "Coluna Radar",
		PublishedAt:                sql.NullString{String: "2026-09-03", Valid: true},
		Excerpt:                    "Fontes afirmam suposta negociação preliminar em andamento no mercado financeiro.",
		ContextLimits:              "Informações preliminares decorrentes de fontes de mercado não auditadas",
		TechnicalConfidence:        0.80,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
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
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if !res.Published {
		t.Fatalf("candidato Grau C com limites explícitos e linguagem cautelosa deveria ser publicado: %+v", res)
	}

	queries := sqlc.New(db)
	claim, err := queries.GetClaimByID(ctx, res.PublishedClaimID)
	if err != nil {
		t.Fatalf("falha ao buscar claim: %v", err)
	}
	if claim.Disposition != "possible_link" {
		t.Errorf("claim.Disposition para Grau C esperado 'possible_link', obtido %q", claim.Disposition)
	}
	if claim.Grade != "C" {
		t.Errorf("claim.Grade esperado 'C', obtido %q", claim.Grade)
	}
}

func TestEvaluateCandidate_GradeC_Quarantined_WithoutLimits(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Candidato Grau C com context_limits vazio e sem linguagem cautelosa
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-grade-c-no-limits",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "associated_with",
		Proposition:                "Daniel Vorcaro controla integralmente as operações financeiras", // Afirmação categórica sem linguagem de rumor/cautela
		SuggestedGrade:             "C",
		SourceUrl:                  "https://noticias.exemplo.com/materia-sem-limites",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-sem-limites",
		SourceTitle:                "Matéria genérica",
		PublisherOrAuthor:          "Autor",
		PublishedAt:                sql.NullString{String: "2026-09-03", Valid: true},
		Excerpt:                    "Trecho literal da matéria...",
		ContextLimits:              "", // Vazio
		TechnicalConfidence:        0.80,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				RecommendedAction:        "publish",
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.Published {
		t.Fatalf("candidato Grau C sem limites explícitos e sem linguagem cautelosa NÃO pode ser publicado automaticamente")
	}
	if res.PolicyAction != domain.PolicyActionQuarantine {
		t.Errorf("ação esperada 'quarantine', obtido %q", res.PolicyAction)
	}

	// Garante que nenhum claim foi criado
	queries := sqlc.New(db)
	candDB, err := queries.GetMonitoringCandidateByID(ctx, candID)
	if err != nil {
		t.Fatalf("falha ao buscar candidato: %v", err)
	}
	if candDB.EditorialStatus != "quarantined" {
		t.Errorf("editorial_status no banco esperado 'quarantined', obtido %q", candDB.EditorialStatus)
	}
	if candDB.PublishedClaimID.Valid {
		t.Errorf("published_claim_id não deveria ser preenchido: %v", candDB.PublishedClaimID)
	}
}

func TestEvaluateCandidate_GradeDE_QuarantinedByDefault(t *testing.T) {
	grades := []string{"D", "E"}

	for _, grade := range grades {
		t.Run("Grau_"+grade, func(t *testing.T) {
			db, ctx := setupTestDB(t)

			subjID := "ent-" + uuid.NewString()[:8]
			targetID := "ent-" + uuid.NewString()[:8]
			seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
			seedEntity(t, db, targetID, "Banco Master", "organization")

			runID := createTestMonitoringRun(t, db)
			candID := "cand-" + uuid.NewString()[:8]
			now := time.Now().UTC().Format(time.RFC3339Nano)

			seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
				ID:                         candID,
				MonitoringRunID:            runID,
				Fingerprint:                "fp-grade-" + grade,
				FingerprintVersion:         1,
				EntityName:                 "Daniel Vorcaro",
				NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
				TargetEntityName:           "Banco Master",
				NormalizedTargetEntityName: normalize.Name("Banco Master"),
				RelationshipType:           "associated_with",
				Proposition:                "Comentários em fórum da internet sobre Daniel Vorcaro e Banco Master",
				SuggestedGrade:             grade,
				SourceUrl:                  "https://forum.exemplo.com/post-1",
				CanonicalUrl:               "https://forum.exemplo.com/post-1",
				SourceTitle:                "Post em Fórum",
				PublisherOrAuthor:          "Anônimo",
				PublishedAt:                sql.NullString{String: "2026-09-03", Valid: true},
				Excerpt:                    "Post anônimo em fórum de discussão...",
				ContextLimits:              "Fonte não qualificada",
				TechnicalConfidence:        0.75,
				RawPayload:                 "{}",
				EditorialStatus:            "quarantined",
				IsDuplicate:                0,
				CreatedAt:                  now,
				UpdatedAt:                  now,
			})

			mockProvider := &mockResearchProvider{
				verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
					return &research.VerifyResult{
						IdentityMatch:            true,
						ClaimSupported:           true,
						ClaimOverstatesSource:    false,
						AttributionExplicit:      true,
						GradeCompatible:          true,
						ContainsIllicitInference: false,
						RecommendedAction:        "publish",
					}, nil
				},
			}

			evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
				DB:             db,
				Provider:       mockProvider,
				SourceVerifier: &mockSourceVerifier{},
			})
			if err != nil {
				t.Fatalf("falha ao criar Evaluator: %v", err)
			}

			res, err := evaluator.EvaluateCandidate(ctx, candID)
			if err != nil {
				t.Fatalf("EvaluateCandidate falhou: %v", err)
			}

			if res.Published {
				t.Fatalf("candidato Grau %s DEVE ser retido em quarentena por padrão", grade)
			}
			if res.PolicyAction != domain.PolicyActionQuarantine {
				t.Errorf("ação esperada 'quarantine', obtido %q", res.PolicyAction)
			}

			var claimsCount int
			_ = db.QueryRowContext(ctx, "SELECT count(*) FROM claims").Scan(&claimsCount)
			if claimsCount != 0 {
				t.Errorf("nenhum claim deveria ser criado para Grau %s, obtido %d", grade, claimsCount)
			}
		})
	}
}

func TestEvaluateCandidate_StructuralGateFailure_SkipsLLMVerify(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Não cria entidade no banco: Daniel Vorcaro não existe
	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-no-entity",
		FingerprintVersion:         1,
		EntityName:                 "Entidade Inexistente",
		NormalizedEntityName:       normalize.Name("Entidade Inexistente"),
		TargetEntityName:           "Outra Inexistente",
		NormalizedTargetEntityName: normalize.Name("Outra Inexistente"),
		RelationshipType:           "partner_of",
		Proposition:                "Proposição de entidade não cadastrada na base",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia",
		CanonicalUrl:               "https://noticias.exemplo.com/materia",
		SourceTitle:                "Título",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria...",
		TechnicalConfidence:        0.9,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	var verifyCalls int32
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			atomic.AddInt32(&verifyCalls, 1)
			return nil, errors.New("não deveria ser chamado!")
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.Published || res.StructuralPassed {
		t.Fatalf("candidato sem entidade existente deveria falhar no gate estrutural")
	}

	// Regra inegociável: se falhou no gate estrutural, NUNCA chama a LLM no gate semântico
	if atomic.LoadInt32(&verifyCalls) != 0 {
		t.Errorf("LLM Verify NÃO deveria ter sido chamada quando gate estrutural falha! Chamadas: %d", verifyCalls)
	}

	queries := sqlc.New(db)
	candDB, err := queries.GetMonitoringCandidateByID(ctx, candID)
	if err != nil {
		t.Fatalf("falha ao consultar candidato: %v", err)
	}
	if candDB.StructuralGatePassed.Int64 != 0 {
		t.Errorf("structural_gate_passed esperado 0, obtido %v", candDB.StructuralGatePassed)
	}
	if candDB.EditorialStatus != "quarantined" {
		t.Errorf("editorial_status esperado 'quarantined', obtido %q", candDB.EditorialStatus)
	}
}

func TestEvaluateCandidate_StructuralGateFailure_PIIDetected(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Candidato contendo CPF na proposição
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-pii-cpf",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro contato pessoal via email usuario.teste@gmail.com ou telefone (11) 98765-4321",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-pii",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-pii",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria sobre a operação...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	var verifyCalls int32
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			atomic.AddInt32(&verifyCalls, 1)
			return nil, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.StructuralPassed {
		t.Fatalf("candidato com PII deveria ter sido rejeitado no gate estrutural")
	}
	if atomic.LoadInt32(&verifyCalls) != 0 {
		t.Errorf("LLM Verify NÃO deveria ter sido chamada em violação de PII")
	}

	var hasPIIReason bool
	for _, r := range res.StructuralReasons {
		if r == "contains_unnecessary_pii" {
			hasPIIReason = true
			break
		}
	}
	if !hasPIIReason {
		t.Errorf("motivo 'contains_unnecessary_pii' esperado em res.StructuralReasons: %v", res.StructuralReasons)
	}
}

func TestEvaluateCandidate_StructuralGateFailure_BlockedFingerprint(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Cria um candidato rejeitado anteriormente com fingerprint "fp-blocked"
	oldRejectedID := "cand-old-rejected"
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         oldRejectedID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-blocked",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Proposição rejeitada",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-rejeitada",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-rejeitada",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria rejeitada...",
		TechnicalConfidence:        0.9,
		RawPayload:                 "{}",
		EditorialStatus:            "rejected", // Status rejeitado pelo admin
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	// 2. Novo candidato com mesmo fingerprint "fp-blocked"
	newCandID := "cand-" + uuid.NewString()[:8]
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         newCandID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-blocked",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Proposição idêntica re-extraída",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-rejeitada",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-rejeitada",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria rejeitada...",
		TechnicalConfidence:        0.9,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	var verifyCalls int32
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			atomic.AddInt32(&verifyCalls, 1)
			return nil, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, newCandID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.StructuralPassed {
		t.Fatalf("candidato com fingerprint previamente rejeitado DEVE falhar no gate estrutural")
	}
	if atomic.LoadInt32(&verifyCalls) != 0 {
		t.Errorf("LLM Verify NÃO deveria ter sido chamada para fingerprint rejeitado")
	}

	var hasBlockedReason bool
	for _, r := range res.StructuralReasons {
		if r == "fingerprint_previously_rejected" {
			hasBlockedReason = true
			break
		}
	}
	if !hasBlockedReason {
		t.Errorf("motivo 'fingerprint_previously_rejected' esperado, obtido: %v", res.StructuralReasons)
	}
}

func TestEvaluateCandidate_StructuralGateFailure_UnreachableSource(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-source-down",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Proposição com link quebrado",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/link-quebrado",
		CanonicalUrl:               "https://noticias.exemplo.com/link-quebrado",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria...",
		TechnicalConfidence:        0.9,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	var verifyCalls int32
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			atomic.AddInt32(&verifyCalls, 1)
			return nil, nil
		},
	}

	mockVerifier := &mockSourceVerifier{
		checkFn: func(ctx context.Context, rawURL string) sourcecheck.CheckResult {
			return sourcecheck.CheckResult{
				URL:             rawURL,
				Status:          domain.SourceAccessUnreachable,
				TechnicalReason: sourcecheck.ReasonHTTP404,
				HTTPStatus:      404,
			}
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: mockVerifier,
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.StructuralPassed {
		t.Fatalf("candidato com fonte inacessível DEVE falhar no gate estrutural")
	}
	if atomic.LoadInt32(&verifyCalls) != 0 {
		t.Errorf("LLM Verify NÃO deveria ser chamada para fonte inacessível")
	}
}

func TestEvaluateCandidate_SemanticGateFailure_OverstatesSource(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-sem-overstates",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Proposição que extrapola a fonte",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-extrapolada",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-extrapolada",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "A matéria relata apenas que o investidor esteve no evento.",
		TechnicalConfidence:        0.9,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           false,
				ClaimOverstatesSource:    true, // Extrapolou a fonte!
				AttributionExplicit:      false,
				GradeCompatible:          false,
				ContainsIllicitInference: true,
				Uncertainties:            []string{"A fonte menciona apenas presença no evento sem aquisição societária"},
				RecommendedAction:        "quarantine",
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.Published || res.SemanticPassed {
		t.Fatalf("candidato que extrapola a fonte DEVE reprovar no gate semântico")
	}

	// Avaliação semântica DEVE estar persistida para auditoria
	queries := sqlc.New(db)
	evals, err := queries.ListSemanticEvaluationsByCandidateID(ctx, candID)
	if err != nil {
		t.Fatalf("falha ao listar semantic_evaluations: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("esperava 1 avaliação semântica gravada para auditoria, obtido %d", len(evals))
	}
	if evals[0].ClaimOverstatesSource != 1 || evals[0].ClaimSupported != 0 {
		t.Errorf("dados de reprovação semântica incorretos: %+v", evals[0])
	}

	// Nenhum claim criado
	var claimsCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM claims").Scan(&claimsCount)
	if claimsCount != 0 {
		t.Errorf("nenhum claim deveria ter sido criado")
	}
}

func TestEvaluateCandidate_OpenRouterError_FailClosedQuarantine(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-provider-err",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Proposição com erro na chamada do provedor",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia",
		CanonicalUrl:               "https://noticias.exemplo.com/materia",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria...",
		TechnicalConfidence:        0.9,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return nil, errors.New("openrouter: 429 rate limit exceeded")
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate não deveria propagar erro operacional como fatal Go: %v", err)
	}

	if res.Published {
		t.Fatalf("erro de provedor LLM deve reter em quarentena (fail closed)")
	}
	if res.EditorialStatus != "quarantined" {
		t.Errorf("status esperado 'quarantined', obtido %q", res.EditorialStatus)
	}

	queries := sqlc.New(db)
	candDB, err := queries.GetMonitoringCandidateByID(ctx, candID)
	if err != nil {
		t.Fatalf("falha ao consultar candidato: %v", err)
	}
	if candDB.EditorialStatus != "quarantined" {
		t.Errorf("editorial_status no banco esperado 'quarantined', obtido %q", candDB.EditorialStatus)
	}
}

func TestEvaluateCandidate_SourceReuse(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	// 1. Pré-insere uma source existente com a mesma URL canônica
	existingSourceID := "src-existing-" + uuid.NewString()[:8]
	now := time.Now().UTC().Format(time.RFC3339Nano)
	queries := sqlc.New(db)
	_, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                    existingSourceID,
		CanonicalUrl:          "https://noticias.exemplo.com/materia-reutilizada",
		Title:                 "Título Antigo",
		PublisherOrAuthor:     "Veículo Original",
		OriginalUrl:           "https://noticias.exemplo.com/materia-reutilizada",
		PublishedAt:           sql.NullString{String: "2026-09-01", Valid: true},
		AccessedAt:            sql.NullString{String: now, Valid: true},
		SourceType:            "article",
		SourceAccessStatus:    "not_checked",
		SourceAccessCheckedAt: sql.NullString{String: now, Valid: true},
		HttpStatus:            sql.NullInt64{Int64: 200, Valid: true},
		NormalizedErrorCode:   sql.NullString{Valid: false},
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	if err != nil {
		t.Fatalf("falha ao criar source existente: %v", err)
	}

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-source-reuse",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro assume como controlador",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-reutilizada",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-reutilizada",
		SourceTitle:                "Título Atualizado",
		PublisherOrAuthor:          "Veículo Original",
		Excerpt:                    "Daniel Vorcaro assume como controlador após assembleia...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				RecommendedAction:        "publish",
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if !res.Published {
		t.Fatalf("publicação deveria ter ocorrido com reuso de source")
	}

	// Valida que o número total de sources para esta URL ainda é 1 (foi reutilizada)
	var sourceCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE canonical_url = ?", "https://noticias.exemplo.com/materia-reutilizada").Scan(&sourceCount)
	if err != nil {
		t.Fatalf("falha ao consultar sources: %v", err)
	}
	if sourceCount != 1 {
		t.Errorf("esperava exatamente 1 registro em sources (reutilizado), obtido %d", sourceCount)
	}

	src, err := queries.GetSourceByCanonicalURL(ctx, "https://noticias.exemplo.com/materia-reutilizada")
	if err != nil {
		t.Fatalf("falha ao buscar source: %v", err)
	}
	if src.ID != existingSourceID {
		t.Errorf("ID da source esperado %q, obtido %q", existingSourceID, src.ID)
	}
	if src.SourceAccessStatus != string(domain.SourceAccessReachable) {
		t.Errorf("status da source deveria ter sido atualizado para 'reachable', obtido %q", src.SourceAccessStatus)
	}
}

func TestEvaluateRunCandidates(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Candidato 1: Grau A Válido -> Deve publicar
	cand1ID := "cand-run-01"
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         cand1ID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-batch-01",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro é controlador do Banco Master",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/batch-01",
		CanonicalUrl:               "https://noticias.exemplo.com/batch-01",
		SourceTitle:                "Matéria 1",
		PublisherOrAuthor:          "Autor 1",
		Excerpt:                    "Trecho literal da matéria 1...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	// Candidato 2: Grau D -> Deve permanecer em quarentena
	cand2ID := "cand-run-02"
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         cand2ID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-batch-02",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Rumor não qualificado",
		SuggestedGrade:             "D",
		SourceUrl:                  "https://forum.exemplo.com/batch-02",
		CanonicalUrl:               "https://forum.exemplo.com/batch-02",
		SourceTitle:                "Matéria 2",
		PublisherOrAuthor:          "Autor 2",
		Excerpt:                    "Trecho literal da matéria 2...",
		TechnicalConfidence:        0.8,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	// Candidato 3: Duplicata intra-run -> Deve ser ignorado pelo pipeline canônico
	cand3ID := "cand-run-03"
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         cand3ID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-batch-01",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro é controlador do Banco Master",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/batch-01",
		CanonicalUrl:               "https://noticias.exemplo.com/batch-01",
		SourceTitle:                "Matéria 1",
		PublisherOrAuthor:          "Autor 1",
		Excerpt:                    "Trecho literal da matéria 1...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                1,
		DuplicateReason:            "same_run_duplicate",
		CanonicalCandidateID:       sql.NullString{String: cand1ID, Valid: true},
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				RecommendedAction:        "publish",
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	results, err := evaluator.EvaluateCandidatesInRun(ctx, runID)
	if err != nil {
		t.Fatalf("EvaluateCandidatesInRun falhou: %v", err)
	}

	// 2 candidatos canônicos (candidato duplicado foi ignorado)
	if len(results) != 2 {
		t.Fatalf("esperava 2 resultados avaliados no run (canônicos), obtido %d", len(results))
	}

	var pubCount, quarCount int
	for _, r := range results {
		if r.Published {
			pubCount++
		} else {
			quarCount++
		}
	}

	if pubCount != 1 {
		t.Errorf("esperava exatamente 1 publicado (cand 1), obtido %d", pubCount)
	}
	if quarCount != 1 {
		t.Errorf("esperava 1 em quarentena (cand 2), obtido %d", quarCount)
	}
}

func TestEvaluateCandidate_AmbiguousEntityResolution_Quarantined(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Cria duas entidades distintas com o mesmo normalized_name (ex: homônimos com IDs e slugs distintos)
	subjID1 := "ent-homonimo-1"
	subjID2 := "ent-homonimo-2"
	seedEntity(t, db, subjID1, "Daniel Vorcaro", "person")

	// Insere a segunda com mesmo normalized_name diretamente
	now := time.Now().UTC().Format(time.RFC3339Nano)
	queries := sqlc.New(db)
	_, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 subjID2,
		Type:               "person",
		Name:               "Daniel Vorcaro Filho",
		NormalizedName:     normalize.Name("Daniel Vorcaro"), // mesmo normalized_name propositalmente para simular ambiguidade
		Slug:               "daniel-vorcaro-filho",
		Category:           "pessoa",
		RoleOrContext:      "Contexto 2",
		Reach:              "nacional",
		Summary:            "Resumo",
		Relevance:          2,
		RelevanceRationale: "Homonimo",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar segunda entidade: %v", err)
	}

	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-ambiguous-ent",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Proposição com ambiguidade de entidade",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia",
		CanonicalUrl:               "https://noticias.exemplo.com/materia",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	var verifyCalls int32
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			atomic.AddInt32(&verifyCalls, 1)
			return nil, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.StructuralPassed {
		t.Fatalf("entidade ambígua DEVE reprovar no gate estrutural")
	}
	if atomic.LoadInt32(&verifyCalls) != 0 {
		t.Errorf("LLM Verify NÃO deveria ter sido chamada para entidade ambígua")
	}

	var hasAmbiguousReason bool
	for _, r := range res.StructuralReasons {
		if r == "ambiguous_subject_entity" {
			hasAmbiguousReason = true
			break
		}
	}
	if !hasAmbiguousReason {
		t.Errorf("motivo 'ambiguous_subject_entity' esperado, obtido: %v", res.StructuralReasons)
	}
}

func TestEvaluateCandidate_NoSelfRelation_StructuralGateFailure(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")

	// Cria um alias para Daniel Vorcaro que tem um nome diferente, mas aponta para o mesmo ID
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := queries.CreateEntityAlias(ctx, sqlc.CreateEntityAliasParams{
		ID:              "alias-" + uuid.NewString()[:8],
		EntityID:        subjID,
		Alias:           "Daniel Vorcaro Holding",
		NormalizedAlias: normalize.Name("Daniel Vorcaro Holding"),
		CreatedAt:       now,
	})
	if err != nil {
		t.Fatalf("falha ao criar alias: %v", err)
	}

	runID := createTestMonitoringRun(t, db)
	candID := "cand-" + uuid.NewString()[:8]

	// Candidato onde TargetEntityName resolve para a mesma entidade do sujeito via alias
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-self-rel",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Daniel Vorcaro Holding",
		NormalizedTargetEntityName: normalize.Name("Daniel Vorcaro Holding"),
		RelationshipType:           "partner_of",
		Proposition:                "Daniel Vorcaro é sócio de sua própria holding",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/materia-auto",
		CanonicalUrl:               "https://noticias.exemplo.com/materia-auto",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho literal da matéria sobre a holding...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	var verifyCalls int32
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			atomic.AddInt32(&verifyCalls, 1)
			return nil, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}

	if res.StructuralPassed {
		t.Fatalf("auto-relação deve reprovar no gate estrutural")
	}
	if atomic.LoadInt32(&verifyCalls) != 0 {
		t.Errorf("LLM Verify NÃO deveria ter sido chamada em auto-relação")
	}

	var hasSelfRelReason bool
	for _, r := range res.StructuralReasons {
		if r == "self_relation_prohibited" {
			hasSelfRelReason = true
			break
		}
	}
	if !hasSelfRelReason {
		t.Errorf("motivo 'self_relation_prohibited' esperado em res.StructuralReasons: %v", res.StructuralReasons)
	}
}

func TestEvaluatePendingQuarantined(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	candID := "cand-pending-01"
	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-pending-01",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro é controlador do Banco Master",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/pending-01",
		CanonicalUrl:               "https://noticias.exemplo.com/pending-01",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho comprobatório...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				RecommendedAction:        "publish",
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	results, err := evaluator.EvaluatePendingQuarantined(ctx, 10)
	if err != nil {
		t.Fatalf("EvaluatePendingQuarantined falhou: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("esperava 1 candidato em quarentena avaliado, obtido %d", len(results))
	}
	if !results[0].Published {
		t.Errorf("candidato pendente válido deveria ter sido publicado")
	}
}

func TestEvaluateCandidate_SourceCheckPersistsRealNon200StatusAndCodes(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	effectiveTime := time.Date(2026, 9, 11, 15, 30, 0, 0, time.UTC)

	candID := "cand-src-check-" + uuid.NewString()[:8]
	canonicalURL := "https://noticias.exemplo.com/materia-status-204"

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-src-check-204",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro no Banco Master",
		SuggestedGrade:             "A",
		SourceUrl:                  canonicalURL,
		CanonicalUrl:               canonicalURL,
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho comprobatório...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	mockVerifier := &mockSourceVerifier{
		checkFn: func(ctx context.Context, rawURL string) sourcecheck.CheckResult {
			return sourcecheck.CheckResult{
				URL:                 rawURL,
				SanitizedURL:        rawURL,
				Status:              domain.SourceAccessReachable,
				TechnicalReason:     sourcecheck.ReasonOK,
				HTTPStatus:          204,
				NormalizedErrorCode: "204_no_content",
				CheckedAt:           effectiveTime,
			}
		},
	}

	mockProvider := &mockResearchProvider{
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
			}, nil
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: mockVerifier,
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate falhou: %v", err)
	}
	if !res.Published {
		t.Fatalf("candidato deveria ter sido publicado com sucesso")
	}

	// Consulta o registro da source no banco e valida os campos reais do sourcecheck
	queries := sqlc.New(db)
	source, err := queries.GetSourceByCanonicalURL(ctx, canonicalURL)
	if err != nil {
		t.Fatalf("GetSourceByCanonicalURL falhou: %v", err)
	}

	if source.SourceAccessStatus != string(domain.SourceAccessReachable) {
		t.Errorf("status de acesso esperado 'reachable', obtido %q", source.SourceAccessStatus)
	}
	if !source.HttpStatus.Valid || source.HttpStatus.Int64 != 204 {
		t.Errorf("http_status no banco esperado 204, obtido %v", source.HttpStatus)
	}
	if !source.NormalizedErrorCode.Valid || source.NormalizedErrorCode.String != "204_no_content" {
		t.Errorf("normalized_error_code esperado '204_no_content', obtido %v", source.NormalizedErrorCode)
	}
	if !source.SourceAccessCheckedAt.Valid || source.SourceAccessCheckedAt.String != effectiveTime.Format(time.RFC3339Nano) {
		t.Errorf("source_access_checked_at esperado %s, obtido %v", effectiveTime.Format(time.RFC3339Nano), source.SourceAccessCheckedAt)
	}
}

func TestEvaluateCandidate_ConcurrentEvaluationPublishesExactlyOnce(t *testing.T) {
	for iteration := 1; iteration <= 10; iteration++ {
		t.Run(fmt.Sprintf("Iteration_%d", iteration), func(t *testing.T) {
			db, ctx := setupTestDB(t)

			subjID := "ent-" + uuid.NewString()[:8]
			targetID := "ent-" + uuid.NewString()[:8]
			seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
			seedEntity(t, db, targetID, "Banco Master", "organization")

			runID := createTestMonitoringRun(t, db)
			now := time.Now().UTC().Format(time.RFC3339Nano)

			candID := "cand-concurrent-" + uuid.NewString()[:8]
			canonicalURL := fmt.Sprintf("https://noticias.exemplo.com/materia-concorrente-%d", iteration)

			seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
				ID:                         candID,
				MonitoringRunID:            runID,
				Fingerprint:                fmt.Sprintf("fp-concurrent-%d-%s", iteration, uuid.NewString()[:6]),
				FingerprintVersion:         1,
				EntityName:                 "Daniel Vorcaro",
				NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
				TargetEntityName:           "Banco Master",
				NormalizedTargetEntityName: normalize.Name("Banco Master"),
				RelationshipType:           "controller_of",
				Proposition:                "Daniel Vorcaro é controlador do Banco Master",
				SuggestedGrade:             "A",
				SourceUrl:                  canonicalURL,
				CanonicalUrl:               canonicalURL,
				SourceTitle:                "Matéria Concorrente",
				PublisherOrAuthor:          "Autor",
				Excerpt:                    "Trecho comprobatório...",
				TechnicalConfidence:        0.95,
				RawPayload:                 "{}",
				EditorialStatus:            "quarantined",
				IsDuplicate:                0,
				CreatedAt:                  now,
				UpdatedAt:                  now,
			})

			mockProvider := &mockResearchProvider{
				verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
					// Simula pequena latência de I/O externo
					time.Sleep(5 * time.Millisecond)
					return &research.VerifyResult{
						IdentityMatch:            true,
						ClaimSupported:           true,
						ClaimOverstatesSource:    false,
						AttributionExplicit:      true,
						GradeCompatible:          true,
						ContainsIllicitInference: false,
						Uncertainties:            []string{},
						RecommendedAction:        "publish",
					}, nil
				},
			}

			evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
				DB:             db,
				Provider:       mockProvider,
				SourceVerifier: &mockSourceVerifier{},
			})
			if err != nil {
				t.Fatalf("falha ao criar Evaluator: %v", err)
			}

			// Executa 2 goroutines chamando EvaluateCandidate concorrentemente no mesmo candidato
			type evalOutput struct {
				res *monitoring.EvaluationResult
				err error
			}
			resultsChan := make(chan evalOutput, 2)

			for i := 0; i < 2; i++ {
				go func() {
					r, e := evaluator.EvaluateCandidate(ctx, candID)
					resultsChan <- evalOutput{res: r, err: e}
				}()
			}

			out1 := <-resultsChan
			out2 := <-resultsChan

			// Nenhuma execução pode falhar com erro genérico de SQLite (ex: database is locked)
			for idx, out := range []evalOutput{out1, out2} {
				if out.err != nil {
					errMsg := strings.ToLower(out.err.Error())
					if strings.Contains(errMsg, "database is locked") || strings.Contains(errMsg, "sqlite_busy") {
						t.Fatalf("execução %d retornou erro de lock não tratado: %v", idx+1, out.err)
					}
				}
			}

			// Identifica vencedores (publicação bem-sucedida) e perdedores
			var winners, losers []evalOutput
			for _, out := range []evalOutput{out1, out2} {
				if out.err == nil && out.res != nil && out.res.Published && out.res.PublishedClaimID != "" {
					winners = append(winners, out)
				} else {
					losers = append(losers, out)
				}
			}

			if len(winners) == 0 {
				t.Fatalf("nenhuma execução conseguiu publicar com sucesso: out1=(%+v, %v), out2=(%+v, %v)", out1.res, out1.err, out2.res, out2.err)
			}

			// Se ambos retornaram res.Published == true, devem apontar para o mesmo claim (uma publicou e a outra leu como já publicado)
			if len(winners) == 2 {
				if winners[0].res.PublishedClaimID != winners[1].res.PublishedClaimID {
					t.Fatalf("ambas publicaram com IDs diferentes: %s vs %s", winners[0].res.PublishedClaimID, winners[1].res.PublishedClaimID)
				}
			} else {
				// 1 winner, 1 loser: o loser DEVE retornar ErrCandidateNoLongerEligibleForPublication
				loser := losers[0]
				if loser.err == nil {
					t.Fatalf("execução perdedora deveria ter retornado erro sentinela ou detectado publicação, obtido res=%+v", loser.res)
				}
				if !errors.Is(loser.err, monitoring.ErrCandidateNoLongerEligibleForPublication) {
					t.Fatalf("execução perdedora retornou erro inesperado: %v (esperava %v)", loser.err, monitoring.ErrCandidateNoLongerEligibleForPublication)
				}
			}

			// Validações estritas de integridade no banco de dados
			var claimCount, evidenceCount, evidenceSourceCount, semanticEvalCount int
			err = db.QueryRowContext(ctx, "SELECT count(*) FROM claims WHERE origin = 'openrouter'").Scan(&claimCount)
			if err != nil {
				t.Fatalf("falha ao contar claims: %v", err)
			}
			if claimCount != 1 {
				t.Errorf("esperava exatamente 1 claim publicado na concorrência, obtido %d", claimCount)
			}

			err = db.QueryRowContext(ctx, "SELECT count(*) FROM evidence").Scan(&evidenceCount)
			if err != nil {
				t.Fatalf("falha ao contar evidence: %v", err)
			}
			if evidenceCount != 1 {
				t.Errorf("esperava exatamente 1 evidence na concorrência, obtido %d", evidenceCount)
			}

			err = db.QueryRowContext(ctx, "SELECT count(*) FROM evidence_sources").Scan(&evidenceSourceCount)
			if err != nil {
				t.Fatalf("falha ao contar evidence_sources: %v", evidenceSourceCount)
			}
			if evidenceSourceCount != 1 {
				t.Errorf("esperava exatamente 1 evidence_source na concorrência, obtido %d", evidenceSourceCount)
			}

			err = db.QueryRowContext(ctx, "SELECT count(*) FROM semantic_evaluations WHERE monitoring_candidate_id = ?", candID).Scan(&semanticEvalCount)
			if err != nil {
				t.Fatalf("falha ao contar semantic_evaluations: %v", err)
			}
			if semanticEvalCount != 1 {
				t.Errorf("esperava exatamente 1 semantic_evaluations gravado, obtido %d", semanticEvalCount)
			}

			queries := sqlc.New(db)
			cand, err := queries.GetMonitoringCandidateByID(ctx, candID)
			if err != nil {
				t.Fatalf("GetMonitoringCandidateByID falhou: %v", err)
			}
			if cand.EditorialStatus != "published" {
				t.Errorf("status do candidato esperado 'published', obtido %q", cand.EditorialStatus)
			}
			if !cand.PublishedClaimID.Valid || cand.PublishedClaimID.String == "" {
				t.Errorf("candidato deve possuir published_claim_id preenchido")
			}
		})
	}
}

func TestEvaluateCandidate_VerifyErrorPersistenceFailureAudit(t *testing.T) {
	db, ctx := setupTestDB(t)

	subjID := "ent-" + uuid.NewString()[:8]
	targetID := "ent-" + uuid.NewString()[:8]
	seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
	seedEntity(t, db, targetID, "Banco Master", "organization")

	runID := createTestMonitoringRun(t, db)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	candID := "cand-verify-err-" + uuid.NewString()[:8]

	seedCandidate(t, db, sqlc.CreateMonitoringCandidateParams{
		ID:                         candID,
		MonitoringRunID:            runID,
		Fingerprint:                "fp-verify-err",
		FingerprintVersion:         1,
		EntityName:                 "Daniel Vorcaro",
		NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
		TargetEntityName:           "Banco Master",
		NormalizedTargetEntityName: normalize.Name("Banco Master"),
		RelationshipType:           "controller_of",
		Proposition:                "Daniel Vorcaro é controlador do Banco Master",
		SuggestedGrade:             "A",
		SourceUrl:                  "https://noticias.exemplo.com/verify-err",
		CanonicalUrl:               "https://noticias.exemplo.com/verify-err",
		SourceTitle:                "Matéria",
		PublisherOrAuthor:          "Autor",
		Excerpt:                    "Trecho comprobatório...",
		TechnicalConfidence:        0.95,
		RawPayload:                 "{}",
		EditorialStatus:            "quarantined",
		IsDuplicate:                0,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	})

	// Provedor falha no Verify
	mockProvider := &mockResearchProvider{
		verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
			return nil, errors.New("openrouter: timeout interno")
		},
	}

	evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
		DB:             db,
		Provider:       mockProvider,
		SourceVerifier: &mockSourceVerifier{},
	})
	if err != nil {
		t.Fatalf("falha ao criar Evaluator: %v", err)
	}

	res, err := evaluator.EvaluateCandidate(ctx, candID)
	if err != nil {
		t.Fatalf("EvaluateCandidate com erro de verify gravado com sucesso no banco não deve retornar erro Go fatal: %v", err)
	}

	if res.Published {
		t.Errorf("candidato com erro de verify NÃO pode ser publicado")
	}
	if res.EditorialStatus != "quarantined" {
		t.Errorf("status editorial esperado 'quarantined', obtido %q", res.EditorialStatus)
	}
	if res.Error == nil || !strings.Contains(res.Error.Error(), "timeout interno") {
		t.Errorf("esperava erro contextualizado de timeout interno em res.Error, obtido %v", res.Error)
	}

	// Garante que nenhum claim público foi criado
	var claimCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM claims").Scan(&claimCount)
	if claimCount != 0 {
		t.Errorf("nenhum claim deveria ter sido criado, obtido %d", claimCount)
	}
}
