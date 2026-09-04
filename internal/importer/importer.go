package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

const (
	RootCaseSlug        = "recorte-vorcaro-master"
	RootCaseName        = "Recorte Daniel Vorcaro / Banco Master"
	RootCaseDescription = "Recorte investigativo inicial de pessoas e entidades vinculadas ao caso Daniel Vorcaro e Banco Master."
)

// Importer orquestra o processo de ingestão da planilha curated_seed.
type Importer struct {
	db          *sql.DB
	mappingPath string
}

// NewImporter instancia um novo importador configurado com o banco e o caminho do arquivo de mapping.
func NewImporter(db *sql.DB, mappingPath string) *Importer {
	return &Importer{
		db:          db,
		mappingPath: mappingPath,
	}
}

// ImportFromFile executa a importação a partir do caminho do arquivo XLSX fornecido.
func (imp *Importer) ImportFromFile(ctx context.Context, opts ImportOptions) (*ImportResult, error) {
	if strings.TrimSpace(opts.FilePath) == "" {
		return nil, fmt.Errorf("importer: caminho do arquivo é obrigatório")
	}

	// 1. Validação de extensão
	if !strings.HasSuffix(strings.ToLower(opts.FilePath), ".xlsx") {
		return nil, fmt.Errorf("importer: arquivo %q possui extensão incompatível, esperado .xlsx", opts.FilePath)
	}

	// 2. Leitura dos bytes originais para cálculo do hash SHA-256
	fileBytes, err := os.ReadFile(opts.FilePath)
	if err != nil {
		return nil, fmt.Errorf("importer: falha ao ler arquivo %q: %w", opts.FilePath, err)
	}

	hasher := sha256.New()
	hasher.Write(fileBytes)
	fileHash := hex.EncodeToString(hasher.Sum(nil))

	// 3. Carregamento e validação estrita do mapping versionado
	mapping, err := LoadMappingFromFile(imp.mappingPath)
	if err != nil {
		return nil, fmt.Errorf("importer: falha no arquivo de mapping: %w", err)
	}

	// 4. Verificação de idempotência (execuções aplicadas anteriores)
	queries := sqlc.New(imp.db)
	appliedRun, err := queries.GetAppliedImportRun(ctx, sqlc.GetAppliedImportRunParams{
		FileHash:       fileHash,
		MappingVersion: mapping.Version,
	})
	if err == nil && appliedRun.ID != "" {
		// Importação aplicada anterior detectada: idempotência garantida
		return &ImportResult{
			FileHash:        fileHash,
			MappingVersion:  mapping.Version,
			DryRun:          opts.DryRun,
			AlreadyImported: true,
			SummaryCounts:   SummaryCounts{},
		}, nil
	}

	// 5. Leitura da planilha XLSX via Excelize
	rawRows, ignoredCount, readWarnings, err := ReadSpreadsheet(bytes.NewReader(fileBytes), mapping.Source.Sheet, mapping.Source.HeaderRow)
	if err != nil {
		return nil, fmt.Errorf("importer: erro ao ler planilha: %w", err)
	}

	// 6. Processamento e validação das linhas em memória
	parsedRows, rowErrors, validationWarnings := imp.parseAndValidateRows(rawRows, mapping)
	allWarnings := append(readWarnings, validationWarnings...)

	result := &ImportResult{
		FileHash:       fileHash,
		MappingVersion: mapping.Version,
		DryRun:         opts.DryRun,
		Warnings:       allWarnings,
		RowErrors:      rowErrors,
		SummaryCounts: SummaryCounts{
			TotalRows:       ignoredCount + len(parsedRows) + len(rowErrors),
			IgnoredRows:     ignoredCount,
			ValidRows:       len(parsedRows),
			ErrorRows:       len(rowErrors),
			PublishedRows:   0,
			QuarantinedRows: 0,
		},
	}

	// 7. Contabilização preliminar de publicação vs quarentena e registro de detalhes
	var quarantinedDetails []string
	for _, pr := range parsedRows {
		if pr.ShouldQuarantine() {
			result.SummaryCounts.QuarantinedRows++
			quarantinedDetails = append(quarantinedDetails, fmt.Sprintf("Linha %d (%s): quarentena %v", pr.Raw.RowNumber, pr.Raw.Nome, pr.QuarantineReasons))
		} else {
			result.SummaryCounts.PublishedRows++
		}
	}
	result.QuarantinedDetails = quarantinedDetails

	// 8. Se for Dry-Run, simula contagens de criação vs reuso sem gravar nada
	if opts.DryRun {
		imp.simulateDryRunCounts(ctx, queries, parsedRows, result)
		return result, nil
	}

	// 9. Execução aplicada em transação atômica única
	now := time.Now().UTC().Format(time.RFC3339Nano)
	runID := uuid.NewString()

	err = store.ExecTx(ctx, imp.db, func(q *sqlc.Queries) error {
		// 9.1 Registra o import_run com status 'running'
		// O índice único uq_import_runs_applied garante proteção de concorrência a nível de banco
		_, err := q.CreateImportRun(ctx, sqlc.CreateImportRunParams{
			ID:             runID,
			FilePath:       opts.FilePath,
			FileHash:       fileHash,
			Origin:         mapping.Origin,
			MappingVersion: mapping.Version,
			Status:         string(domain.ImportRunStatusRunning),
			IsDryRun:       0,
			SummaryCounts:  "{}",
			SummaryReport:  "",
			CreatedAt:      now,
		})
		if err != nil {
			return fmt.Errorf("falha ao registrar import_run em andamento: %w", err)
		}

		// 9.2 Garante existência do caso raiz determinístico
		rootCase, err := imp.ensureRootCase(ctx, q, now)
		if err != nil {
			return fmt.Errorf("falha ao preparar caso raiz: %w", err)
		}

		// Cache em lote durante a transação para deduplicação/reuso
		entityByNorm := make(map[string]string)  // normalized_name -> entity_id
		slugUsed := make(map[string]string)      // slug -> normalized_name
		sourceByCanon := make(map[string]string) // canonical_url -> source_id

		for _, row := range parsedRows {
			// 9.3 Entidade: reutiliza ou cria
			entityID, isReused, err := imp.persistOrReuseEntity(ctx, q, row, now, entityByNorm, slugUsed)
			if err != nil {
				return fmt.Errorf("linha %d: falha na entidade: %w", row.Raw.RowNumber, err)
			}
			if isReused {
				result.SummaryCounts.EntitiesReused++
			} else {
				result.SummaryCounts.EntitiesCreated++
			}

			// 9.4 Relacionamento: entidade -> caso raiz
			relID := uuid.NewString()
			_, err = q.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
				ID:               relID,
				SubjectEntityID:  entityID,
				CaseID:           sql.NullString{String: rootCase.ID, Valid: true},
				RelationshipType: row.Raw.TipoDeVinculo,
				Summary:          row.Raw.OQueEstaDocumentado,
				ContextLimits:    row.Raw.ContrapontoLimite,
				CreatedAt:        now,
				UpdatedAt:        now,
			})
			if err != nil {
				return fmt.Errorf("linha %d: falha ao criar relacionamento: %w", row.Raw.RowNumber, err)
			}

			// 9.5 Fonte principal: reutiliza ou cria
			sourceID, srcReused, err := imp.persistOrReuseSource(ctx, q, row.PrimaryCanonURL, row.Raw.FontePrincipal, now, sourceByCanon)
			if err != nil {
				return fmt.Errorf("linha %d: falha na fonte principal: %w", row.Raw.RowNumber, err)
			}
			if srcReused {
				result.SummaryCounts.SourcesReused++
			} else {
				result.SummaryCounts.SourcesCreated++
			}

			// 9.6 Claim
			claimStatus := row.InitialState
			metricEligible := int64(0)
			if row.MetricEligible && !row.ShouldQuarantine() {
				metricEligible = 1
			}
			if row.ShouldQuarantine() {
				claimStatus = domain.ClaimStatusQuarantined
			}

			claimID := uuid.NewString()
			_, err = q.CreateClaim(ctx, sqlc.CreateClaimParams{
				ID:                claimID,
				RelationshipID:    relID,
				Proposition:       row.Raw.OQueEstaDocumentado,
				Attribution:       "", // Atribuição própria vazia no seed; não recebe "Situação"
				Origin:            mapping.Origin,
				Grade:             string(row.Grade),
				Disposition:       string(row.Disposition),
				MetricEligible:    metricEligible,
				Status:            string(claimStatus),
				ContextStatus:     NormalizeString(row.Raw.Situacao),
				QuarantineReasons: row.QuarantineReasonsJSON(),
				ImportRunID:       sql.NullString{String: runID, Valid: true},
				CreatedAt:         now,
				UpdatedAt:         now,
			})
			if err != nil {
				return fmt.Errorf("linha %d: falha ao criar claim: %w", row.Raw.RowNumber, err)
			}
			result.SummaryCounts.ClaimsCreated++

			// 9.7 Evidência (resumo editorial documentado)
			evID := uuid.NewString()
			_, err = q.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
				ID:           evID,
				ClaimID:      claimID,
				Summary:      row.Raw.OQueEstaDocumentado,
				EvidenceType: "document",
				CreatedAt:    now,
				UpdatedAt:    now,
			})
			if err != nil {
				return fmt.Errorf("linha %d: falha ao criar evidence: %w", row.Raw.RowNumber, err)
			}

			// 9.8 EvidenceSource principal (excerpt vazio: seed não é citação literal)
			esID := uuid.NewString()
			_, err = q.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
				ID:         esID,
				EvidenceID: evID,
				SourceID:   sourceID,
				Excerpt:    "", // seed não fornece trecho literal
				Locator:    "",
				Role:       string(domain.RoleSupports),
				Status:     string(domain.EvidenceSourceStatusActive),
				CreatedAt:  now,
				UpdatedAt:  now,
			})
			if err != nil {
				return fmt.Errorf("linha %d: falha ao criar evidence_source: %w", row.Raw.RowNumber, err)
			}

			// 9.9 Fonte adicional (opcional)
			if row.AddCanonURL != "" {
				addSourceID, addReused, addErr := imp.persistOrReuseSource(ctx, q, row.AddCanonURL, row.Raw.FonteAdicional, now, sourceByCanon)
				if addErr != nil {
					return fmt.Errorf("linha %d: falha na fonte adicional: %w", row.Raw.RowNumber, addErr)
				}
				if addReused {
					result.SummaryCounts.SourcesReused++
				} else {
					result.SummaryCounts.SourcesCreated++
				}

				addESID := uuid.NewString()
				_, err = q.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
					ID:         addESID,
					EvidenceID: evID,
					SourceID:   addSourceID,
					Excerpt:    "",
					Locator:    "",
					Role:       string(domain.RoleContextualizes),
					Status:     string(domain.EvidenceSourceStatusActive),
					CreatedAt:  now,
					UpdatedAt:  now,
				})
				if err != nil {
					return fmt.Errorf("linha %d: falha ao criar evidence_source adicional: %w", row.Raw.RowNumber, err)
				}
			}
		}

		// 9.10 Finalização do import_run
		finalStatus := domain.ImportRunStatusCompleted
		if len(result.RowErrors) > 0 {
			finalStatus = domain.ImportRunStatusPartial
		}

		_, err = q.UpdateImportRun(ctx, sqlc.UpdateImportRunParams{
			ID:            runID,
			Status:        string(finalStatus),
			SummaryCounts: result.SummaryCounts.JSON(),
			SummaryReport: result.SummaryReport(),
			CompletedAt:   sql.NullString{String: now, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("falha ao atualizar status final do import_run: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("importer: transação de importação falhou (rollback realizado): %w", err)
	}

	return result, nil
}

// parseAndValidateRows analisa os campos das linhas brutas, aplicando as regras de quarentena e erros estruturais.
func (imp *Importer) parseAndValidateRows(rawRows []RawRow, mapping *MappingConfig) ([]ParsedRow, []RowError, []string) {
	var parsed []ParsedRow
	var rowErrors []RowError
	var warnings []string

	for _, raw := range rawRows {
		nome := NormalizeString(raw.Nome)
		if nome == "" {
			rowErrors = append(rowErrors, RowError{
				RowNumber: raw.RowNumber,
				Field:     "Nome",
				Code:      "MISSING_NAME",
				Message:   "nome obrigatório ausente",
			})
			continue
		}

		// Validação de URL da Fonte principal
		primaryURL := NormalizeString(raw.FontePrincipal)
		if primaryURL == "" {
			rowErrors = append(rowErrors, RowError{
				RowNumber: raw.RowNumber,
				Field:     "Fonte principal",
				Code:      "MISSING_PRIMARY_SOURCE",
				Message:   "fonte principal ausente",
			})
			continue
		}

		primaryCanon, err := CanonicalURL(primaryURL)
		if err != nil {
			rowErrors = append(rowErrors, RowError{
				RowNumber: raw.RowNumber,
				Field:     "Fonte principal",
				Code:      "INVALID_URL",
				Message:   fmt.Sprintf("url da fonte principal inválida: %v", err),
			})
			continue
		}

		porQueImporta := NormalizeString(raw.PorQueImporta)
		if porQueImporta == "" {
			rowErrors = append(rowErrors, RowError{
				RowNumber: raw.RowNumber,
				Field:     "Por que importa",
				Code:      "MISSING_RELEVANCE_RATIONALE",
				Message:   "justificativa da relevância ausente (exigida para persistência da entidade)",
			})
			continue
		}

		oQueEstaDoc := NormalizeString(raw.OQueEstaDocumentado)
		if oQueEstaDoc == "" {
			rowErrors = append(rowErrors, RowError{
				RowNumber: raw.RowNumber,
				Field:     "O que está documentado",
				Code:      "MISSING_DOCUMENTED_SYNTHESIS",
				Message:   "síntese documentada ausente (exigida para proposition e evidência)",
			})
			continue
		}

		// URL da Fonte adicional (opcional): url inválida gera aviso estável sem descartar a linha
		addURL := NormalizeString(raw.FonteAdicional)
		var addCanon string
		if addURL != "" {
			c, err := CanonicalURL(addURL)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Linha %d [%s]: url da fonte adicional inválida ignorada (%q): %v", raw.RowNumber, WarningCodeInvalidAdditionalSourceURL, addURL, err))
			} else {
				addCanon = c
			}
		}

		var quarantineReasons []string
		var quarantineNotes []string

		// Grau legado
		rule, found := mapping.LookupGrade(raw.GrauConfirmacao)
		if !found {
			quarantineReasons = append(quarantineReasons, QuarantineCodeUnknownLegacyGrade)
			quarantineNotes = append(quarantineNotes, fmt.Sprintf("grau legado desconhecido %q", raw.GrauConfirmacao))
		} else if rule.InitialState == string(domain.ClaimStatusQuarantined) {
			quarantineReasons = append(quarantineReasons, QuarantineCodeMappedInitialState)
			quarantineNotes = append(quarantineNotes, fmt.Sprintf("estado inicial em quarentena definido pelo mapeamento (%s)", raw.GrauConfirmacao))
		}

		// Relevância
		rel, err := ParseRelevance(raw.Relevancia)
		if err != nil {
			quarantineReasons = append(quarantineReasons, QuarantineCodeInvalidRelevance)
			quarantineNotes = append(quarantineNotes, fmt.Sprintf("relevância inválida %q", raw.Relevancia))
			rel = domain.Relevance(1) // fallback defensivo para quarentena
		}

		// Validações essenciais para permitir publicação (B.9)
		if NormalizeString(raw.CargoPapel) == "" {
			quarantineReasons = append(quarantineReasons, QuarantineCodeMissingRoleOrContext)
			quarantineNotes = append(quarantineNotes, "cargo ou contexto ausente")
		}
		if NormalizeString(raw.TipoDeVinculo) == "" {
			quarantineReasons = append(quarantineReasons, QuarantineCodeMissingRelationshipExplanation)
			quarantineNotes = append(quarantineNotes, "explicação do vínculo ausente")
		}

		parsed = append(parsed, ParsedRow{
			Raw:               raw,
			NormalizedName:    NormalizeName(nome),
			Slug:              GenerateSlug(nome),
			Relevance:         rel,
			Grade:             domain.EvidenceGrade(rule.Grade),
			InitialState:      domain.ClaimStatus(rule.InitialState),
			Disposition:       domain.ClaimDisposition(rule.Disposition),
			MetricEligible:    rule.MetricEligible,
			PrimaryURL:        primaryURL,
			PrimaryCanonURL:   primaryCanon,
			AddURL:            addURL,
			AddCanonURL:       addCanon,
			QuarantineReasons: quarantineReasons,
			QuarantineNotes:   quarantineNotes,
		})
	}

	return parsed, rowErrors, warnings
}

// simulateDryRunCounts consulta o banco em modo leitura para computar com precisão reuso vs criação no dry-run.
func (imp *Importer) simulateDryRunCounts(ctx context.Context, q *sqlc.Queries, parsed []ParsedRow, res *ImportResult) {
	seenNorm := make(map[string]bool)
	seenCanon := make(map[string]bool)

	for _, row := range parsed {
		// Entidade
		if !seenNorm[row.NormalizedName] {
			seenNorm[row.NormalizedName] = true
			if _, err := q.GetEntityByNormalizedName(ctx, row.NormalizedName); err == nil {
				res.SummaryCounts.EntitiesReused++
			} else {
				res.SummaryCounts.EntitiesCreated++
			}
		} else {
			res.SummaryCounts.EntitiesReused++
		}

		// Fonte principal
		if !seenCanon[row.PrimaryCanonURL] {
			seenCanon[row.PrimaryCanonURL] = true
			if _, err := q.GetSourceByCanonicalURL(ctx, row.PrimaryCanonURL); err == nil {
				res.SummaryCounts.SourcesReused++
			} else {
				res.SummaryCounts.SourcesCreated++
			}
		} else {
			res.SummaryCounts.SourcesReused++
		}

		// Fonte adicional
		if row.AddCanonURL != "" {
			if !seenCanon[row.AddCanonURL] {
				seenCanon[row.AddCanonURL] = true
				if _, err := q.GetSourceByCanonicalURL(ctx, row.AddCanonURL); err == nil {
					res.SummaryCounts.SourcesReused++
				} else {
					res.SummaryCounts.SourcesCreated++
				}
			} else {
				res.SummaryCounts.SourcesReused++
			}
		}

		res.SummaryCounts.ClaimsCreated++
	}
}

// ensureRootCase localiza o caso raiz pelo slug ou cria caso ainda não exista.
func (imp *Importer) ensureRootCase(ctx context.Context, q *sqlc.Queries, now string) (sqlc.Case, error) {
	c, err := q.GetCaseBySlug(ctx, RootCaseSlug)
	if err == nil {
		return c, nil
	}

	newCase, err := q.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          uuid.NewString(),
		Name:        RootCaseName,
		Slug:        RootCaseSlug,
		Description: RootCaseDescription,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return sqlc.Case{}, err
	}
	return newCase, nil
}

