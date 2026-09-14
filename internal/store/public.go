package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// ErrNotFound indica que o registro público solicitado não existe ou não é visível.
var ErrNotFound = errors.New("store: registro público não encontrado")

// Constantes e allowlist de ordenação pública (B.3)
const (
	OrderFieldName      = "name"
	OrderFieldRelevance = "relevance"
	OrderFieldUpdated   = "updated"

	OrderDirAsc  = "asc"
	OrderDirDesc = "desc"

	DefaultPageSize = 15
	MaxPageSize     = 50
)

var allowedOrderFields = map[string]bool{
	OrderFieldName:      true,
	OrderFieldRelevance: true,
	OrderFieldUpdated:   true,
}

var allowedOrderDirs = map[string]bool{
	OrderDirAsc:  true,
	OrderDirDesc: true,
}

// EscapeLike escapa os caracteres curinga do operador LIKE do SQLite (%, _, \).
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// AllowedPeriodPresets define os identificadores permitidos de filtro temporal.
var allowedPeriodPresets = map[string]bool{
	"7d":  true,
	"30d": true,
	"90d": true,
	"all": true,
}

// ResolvePeriod converte um preset temporal em timestamp UTC RFC3339 a partir do relógio fornecido.
func ResolvePeriod(preset string, now time.Time) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "7d":
		return now.AddDate(0, 0, -7).Format(time.RFC3339)
	case "30d":
		return now.AddDate(0, 0, -30).Format(time.RFC3339)
	case "90d":
		return now.AddDate(0, 0, -90).Format(time.RFC3339)
	default:
		return ""
	}
}

// PublicEntityFilter encapsula os parâmetros de consulta pública.
type PublicEntityFilter struct {
	Search      string
	Category    string
	Grade       string
	Relevance   int
	Period      string // preset: 7d, 30d, 90d, all
	PeriodSince string // timestamp RFC3339 gerado em Go
	OrderBy     string
	OrderDir    string
	Page        int
	PageSize    int
}

// PublicEntitiesResult contém a lista paginada e os metadados de paginação.
type PublicEntitiesResult struct {
	Entities   []sqlc.ListPublicEntitiesRow
	TotalCount int64
	Page       int
	PageSize   int
	TotalPages int
}

// PublicClaimWithSources agrega um claim público e suas respectivas fontes agrupadas.
type PublicClaimWithSources struct {
	Claim   sqlc.ListPublicClaimsByEntityIDRow
	Sources []sqlc.ListPublicEvidenceSourcesByClaimIDRow
}

// PublicEntityDetail agrega todos os dados públicos necessários para a visualização detalhada de uma entidade.
type PublicEntityDetail struct {
	Entity           sqlc.GetPublicEntityBySlugRow
	Claims           []PublicClaimWithSources
	EditorialHistory []domain.PublicEditorialEvent
}

// SanitizeFilter valida parâmetros e aplica defaults seguros e conservadores utilizando o horário atual.
func SanitizeFilter(f PublicEntityFilter) PublicEntityFilter {
	return SanitizeFilterWithClock(f, time.Now().UTC())
}

