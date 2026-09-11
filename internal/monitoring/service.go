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
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// Erros sentinela do serviço de monitoramento.
var (
	ErrNilDB                 = errors.New("monitoring: conexão com banco de dados não informada")
	ErrNilProvider           = errors.New("monitoring: provider de pesquisa não informado")
	ErrEmptyQuery            = errors.New("monitoring: consulta de pesquisa vazia")
	ErrMonitoringRunNotFound = errors.New("monitoring: execução de monitoramento não encontrada")
)

// ServiceConfig define os parâmetros de inicialização do serviço de ingestão e monitoramento.
type ServiceConfig struct {
	DB             *sql.DB
	Provider       research.ResearchProvider
	DiscoveryModel string
	NowFunc        func() time.Time
}

// Service orquestra a descoberta, normalização, deduplicação e persistência auditável de candidatos.
type Service struct {
	db             *sql.DB
	provider       research.ResearchProvider
	discoveryModel string
	nowFunc        func() time.Time
}

// NewService cria uma nova instância de Service.
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.DB == nil {
		return nil, ErrNilDB
	}
	if cfg.Provider == nil {
		return nil, ErrNilProvider
	}

	model := strings.TrimSpace(cfg.DiscoveryModel)
	if model == "" {
		model = "openai/gpt-4.1-mini"
	}

	nowFunc := cfg.NowFunc
	if nowFunc == nil {
		nowFunc = func() time.Time { return time.Now().UTC() }
	}

	return &Service{
		db:             cfg.DB,
		provider:       cfg.Provider,
		discoveryModel: model,
		nowFunc:        nowFunc,
	}, nil
}

// RunInput encapsula os parâmetros de entrada para um ciclo de monitoramento.
type RunInput struct {
	Query string `json:"query"`
}

// RunResult encapsula o relatório final de uma execução de monitoramento.
type RunResult struct {
	RunID               string `json:"run_id"`
	Status              string `json:"status"`
	TotalCandidates     int    `json:"total_candidates"`
	UniqueCandidates    int    `json:"unique_candidates"`
	DuplicateCandidates int    `json:"duplicate_candidates"`
	PromptTokens        int    `json:"prompt_tokens"`
	CompletionTokens    int    `json:"completion_tokens"`
	TotalTokens         int    `json:"total_tokens"`
	WebSearchCalls      int    `json:"web_search_calls"`
	ErrorMessage        string `json:"error_message,omitempty"`
	CreatedAt           string `json:"created_at"`
	CompletedAt         string `json:"completed_at,omitempty"`
}

