package monitoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/sourcecheck"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// Erros sentinela do avaliador.
var (
	ErrNilSourceVerifier                       = errors.New("monitoring: verificador de fontes não informado")
	ErrCandidateNoLongerEligibleForPublication = errors.New("monitoring: candidato não está mais elegível para publicação")
)

// EvaluatorConfig define as dependências e configurações para o avaliador de candidatos.
type EvaluatorConfig struct {
	DB                *sql.DB
	Provider          research.ResearchProvider
	SourceVerifier    sourcecheck.SourceVerifier
	VerificationModel string
	NowFunc           func() time.Time
}

// Evaluator orquestra os gates estrutural e semântico, a política Go e a materialização editorial atômica.
type Evaluator struct {
	db                *sql.DB
	provider          research.ResearchProvider
	sourceVerifier    sourcecheck.SourceVerifier
	verificationModel string
	nowFunc           func() time.Time
}

// NewEvaluator cria uma nova instância de Evaluator.
func NewEvaluator(cfg EvaluatorConfig) (*Evaluator, error) {
	if cfg.DB == nil {
		return nil, ErrNilDB
	}
	if cfg.Provider == nil {
		return nil, ErrNilProvider
	}
	if cfg.SourceVerifier == nil {
		return nil, ErrNilSourceVerifier
	}

	model := strings.TrimSpace(cfg.VerificationModel)
	if model == "" {
		model = "openai/gpt-4.1-mini"
	}

	nowFunc := cfg.NowFunc
	if nowFunc == nil {
		nowFunc = func() time.Time { return time.Now().UTC() }
	}

	return &Evaluator{
		db:                cfg.DB,
		provider:          cfg.Provider,
		sourceVerifier:    cfg.SourceVerifier,
		verificationModel: model,
		nowFunc:           nowFunc,
	}, nil
}

// EvaluationResult consolida os resultados dos gates e a deliberação editorial de um candidato.
type EvaluationResult struct {
	CandidateID       string              `json:"candidate_id"`
	StructuralPassed  bool                `json:"structural_passed"`
	StructuralReasons []string            `json:"structural_reasons"`
	SemanticPassed    bool                `json:"semantic_passed"`
	SemanticReasons   []string            `json:"semantic_reasons"`
	PolicyAction      domain.PolicyAction `json:"policy_action"`
	PolicyReasons     []string            `json:"policy_reasons"`
	EditorialStatus   string              `json:"editorial_status"`
	PublishedClaimID  string              `json:"published_claim_id,omitempty"`
	Published         bool                `json:"published"`
	Model             string              `json:"model,omitempty"`
	PromptTokens      int                 `json:"prompt_tokens,omitempty"`
	CompletionTokens  int                 `json:"completion_tokens,omitempty"`
	TotalTokens       int                 `json:"total_tokens,omitempty"`
	Cost              float64             `json:"cost,omitempty"`
	CostMicros        int64               `json:"cost_micros,omitempty"`
	Error             error               `json:"error,omitempty"`
}