// SanitizeFilterWithClock valida parâmetros e resolve períodos temporais a partir de um relógio injetável.
func SanitizeFilterWithClock(f PublicEntityFilter, now time.Time) PublicEntityFilter {
	// 1. Busca textual
	f.Search = strings.TrimSpace(f.Search)

	// 2. Categoria e Grau
	f.Category = strings.TrimSpace(f.Category)
	f.Grade = strings.ToUpper(strings.TrimSpace(f.Grade))
	if f.Grade != "" && f.Grade != "A" && f.Grade != "B" && f.Grade != "C" && f.Grade != "D" && f.Grade != "E" {
		f.Grade = ""
	}

	// 3. Relevância (1..5 ou 0 para todas)
	if f.Relevance < 1 || f.Relevance > 5 {
		f.Relevance = 0
	}

	// 4. Período: presets com allowlist estrita; não aceita timestamps arbitrários da URL
	preset := strings.ToLower(strings.TrimSpace(f.Period))
	if allowedPeriodPresets[preset] && preset != "all" {
		f.Period = preset
		f.PeriodSince = ResolvePeriod(preset, now)
	} else {
		f.Period = ""
		f.PeriodSince = ""
	}

	// 5. Ordenação com allowlist estrita em Go
	f.OrderBy = strings.ToLower(strings.TrimSpace(f.OrderBy))
	if !allowedOrderFields[f.OrderBy] {
		f.OrderBy = OrderFieldName
	}

	f.OrderDir = strings.ToLower(strings.TrimSpace(f.OrderDir))
	if !allowedOrderDirs[f.OrderDir] {
		if f.OrderBy == OrderFieldRelevance || f.OrderBy == OrderFieldUpdated {
			f.OrderDir = OrderDirDesc
		} else {
			f.OrderDir = OrderDirAsc
		}
	}

	// 6. Paginação previsível e conservadora
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = DefaultPageSize
	} else if f.PageSize > MaxPageSize {
		f.PageSize = MaxPageSize
	}

	return f
}

// ListPublicEntities busca entidades públicas ativas respeitando busca, filtros, ordenação e paginação.
func ListPublicEntities(ctx context.Context, db *sql.DB, rawFilter PublicEntityFilter) (*PublicEntitiesResult, error) {
	filter := SanitizeFilter(rawFilter)
	q := sqlc.New(db)

	searchQuery := ""
	if filter.Search != "" {
		searchQuery = "%" + EscapeLike(filter.Search) + "%"
	}

	offset := int64((filter.Page - 1) * filter.PageSize)
	limit := int64(filter.PageSize)

	// 1. Contagem total de itens correspondentes
	totalCount, err := q.CountPublicEntities(ctx, sqlc.CountPublicEntitiesParams{
		FilterCategory:    filter.Category,
		FilterGrade:       filter.Grade,
		FilterRelevance:   int64(filter.Relevance),
		FilterPeriodSince: filter.PeriodSince,
		SearchQuery:       searchQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar entidades públicas: %w", err)
	}

	totalPages := int((totalCount + limit - 1) / limit)
	if totalPages < 1 {
		totalPages = 1
	}

	// 2. Consulta da página de entidades
	rows, err := q.ListPublicEntities(ctx, sqlc.ListPublicEntitiesParams{
		OrderBy:           filter.OrderBy,
		OrderDir:          filter.OrderDir,
		FilterCategory:    filter.Category,
		FilterGrade:       filter.Grade,
		FilterRelevance:   int64(filter.Relevance),
		FilterPeriodSince: filter.PeriodSince,
		SearchQuery:       searchQuery,
		PageOffset:        offset,
		PageLimit:         limit,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar entidades públicas: %w", err)
	}

	return &PublicEntitiesResult{
		Entities:   rows,
		TotalCount: totalCount,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// GetPublicEntityDetail obtém a entidade pelo slug e carrega seus claims públicos com as fontes segregadas e o histórico editorial.
func GetPublicEntityDetail(ctx context.Context, db *sql.DB, slug string) (*PublicEntityDetail, error) {
	q := sqlc.New(db)

	ent, err := q.GetPublicEntityBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao buscar entidade pública por slug: %w", err)
	}

	claims, err := q.ListPublicClaimsByEntityID(ctx, ent.ID)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar claims públicos da entidade: %w", err)
	}

	var claimsWithSources []PublicClaimWithSources
	for _, c := range claims {
		srcs, err := q.ListPublicEvidenceSourcesByClaimID(ctx, c.ClaimID)
		if err != nil {
			return nil, fmt.Errorf("store: falha ao listar fontes do claim %s: %w", c.ClaimID, err)
		}
		claimsWithSources = append(claimsWithSources, PublicClaimWithSources{
			Claim:   c,
			Sources: srcs,
		})
	}

	history, err := GetPublicEditorialHistoryForEntity(ctx, db, ent.ID, claimsWithSources, DefaultEditorialHistoryLimit)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao carregar histórico editorial da entidade: %w", err)
	}

	return &PublicEntityDetail{
		Entity:           ent,
		Claims:           claimsWithSources,
		EditorialHistory: history,
	}, nil
}

// ListPublicCategories retorna as categorias distintas presentes em entidades públicas ativas.
func ListPublicCategories(ctx context.Context, db *sql.DB) ([]string, error) {
	q := sqlc.New(db)
	cats, err := q.ListPublicCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar categorias públicas: %w", err)
	}
	return cats, nil
}