// ExecuteRun executa o fluxo completo de monitoramento:
// 1. Cria monitoring_run com status 'running' (transação curta);
// 2. Chama o ResearchProvider SEM transação aberta;
// 3. Em caso de falha de chamada, validação, normalização ou persistência, atualiza o run para 'failed';
// 4. Valida todo CandidateExtraction defensivamente e normaliza URLs estritamente (sem fallback de URL inválida);
// 5. Calcula fingerprint determinístico v1 e realiza deduplicação intra e cross-run;
// 6. Persiste candidatos estruturados em 'quarantined' e finaliza run (transação curta).
func (s *Service) ExecuteRun(ctx context.Context, input RunInput) (*RunResult, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, ErrEmptyQuery
	}

	runID := uuid.NewString()
	now := s.nowFunc().Format(time.RFC3339Nano)

	// 1. Inicializa o run operacional no banco
	queries := sqlc.New(s.db)
	run, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                runID,
		Status:            "running",
		Query:             query,
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    s.discoveryModel,
		CreatedAt:         now,
	})
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao criar monitoring_run: %w", err)
	}

	// 2. Executa descoberta via OpenRouter FORA de qualquer transação de banco
	discoverRes, err := s.provider.Discover(ctx, research.DiscoverInput{
		Query: query,
	})
	if err != nil {
		return s.failRun(ctx, queries, runID, run.CreatedAt, err, "Falha na etapa de descoberta do provedor")
	}

	// 3. Processamento, normalização e deduplicação defensiva de candidatos
	type processedCandidate struct {
		id                         string
		fingerprint                string
		fingerprintVersion         int64
		entityName                 string
		normalizedEntityName       string
		targetEntityName           string
		normalizedTargetEntityName string
		caseName                   string
		normalizedCaseName         string
		relationshipType           string
		proposition                string
		suggestedGrade             string
		sourceURL                  string
		canonicalURL               string
		sourceTitle                string
		publisherOrAuthor          string
		publishedAt                sql.NullString
		excerpt                    string
		locator                    string
		contextLimits              string
		technicalConfidence        float64
		rawPayload                 string
		editorialStatus            string
		isDuplicate                int64
		duplicateReason            string
		canonicalCandidateID       sql.NullString
	}

	var candidatesToInsert []processedCandidate
	runSeenFingerprints := make(map[string]string) // fingerprint -> candidateID canonical neste run

	for i, rawCand := range discoverRes.Candidates {
		// Validação defensiva do candidato em Go
		if valErr := rawCand.Validate(); valErr != nil {
			return s.failRun(ctx, queries, runID, run.CreatedAt,
				fmt.Errorf("monitoring: candidato[%d] inválido: %w", i, valErr),
				"Falha de validação de candidato estruturado")
		}

		// Normalização canônica estrita de URL (sem fallback permissivo)
		canonicalURL, urlErr := normalize.CanonicalURL(rawCand.SourceURL)
		if urlErr != nil {
			return s.failRun(ctx, queries, runID, run.CreatedAt,
				fmt.Errorf("monitoring: candidato[%d] com URL inválida %q: %w", i, rawCand.SourceURL, urlErr),
				"Falha na normalização da URL da fonte")
		}

		candID := uuid.NewString()

		// Normalização pura
		cleanEntity := normalize.String(rawCand.EntityName)
		normEntity := normalize.Name(cleanEntity)
		cleanTarget := normalize.String(rawCand.TargetEntityName)
		normTarget := normalize.Name(cleanTarget)
		cleanCase := normalize.String(rawCand.CaseName)
		normCase := normalize.Name(cleanCase)
		cleanRelType := normalize.String(rawCand.RelationshipType)
		cleanProp := normalize.String(normalize.Unicode(rawCand.Proposition))
		cleanExcerpt := normalize.String(normalize.Unicode(rawCand.Excerpt))
		cleanTitle := normalize.String(rawCand.SourceTitle)
		cleanPublisher := normalize.String(rawCand.PublisherOrAuthor)
		cleanLocator := normalize.String(rawCand.Locator)
		cleanLimits := normalize.String(rawCand.ContextLimits)

		// Fingerprint versionado v1
		fp := normalize.FingerprintV1(canonicalURL, cleanEntity, cleanProp, cleanExcerpt)

		rawBytes, err := json.Marshal(rawCand)
		if err != nil {
			return s.failRun(ctx, queries, runID, run.CreatedAt,
				fmt.Errorf("monitoring: falha ao serializar raw_payload do candidato[%d]: %w", i, err),
				"Falha na serialização do payload do candidato")
		}

		var pubAt sql.NullString
		if cleanPub := normalize.String(rawCand.PublishedAt); cleanPub != "" {
			pubAt = sql.NullString{String: cleanPub, Valid: true}
		}

		isDup := int64(0)
		dupReason := ""
		var canonicalID sql.NullString

		// Deduplicação intra-run
		if firstCandID, exists := runSeenFingerprints[fp]; exists {
			isDup = 1
			dupReason = "same_run_duplicate"
			canonicalID = sql.NullString{String: firstCandID, Valid: true}
		} else {
			// Deduplicação cross-run contra candidatos existentes no SQLite
			existingCanonical, err := queries.GetCanonicalCandidateByFingerprint(ctx, fp)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					// Ausência comprovada de candidato anterior
					isDup = 0
					dupReason = ""
					canonicalID = sql.NullString{Valid: false}
					runSeenFingerprints[fp] = candID
				} else {
					// Erro de banco de dados real NÃO pode ser ignorado silenciosamente
					return s.failRun(ctx, queries, runID, run.CreatedAt,
						fmt.Errorf("monitoring: falha ao consultar candidato canônico por fingerprint: %w", err),
						"Falha na verificação de deduplicação no banco de dados")
				}
			} else if existingCanonical.ID != "" {
				isDup = 1
				dupReason = "existing_fingerprint"
				canonicalID = sql.NullString{String: existingCanonical.ID, Valid: true}
				runSeenFingerprints[fp] = existingCanonical.ID
			}
		}

		candidatesToInsert = append(candidatesToInsert, processedCandidate{
			id:                         candID,
			fingerprint:                fp,
			fingerprintVersion:         1,
			entityName:                 cleanEntity,
			normalizedEntityName:       normEntity,
			targetEntityName:           cleanTarget,
			normalizedTargetEntityName: normTarget,
			caseName:                   cleanCase,
			normalizedCaseName:         normCase,
			relationshipType:           cleanRelType,
			proposition:                cleanProp,
			suggestedGrade:             rawCand.SuggestedGrade,
			sourceURL:                  rawCand.SourceURL,
			canonicalURL:               canonicalURL,
			sourceTitle:                cleanTitle,
			publisherOrAuthor:          cleanPublisher,
			publishedAt:                pubAt,
			excerpt:                    cleanExcerpt,
			locator:                    cleanLocator,
			contextLimits:              cleanLimits,
			technicalConfidence:        rawCand.TechnicalConfidence,
			rawPayload:                 string(rawBytes),
			editorialStatus:            "quarantined", // Todo candidato novo inicia seguro em quarentena
			isDuplicate:                isDup,
			duplicateReason:            dupReason,
			canonicalCandidateID:       canonicalID,
		})
	}

	// 4. Persistência atômica curta no banco
	totalCount := len(candidatesToInsert)
	uniqueCount := 0
	duplicateCount := 0

	for _, c := range candidatesToInsert {
		if c.isDuplicate == 0 {
			uniqueCount++
		} else {
			duplicateCount++
		}
	}

	summaryCountsMap := map[string]int{
		"total_candidates":     totalCount,
		"unique_candidates":    uniqueCount,
		"duplicate_candidates": duplicateCount,
	}
	summaryCountsBytes, _ := json.Marshal(summaryCountsMap)

	completedNow := s.nowFunc().Format(time.RFC3339Nano)
	technicalSummary := fmt.Sprintf("Descoberta concluída com %d candidatos (%d únicos, %d duplicados). %d chamadas de busca.",
		totalCount, uniqueCount, duplicateCount, discoverRes.WebSearchCalls)

	txErr := store.ExecTx(ctx, s.db, func(txQ *sqlc.Queries) error {
		for _, c := range candidatesToInsert {
			_, err := txQ.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
				ID:                         c.id,
				MonitoringRunID:            runID,
				Fingerprint:                c.fingerprint,
				FingerprintVersion:         c.fingerprintVersion,
				EntityName:                 c.entityName,
				NormalizedEntityName:       c.normalizedEntityName,
				TargetEntityName:           c.targetEntityName,
				NormalizedTargetEntityName: c.normalizedTargetEntityName,
				CaseName:                   c.caseName,
				NormalizedCaseName:         c.normalizedCaseName,
				RelationshipType:           c.relationshipType,
				Proposition:                c.proposition,
				SuggestedGrade:             c.suggestedGrade,
				SourceUrl:                  c.sourceURL,
				CanonicalUrl:               c.canonicalURL,
				SourceTitle:                c.sourceTitle,
				PublisherOrAuthor:          c.publisherOrAuthor,
				PublishedAt:                c.publishedAt,
				Excerpt:                    c.excerpt,
				Locator:                    c.locator,
				ContextLimits:              c.contextLimits,
				TechnicalConfidence:        c.technicalConfidence,
				RawPayload:                 c.rawPayload,
				EditorialStatus:            c.editorialStatus,
				IsDuplicate:                c.isDuplicate,
				DuplicateReason:            c.duplicateReason,
				CanonicalCandidateID:       c.canonicalCandidateID,
				StructuralGateReasons:      "[]",
				SemanticGateReasons:        "[]",
				PolicyReasons:              "[]",
				CreatedAt:                  completedNow,
				UpdatedAt:                  completedNow,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao inserir candidato %s: %w", c.id, err)
			}
		}

		_, err = txQ.UpdateMonitoringRunStatus(ctx, sqlc.UpdateMonitoringRunStatusParams{
			ID:               runID,
			Status:           "completed",
			SummaryCounts:    string(summaryCountsBytes),
			TechnicalSummary: technicalSummary,
			PromptTokens:     int64(discoverRes.PromptTokens),
			CompletionTokens: int64(discoverRes.CompletionTokens),
			TotalTokens:      int64(discoverRes.TotalTokens),
			WebSearchCalls:   int64(discoverRes.WebSearchCalls),
			CompletedAt:      sql.NullString{String: completedNow, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("monitoring: falha ao atualizar status do run: %w", err)
		}

		return nil
	})

	if txErr != nil {
		return s.failRun(ctx, queries, runID, run.CreatedAt,
			fmt.Errorf("monitoring: erro na transação de persistência: %w", txErr),
			"Falha na transação de persistência dos candidatos")
	}

	return &RunResult{
		RunID:               runID,
		Status:              "completed",
		TotalCandidates:     totalCount,
		UniqueCandidates:    uniqueCount,
		DuplicateCandidates: duplicateCount,
		PromptTokens:        discoverRes.PromptTokens,
		CompletionTokens:    discoverRes.CompletionTokens,
		TotalTokens:         discoverRes.TotalTokens,
		WebSearchCalls:      discoverRes.WebSearchCalls,
		CreatedAt:           run.CreatedAt,
		CompletedAt:         completedNow,
	}, nil
}

