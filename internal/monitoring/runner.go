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

// Erros sentinela do Runner.
var (
	ErrDailyBudgetExhausted = errors.New("monitoring: orçamento diário de monitoramento já esgotado")
)

// RunnerConfig define as dependências e limites para execução orquestrada do monitoramento.
type RunnerConfig struct {
	DB                     *sql.DB
	Provider               research.ResearchProvider
	SourceVerifier         sourcecheck.SourceVerifier
	DiscoveryModel         string
	VerificationModel      string
	MonitorWindow          time.Duration
	MaxCandidatesPerRun    int
	MaxVerificationsPerRun int
	MaxCostPerRunUSD       float64
	MaxCostPerRunMicroUSD  int64
	MaxCostPerDayUSD       float64
	MaxCostPerDayMicroUSD  int64
	LockTTL                time.Duration
	NowFunc                func() time.Time
}

// Runner orquestra o ciclo completo de monitoramento: lock, janela incremental, descoberta,
// ingestão de candidatos, avaliação com limites de orçamento e finalização do run.
type Runner struct {
	db                     *sql.DB
	provider               research.ResearchProvider
	sourceVerifier         sourcecheck.SourceVerifier
	discoveryModel         string
	verificationModel      string
	monitorWindow          time.Duration
	maxCandidatesPerRun    int
	maxVerificationsPerRun int
	maxCostPerRunUSD       float64
	maxCostPerRunMicroUSD  int64
	maxCostPerDayUSD       float64
	maxCostPerDayMicroUSD  int64
	lockTTL                time.Duration
	nowFunc                func() time.Time
}

// NewRunner cria e valida uma nova instância de Runner.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	if cfg.DB == nil {
		return nil, ErrNilDB
	}
	if cfg.Provider == nil {
		return nil, ErrNilProvider
	}
	if cfg.SourceVerifier == nil {
		return nil, ErrNilSourceVerifier
	}

	discModel := strings.TrimSpace(cfg.DiscoveryModel)
	if discModel == "" {
		discModel = "openai/gpt-4.1-mini"
	}

	verModel := strings.TrimSpace(cfg.VerificationModel)
	if verModel == "" {
		verModel = "openai/gpt-4.1-mini"
	}

	win := cfg.MonitorWindow
	if win <= 0 {
		win = 24 * time.Hour
	}

	maxCand := cfg.MaxCandidatesPerRun
	if maxCand <= 0 {
		maxCand = 10
	}

	maxVer := cfg.MaxVerificationsPerRun
	if maxVer <= 0 {
		maxVer = 10
	}

	maxCostRunMicros := cfg.MaxCostPerRunMicroUSD
	if maxCostRunMicros <= 0 {
		if cfg.MaxCostPerRunUSD > 0 {
			maxCostRunMicros = ToMicroUSD(cfg.MaxCostPerRunUSD)
		} else {
			maxCostRunMicros = 250000 // $0.25
		}
	}

	maxCostDayMicros := cfg.MaxCostPerDayMicroUSD
	if maxCostDayMicros <= 0 {
		if cfg.MaxCostPerDayUSD > 0 {
			maxCostDayMicros = ToMicroUSD(cfg.MaxCostPerDayUSD)
		} else {
			maxCostDayMicros = 1000000 // $1.00
		}
	}

	lockTTL := cfg.LockTTL
	if lockTTL <= 0 {
		lockTTL = 10 * time.Minute
	}

	nowFunc := cfg.NowFunc
	if nowFunc == nil {
		nowFunc = func() time.Time { return time.Now().UTC() }
	}

	return &Runner{
		db:                     cfg.DB,
		provider:               cfg.Provider,
		sourceVerifier:         cfg.SourceVerifier,
		discoveryModel:         discModel,
		verificationModel:      verModel,
		monitorWindow:          win,
		maxCandidatesPerRun:    maxCand,
		maxVerificationsPerRun: maxVer,
		maxCostPerRunUSD:       MicroUSDToFloat(maxCostRunMicros),
		maxCostPerRunMicroUSD:  maxCostRunMicros,
		maxCostPerDayUSD:       MicroUSDToFloat(maxCostDayMicros),
		maxCostPerDayMicroUSD:  maxCostDayMicros,
		lockTTL:                lockTTL,
		nowFunc:                nowFunc,
	}, nil
}