// EvaluateCandidate executa o pipeline completo para um candidato:
// 1. Resolução determinística e unívoca de entidades e casos existentes;
// 2. Verificação de fingerprint rejeitado;
// 3. Verificação de acessibilidade da fonte via GET seguro (fora de transação);
// 4. Gate estrutural puro em Go;
// 5. Gate semântico via OpenRouter (fora de transação, somente se gate estrutural passou);
// 6. Política Go determinística por grau A–E;
// 7. Materialização atômica curta no SQLite (se aprovado para publicação) ou quarentena auditável.
func (e *Evaluator) EvaluateCandidate(ctx context.Context, candidateID string) (*EvaluationResult, error) {
	queries := sqlc.New(e.db)
	cand, err := queries.GetMonitoringCandidateByID(ctx, candidateID)
	if err != nil {
		return nil, fmt.Errorf("monitoring: candidato não encontrado (%s): %w", candidateID, err)
	}

	// Candidato duplicado é ignorado da avaliação canônica
	if cand.IsDuplicate == 1 {
		return &EvaluationResult{
			CandidateID:       candidateID,
			EditorialStatus:   cand.EditorialStatus,
			StructuralPassed:  false,
			StructuralReasons: []string{"duplicate_candidate"},
			PolicyAction:      domain.PolicyActionQuarantine,
			PolicyReasons:     []string{"duplicate_candidate"},
			Published:         false,
		}, nil
	}

	// Candidato já publicado
	if cand.EditorialStatus == string(domain.ClaimStatusPublished) && cand.PublishedClaimID.Valid {
		return &EvaluationResult{
			CandidateID:      candidateID,
			EditorialStatus:  cand.EditorialStatus,
			StructuralPassed: cand.StructuralGatePassed.Int64 == 1,
			SemanticPassed:   cand.SemanticGatePassed.Int64 == 1,
			PolicyAction:     domain.PolicyAction(cand.PolicyAction),
			PublishedClaimID: cand.PublishedClaimID.String,
			Published:        true,
		}, nil
	}

	// 1. Resolução determinística de entidades e casos existentes
	subjID, subjAmbiguous, err := e.resolveSubject(ctx, queries, cand.NormalizedEntityName)
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao resolver sujeito: %w", err)
	}

	targetID, targetAmbiguous, err := e.resolveTarget(ctx, queries, cand.NormalizedTargetEntityName)
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao resolver alvo: %w", err)
	}

	caseID, caseAmbiguous, err := e.resolveCase(ctx, queries, cand.CaseName, cand.NormalizedCaseName)
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao resolver caso: %w", err)
	}

	// 2. Consulta de bloqueio por fingerprint rejeitado anteriormente
	isRejected, err := queries.HasRejectedCandidateByFingerprint(ctx, cand.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao consultar histórico de fingerprint: %w", err)
	}

	// 3. Verificação de acessibilidade da fonte via GET seguro (fora de transação de banco)
	var sourceCheckResult sourcecheck.CheckResult
	var accessStatus domain.SourceAccessStatus = domain.SourceAccessNotChecked
	if e.sourceVerifier != nil && cand.CanonicalUrl != "" {
		sourceCheckResult = e.sourceVerifier.Check(ctx, cand.CanonicalUrl)
		accessStatus = sourceCheckResult.Status
	}

	// 4. Executa gate estrutural puro em Go
	structuralInput := domain.StructuralGateInput{
		Fingerprint:                   cand.Fingerprint,
		FingerprintVersion:            cand.FingerprintVersion,
		IsDuplicate:                   cand.IsDuplicate == 1,
		EntityName:                    cand.EntityName,
		NormalizedEntityName:          cand.NormalizedEntityName,
		TargetEntityName:              cand.TargetEntityName,
		NormalizedTargetEntityName:    cand.NormalizedTargetEntityName,
		CaseName:                      cand.CaseName,
		NormalizedCaseName:            cand.NormalizedCaseName,
		RelationshipType:              cand.RelationshipType,
		Proposition:                   cand.Proposition,
		SuggestedGrade:                domain.EvidenceGrade(cand.SuggestedGrade),
		SourceURL:                     cand.SourceUrl,
		CanonicalURL:                  cand.CanonicalUrl,
		SourceTitle:                   cand.SourceTitle,
		PublisherOrAuthor:             cand.PublisherOrAuthor,
		PublishedAt:                   cand.PublishedAt.String,
		Excerpt:                       cand.Excerpt,
		Locator:                       cand.Locator,
		ContextLimits:                 cand.ContextLimits,
		SubjectResolvedID:             subjID,
		SubjectAmbiguous:              subjAmbiguous,
		TargetResolvedID:              targetID,
		TargetAmbiguous:               targetAmbiguous,
		CaseResolvedID:                caseID,
		CaseAmbiguous:                 caseAmbiguous,
		PreviouslyRejectedFingerprint: isRejected,
		SourceAccessStatus:            accessStatus,
	}

	structuralRes := domain.EvaluateStructuralGate(structuralInput)
	nowStr := e.nowFunc().Format(time.RFC3339Nano)

	// Se reprovado no gate estrutural: NÃO chama Verify na LLM, grava motivos e mantém quarentena
	if !structuralRes.Passed {
		reasonsBytes, _ := json.Marshal(structuralRes.Reasons)
		policyReasonsBytes, _ := json.Marshal(append([]string{"structural_gate_failed"}, structuralRes.Reasons...))

		var subjNull, targetNull, caseNull sql.NullString
		if subjID != "" {
			subjNull = sql.NullString{String: subjID, Valid: true}
		}
		if targetID != "" && targetID != subjID {
			targetNull = sql.NullString{String: targetID, Valid: true}
		}
		if caseID != "" {
			caseNull = sql.NullString{String: caseID, Valid: true}
		}

		_, updateErr := queries.UpdateMonitoringCandidateGates(ctx, sqlc.UpdateMonitoringCandidateGatesParams{
			ID:                      cand.ID,
			StructuralGatePassed:    sql.NullInt64{Int64: 0, Valid: true},
			StructuralGateReasons:   string(reasonsBytes),
			SemanticGatePassed:      sql.NullInt64{Valid: false},
			SemanticGateReasons:     "[]",
			PolicyAction:            string(domain.PolicyActionQuarantine),
			PolicyReasons:           string(policyReasonsBytes),
			EditorialStatus:         string(domain.ClaimStatusQuarantined),
			ResolvedSubjectEntityID: subjNull,
			ResolvedTargetEntityID:  targetNull,
			ResolvedCaseID:          caseNull,
			PublishedClaimID:        sql.NullString{Valid: false},
			UpdatedAt:               nowStr,
		})
		if updateErr != nil {
			return nil, fmt.Errorf("monitoring: falha ao atualizar candidato reprovado no gate estrutural: %w", updateErr)
		}

		return &EvaluationResult{
			CandidateID:       cand.ID,
			StructuralPassed:  false,
			StructuralReasons: structuralRes.Reasons,
			SemanticPassed:    false,
			PolicyAction:      domain.PolicyActionQuarantine,
			PolicyReasons:     append([]string{"structural_gate_failed"}, structuralRes.Reasons...),
			EditorialStatus:   string(domain.ClaimStatusQuarantined),
			Published:         false,
		}, nil
	}

	// 5. Gate estrutural aprovado -> Executa Verify via OpenRouter FORA de transação SQLite
	verifyInput := research.VerifyInput{
		SubjectName:       cand.EntityName,
		TargetEntityName:  cand.TargetEntityName,
		CaseName:          cand.CaseName,
		RelationshipType:  cand.RelationshipType,
		Proposition:       cand.Proposition,
		Excerpt:           cand.Excerpt,
		SourceTitle:       cand.SourceTitle,
		PublisherOrAuthor: cand.PublisherOrAuthor,
		SourceURL:         cand.CanonicalUrl,
		Grade:             cand.SuggestedGrade,
		ContextLimits:     cand.ContextLimits,
	}

	verifyResult, verifyErr := e.provider.Verify(ctx, verifyInput)
	if verifyErr != nil || verifyResult == nil {
		if verifyErr == nil {
			verifyErr = research.ErrEmptyResponse
		}
		// Falha no provedor -> Fail closed em quarentena auditável
		errReasons := []string{"openrouter_verify_failed: " + verifyErr.Error()}
		errReasonsBytes, _ := json.Marshal(errReasons)
		policyReasonsBytes, _ := json.Marshal(append([]string{"semantic_gate_failed"}, errReasons...))

		var subjNull, targetNull, caseNull sql.NullString
		if subjID != "" {
			subjNull = sql.NullString{String: subjID, Valid: true}
		}
		if targetID != "" && targetID != subjID {
			targetNull = sql.NullString{String: targetID, Valid: true}
		}
		if caseID != "" {
			caseNull = sql.NullString{String: caseID, Valid: true}
		}

		_, updateErr := queries.UpdateMonitoringCandidateGates(ctx, sqlc.UpdateMonitoringCandidateGatesParams{
			ID:                      cand.ID,
			StructuralGatePassed:    sql.NullInt64{Int64: 1, Valid: true},
			StructuralGateReasons:   "[]",
			SemanticGatePassed:      sql.NullInt64{Int64: 0, Valid: true},
			SemanticGateReasons:     string(errReasonsBytes),
			PolicyAction:            string(domain.PolicyActionQuarantine),
			PolicyReasons:           string(policyReasonsBytes),
			EditorialStatus:         string(domain.ClaimStatusQuarantined),
			ResolvedSubjectEntityID: subjNull,
			ResolvedTargetEntityID:  targetNull,
			ResolvedCaseID:          caseNull,
			PublishedClaimID:        sql.NullString{Valid: false},
			UpdatedAt:               nowStr,
		})
		if updateErr != nil {
			return nil, fmt.Errorf("monitoring: falha operacional no verify (%v) e falha crítica ao persistir quarentena no banco (%w)", verifyErr, updateErr)
		}

		return &EvaluationResult{
			CandidateID:      cand.ID,
			StructuralPassed: true,
			SemanticPassed:   false,
			SemanticReasons:  errReasons,
			PolicyAction:     domain.PolicyActionQuarantine,
			PolicyReasons:    append([]string{"semantic_gate_failed"}, errReasons...),
			EditorialStatus:  string(domain.ClaimStatusQuarantined),
			Published:        false,
			Error:            verifyErr,
		}, nil
	}

	// 6. Avaliação semântica e deliberação da Política Go
	semanticRes := domain.EvaluateSemanticGate(domain.SemanticGateInput{
		IdentityMatch:            verifyResult.IdentityMatch,
		ClaimSupported:           verifyResult.ClaimSupported,
		ClaimOverstatesSource:    verifyResult.ClaimOverstatesSource,
		AttributionExplicit:      verifyResult.AttributionExplicit,
		GradeCompatible:          verifyResult.GradeCompatible,
		ContainsIllicitInference: verifyResult.ContainsIllicitInference,
		Uncertainties:            verifyResult.Uncertainties,
	})

	policyRes := domain.EvaluatePublishingPolicy(domain.PublishingPolicyInput{
		Grade:             domain.EvidenceGrade(cand.SuggestedGrade),
		StructuralPassed:  true,
		SemanticPassed:    semanticRes.Passed,
		Proposition:       cand.Proposition,
		ContextLimits:     cand.ContextLimits,
		StructuralReasons: structuralRes.Reasons,
		SemanticReasons:   semanticRes.Reasons,
	})

	var subjNull, targetNull, caseNull sql.NullString
	if subjID != "" {
		subjNull = sql.NullString{String: subjID, Valid: true}
	}
	if targetID != "" && targetID != subjID {
		targetNull = sql.NullString{String: targetID, Valid: true}
	}
	if caseID != "" {
		caseNull = sql.NullString{String: caseID, Valid: true}
	}

	semanticReasonsBytes, _ := json.Marshal(semanticRes.Reasons)
	policyReasonsBytes, _ := json.Marshal(policyRes.Reasons)

	// 7. Persistência e Materialização Atômica no SQLite
	var publishedClaimID string
	txErr := store.ExecTx(ctx, e.db, func(txQ *sqlc.Queries) error {
		if policyRes.Action == domain.PolicyActionPublish {
			// 1. Trava atômica condicional do candidato como PRIMEIRA operação de escrita
			claimedCand, err := txQ.ClaimQuarantinedCandidateForPublication(ctx, sqlc.ClaimQuarantinedCandidateForPublicationParams{
				UpdatedAt: nowStr,
				ID:        cand.ID,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("monitoring: candidato %s não está mais em quarentena ou já foi publicado/alterado: %w", cand.ID, ErrCandidateNoLongerEligibleForPublication)
				}
				return fmt.Errorf("monitoring: falha ao travar candidato para publicação: %w", err)
			}

			// 2. Comparação exaustiva de todos os campos que influenciam publicação/materialização
			if claimedCand.Fingerprint != cand.Fingerprint ||
				claimedCand.FingerprintVersion != cand.FingerprintVersion ||
				claimedCand.EntityName != cand.EntityName ||
				claimedCand.NormalizedEntityName != cand.NormalizedEntityName ||
				claimedCand.TargetEntityName != cand.TargetEntityName ||
				claimedCand.NormalizedTargetEntityName != cand.NormalizedTargetEntityName ||
				claimedCand.CaseName != cand.CaseName ||
				claimedCand.NormalizedCaseName != cand.NormalizedCaseName ||
				claimedCand.RelationshipType != cand.RelationshipType ||
				claimedCand.Proposition != cand.Proposition ||
				claimedCand.SuggestedGrade != cand.SuggestedGrade ||
				claimedCand.SourceUrl != cand.SourceUrl ||
				claimedCand.CanonicalUrl != cand.CanonicalUrl ||
				claimedCand.SourceTitle != cand.SourceTitle ||
				claimedCand.PublisherOrAuthor != cand.PublisherOrAuthor ||
				claimedCand.PublishedAt.String != cand.PublishedAt.String ||
				claimedCand.Excerpt != cand.Excerpt ||
				claimedCand.Locator != cand.Locator ||
				claimedCand.ContextLimits != cand.ContextLimits ||
				claimedCand.TechnicalConfidence != cand.TechnicalConfidence ||
				claimedCand.IsDuplicate != cand.IsDuplicate ||
				claimedCand.EditorialStatus != "quarantined" {
				return fmt.Errorf("monitoring: dados do candidato divergiram do snapshot avaliado: %w", ErrCandidateNoLongerEligibleForPublication)
			}

			// 3. Revalidação de fingerprint rejeitado dentro da mesma transação
			isRej, err := txQ.HasRejectedCandidateByFingerprint(ctx, claimedCand.Fingerprint)
			if err != nil {
				return fmt.Errorf("monitoring: falha ao verificar fingerprint rejeitado: %w", err)
			}
			if isRej {
				return fmt.Errorf("monitoring: candidato bloqueado por fingerprint rejeitado concorrentemente: %w", ErrCandidateNoLongerEligibleForPublication)
			}

			// 4. Grava histórico da avaliação semântica somente se a trava foi obtida
			uncertaintiesBytes, _ := json.Marshal(verifyResult.Uncertainties)
			rawRespBytes, _ := json.Marshal(verifyResult)

			modelUsed := verifyResult.Model
			if modelUsed == "" {
				modelUsed = e.verificationModel
			}

			costMicros := verifyResult.CostMicros

			_, err = txQ.CreateSemanticEvaluation(ctx, sqlc.CreateSemanticEvaluationParams{
				ID:                       uuid.NewString(),
				MonitoringCandidateID:    cand.ID,
				Provider:                 "openrouter",
				Model:                    modelUsed,
				SchemaVersion:            "v1",
				IdentityMatch:            boolToInt(verifyResult.IdentityMatch),
				ClaimSupported:           boolToInt(verifyResult.ClaimSupported),
				ClaimOverstatesSource:    boolToInt(verifyResult.ClaimOverstatesSource),
				AttributionExplicit:      boolToInt(verifyResult.AttributionExplicit),
				GradeCompatible:          boolToInt(verifyResult.GradeCompatible),
				ContainsIllicitInference: boolToInt(verifyResult.ContainsIllicitInference),
				Uncertainties:            string(uncertaintiesBytes),
				RecommendedAction:        verifyResult.RecommendedAction,
				RawResponse:              string(rawRespBytes),
				PromptTokens:             int64(verifyResult.PromptTokens),
				CompletionTokens:         int64(verifyResult.CompletionTokens),
				TotalTokens:              int64(verifyResult.TotalTokens),
				CostMicrousd:             costMicros,
				CreatedAt:                nowStr,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao persistir semantic_evaluations: %w", err)
			}

			// Atualização atômica do consumo acumulado na monitoring_run dentro da mesma transação
			if cand.MonitoringRunID != "" {
				_, err = txQ.IncrementMonitoringRunUsage(ctx, sqlc.IncrementMonitoringRunUsageParams{
					PromptTokens:       int64(verifyResult.PromptTokens),
					CompletionTokens:   int64(verifyResult.CompletionTokens),
					TotalTokens:        int64(verifyResult.TotalTokens),
					VerificationTokens: int64(verifyResult.TotalTokens),
					CostMicrousd:       costMicros,
					ID:                 cand.MonitoringRunID,
				})
				if err != nil {
					return fmt.Errorf("monitoring: falha ao atualizar consumo acumulado da run na transação: %w", err)
				}
			}

			// 5. Montagem dos dados reais retornados pelo sourcecheck
			var srcCheckedAt sql.NullString
			var srcHTTPStatus sql.NullInt64
			var srcErrCode sql.NullString

			if !sourceCheckResult.CheckedAt.IsZero() {
				srcCheckedAt = sql.NullString{String: sourceCheckResult.CheckedAt.Format(time.RFC3339Nano), Valid: true}
			} else {
				srcCheckedAt = sql.NullString{String: nowStr, Valid: true}
			}
			if sourceCheckResult.HTTPStatus > 0 {
				srcHTTPStatus = sql.NullInt64{Int64: int64(sourceCheckResult.HTTPStatus), Valid: true}
			}
			if sourceCheckResult.NormalizedErrorCode != "" {
				srcErrCode = sql.NullString{String: sourceCheckResult.NormalizedErrorCode, Valid: true}
			}

			// A. Reutiliza ou cria Source
			sourceID := uuid.NewString()
			existingSource, err := txQ.GetSourceByCanonicalURL(ctx, cand.CanonicalUrl)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("monitoring: falha ao buscar source existente por canonical_url: %w", err)
			}
			if err == nil && existingSource.ID != "" {
				sourceID = existingSource.ID
				_, updateSrcErr := txQ.UpdateSourceAccessStatus(ctx, sqlc.UpdateSourceAccessStatusParams{
					ID:                    sourceID,
					SourceAccessStatus:    string(accessStatus),
					SourceAccessCheckedAt: srcCheckedAt,
					HttpStatus:            srcHTTPStatus,
					NormalizedErrorCode:   srcErrCode,
					UpdatedAt:             nowStr,
				})
				if updateSrcErr != nil {
					return fmt.Errorf("monitoring: falha ao atualizar status de acesso da source existente: %w", updateSrcErr)
				}
			} else {
				srcTitle := cand.SourceTitle
				if srcTitle == "" {
					srcTitle = cand.CanonicalUrl
				}
				srcPublisher := cand.PublisherOrAuthor
				if srcPublisher == "" {
					srcPublisher = "Desconhecido"
				}

				_, err = txQ.CreateSource(ctx, sqlc.CreateSourceParams{
					ID:                    sourceID,
					Title:                 srcTitle,
					PublisherOrAuthor:     srcPublisher,
					OriginalUrl:           cand.SourceUrl,
					CanonicalUrl:          cand.CanonicalUrl,
					PublishedAt:           cand.PublishedAt,
					AccessedAt:            sql.NullString{String: nowStr, Valid: true},
					SourceType:            "article",
					SourceAccessStatus:    string(accessStatus),
					SourceAccessCheckedAt: srcCheckedAt,
					HttpStatus:            srcHTTPStatus,
					NormalizedErrorCode:   srcErrCode,
					CreatedAt:             nowStr,
					UpdatedAt:             nowStr,
				})
				if err != nil {
					return fmt.Errorf("monitoring: falha ao criar source: %w", err)
				}
			}

			// B. Reutiliza ou cria Relationship
			relID := uuid.NewString()
			existingRel, err := txQ.FindRelationshipByComponents(ctx, sqlc.FindRelationshipByComponentsParams{
				SubjectEntityID:  subjID,
				TargetEntityID:   targetNull,
				CaseID:           caseNull,
				RelationshipType: cand.RelationshipType,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("monitoring: falha ao buscar relationship existente: %w", err)
			}
			if err == nil && existingRel.ID != "" {
				relID = existingRel.ID
			} else {
				_, err = txQ.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
					ID:               relID,
					SubjectEntityID:  subjID,
					TargetEntityID:   targetNull,
					CaseID:           caseNull,
					RelationshipType: cand.RelationshipType,
					Summary:          cand.Proposition,
					ContextLimits:    cand.ContextLimits,
					CreatedAt:        nowStr,
					UpdatedAt:        nowStr,
				})
				if err != nil {
					return fmt.Errorf("monitoring: falha ao criar relationship: %w", err)
				}
			}

			// C. Cria Claim com metadados e disposição da política
			claimID := uuid.NewString()
			var metricEligibleInt int64 = 0
			if policyRes.MetricEligible {
				metricEligibleInt = 1
			}

			_, err = txQ.CreateClaim(ctx, sqlc.CreateClaimParams{
				ID:                claimID,
				RelationshipID:    relID,
				Proposition:       cand.Proposition,
				Attribution:       cand.PublisherOrAuthor,
				Origin:            string(domain.ClaimOriginOpenRouter),
				Grade:             cand.SuggestedGrade,
				Disposition:       string(policyRes.Disposition),
				MetricEligible:    metricEligibleInt,
				Status:            string(domain.ClaimStatusPublished),
				ContextStatus:     "",
				QuarantineReasons: "",
				ImportRunID:       sql.NullString{Valid: false},
				CreatedAt:         nowStr,
				UpdatedAt:         nowStr,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao criar claim: %w", err)
			}

			// D. Cria Evidence
			evidenceID := uuid.NewString()
			_, err = txQ.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
				ID:           evidenceID,
				ClaimID:      claimID,
				Summary:      cand.Proposition,
				EvidenceType: "document",
				CreatedAt:    nowStr,
				UpdatedAt:    nowStr,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao criar evidence: %w", err)
			}

			// E. Cria EvidenceSource ativo com papel supports
			evidenceSourceID := uuid.NewString()
			_, err = txQ.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
				ID:         evidenceSourceID,
				EvidenceID: evidenceID,
				SourceID:   sourceID,
				Excerpt:    cand.Excerpt,
				Locator:    cand.Locator,
				Role:       string(domain.RoleSupports),
				Status:     string(domain.EvidenceSourceStatusActive),
				CreatedAt:  nowStr,
				UpdatedAt:  nowStr,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao criar evidence_source: %w", err)
			}

			// F. Atualiza candidato com published_claim_id e editorial_status = 'published'
			_, err = txQ.UpdateMonitoringCandidateGates(ctx, sqlc.UpdateMonitoringCandidateGatesParams{
				ID:                      cand.ID,
				StructuralGatePassed:    sql.NullInt64{Int64: 1, Valid: true},
				StructuralGateReasons:   "[]",
				SemanticGatePassed:      sql.NullInt64{Int64: 1, Valid: true},
				SemanticGateReasons:     string(semanticReasonsBytes),
				PolicyAction:            string(domain.PolicyActionPublish),
				PolicyReasons:           string(policyReasonsBytes),
				EditorialStatus:         string(domain.ClaimStatusPublished),
				ResolvedSubjectEntityID: subjNull,
				ResolvedTargetEntityID:  targetNull,
				ResolvedCaseID:          caseNull,
				PublishedClaimID:        sql.NullString{String: claimID, Valid: true},
				UpdatedAt:               nowStr,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao atualizar candidato como publicado: %w", err)
			}

			publishedClaimID = claimID
			return nil
		}

		// Se a política determinou quarentena:
		uncertaintiesBytes, _ := json.Marshal(verifyResult.Uncertainties)
		rawRespBytes, _ := json.Marshal(verifyResult)

		modelUsed := verifyResult.Model
		if modelUsed == "" {
			modelUsed = e.verificationModel
		}

		costMicros := verifyResult.CostMicros

		_, err := txQ.CreateSemanticEvaluation(ctx, sqlc.CreateSemanticEvaluationParams{
			ID:                       uuid.NewString(),
			MonitoringCandidateID:    cand.ID,
			Provider:                 "openrouter",
			Model:                    modelUsed,
			SchemaVersion:            "v1",
			IdentityMatch:            boolToInt(verifyResult.IdentityMatch),
			ClaimSupported:           boolToInt(verifyResult.ClaimSupported),
			ClaimOverstatesSource:    boolToInt(verifyResult.ClaimOverstatesSource),
			AttributionExplicit:      boolToInt(verifyResult.AttributionExplicit),
			GradeCompatible:          boolToInt(verifyResult.GradeCompatible),
			ContainsIllicitInference: boolToInt(verifyResult.ContainsIllicitInference),
			Uncertainties:            string(uncertaintiesBytes),
			RecommendedAction:        verifyResult.RecommendedAction,
			RawResponse:              string(rawRespBytes),
			PromptTokens:             int64(verifyResult.PromptTokens),
			CompletionTokens:         int64(verifyResult.CompletionTokens),
			TotalTokens:              int64(verifyResult.TotalTokens),
			CostMicrousd:             costMicros,
			CreatedAt:                nowStr,
		})
		if err != nil {
			return fmt.Errorf("monitoring: falha ao persistir semantic_evaluations: %w", err)
		}

		// Atualização atômica do consumo acumulado na monitoring_run dentro da mesma transação
		if cand.MonitoringRunID != "" {
			_, err = txQ.IncrementMonitoringRunUsage(ctx, sqlc.IncrementMonitoringRunUsageParams{
				PromptTokens:       int64(verifyResult.PromptTokens),
				CompletionTokens:   int64(verifyResult.CompletionTokens),
				TotalTokens:        int64(verifyResult.TotalTokens),
				VerificationTokens: int64(verifyResult.TotalTokens),
				CostMicrousd:       costMicros,
				ID:                 cand.MonitoringRunID,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao atualizar consumo acumulado da run na transação: %w", err)
			}
		}

		// Se a política determinou quarentena:
		_, err = txQ.UpdateMonitoringCandidateGates(ctx, sqlc.UpdateMonitoringCandidateGatesParams{
			ID:                      cand.ID,
			StructuralGatePassed:    sql.NullInt64{Int64: 1, Valid: true},
			StructuralGateReasons:   "[]",
			SemanticGatePassed:      sql.NullInt64{Int64: boolToInt(semanticRes.Passed), Valid: true},
			SemanticGateReasons:     string(semanticReasonsBytes),
			PolicyAction:            string(domain.PolicyActionQuarantine),
			PolicyReasons:           string(policyReasonsBytes),
			EditorialStatus:         string(domain.ClaimStatusQuarantined),
			ResolvedSubjectEntityID: subjNull,
			ResolvedTargetEntityID:  targetNull,
			ResolvedCaseID:          caseNull,
			PublishedClaimID:        sql.NullString{Valid: false},
			UpdatedAt:               nowStr,
		})
		if err != nil {
			return fmt.Errorf("monitoring: falha ao atualizar candidato em quarentena: %w", err)
		}

		return nil
	})

	if txErr != nil {
		return nil, fmt.Errorf("monitoring: falha na transação editorial: %w", txErr)
	}

	isPub := policyRes.Action == domain.PolicyActionPublish
	edStatus := string(domain.ClaimStatusQuarantined)
	if isPub {
		edStatus = string(domain.ClaimStatusPublished)
	}

	costMicros := verifyResult.CostMicros

	return &EvaluationResult{
		CandidateID:       cand.ID,
		StructuralPassed:  true,
		StructuralReasons: structuralRes.Reasons,
		SemanticPassed:    semanticRes.Passed,
		SemanticReasons:   semanticRes.Reasons,
		PolicyAction:      policyRes.Action,
		PolicyReasons:     policyRes.Reasons,
		EditorialStatus:   edStatus,
		PublishedClaimID:  publishedClaimID,
		Published:         isPub,
		Model:             verifyResult.Model,
		PromptTokens:      verifyResult.PromptTokens,
		CompletionTokens:  verifyResult.CompletionTokens,
		TotalTokens:       verifyResult.TotalTokens,
		Cost:              research.MicroUSDToFloat(costMicros),
		CostMicros:        costMicros,
	}, nil
}