// failRun atualiza o status de um monitoring_run para 'failed' garantindo integridade de timestamps e contagens.
// Se a própria atualização de falha falhar, retorna ambos os erros contextualizados.
func (s *Service) failRun(ctx context.Context, queries *sqlc.Queries, runID, createdAt string, runErr error, technicalSummary string) (*RunResult, error) {
	completedNow := s.nowFunc().Format(time.RFC3339Nano)
	errMsg := runErr.Error()
	if technicalSummary == "" {
		technicalSummary = "Execução de monitoramento falhou"
	}

	_, updateErr := queries.UpdateMonitoringRunStatus(ctx, sqlc.UpdateMonitoringRunStatusParams{
		ID:               runID,
		Status:           "failed",
		ErrorMessage:     sql.NullString{String: errMsg, Valid: true},
		SummaryCounts:    `{"total_candidates":0,"unique_candidates":0,"duplicate_candidates":0}`,
		TechnicalSummary: technicalSummary,
		CompletedAt:      sql.NullString{String: completedNow, Valid: true},
	})
	if updateErr != nil {
		return nil, fmt.Errorf("monitoring: falha operacional (%v) e falha crítica ao registrar status 'failed' no banco (%w)", runErr, updateErr)
	}

	return &RunResult{
		RunID:        runID,
		Status:       "failed",
		ErrorMessage: errMsg,
		CreatedAt:    createdAt,
		CompletedAt:  completedNow,
	}, nil
}

// GetRun retorna os dados operacionais de uma execução pelo ID.
func (s *Service) GetRun(ctx context.Context, runID string) (*sqlc.MonitoringRun, error) {
	queries := sqlc.New(s.db)
	run, err := queries.GetMonitoringRunByID(ctx, runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMonitoringRunNotFound
		}
		return nil, err
	}
	return &run, nil
}

// ListCandidates retorna os candidatos gerados por um run específico.
func (s *Service) ListCandidates(ctx context.Context, runID string) ([]sqlc.MonitoringCandidate, error) {
	queries := sqlc.New(s.db)
	return queries.ListCandidatesByRunID(ctx, runID)
}

// GetCandidate retorna um candidato pelo seu ID.
func (s *Service) GetCandidate(ctx context.Context, id string) (*sqlc.MonitoringCandidate, error) {
	queries := sqlc.New(s.db)
	cand, err := queries.GetMonitoringCandidateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &cand, nil
}