// PublicExportData agrega todas as fatias públicas necessárias para a exportação XLSX.
type PublicExportData struct {
	Entities []sqlc.ListPublicEntitiesForExportRow
	Claims   []sqlc.ListPublicClaimsForExportRow
	Sources  []sqlc.ListPublicEvidenceSourcesForExportRow
}

// GetPublicExportData extrai atomicamente os conjuntos de entidades, alegações e evidências/fontes públicas ativas sob uma mesma transação de leitura.
func GetPublicExportData(ctx context.Context, db *sql.DB) (*PublicExportData, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao iniciar transação de leitura para exportação: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := sqlc.New(tx)

	entities, err := q.ListPublicEntitiesForExport(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar entidades para exportação: %w", err)
	}

	claims, err := q.ListPublicClaimsForExport(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar claims para exportação: %w", err)
	}

	sources, err := q.ListPublicEvidenceSourcesForExport(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar evidências/fontes para exportação: %w", err)
	}

	if err := tx.Commit(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return nil, fmt.Errorf("store: falha ao finalizar transação de leitura de exportação: %w", err)
	}

	return &PublicExportData{
		Entities: entities,
		Claims:   claims,
		Sources:  sources,
	}, nil
}

// GetPublicDocumentDetail busca um documento público e sua sequência contextual ordenada deterministicamente.
// Retorna ErrNotFound caso o documento não exista ou não possua alegações públicas ativas.
func GetPublicDocumentDetail(ctx context.Context, db *sql.DB, sourceID string) (*domain.DocumentSourceDetail, error) {
	if strings.TrimSpace(sourceID) == "" {
		return nil, ErrNotFound
	}

	q := sqlc.New(db)

	src, err := q.GetPublicDocumentSourceByID(ctx, sourceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao buscar documento público %s: %w", sourceID, err)
	}

	seqRows, err := q.ListPublicDocumentSequenceBySourceID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar sequência pública do documento %s: %w", sourceID, err)
	}

	var items []domain.DocumentSequenceItem
	for _, row := range seqRows {
		targetID := ""
		targetName := ""
		targetSlug := ""
		if row.TargetEntityID.Valid {
			targetID = row.TargetEntityID.String
			targetName = row.TargetEntityName.String
			targetSlug = row.TargetEntitySlug.String
		}

		caseID := ""
		caseName := ""
		caseSlug := ""
		if row.CaseID.Valid {
			caseID = row.CaseID.String
			caseName = row.CaseName.String
			caseSlug = row.CaseSlug.String
		}

		items = append(items, domain.DocumentSequenceItem{
			ID:                  row.EvidenceSourceID,
			EvidenceID:          row.EvidenceID,
			Excerpt:             row.Excerpt,
			Locator:             row.Locator,
			Role:                domain.EvidenceSourceRole(row.Role),
			Status:              domain.EvidenceSourceStatus(row.EvidenceSourceStatus),
			ClaimID:             row.ClaimID,
			ClaimProposition:    row.ClaimProposition,
			ClaimGrade:          domain.EvidenceGrade(row.ClaimGrade),
			ClaimDisposition:    domain.ClaimDisposition(row.ClaimDisposition),
			ClaimStatus:         domain.ClaimStatus(row.ClaimStatus),
			ClaimMetricEligible: row.ClaimMetricEligible == 1,
			RelationshipType:    row.RelationshipType,
			RelationshipSummary: row.RelationshipSummary,
			ContextLimits:       row.ContextLimits,
			SubjectEntityID:     row.SubjectEntityID,
			SubjectEntityName:   row.SubjectEntityName,
			SubjectEntitySlug:   row.SubjectEntitySlug,
			TargetEntityID:      targetID,
			TargetEntityName:    targetName,
			TargetEntitySlug:    targetSlug,
			CaseID:              caseID,
			CaseName:            caseName,
			CaseSlug:            caseSlug,
			CreatedAt:           row.EvidenceSourceCreatedAt,
			UpdatedAt:           row.EvidenceSourceUpdatedAt,
		})
	}

	// Ordenação determinística e natural da sequência
	sort.SliceStable(items, func(i, j int) bool {
		cmp := normalize.CompareLocators(items[i].Locator, items[j].Locator)
		if cmp != 0 {
			return cmp < 0
		}
		if items[i].CreatedAt != items[j].CreatedAt {
			return items[i].CreatedAt < items[j].CreatedAt
		}
		return items[i].ID < items[j].ID
	})

	var publishedAt string
	if src.PublishedAt.Valid {
		publishedAt = src.PublishedAt.String
	}
	var accessedAt string
	if src.AccessedAt.Valid {
		accessedAt = src.AccessedAt.String
	}
	var checkedAt string
	if src.SourceAccessCheckedAt.Valid {
		checkedAt = src.SourceAccessCheckedAt.String
	}
	var httpStatus int
	if src.HttpStatus.Valid {
		httpStatus = int(src.HttpStatus.Int64)
	}
	var normErr string
	if src.NormalizedErrorCode.Valid {
		normErr = src.NormalizedErrorCode.String
	}

	history, err := GetPublicEditorialHistoryForSource(ctx, db, sourceID, items, DefaultEditorialHistoryLimit)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao carregar histórico editorial do documento: %w", err)
	}

	detail := &domain.DocumentSourceDetail{
		ID:                    src.ID,
		Title:                 src.Title,
		PublisherOrAuthor:     src.PublisherOrAuthor,
		OriginalURL:           src.OriginalUrl,
		CanonicalURL:          src.CanonicalUrl,
		PublishedAt:           publishedAt,
		AccessedAt:            accessedAt,
		SourceType:            domain.SourceType(src.SourceType),
		SourceAccessStatus:    domain.SourceAccessStatus(src.SourceAccessStatus),
		SourceAccessCheckedAt: checkedAt,
		HTTPStatus:            httpStatus,
		NormalizedErrorCode:   normErr,
		CreatedAt:             src.CreatedAt,
		UpdatedAt:             src.UpdatedAt,
		Sequence:              items,
		EditorialHistory:      history,
	}

	return detail, nil
}