// EvaluateCandidatesInRun avalia todos os candidatos canônicos gerados por uma execução.
func (e *Evaluator) EvaluateCandidatesInRun(ctx context.Context, runID string) ([]EvaluationResult, error) {
	queries := sqlc.New(e.db)
	candidates, err := queries.ListCandidatesByRunID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao listar candidatos do run %s: %w", runID, err)
	}

	var results []EvaluationResult
	for _, cand := range candidates {
		if cand.IsDuplicate == 1 {
			continue
		}
		res, err := e.EvaluateCandidate(ctx, cand.ID)
		if err != nil {
			return results, err
		}
		results = append(results, *res)
	}

	return results, nil
}

// EvaluatePendingQuarantined avalia até N candidatos canônicos que estão atualmente em quarentena.
func (e *Evaluator) EvaluatePendingQuarantined(ctx context.Context, limit int) ([]EvaluationResult, error) {
	if limit <= 0 {
		limit = 50
	}
	queries := sqlc.New(e.db)
	candidates, err := queries.ListQuarantinedCandidatesForEvaluation(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao listar candidatos em quarentena: %w", err)
	}

	var results []EvaluationResult
	for _, cand := range candidates {
		res, err := e.EvaluateCandidate(ctx, cand.ID)
		if err != nil {
			return results, err
		}
		results = append(results, *res)
	}

	return results, nil
}