// persistOrReuseEntity busca por nome normalizado para reuso; se não existir, cria com slug único e determinístico.
func (imp *Importer) persistOrReuseEntity(
	ctx context.Context,
	q *sqlc.Queries,
	row ParsedRow,
	now string,
	entityByNorm map[string]string,
	slugUsed map[string]string,
) (string, bool, error) {
	// 1. Verifica cache local da transação
	if id, ok := entityByNorm[row.NormalizedName]; ok {
		return id, true, nil
	}

	// 2. Verifica banco de dados
	if ent, err := q.GetEntityByNormalizedName(ctx, row.NormalizedName); err == nil {
		entityByNorm[row.NormalizedName] = ent.ID
		slugUsed[ent.Slug] = ent.NormalizedName
		return ent.ID, true, nil
	}

	// 3. Determina slug com tratamento determinístico de colisão
	baseSlug := row.Slug
	targetSlug := baseSlug
	counter := 1

	for {
		// Verifica se já está em uso na transação
		if owner, ok := slugUsed[targetSlug]; ok && owner != row.NormalizedName {
			counter++
			targetSlug = DisambiguateSlug(baseSlug, counter)
			continue
		}

		// Verifica se já está em uso no banco
		if existing, err := q.GetEntityBySlug(ctx, targetSlug); err == nil && existing.NormalizedName != row.NormalizedName {
			counter++
			targetSlug = DisambiguateSlug(baseSlug, counter)
			continue
		}

		break
	}

	entID := uuid.NewString()
	_, err := q.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 entID,
		Type:               string(domain.EntityTypePerson),
		Name:               NormalizeString(row.Raw.Nome),
		NormalizedName:     row.NormalizedName,
		Slug:               targetSlug,
		Category:           NormalizeString(row.Raw.Area),
		RoleOrContext:      NormalizeString(row.Raw.CargoPapel),
		Reach:              NormalizeString(row.Raw.Alcance),
		Summary:            "",
		Relevance:          int64(row.Relevance),
		RelevanceRationale: NormalizeString(row.Raw.PorQueImporta),
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		return "", false, err
	}

	entityByNorm[row.NormalizedName] = entID
	slugUsed[targetSlug] = row.NormalizedName
	return entID, false, nil
}

// persistOrReuseSource busca fonte pela URL canônica para reuso; se não existir, cria sem inventar título/autor.
func (imp *Importer) persistOrReuseSource(
	ctx context.Context,
	q *sqlc.Queries,
	canonicalURL string,
	originalURL string,
	now string,
	sourceByCanon map[string]string,
) (string, bool, error) {
	if id, ok := sourceByCanon[canonicalURL]; ok {
		return id, true, nil
	}

	if src, err := q.GetSourceByCanonicalURL(ctx, canonicalURL); err == nil {
		sourceByCanon[canonicalURL] = src.ID
		return src.ID, true, nil
	}

	sourceID := uuid.NewString()
	_, err := q.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 sourceID,
		Title:              "", // seed não fornece título
		PublisherOrAuthor:  "", // seed não fornece autor
		OriginalUrl:        originalURL,
		CanonicalUrl:       canonicalURL,
		SourceType:         "article",
		SourceAccessStatus: string(domain.SourceAccessNotChecked),
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		return "", false, err
	}

	sourceByCanon[canonicalURL] = sourceID
	return sourceID, false, nil
}