// RunSummary encapsula o resultado operacional e financeiro consolidado de uma execução.
type RunSummary struct {
	RunID                  string   `json:"run_id"`
	Status                 string   `json:"status"`
	Query                  string   `json:"query"`
	WindowStart            string   `json:"window_start"`
	WindowEnd              string   `json:"window_end"`
	TotalCandidates        int      `json:"total_candidates"`
	UniqueCandidates       int      `json:"unique_candidates"`
	DuplicateCandidates    int      `json:"duplicate_candidates"`
	VerificationsExecuted  int      `json:"verifications_executed"`
	PublishedCount         int      `json:"published_count"`
	QuarantinedCount       int      `json:"quarantined_count"`
	RejectedCount          int      `json:"rejected_count"`
	PromptTokens           int      `json:"prompt_tokens"`
	CompletionTokens       int      `json:"completion_tokens"`
	TotalTokens            int      `json:"total_tokens"`
	DiscoveryTokens        int      `json:"discovery_tokens"`
	VerificationTokens     int      `json:"verification_tokens"`
	WebSearchCalls         int      `json:"web_search_calls"`
	DiscoveryCostUSD       float64  `json:"discovery_cost_usd"`
	DiscoveryCostMicros    int64    `json:"discovery_cost_micros"`
	VerificationCostUSD    float64  `json:"verification_cost_usd"`
	VerificationCostMicros int64    `json:"verification_cost_micros"`
	TotalCostUSD           float64  `json:"total_cost_usd"`
	TotalCostMicros        int64    `json:"total_cost_micros"`
	DailyTotalCostUSD      float64  `json:"daily_total_cost_usd"`
	DailyTotalCostMicros   int64    `json:"daily_total_cost_micros"`
	PartialReasons         []string `json:"partial_reasons,omitempty"`
	ErrorMessage           string   `json:"error_message,omitempty"`
	TechnicalSummary       string   `json:"technical_summary"`
	CreatedAt              string   `json:"created_at"`
	CompletedAt            string   `json:"completed_at"`
}