// DefaultEditorialHistoryLimit define o limite conservador padrão de eventos para páginas públicas.
const DefaultEditorialHistoryLimit = 50

// GetPublicEditorialHistoryForEntity recupera e sintetiza o histórico público de alterações editoriais de uma entidade.
// Aplica política estrita de redação: omite operadores (actor), fingerprints, payloads internos e texto de claims não-públicos.
func GetPublicEditorialHistoryForEntity(ctx context.Context, db *sql.DB, entityID string, publishedClaims []PublicClaimWithSources, limit int64) ([]domain.PublicEditorialEvent, error) {
	if limit <= 0 {
		limit = DefaultEditorialHistoryLimit
	}

	q := sqlc.New(db)
	rows, err := q.ListPublicEditorialHistoryByEntityID(ctx, sqlc.ListPublicEditorialHistoryByEntityIDParams{
		EntityID:   entityID,
		EventLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao consultar histórico editorial da entidade %s: %w", entityID, err)
	}

	seenEvents := make(map[string]bool)
	var events []domain.PublicEditorialEvent

	for _, row := range rows {
		isClaimCurrentlyPublic := row.IsClaimCurrentlyPublic == 1
		hadPriorApproval := row.HadPriorApproval == 1
		wasClaimEverPublic := isClaimCurrentlyPublic || hadPriorApproval

		// Omitir qualquer evento (claim ou evidence_source) se o claim associado nunca esteve publicamente disponível
		if !wasClaimEverPublic {
			continue
		}

		ev := domain.PublicEditorialEvent{
			ID:             row.DecisionID,
			CreatedAt:      row.CreatedAt,
			TargetType:     domain.PublicEditorialTargetType(row.TargetType),
			Action:         domain.PublicEditorialAction(row.Action),
			IsTargetPublic: isClaimCurrentlyPublic,
		}

		if row.TargetType == "claim" {
			ev.TargetID = row.AssociatedClaimID
			ev.ClaimID = row.AssociatedClaimID
			ev.ClaimGrade = domain.EvidenceGrade(row.AssociatedClaimGrade)
			ev.TargetLabel = fmt.Sprintf("Alegação (Grau %s)", row.AssociatedClaimGrade)

			if isClaimCurrentlyPublic {
				ev.ClaimProposition = row.AssociatedClaimProposition
			} else {
				ev.ClaimProposition = "" // Redação de itens não públicos ou retirados
			}

			switch row.Action {
			case "approve":
				ev.ActionLabel = "Aprovação e Publicação"
				ev.Summary = "Alegação aprovada pela equipe editorial com suporte documental ativo e incluída na área pública."
				ev.ImpactLabel = "Publicada e computada em métricas"
			case "reject":
				ev.ActionLabel = "Retirada Editorial"
				ev.Summary = "Alegação desaprovada e retirada da visualização pública e das métricas após deliberação editorial."
				ev.ImpactLabel = "Removida da visualização pública"
			case "restore":
				ev.ActionLabel = "Restauração para Quarentena"
				ev.Summary = "Registro restaurado para quarentena intermediária para reanálise editorial."
				ev.ImpactLabel = "Retido em quarentena"
			default:
				ev.ActionLabel = "Alteração Editorial"
				ev.Summary = "Atualização editorial no registro da alegação."
				ev.ImpactLabel = "Registro atualizado"
			}
		} else { // evidence_source
			loc := normalize.SafeLocator(row.TargetEvidenceSourceLocator)
			ev.TargetID = row.TargetEvidenceSourceID
			ev.Locator = loc
			ev.SourceID = row.TargetSourceID
			ev.SourceTitle = row.TargetSourceTitle
			ev.ClaimID = row.AssociatedClaimID
			ev.IsTargetPublic = isClaimCurrentlyPublic && row.TargetEvidenceSourceStatus == "active"
			ev.TargetLabel = "Uso de Suporte Documental"

			if isClaimCurrentlyPublic {
				ev.ClaimProposition = row.AssociatedClaimProposition
				ev.ClaimGrade = domain.EvidenceGrade(row.AssociatedClaimGrade)
			} else {
				ev.ClaimProposition = ""
			}

			switch row.Action {
			case "reject":
				ev.ActionLabel = "Desativação de Suporte"
				if row.TargetSourceTitle != "" && loc != "" {
					ev.Summary = fmt.Sprintf("Uso da fonte %q (%s) desativado após revisão editorial do trecho.", row.TargetSourceTitle, loc)
				} else if row.TargetSourceTitle != "" {
					ev.Summary = fmt.Sprintf("Uso da fonte %q desativado após revisão editorial.", row.TargetSourceTitle)
				} else {
					ev.Summary = "Uso de suporte documental desativado após revisão editorial."
				}
				ev.ImpactLabel = "Suporte documental desativado"
			case "restore":
				ev.ActionLabel = "Reativação de Suporte"
				if row.TargetSourceTitle != "" && loc != "" {
					ev.Summary = fmt.Sprintf("Uso da fonte %q (%s) reativado após reavaliação editorial.", row.TargetSourceTitle, loc)
				} else if row.TargetSourceTitle != "" {
					ev.Summary = fmt.Sprintf("Uso da fonte %q reativado após reavaliação editorial.", row.TargetSourceTitle)
				} else {
					ev.Summary = "Uso de suporte documental reativado após reavaliação editorial."
				}
				ev.ImpactLabel = "Suporte documental reativado"
			default:
				ev.ActionLabel = "Revisão de Suporte"
				ev.Summary = "Reavaliação editorial de uso de fonte documental."
				ev.ImpactLabel = "Suporte documental atualizado"
			}
		}

		if !seenEvents[ev.ID] {
			seenEvents[ev.ID] = true
			events = append(events, ev)
		}
	}

	// Sintetiza eventos de publicação inicial para claims públicos que não possuem registro de 'approve' explícito em moderation_decisions
	for _, c := range publishedClaims {
		pubID := "pub-" + c.Claim.ClaimID
		hasApprove := false
		for _, e := range events {
			if e.ClaimID == c.Claim.ClaimID && e.Action == domain.PublicEditorialActionApprove {
				hasApprove = true
				break
			}
		}

		if !hasApprove && !seenEvents[pubID] {
			seenEvents[pubID] = true
			events = append(events, domain.PublicEditorialEvent{
				ID:               pubID,
				CreatedAt:        c.Claim.CreatedAt,
				TargetType:       domain.PublicEditorialTargetClaim,
				TargetID:         c.Claim.ClaimID,
				Action:           domain.PublicEditorialActionInitialPublish,
				ActionLabel:      "Publicação do Registro",
				TargetLabel:      fmt.Sprintf("Alegação (Grau %s)", c.Claim.Grade),
				Summary:          "Registro documental compilado e publicado na plataforma com suporte de fontes verificadas.",
				ImpactLabel:      "Publicado e computado em métricas",
				IsTargetPublic:   true,
				ClaimID:          c.Claim.ClaimID,
				ClaimProposition: c.Claim.Proposition,
				ClaimGrade:       domain.EvidenceGrade(c.Claim.Grade),
			})
		}
	}

	// Ordenação determinística decrescente (mais recente primeiro)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].CreatedAt != events[j].CreatedAt {
			return events[i].CreatedAt > events[j].CreatedAt
		}
		return events[i].ID > events[j].ID
	})

	if limit > 0 && int64(len(events)) > limit {
		events = events[:limit]
	}

	return events, nil
}

