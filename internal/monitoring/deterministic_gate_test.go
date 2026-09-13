package monitoring_test

import (
	"context"
	"database/sql"
	"errors"
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

// TestDeterministic_PublishAndQuarantine_TableDriven é o gate final de testes determinísticos
// que comprova a matriz completa de publicação automática (Graus A, B, C) versus quarentena (D, E e falhas).
func TestDeterministic_PublishAndQuarantine_TableDriven(t *testing.T) {
	type testCase struct {
		name                 string
		grade                string
		proposition          string
		contextLimits        string
		targetType           string // "entity" ou "case"
		mutateCandidate      func(p *sqlc.CreateMonitoringCandidateParams)
		sourceStatus         domain.SourceAccessStatus
		verifyResult         *research.VerifyResult
		verifyErr            error
		expectPublished      bool
		expectEditorialState string
		expectDisposition    domain.ClaimDisposition
		expectMetricEligible bool
		expectPolicyAction   domain.PolicyAction
		expectReasonContains string
		expectVerifyCalled   bool
	}

	tests := []testCase{
		// --- GRAU A ---
		{
			name:          "Grau A: Gates estrutural e semântico aprovados -> Publicação com supports_link e metric_eligible=1",
			grade:         "A",
			proposition:   "Aquisição de controle societário do Banco Master por Daniel Vorcaro conforme registro formal",
			contextLimits: "Aprovado pelo regulador",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
			},
			expectPublished:      true,
			expectEditorialState: "published",
			expectDisposition:    domain.DispositionSupportsLink,
			expectMetricEligible: true,
			expectPolicyAction:   domain.PolicyActionPublish,
			expectReasonContains: "grade_ab_approved",
			expectVerifyCalled:   true,
		},
		{
			name:          "Grau A: Falha semântica (extrapolação de fonte) -> Quarentena auditável",
			grade:         "A",
			proposition:   "Aquisição de controle societário do Banco Master",
			contextLimits: "Limites societários",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    true, // Extrapolou a fonte!
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "quarantine",
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "semantic_claim_overstates_source",
			expectVerifyCalled:   true,
		},
		{
			name:          "Grau A: Falha semântica (presença de incertezas) -> Quarentena auditável",
			grade:         "A",
			proposition:   "Aquisição de controle societário do Banco Master",
			contextLimits: "Limites societários",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{"Data do documento incompatível com o relato"},
				RecommendedAction:        "quarantine",
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "semantic_uncertainty",
			expectVerifyCalled:   true,
		},
		{
			name:                 "Grau A: Falha estrutural (fonte unreachable) -> Quarentena sem chamar LLM Verify",
			grade:                "A",
			proposition:          "Aquisição societária",
			contextLimits:        "Limites",
			targetType:           "entity",
			sourceStatus:         domain.SourceAccessUnreachable,
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "source_unreachable",
			expectVerifyCalled:   false,
		},

		// --- GRAU B ---
		{
			name:          "Grau B: Gates aprovados com caso contextual -> Publicação com supports_link e metric_eligible=1",
			grade:         "B",
			proposition:   "Depoimento prestado no âmbito da Operação Desdobramento",
			contextLimits: "Fase de instrução",
			targetType:    "case",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
			},
			expectPublished:      true,
			expectEditorialState: "published",
			expectDisposition:    domain.DispositionSupportsLink,
			expectMetricEligible: true,
			expectPolicyAction:   domain.PolicyActionPublish,
			expectReasonContains: "grade_ab_approved",
			expectVerifyCalled:   true,
		},
		{
			name:          "Grau B: Falha semântica (atribuição não explícita) -> Quarentena",
			grade:         "B",
			proposition:   "Depoimento prestado no âmbito da Operação Desdobramento",
			contextLimits: "Fase de instrução",
			targetType:    "case",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      false, // Sem atribuição explícita!
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "quarantine",
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "semantic_attribution_not_explicit",
			expectVerifyCalled:   true,
		},

		// --- GRAU C ---
		{
			name:          "Grau C: Gates aprovados + context_limits explícito + linguagem cautelosa -> Publicação com possible_link",
			grade:         "C",
			proposition:   "Alega-se possível participação em reunião preliminar citada por interlocutores",
			contextLimits: "Relato preliminar sem auditoria pericial",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
			},
			expectPublished:      true,
			expectEditorialState: "published",
			expectDisposition:    domain.DispositionPossibleLink,
			expectMetricEligible: true,
			expectPolicyAction:   domain.PolicyActionPublish,
			expectReasonContains: "grade_c_approved_with_limits",
			expectVerifyCalled:   true,
		},
		{
			name:          "Grau C: Ausência de context_limits -> Quarentena",
			grade:         "C",
			proposition:   "Pessoa citada em apuração preliminar",
			contextLimits: "   ", // Vazio!
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "grade_c_missing_context_limits",
			expectVerifyCalled:   true,
		},
		{
			name:          "Grau C: Proposição afirmativa/categórica sem termos cautelares -> Quarentena",
			grade:         "C",
			proposition:   "Controle definitivo e total do patrimônio no exercício financeiro", // Sem termos de cautela/associação
			contextLimits: "Limites presentes",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "grade_c_lacks_conservative_language",
			expectVerifyCalled:   true,
		},

		// --- GRAUS D E E ---
		{
			name:          "Grau D: Sempre quarentena por padrão mesmo com parecer favorável da LLM",
			grade:         "D",
			proposition:   "Menção em artigo opinativo sobre o mercado",
			contextLimits: "Opinião de colunista",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish", // LLM recomendou publish, mas política Go deve forçar quarentena
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "grade_de_quarantine_default",
			expectVerifyCalled:   true,
		},
		{
			name:          "Grau E: Sempre quarentena por padrão mesmo com parecer favorável da LLM",
			grade:         "E",
			proposition:   "Citação indireta em comentário periférico",
			contextLimits: "Mera menção contextual",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			verifyResult: &research.VerifyResult{
				IdentityMatch:            true,
				ClaimSupported:           true,
				ClaimOverstatesSource:    false,
				AttributionExplicit:      true,
				GradeCompatible:          true,
				ContainsIllicitInference: false,
				Uncertainties:            []string{},
				RecommendedAction:        "publish",
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "grade_de_quarantine_default",
			expectVerifyCalled:   true,
		},

		// --- FALHAS DE PROVEDOR OPENROUTER ---
		{
			name:                 "OpenRouter: Timeout na chamada de verificação -> Fail-closed em quarentena",
			grade:                "A",
			proposition:          "Aquisição de controle societário no Banco Master",
			contextLimits:        "Limites contratuais",
			targetType:           "entity",
			sourceStatus:         domain.SourceAccessReachable,
			verifyErr:            context.DeadlineExceeded,
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "openrouter_verify_failed",
			expectVerifyCalled:   true,
		},
		{
			name:                 "OpenRouter: Erro 503 Service Unavailable -> Fail-closed em quarentena auditável",
			grade:                "A",
			proposition:          "Aquisição de controle societário no Banco Master",
			contextLimits:        "Limites contratuais",
			targetType:           "entity",
			sourceStatus:         domain.SourceAccessReachable,
			verifyErr:            errors.New("openrouter: status HTTP 503 - model overloaded"),
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "openrouter_verify_failed",
			expectVerifyCalled:   true,
		},

		// --- FALHAS ESTRUTURAIS (VERIFY NUNCA CHAMADO) ---
		{
			name:          "Gate Estrutural: Detecção de PII proibida (CPF na proposição) -> Quarentena sem chamar LLM",
			grade:         "A",
			proposition:   "Operação formalizada pelo portador do CPF 123.456.789-09 em cartório",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.Proposition = "Operação formalizada pelo portador do CPF 123.456.789-09 em cartório"
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "contains_unnecessary_pii",
			expectVerifyCalled:   false,
		},
		{
			name:          "Gate Estrutural: Detecção de PII proibida (Endereço residencial na proposição) -> Quarentena sem chamar LLM",
			grade:         "A",
			proposition:   "Mandado judicial cumprido na Rua Oscar Freire, 1500 em São Paulo",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.Proposition = "Mandado judicial cumprido na Rua Oscar Freire, 1500 em São Paulo"
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "contains_unnecessary_pii",
			expectVerifyCalled:   false,
		},
		{
			name:          "Gate Estrutural: Sujeito não existente na base -> Quarentena sem chamar LLM",
			grade:         "A",
			proposition:   "Proposição de entidade inexistente",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.EntityName = "Pessoa Não Cadastrada"
				p.NormalizedEntityName = "pessoa nao cadastrada"
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "unresolved_subject_entity",
			expectVerifyCalled:   false,
		},
		{
			name:          "Gate Estrutural: Alvo e Caso vazios simultaneamente (violação XOR) -> Quarentena",
			grade:         "A",
			proposition:   "Proposição sem alvo nem caso",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.TargetEntityName = ""
				p.NormalizedTargetEntityName = ""
				p.CaseName = ""
				p.NormalizedCaseName = ""
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "target_case_xor_violated",
			expectVerifyCalled:   false,
		},
		{
			name:          "Gate Estrutural: Alvo entidade não existente na base -> Quarentena",
			grade:         "A",
			proposition:   "Proposição com alvo não cadastrado",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.TargetEntityName = "Empresa Inexistente No Banco"
				p.NormalizedTargetEntityName = "empresa inexistente no banco"
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "unresolved_target_entity",
			expectVerifyCalled:   false,
		},
		{
			name:          "Gate Estrutural: Autorrelação por resolução de ID (via alias) -> Quarentena",
			grade:         "A",
			proposition:   "Daniel Vorcaro é sócio do seu alias",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.TargetEntityName = "Daniel Vorcaro Alias"
				p.NormalizedTargetEntityName = "daniel vorcaro alias"
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "self_relation_prohibited",
			expectVerifyCalled:   false,
		},
		{
			name:          "Gate Estrutural: URL de fonte com esquema inválido (ftp) -> Quarentena",
			grade:         "A",
			proposition:   "Proposição com URL inválida",
			contextLimits: "Limites",
			targetType:    "entity",
			sourceStatus:  domain.SourceAccessReachable,
			mutateCandidate: func(p *sqlc.CreateMonitoringCandidateParams) {
				p.SourceUrl = "ftp://arquivos.exemplo.com/doc.pdf"
				p.CanonicalUrl = "ftp://arquivos.exemplo.com/doc.pdf"
			},
			expectPublished:      false,
			expectEditorialState: "quarantined",
			expectPolicyAction:   domain.PolicyActionQuarantine,
			expectReasonContains: "invalid_canonical_url",
			expectVerifyCalled:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ctx := setupTestDB(t)

			// 1. Seed de Entidades e Casos conhecidos
			subjID := "ent-" + uuid.NewString()[:8]
			seedEntity(t, db, subjID, "Daniel Vorcaro", "person")

			queries := sqlc.New(db)
			now := time.Now().UTC().Format(time.RFC3339Nano)
			_, err := queries.CreateEntityAlias(ctx, sqlc.CreateEntityAliasParams{
				ID:              "alias-" + uuid.NewString()[:8],
				EntityID:        subjID,
				Alias:           "Daniel Vorcaro Alias",
				NormalizedAlias: normalize.Name("Daniel Vorcaro Alias"),
				CreatedAt:       now,
			})
			if err != nil {
				t.Fatalf("falha ao criar alias de teste: %v", err)
			}

			var targetEntityID, targetName, targetNorm string
			var caseID, caseName, caseNorm string

			if tt.targetType == "case" {
				caseID = "case-" + uuid.NewString()[:8]
				caseName = "Operação Desdobramento"
				caseNorm = normalize.Name(caseName)
				seedCase(t, db, caseID, caseName)
			} else {
				targetEntityID = "ent-" + uuid.NewString()[:8]
				targetName = "Banco Master"
				targetNorm = normalize.Name(targetName)
				seedEntity(t, db, targetEntityID, targetName, "organization")
			}

			runID := createTestMonitoringRun(t, db)
			candID := "cand-" + uuid.NewString()[:8]

			candParams := sqlc.CreateMonitoringCandidateParams{
				ID:                         candID,
				MonitoringRunID:            runID,
				Fingerprint:                "fp-" + uuid.NewString()[:12],
				FingerprintVersion:         1,
				EntityName:                 "Daniel Vorcaro",
				NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
				TargetEntityName:           targetName,
				NormalizedTargetEntityName: targetNorm,
				CaseName:                   caseName,
				NormalizedCaseName:         caseNorm,
				RelationshipType:           "societário",
				Proposition:                tt.proposition,
				SuggestedGrade:             tt.grade,
				SourceUrl:                  "https://noticias.exemplo.com/materia-teste-" + candID,
				CanonicalUrl:               "https://noticias.exemplo.com/materia-teste-" + candID,
				SourceTitle:                "Matéria Jornalística Teste",
				PublisherOrAuthor:          "Veículo de Notícias",
				PublishedAt:                sql.NullString{String: "2026-09-03", Valid: true},
				Excerpt:                    "Trecho comprobatório documentado da matéria...",
				Locator:                    "Pág. 1",
				ContextLimits:              tt.contextLimits,
				TechnicalConfidence:        0.90,
				RawPayload:                 "{}",
				EditorialStatus:            "quarantined",
				IsDuplicate:                0,
				CreatedAt:                  now,
				UpdatedAt:                  now,
			}

			if tt.mutateCandidate != nil {
				tt.mutateCandidate(&candParams)
			}

			seedCandidate(t, db, candParams)

			// 2. Mocks
			var verifyCalls int32
			mockProvider := &mockResearchProvider{
				verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
					atomic.AddInt32(&verifyCalls, 1)
					if tt.verifyErr != nil {
						return nil, tt.verifyErr
					}
					return tt.verifyResult, nil
				},
			}

			mockVerifier := &mockSourceVerifier{
				checkFn: func(ctx context.Context, rawURL string) sourcecheck.CheckResult {
					return sourcecheck.CheckResult{
						URL:             rawURL,
						SanitizedURL:    rawURL,
						Status:          tt.sourceStatus,
						TechnicalReason: sourcecheck.ReasonOK,
						HTTPStatus:      200,
						CheckedAt:       time.Now().UTC(),
					}
				},
			}

			evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
				DB:                db,
				Provider:          mockProvider,
				SourceVerifier:    mockVerifier,
				VerificationModel: "openai/gpt-4.1-mini",
			})
			if err != nil {
				t.Fatalf("falha ao instanciar Evaluator: %v", err)
			}

			// 3. Execução
			res, err := evaluator.EvaluateCandidate(ctx, candID)
			if err != nil {
				t.Fatalf("EvaluateCandidate retornou erro inesperado: %v", err)
			}

			// 4. Asserções do Veredito
			if res.Published != tt.expectPublished {
				t.Errorf("Published: esperado %v, obtido %v (motivos: %v)", tt.expectPublished, res.Published, res.PolicyReasons)
			}
			if res.EditorialStatus != tt.expectEditorialState {
				t.Errorf("EditorialStatus: esperado %q, obtido %q", tt.expectEditorialState, res.EditorialStatus)
			}
			if res.PolicyAction != tt.expectPolicyAction {
				t.Errorf("PolicyAction: esperado %q, obtido %q", tt.expectPolicyAction, res.PolicyAction)
			}

			// Validação de chamada da LLM
			called := atomic.LoadInt32(&verifyCalls) > 0
			if called != tt.expectVerifyCalled {
				t.Errorf("LLM Verify: esperado chamado=%v, obtido chamado=%v (calls=%d)", tt.expectVerifyCalled, called, verifyCalls)
			}

			// Validação de motivos
			if tt.expectReasonContains != "" {
				allReasons := append(res.StructuralReasons, append(res.SemanticReasons, res.PolicyReasons...)...)
				found := false
				for _, r := range allReasons {
					if strings.Contains(r, tt.expectReasonContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("esperava que motivos contivessem %q, obtido: %v", tt.expectReasonContains, allReasons)
				}
			}

			// 5. Asserções de Persistência no SQLite
			candDB, err := queries.GetMonitoringCandidateByID(ctx, candID)
			if err != nil {
				t.Fatalf("falha ao buscar candidato no banco: %v", err)
			}
			if candDB.EditorialStatus != tt.expectEditorialState {
				t.Errorf("editorial_status no banco: esperado %q, obtido %q", tt.expectEditorialState, candDB.EditorialStatus)
			}

			if tt.expectPublished {
				if !candDB.PublishedClaimID.Valid || candDB.PublishedClaimID.String == "" {
					t.Fatalf("published_claim_id deveria estar preenchido no candidato publicado")
				}
				claimID := candDB.PublishedClaimID.String
				claim, err := queries.GetClaimByID(ctx, claimID)
				if err != nil {
					t.Fatalf("falha ao buscar claim publicado %s: %v", claimID, err)
				}
				if claim.Status != "published" {
					t.Errorf("claim status: esperado 'published', obtido %q", claim.Status)
				}
				if claim.Disposition != string(tt.expectDisposition) {
					t.Errorf("claim disposition: esperado %q, obtido %q", tt.expectDisposition, claim.Disposition)
				}
				expectedMetricEligible := int64(0)
				if tt.expectMetricEligible {
					expectedMetricEligible = 1
				}
				if claim.MetricEligible != expectedMetricEligible {
					t.Errorf("claim metric_eligible: esperado %d, obtido %d", expectedMetricEligible, claim.MetricEligible)
				}

				// Validação de visibilidade pública
				var viewCount int
				_ = db.QueryRowContext(ctx, "SELECT count(*) FROM public_claims_view WHERE claim_id = ?", claimID).Scan(&viewCount)
				if viewCount != 1 {
					t.Errorf("claim publicado deve constar em public_claims_view, count=%d", viewCount)
				}

				// Validação em métricas públicas
				pubMetrics, err := metrics.GetPublicMetrics(ctx, queries, "", 5)
				if err != nil {
					t.Fatalf("falha ao consultar métricas: %v", err)
				}
				if pubMetrics.Overview.TotalClaims != 1 {
					t.Errorf("TotalClaims em métricas esperado 1, obtido %d", pubMetrics.Overview.TotalClaims)
				}
				if tt.expectMetricEligible && pubMetrics.Network.EligibleClaims != 1 {
					t.Errorf("EligibleClaims em rede esperado 1, obtido %d", pubMetrics.Network.EligibleClaims)
				}
			} else {
				if candDB.PublishedClaimID.Valid {
					t.Errorf("published_claim_id NÃO deveria ser preenchido para candidato não publicado: %v", candDB.PublishedClaimID)
				}
				var claimsCount int
				_ = db.QueryRowContext(ctx, "SELECT count(*) FROM claims").Scan(&claimsCount)
				if claimsCount != 0 {
					t.Errorf("nenhum claim editorial deveria ser criado para candidato em quarentena, obtido: %d", claimsCount)
				}
			}
		})
	}
}