// Run executa o pipeline de monitoramento completo de forma atômica, controlada e resiliente.
func (r *Runner) Run(ctx context.Context, query string) (*RunSummary, error) {
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return nil, ErrEmptyQuery
	}

	// 1. Aquisição de lock com lease atômico no SQLite
	lm, err := NewLockManager(LockConfig{
		DB:       r.db,
		LockName: "monitoring",
		TTL:      r.lockTTL,
		NowFunc:  r.nowFunc,
	})
	if err != nil {
		return nil, fmt.Errorf("monitoring: erro ao inicializar lock manager: %w", err)
	}

	lease, pipelineCtx, err := lm.Acquire(ctx)
	if err != nil {
		return nil, err // Ex: ErrLockHeld
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = lease.Release(cleanupCtx)
	}()

	now := r.nowFunc().UTC()
	queries := sqlc.New(r.db)

	// 2. Consulta gasto diário acumulado desde 00:00:00Z do dia UTC corrente
	startOfUTCDay := StartOfUTCDay(now)
	startOfUTCDayStr := startOfUTCDay.Format(time.RFC3339Nano)
	dailyPriorSpentMicros, err := queries.GetDailyMonitoringCostMicroUSD(pipelineCtx, startOfUTCDayStr)
	if err != nil {
		return nil, fmt.Errorf("monitoring: erro ao consultar custo diário acumulado em micro-USD: %w", err)
	}

	budgetTracker := NewBudgetTracker(r.maxCostPerRunMicroUSD, r.maxCostPerDayMicroUSD, dailyPriorSpentMicros, r.maxVerificationsPerRun)

	// Verificação pré-chamada: se o orçamento diário já estiver esgotado, aborta sem chamadas externas
	canDiscover, discBlockReason := budgetTracker.CanAffordDiscovery()
	if !canDiscover {
		return nil, fmt.Errorf("%w: %s", ErrDailyBudgetExhausted, discBlockReason)
	}

	// 3. Determinação determinística da janela incremental ancorada na última run 'completed'
	win, err := DetermineWindow(pipelineCtx, queries, cleanQuery, now, r.monitorWindow)
	if err != nil {
		return nil, fmt.Errorf("monitoring: erro ao calcular janela incremental: %w", err)
	}

	runID := uuid.NewString()
	createdNow := now.Format(time.RFC3339Nano)

	// 4. Cria registro inicial do run com status 'running'
	_, err = queries.CreateMonitoringRun(pipelineCtx, sqlc.CreateMonitoringRunParams{
		ID:                   runID,
		Status:               "running",
		Query:                cleanQuery,
		WindowStart:          sql.NullString{String: win.StartISO(), Valid: true},
		WindowEnd:            sql.NullString{String: win.EndISO(), Valid: true},
		DiscoveryProvider:    "openrouter",
		DiscoveryModel:       r.discoveryModel,
		VerificationProvider: "openrouter",
		VerificationModel:    r.verificationModel,
		CreatedAt:            createdNow,
	})
	if err != nil {
		return nil, fmt.Errorf("monitoring: falha ao criar monitoring_run: %w", err)
	}

	// 5. Executa descoberta via OpenRouter FORA de transação SQLite
	discoverRes, err := r.provider.Discover(pipelineCtx, research.DiscoverInput{
		Query:       cleanQuery,
		WindowStart: win.StartISO(),
		WindowEnd:   win.EndISO(),
	})
	if err != nil {
		return r.failRun(runID, cleanQuery, win, createdNow, err, "Falha na etapa de descoberta do provedor")
	}

	// Registra custo e tokens reais de descoberta imediatamente no rastreador
	costMicros := discoverRes.CostMicros
	budgetTracker.RecordDiscovery(costMicros)

	// Persiste o consumo da descoberta IMEDIATAMENTE com contexto independente curto,
	// garantindo que perda de lease ou cancelamento não impeça a gravação dos custos reais
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, recordErr := queries.RecordDiscoveryUsage(saveCtx, sqlc.RecordDiscoveryUsageParams{
		ID:                    runID,
		PromptTokens:          int64(discoverRes.PromptTokens),
		CompletionTokens:      int64(discoverRes.CompletionTokens),
		TotalTokens:           int64(discoverRes.TotalTokens),
		DiscoveryTokens:       int64(discoverRes.TotalTokens),
		DiscoveryCostMicrousd: costMicros,
		TotalCostMicrousd:     costMicros,
		WebSearchCalls:        int64(discoverRes.WebSearchCalls),
	})
	saveCancel()
	if recordErr != nil {
		return r.failRun(runID, cleanQuery, win, createdNow, recordErr, "Falha ao registrar uso da descoberta no banco")
	}

	// Se o lease foi perdido durante ou logo após a descoberta, aborta com segurança registrando a falha
	if pipelineCtx.Err() != nil {
		return r.failRun(runID, cleanQuery, win, createdNow, pipelineCtx.Err(), "Lease de execução perdido após descoberta")
	}

	// 6. Ingestão defensiva, normalização e deduplicação de candidatos
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
	runSeenFingerprints := make(map[string]string)

	for i, rawCand := range discoverRes.Candidates {
		if valErr := rawCand.Validate(); valErr != nil {
			return r.failRun(runID, cleanQuery, win, createdNow,
				fmt.Errorf("monitoring: candidato[%d] inválido: %w", i, valErr),
				"Falha de validação de candidato estruturado")
		}

		canonicalURL, urlErr := normalize.CanonicalURL(rawCand.SourceURL)
		if urlErr != nil {
			return r.failRun(runID, cleanQuery, win, createdNow,
				fmt.Errorf("monitoring: candidato[%d] com URL inválida %q: %w", i, rawCand.SourceURL, urlErr),
				"Falha na normalização da URL da fonte")
		}

		candID := uuid.NewString()
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

		fp := normalize.FingerprintV1(canonicalURL, cleanEntity, cleanProp, cleanExcerpt)

		rawBytes, mErr := json.Marshal(rawCand)
		if mErr != nil {
			return r.failRun(runID, cleanQuery, win, createdNow,
				fmt.Errorf("monitoring: falha ao serializar raw_payload do candidato[%d]: %w", i, mErr),
				"Falha na serialização do payload do candidato")
		}

		var pubAt sql.NullString
		if cleanPub := normalize.String(rawCand.PublishedAt); cleanPub != "" {
			pubAt = sql.NullString{String: cleanPub, Valid: true}
		}

		isDup := int64(0)
		dupReason := ""
		var canonicalID sql.NullString

		if firstCandID, exists := runSeenFingerprints[fp]; exists {
			isDup = 1
			dupReason = "same_run_duplicate"
			canonicalID = sql.NullString{String: firstCandID, Valid: true}
		} else {
			existingCanonical, err := queries.GetCanonicalCandidateByFingerprint(pipelineCtx, fp)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					isDup = 0
					dupReason = ""
					canonicalID = sql.NullString{Valid: false}
					runSeenFingerprints[fp] = candID
				} else {
					return r.failRun(runID, cleanQuery, win, createdNow,
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
			editorialStatus:            "quarantined",
			isDuplicate:                isDup,
			duplicateReason:            dupReason,
			canonicalCandidateID:       canonicalID,
		})
	}

	// Persiste candidatos em transação curta
	insertNow := r.nowFunc().UTC().Format(time.RFC3339Nano)
	txErr := store.ExecTx(pipelineCtx, r.db, func(txQ *sqlc.Queries) error {
		for _, c := range candidatesToInsert {
			_, err := txQ.CreateMonitoringCandidate(pipelineCtx, sqlc.CreateMonitoringCandidateParams{
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
				CreatedAt:                  insertNow,
				UpdatedAt:                  insertNow,
			})
			if err != nil {
				return fmt.Errorf("monitoring: falha ao inserir candidato %s: %w", c.id, err)
			}
		}
		return nil
	})
	if txErr != nil {
		return r.failRun(runID, cleanQuery, win, createdNow, txErr, "Falha na transação de persistência de candidatos")
	}

	totalCandidates := len(candidatesToInsert)
	uniqueCandidates := 0
	duplicateCandidates := 0
	var uniqueCands []processedCandidate
	for _, c := range candidatesToInsert {
		if c.isDuplicate == 0 {
			uniqueCandidates++
			uniqueCands = append(uniqueCands, c)
		} else {
			duplicateCandidates++
		}
	}

	// 7. Avaliação e Deliberação dos Candidatos Elegíveis dentro do Orçamento e Limite de Candidatos
	evaluator, err := NewEvaluator(EvaluatorConfig{
		DB:                r.db,
		Provider:          r.provider,
		SourceVerifier:    r.sourceVerifier,
		VerificationModel: r.verificationModel,
		NowFunc:           r.nowFunc,
	})
	if err != nil {
		return r.failRun(runID, cleanQuery, win, createdNow, err, "Falha ao inicializar avaliador")
	}

	isPartial := false
	var partialReasons []string
	publishedCount := 0
	quarantinedCount := 0
	rejectedCount := 0

	// Aplica MONITOR_MAX_CANDIDATES_PER_RUN: restringe deterministicamente os candidatos avaliados na run
	candidatesToEvaluate := uniqueCands
	if len(candidatesToEvaluate) > r.maxCandidatesPerRun {
		candidatesToEvaluate = candidatesToEvaluate[:r.maxCandidatesPerRun]
		isPartial = true
		partialReasons = append(partialReasons, fmt.Sprintf("limite de candidatos por execução atingido (%d de %d processados)", r.maxCandidatesPerRun, len(uniqueCands)))
	}

	for _, c := range candidatesToEvaluate {
		// Verifica se o pipeline foi cancelado (ex: cancelamento de contexto ou perda de lock lease)
		if pipelineCtx.Err() != nil {
			isPartial = true
			partialReasons = append(partialReasons, "Execução interrompida por cancelamento ou perda de lock")
			break
		}

		// Verifica se temos orçamento/limite disponível antes de proceder com nova verificação
		canVerify, blockReason := budgetTracker.CanAffordVerification()
		if !canVerify {
			isPartial = true
			partialReasons = append(partialReasons, blockReason)
			break
		}

		// Avalia candidato (grava semantic_evaluations e atualiza run usage atomicamente na mesma transação)
		evalRes, err := evaluator.EvaluateCandidate(pipelineCtx, c.id)
		if err != nil {
			isPartial = true
			partialReasons = append(partialReasons, fmt.Sprintf("Erro ao avaliar candidato %s: %v", c.id, err))
			continue
		}

		// Registra contabilidade no budgetTracker se houve verificação semântica executada
		if evalRes.TotalTokens > 0 || evalRes.CostMicros > 0 || evalRes.SemanticPassed || len(evalRes.SemanticReasons) > 0 {
			budgetTracker.RecordVerification(evalRes.CostMicros)
		}

		if evalRes.Published {
			publishedCount++
		} else if evalRes.PolicyAction == domain.PolicyActionQuarantine || evalRes.EditorialStatus == string(domain.ClaimStatusQuarantined) {
			quarantinedCount++
		} else if evalRes.EditorialStatus == string(domain.ClaimStatusRejected) {
			rejectedCount++
		}
	}

	// 8. Finalização do Run com Contexto Independente e Curto (Resiliente a Perda de Lease)
	finalStatus := "completed"
	if isPartial {
		finalStatus = "partial"
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()

	// Lê o estado consolidado da run do banco (garantindo consistência com transações atômicas de avaliação)
	latestRun, getErr := queries.GetMonitoringRunByID(cleanupCtx, runID)
	if getErr != nil {
		latestRun = sqlc.MonitoringRun{
			DiscoveryTokens:          int64(discoverRes.TotalTokens),
			DiscoveryCostMicrousd:    costMicros,
			DiscoveryCost:            MicroUSDToFloat(costMicros),
			VerificationTokens:       0,
			VerificationCostMicrousd: budgetTracker.VerificationCostMicros(),
			VerificationCost:         budgetTracker.VerificationCostUSD(),
			TotalCostMicrousd:        budgetTracker.CurrentRunCostMicros(),
			TotalCost:                budgetTracker.CurrentRunCostUSD(),
			TotalTokens:              int64(discoverRes.TotalTokens),
			PromptTokens:             int64(discoverRes.PromptTokens),
			CompletionTokens:         int64(discoverRes.CompletionTokens),
			VerificationsCount:       int64(budgetTracker.VerificationsExecuted()),
		}
	}

	completedNow := r.nowFunc().UTC().Format(time.RFC3339Nano)
	summaryCountsMap := map[string]int{
		"total_candidates":       totalCandidates,
		"unique_candidates":      uniqueCandidates,
		"duplicate_candidates":   duplicateCandidates,
		"verifications_executed": int(latestRun.VerificationsCount),
		"published_claims":       publishedCount,
		"quarantined_claims":     quarantinedCount,
		"rejected_claims":        rejectedCount,
	}
	summaryCountsBytes, _ := json.Marshal(summaryCountsMap)

	var techSummarySb strings.Builder
	techSummarySb.WriteString(fmt.Sprintf("Execução %s (%s). Descoberta: %d candidatos (%d únicos, %d duplicados). Verificações: %d executadas (%d publicadas, %d em quarentena, %d rejeitadas). Custo real: %s (%d tokens).",
		finalStatus, cleanQuery, totalCandidates, uniqueCandidates, duplicateCandidates,
		latestRun.VerificationsCount, publishedCount, quarantinedCount, rejectedCount,
		FormatUSD(latestRun.TotalCost), latestRun.TotalTokens))

	if len(partialReasons) > 0 {
		techSummarySb.WriteString(fmt.Sprintf(" Motivo(s) de interrupção parcial: %s.", strings.Join(partialReasons, "; ")))
	}
	technicalSummary := techSummarySb.String()

	_, err = queries.CompleteMonitoringRun(cleanupCtx, sqlc.CompleteMonitoringRunParams{
		ID:               runID,
		Status:           finalStatus,
		ErrorMessage:     sql.NullString{Valid: false},
		SummaryCounts:    string(summaryCountsBytes),
		TechnicalSummary: technicalSummary,
		CompletedAt:      sql.NullString{String: completedNow, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("monitoring: erro ao registrar finalização do run no SQLite: %w", err)
	}

	return &RunSummary{
		RunID:                  runID,
		Status:                 finalStatus,
		Query:                  cleanQuery,
		WindowStart:            win.StartISO(),
		WindowEnd:              win.EndISO(),
		TotalCandidates:        totalCandidates,
		UniqueCandidates:       uniqueCandidates,
		DuplicateCandidates:    duplicateCandidates,
		VerificationsExecuted:  int(latestRun.VerificationsCount),
		PublishedCount:         publishedCount,
		QuarantinedCount:       quarantinedCount,
		RejectedCount:          rejectedCount,
		PromptTokens:           int(latestRun.PromptTokens),
		CompletionTokens:       int(latestRun.CompletionTokens),
		TotalTokens:            int(latestRun.TotalTokens),
		DiscoveryTokens:        int(latestRun.DiscoveryTokens),
		VerificationTokens:     int(latestRun.VerificationTokens),
		WebSearchCalls:         discoverRes.WebSearchCalls,
		DiscoveryCostUSD:       latestRun.DiscoveryCost,
		DiscoveryCostMicros:    latestRun.DiscoveryCostMicrousd,
		VerificationCostUSD:    latestRun.VerificationCost,
		VerificationCostMicros: latestRun.VerificationCostMicrousd,
		TotalCostUSD:           latestRun.TotalCost,
		TotalCostMicros:        latestRun.TotalCostMicrousd,
		DailyTotalCostUSD:      budgetTracker.TotalDayCostUSD(),
		DailyTotalCostMicros:   budgetTracker.TotalDayCostMicros(),
		PartialReasons:         partialReasons,
		TechnicalSummary:       technicalSummary,
		CreatedAt:              createdNow,
		CompletedAt:            completedNow,
	}, nil
}

// failRun registra com segurança o estado 'failed' do run preservando tokens e custos previamente consumidos.
func (r *Runner) failRun(runID, query string, win Window, createdAt string, runErr error, technicalSummary string) (*RunSummary, error) {
	completedNow := r.nowFunc().UTC().Format(time.RFC3339Nano)
	errMsg := runErr.Error()
	if technicalSummary == "" {
		technicalSummary = "Execução de monitoramento falhou"
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()

	queries := sqlc.New(r.db)

	_, updateErr := queries.FailMonitoringRun(cleanupCtx, sqlc.FailMonitoringRunParams{
		ID:               runID,
		ErrorMessage:     sql.NullString{String: errMsg, Valid: true},
		TechnicalSummary: technicalSummary + ": " + errMsg,
		CompletedAt:      sql.NullString{String: completedNow, Valid: true},
	})
	if updateErr != nil {
		return nil, fmt.Errorf("monitoring: falha operacional (%v) e falha ao registrar status 'failed' (%w)", runErr, updateErr)
	}

	// Consulta o estado persistido real da run para preencher o sumário
	latestRun, getErr := queries.GetMonitoringRunByID(cleanupCtx, runID)
	if getErr != nil {
		latestRun = sqlc.MonitoringRun{
			TotalCost: latestRun.TotalCost,
		}
	}

	return &RunSummary{
		RunID:                  runID,
		Status:                 "failed",
		Query:                  query,
		WindowStart:            win.StartISO(),
		WindowEnd:              win.EndISO(),
		PromptTokens:           int(latestRun.PromptTokens),
		CompletionTokens:       int(latestRun.CompletionTokens),
		TotalTokens:            int(latestRun.TotalTokens),
		DiscoveryTokens:        int(latestRun.DiscoveryTokens),
		VerificationTokens:     int(latestRun.VerificationTokens),
		DiscoveryCostUSD:       latestRun.DiscoveryCost,
		DiscoveryCostMicros:    latestRun.DiscoveryCostMicrousd,
		VerificationCostUSD:    latestRun.VerificationCost,
		VerificationCostMicros: latestRun.VerificationCostMicrousd,
		TotalCostUSD:           latestRun.TotalCost,
		TotalCostMicros:        latestRun.TotalCostMicrousd,
		ErrorMessage:           errMsg,
		TechnicalSummary:       technicalSummary + ": " + errMsg,
		CreatedAt:              createdAt,
		CompletedAt:            completedNow,
	}, runErr
}