func (e *Evaluator) resolveSubject(ctx context.Context, queries *sqlc.Queries, normName string) (string, bool, error) {
	if strings.TrimSpace(normName) == "" {
		return "", false, nil
	}
	ents, err := queries.FindEntitiesByNormalizedName(ctx, normName)
	if err != nil {
		return "", false, err
	}
	aliases, err := queries.FindEntitiesByNormalizedAlias(ctx, normName)
	if err != nil {
		return "", false, err
	}

	seen := make(map[string]struct{})
	for _, ent := range ents {
		seen[ent.ID] = struct{}{}
	}
	for _, ent := range aliases {
		seen[ent.ID] = struct{}{}
	}

	if len(seen) == 0 {
		return "", false, nil
	}
	if len(seen) > 1 {
		return "", true, nil // Múltiplas entidades distintas encontradas (ambiguidade/homônimo)
	}
	for singleID := range seen {
		return singleID, false, nil
	}
	return "", false, nil
}

func (e *Evaluator) resolveTarget(ctx context.Context, queries *sqlc.Queries, normName string) (string, bool, error) {
	return e.resolveSubject(ctx, queries, normName)
}

func (e *Evaluator) resolveCase(ctx context.Context, queries *sqlc.Queries, rawCaseName, normCaseName string) (string, bool, error) {
	if strings.TrimSpace(normCaseName) == "" && strings.TrimSpace(rawCaseName) == "" {
		return "", false, nil
	}
	slug := normalize.Slug(rawCaseName)
	lowerName := strings.ToLower(strings.TrimSpace(rawCaseName))

	cases, err := queries.FindCasesByNormalizedNameOrSlug(ctx, sqlc.FindCasesByNormalizedNameOrSlugParams{
		Slug: slug,
		Name: lowerName,
	})
	if err != nil {
		return "", false, err
	}

	seen := make(map[string]struct{})
	for _, c := range cases {
		seen[c.ID] = struct{}{}
	}

	if len(seen) == 0 {
		return "", false, nil
	}
	if len(seen) > 1 {
		return "", true, nil // Múltiplos casos com mesmo padrão (ambiguidade)
	}
	for singleID := range seen {
		return singleID, false, nil
	}
	return "", false, nil
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