// GetPublicEditorialHistoryForSource recupera e sintetiza o histórico público de alterações editoriais de uma fonte/documento.
func GetPublicEditorialHistoryForSource(ctx context.Context, db *sql.DB, sourceID string, sequenceItems []domain.DocumentSequenceItem, limit int64) ([]domain.PublicEditorialEvent, error) {
	if limit <= 0 {
		limit = DefaultEditorialHistoryLimit
	}

	q := sqlc.New(db)
	rows, err := q.ListPublicEditorialHistoryBySourceID(ctx, sqlc.ListPublicEditorialHistoryBySourceIDParams{
		SourceID:   sourceID,
		EventLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao consultar histórico editorial da fonte %s: %w", sourceID, err)
	}

	seenEvents := make(map[string]bool)
	var events []domain.PublicEditorialEvent

	for _, row := range rows {
		isClaimCurrentlyPublic := row.IsClaimCurrentlyPublic == 1
		hadPriorApproval := row.HadPriorApproval == 1
		wasClaimEverPublic := isClaimCurrentlyPublic || hadPriorApproval

		// Omitir qualquer evento (claim ou evidence_source) se o claim associado nunca esteve publicamente disponível
		if !wasClaimEverPublic {
			continue
		}

		ev := domain.PublicEditorialEvent{
			ID:             row.DecisionID,
			CreatedAt:      row.CreatedAt,
			TargetType:     domain.PublicEditorialTargetType(row.TargetType),
			Action:         domain.PublicEditorialAction(row.Action),
			IsTargetPublic: isClaimCurrentlyPublic,
		}

		if row.TargetType == "claim" {
			ev.TargetID = row.AssociatedClaimID
			ev.ClaimID = row.AssociatedClaimID
			ev.ClaimGrade = domain.EvidenceGrade(row.AssociatedClaimGrade)
			ev.TargetLabel = fmt.Sprintf("Alegação Vinculada (Grau %s)", row.AssociatedClaimGrade)

			if isClaimCurrentlyPublic {
				ev.ClaimProposition = row.AssociatedClaimProposition
			} else {
				ev.ClaimProposition = "" // Redação de itens não públicos ou retirados
			}

			switch row.Action {
			case "approve":
				ev.ActionLabel = "Aprovação e Publicação"
				ev.Summary = "Alegação sustentada por este documento foi aprovada e disponibilizada na área pública."
				ev.ImpactLabel = "Publicada e ativa"
			case "reject":
				ev.ActionLabel = "Retirada Editorial"
				ev.Summary = "Alegação associada foi desaprovada e retirada da visualização pública após revisão editorial."
				ev.ImpactLabel = "Removida da área pública"
			case "restore":
				ev.ActionLabel = "Restauração para Quarentena"
				ev.Summary = "Alegação associada foi recolhida para quarentena intermediária para reanálise."
				ev.ImpactLabel = "Retida em quarentena"
			}
		} else { // evidence_source
			loc := normalize.SafeLocator(row.TargetEvidenceSourceLocator)
			ev.TargetID = row.TargetEvidenceSourceID
			ev.Locator = loc
			ev.SourceID = row.TargetSourceID
			ev.SourceTitle = row.TargetSourceTitle
			ev.ClaimID = row.AssociatedClaimID
			ev.IsTargetPublic = isClaimCurrentlyPublic && row.TargetEvidenceSourceStatus == "active"
			ev.TargetLabel = "Trecho Documental"

			if isClaimCurrentlyPublic {
				ev.ClaimProposition = row.AssociatedClaimProposition
				ev.ClaimGrade = domain.EvidenceGrade(row.AssociatedClaimGrade)
			} else {
				ev.ClaimProposition = ""
			}

			switch row.Action {
			case "reject":
				ev.ActionLabel = "Desativação de Trecho"
				if loc != "" {
					ev.Summary = fmt.Sprintf("Trecho documental (%s) desativado após revisão editorial de uso.", loc)
				} else {
					ev.Summary = "Trecho documental desativado após revisão editorial de uso."
				}
				ev.ImpactLabel = "Trecho desativado"
			case "restore":
				ev.ActionLabel = "Reativação de Trecho"
				if loc != "" {
					ev.Summary = fmt.Sprintf("Trecho documental (%s) reativado após reavaliação editorial.", loc)
				} else {
					ev.Summary = "Trecho documental reativado após reavaliação editorial."
				}
				ev.ImpactLabel = "Trecho reativado"
			}
		}

		if !seenEvents[ev.ID] {
			seenEvents[ev.ID] = true
			events = append(events, ev)
		}
	}

	// Sintetiza eventos de publicação inicial para os trechos atualmente ativos da sequência
	for _, item := range sequenceItems {
		pubID := "pub-doc-" + item.ID
		hasApprove := false
		for _, e := range events {
			if e.TargetID == item.ID && (e.Action == domain.PublicEditorialActionApprove || e.Action == domain.PublicEditorialActionRestore) {
				hasApprove = true
				break
			}
		}

		if !hasApprove && !seenEvents[pubID] {
			seenEvents[pubID] = true
			events = append(events, domain.PublicEditorialEvent{
				ID:               pubID,
				CreatedAt:        item.CreatedAt,
				TargetType:       domain.PublicEditorialTargetEvidenceSource,
				TargetID:         item.ID,
				Action:           domain.PublicEditorialActionInitialPublish,
				ActionLabel:      "Inclusão Documental",
				TargetLabel:      "Trecho Documental",
				Summary:          fmt.Sprintf("Trecho documental (%s) incluído e referenciado em alegação pública.", normalize.SafeLocator(item.Locator)),
				ImpactLabel:      "Ativo e publicado",
				IsTargetPublic:   true,
				ClaimID:          item.ClaimID,
				ClaimProposition: item.ClaimProposition,
				ClaimGrade:       item.ClaimGrade,
				SourceID:         sourceID,
				Locator:          normalize.SafeLocator(item.Locator),
			})
		}
	}

	// Ordenação determinística decrescente
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].CreatedAt != events[j].CreatedAt {
			return events[i].CreatedAt > events[j].CreatedAt
		}
		return events[i].ID > events[j].ID
	})

	if limit > 0 && int64(len(events)) > limit {
		events = events[:limit]
	}

	return events, nil
}
