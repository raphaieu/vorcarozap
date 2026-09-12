package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

const (
	DefaultAdminPageSize = 20
	MaxAdminPageSize     = 100
)

var allowedCandidateStatuses = map[string]bool{
	"quarantined": true,
	"published":   true,
	"rejected":    true,
	"archived":    true,
}

var allowedGrades = map[string]bool{
	"A": true,
	"B": true,
	"C": true,
	"D": true,
	"E": true,
}

var allowedClaimStatuses = map[string]bool{
	"published":   true,
	"quarantined": true,
	"rejected":    true,
	"archived":    true,
}

var allowedEvidenceSourceStatuses = map[string]bool{
	"active":   true,
	"rejected": true,
}

var allowedClaimOrigins = map[string]bool{
	"openrouter":   true,
	"curated_seed": true,
	"admin":        true,
}

var allowedEvidenceRoles = map[string]bool{
	"supports":       true,
	"contradicts":    true,
	"contextualizes": true,
}

var allowedSourceAccessStatuses = map[string]bool{
	"cited_by_provider": true,
	"reachable":         true,
	"unreachable":       true,
	"not_checked":       true,
}

// AdminSourceTypeOption define uma opção canônica de tipo de fonte para filtragem e exibição administrativa.
type AdminSourceTypeOption struct {
	Value string
	Label string
}

// adminSourceTypeOptions define a lista canônica, ordenada e imutável de tipos administrativos de fonte.
var adminSourceTypeOptions = []AdminSourceTypeOption{
	{Value: "article", Label: "Artigo / Notícia"},
	{Value: "official_statement", Label: "Nota Oficial / Comunicado"},
	{Value: "court_document", Label: "Peça Judicial / Decisão"},
	{Value: "police_report", Label: "Relatório Policial / Pericial"},
	{Value: "interview", Label: "Entrevista"},
	{Value: "social_media", Label: "Rede Social"},
}

// allowedAdminSourceTypesSet é o mapa de consulta rápida derivado da lista canônica ordenada.
var allowedAdminSourceTypesSet = func() map[string]bool {
	m := make(map[string]bool, len(adminSourceTypeOptions))
	for _, opt := range adminSourceTypeOptions {
		m[opt.Value] = true
	}
	return m
}()

// GetAdminSourceTypeOptions retorna uma cópia segura e ordenada das opções canônicas de tipos de fonte.
func GetAdminSourceTypeOptions() []AdminSourceTypeOption {
	out := make([]AdminSourceTypeOption, len(adminSourceTypeOptions))
	copy(out, adminSourceTypeOptions)
	return out
}

// IsAllowedAdminSourceType valida se um valor de source_type faz parte da allowlist canônica.
func IsAllowedAdminSourceType(sourceType string) bool {
	return allowedAdminSourceTypesSet[strings.ToLower(strings.TrimSpace(sourceType))]
}

// AdminCandidateFilter encapsula os parâmetros de filtro para a listagem de candidatos de monitoramento.
type AdminCandidateFilter struct {
	Status      string // quarantined, published, rejected, archived
	Grade       string // A-E
	RunID       string // ID da monitoring_run
	Period      string // preset: 7d, 30d, 90d, all
	PeriodSince string // RFC3339 gerado em Go
	Search      string
	Page        int
	PageSize    int
}

// AdminCandidateListResult contém a lista paginada e os metadados de paginação de candidatos.
type AdminCandidateListResult struct {
	Candidates []sqlc.ListAdminCandidatesRow
	TotalCount int64
	Page       int
	PageSize   int
	TotalPages int
}

// AdminCandidateDetail agrega os dados de um candidato e seu histórico de avaliações semânticas.
type AdminCandidateDetail struct {
	Candidate   sqlc.GetAdminCandidateByIDRow
	Evaluations []sqlc.ListAdminSemanticEvaluationsByCandidateIDRow
}

// AdminClaimDetail agrega os dados de uma alegação, seus usos de evidência e o histórico de moderação.
type AdminClaimDetail struct {
	Claim               sqlc.GetAdminClaimByIDRow
	EvidenceSources     []sqlc.ListAdminEvidenceSourcesByClaimIDRow
	ActiveSupportsCount int64
	Decisions           []sqlc.ModerationDecision
	CandidateID         string
}

// AdminEvidenceSourceDetail agrega os dados de um uso de evidência, seu claim associado, fonte documental e histórico de moderação.
type AdminEvidenceSourceDetail struct {
	EvidenceSource           sqlc.GetAdminEvidenceSourceByIDRow
	ActiveSupportsCount      int64
	OtherActiveSupportsCount int64
	Decisions                []sqlc.ModerationDecision
	CandidateID              string
}

// SanitizeAdminCandidateFilter valida e normaliza parâmetros do filtro de candidatos usando o relógio atual.
func SanitizeAdminCandidateFilter(f AdminCandidateFilter) AdminCandidateFilter {
	return SanitizeAdminCandidateFilterWithClock(f, time.Now().UTC())
}

// SanitizeAdminCandidateFilterWithClock valida e normaliza parâmetros de candidatos a partir de relógio injetável.
func SanitizeAdminCandidateFilterWithClock(f AdminCandidateFilter, now time.Time) AdminCandidateFilter {
	f.Search = strings.TrimSpace(f.Search)
	if len(f.Search) > 200 {
		f.Search = f.Search[:200]
	}

	st := strings.ToLower(strings.TrimSpace(f.Status))
	if allowedCandidateStatuses[st] {
		f.Status = st
	} else {
		f.Status = ""
	}

	gr := strings.ToUpper(strings.TrimSpace(f.Grade))
	if allowedGrades[gr] {
		f.Grade = gr
	} else {
		f.Grade = ""
	}

	f.RunID = strings.TrimSpace(f.RunID)
	if len(f.RunID) > 100 {
		f.RunID = f.RunID[:100]
	}

	preset := strings.ToLower(strings.TrimSpace(f.Period))
	if allowedPeriodPresets[preset] && preset != "all" {
		f.Period = preset
		f.PeriodSince = ResolvePeriod(preset, now)
	} else {
		f.Period = ""
		f.PeriodSince = ""
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = DefaultAdminPageSize
	} else if f.PageSize > MaxAdminPageSize {
		f.PageSize = MaxAdminPageSize
	}

	return f
}

// AdminEvidenceFilter encapsula os parâmetros de filtro para a listagem de usos de evidência (evidence_sources).
type AdminEvidenceFilter struct {
	ClaimStatus          string // published, quarantined, rejected, archived
	EvidenceSourceStatus string // active, rejected
	Origin               string // openrouter, curated_seed, admin
	Grade                string // A-E
	Role                 string // supports, contradicts, contextualizes
	Period               string // preset: 7d, 30d, 90d, all
	PeriodSince          string // RFC3339 gerado em Go
	Search               string
	Page                 int
	PageSize             int
}

// AdminEvidenceListResult contém a lista paginada e os metadados de paginação de usos de evidência.
type AdminEvidenceListResult struct {
	EvidenceSources []sqlc.ListAdminEvidenceSourcesRow
	TotalCount      int64
	Page            int
	PageSize        int
	TotalPages      int
}

// SanitizeAdminEvidenceFilter valida e normaliza parâmetros do filtro de evidências usando o relógio atual.
func SanitizeAdminEvidenceFilter(f AdminEvidenceFilter) AdminEvidenceFilter {
	return SanitizeAdminEvidenceFilterWithClock(f, time.Now().UTC())
}

// SanitizeAdminEvidenceFilterWithClock valida e normaliza parâmetros de evidências com relógio injetável.
func SanitizeAdminEvidenceFilterWithClock(f AdminEvidenceFilter, now time.Time) AdminEvidenceFilter {
	f.Search = strings.TrimSpace(f.Search)
	if len(f.Search) > 200 {
		f.Search = f.Search[:200]
	}

	cst := strings.ToLower(strings.TrimSpace(f.ClaimStatus))
	if allowedClaimStatuses[cst] {
		f.ClaimStatus = cst
	} else {
		f.ClaimStatus = ""
	}

	esSt := strings.ToLower(strings.TrimSpace(f.EvidenceSourceStatus))
	if allowedEvidenceSourceStatuses[esSt] {
		f.EvidenceSourceStatus = esSt
	} else {
		f.EvidenceSourceStatus = ""
	}

	orig := strings.ToLower(strings.TrimSpace(f.Origin))
	if allowedClaimOrigins[orig] {
		f.Origin = orig
	} else {
		f.Origin = ""
	}

	gr := strings.ToUpper(strings.TrimSpace(f.Grade))
	if allowedGrades[gr] {
		f.Grade = gr
	} else {
		f.Grade = ""
	}

	role := strings.ToLower(strings.TrimSpace(f.Role))
	if allowedEvidenceRoles[role] {
		f.Role = role
	} else {
		f.Role = ""
	}

	preset := strings.ToLower(strings.TrimSpace(f.Period))
	if allowedPeriodPresets[preset] && preset != "all" {
		f.Period = preset
		f.PeriodSince = ResolvePeriod(preset, now)
	} else {
		f.Period = ""
		f.PeriodSince = ""
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = DefaultAdminPageSize
	} else if f.PageSize > MaxAdminPageSize {
		f.PageSize = MaxAdminPageSize
	}

	return f
}

// AdminSourceFilter encapsula os parâmetros de filtro para a listagem de fontes.
type AdminSourceFilter struct {
	AccessStatus string // cited_by_provider, reachable, unreachable, not_checked
	SourceType   string
	Period       string // preset: 7d, 30d, 90d, all
	PeriodSince  string // RFC3339 gerado em Go
	Search       string
	Page         int
	PageSize     int
}

// AdminSourceListResult contém a lista paginada e os metadados de paginação de fontes.
type AdminSourceListResult struct {
	Sources    []sqlc.ListAdminSourcesRow
	TotalCount int64
	Page       int
	PageSize   int
	TotalPages int
}

// SanitizeAdminSourceFilter valida e normaliza parâmetros do filtro de fontes usando o relógio atual.
func SanitizeAdminSourceFilter(f AdminSourceFilter) AdminSourceFilter {
	return SanitizeAdminSourceFilterWithClock(f, time.Now().UTC())
}

// SanitizeAdminSourceFilterWithClock valida e normaliza parâmetros de fontes com relógio injetável.
func SanitizeAdminSourceFilterWithClock(f AdminSourceFilter, now time.Time) AdminSourceFilter {
	f.Search = strings.TrimSpace(f.Search)
	if len(f.Search) > 200 {
		f.Search = f.Search[:200]
	}

	acc := strings.ToLower(strings.TrimSpace(f.AccessStatus))
	if allowedSourceAccessStatuses[acc] {
		f.AccessStatus = acc
	} else {
		f.AccessStatus = ""
	}

	st := strings.ToLower(strings.TrimSpace(f.SourceType))
	if IsAllowedAdminSourceType(st) {
		f.SourceType = st
	} else {
		f.SourceType = ""
	}

	preset := strings.ToLower(strings.TrimSpace(f.Period))
	if allowedPeriodPresets[preset] && preset != "all" {
		f.Period = preset
		f.PeriodSince = ResolvePeriod(preset, now)
	} else {
		f.Period = ""
		f.PeriodSince = ""
	}

	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = DefaultAdminPageSize
	} else if f.PageSize > MaxAdminPageSize {
		f.PageSize = MaxAdminPageSize
	}

	return f
}

// GetAdminOverviewCounts consulta os totais e contagens operacionais para o dashboard administrativo.
func GetAdminOverviewCounts(ctx context.Context, db *sql.DB) (*sqlc.GetAdminOverviewCountsRow, error) {
	q := sqlc.New(db)
	stats, err := q.GetAdminOverviewCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao consultar estatísticas do painel administrativo: %w", err)
	}
	return &stats, nil
}

// ListAdminCandidates pesquisa candidatos de monitoramento com filtros, busca textual, paginação e ordenação determinística.
func ListAdminCandidates(ctx context.Context, db *sql.DB, rawFilter AdminCandidateFilter) (*AdminCandidateListResult, error) {
	filter := SanitizeAdminCandidateFilter(rawFilter)
	q := sqlc.New(db)

	searchQuery := ""
	if filter.Search != "" {
		searchQuery = "%" + EscapeLike(filter.Search) + "%"
	}

	offset := int64((filter.Page - 1) * filter.PageSize)
	limit := int64(filter.PageSize)

	totalCount, err := q.CountAdminCandidates(ctx, sqlc.CountAdminCandidatesParams{
		FilterStatus:      filter.Status,
		FilterGrade:       filter.Grade,
		FilterRunID:       filter.RunID,
		FilterPeriodSince: filter.PeriodSince,
		SearchQuery:       searchQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar candidatos administrativos: %w", err)
	}

	totalPages := int((totalCount + limit - 1) / limit)
	if totalPages < 1 {
		totalPages = 1
	}

	rows, err := q.ListAdminCandidates(ctx, sqlc.ListAdminCandidatesParams{
		FilterStatus:      filter.Status,
		FilterGrade:       filter.Grade,
		FilterRunID:       filter.RunID,
		FilterPeriodSince: filter.PeriodSince,
		SearchQuery:       searchQuery,
		PageOffset:        offset,
		PageLimit:         limit,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar candidatos administrativos: %w", err)
	}

	return &AdminCandidateListResult{
		Candidates: rows,
		TotalCount: totalCount,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// GetAdminCandidateDetail carrega os detalhes completos de um candidato e seu histórico de avaliações semânticas.
func GetAdminCandidateDetail(ctx context.Context, db *sql.DB, id string) (*AdminCandidateDetail, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrNotFound
	}

	q := sqlc.New(db)
	candidate, err := q.GetAdminCandidateByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao carregar candidato %s: %w", id, err)
	}

	evals, err := q.ListAdminSemanticEvaluationsByCandidateID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao carregar avaliações semânticas do candidato %s: %w", id, err)
	}

	return &AdminCandidateDetail{
		Candidate:   candidate,
		Evaluations: evals,
	}, nil
}

// GetAdminClaimDetail carrega os detalhes completos de uma alegação, seus usos de evidência e o histórico de moderação.
func GetAdminClaimDetail(ctx context.Context, db *sql.DB, id string) (*AdminClaimDetail, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrNotFound
	}

	q := sqlc.New(db)
	claim, err := q.GetAdminClaimByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao carregar alegação %s: %w", id, err)
	}

	evidenceSources, err := q.ListAdminEvidenceSourcesByClaimID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao carregar evidências da alegação %s: %w", id, err)
	}

	decisions, err := q.ListAdminModerationDecisionsByClaimID(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao carregar decisões de moderação da alegação %s: %w", id, err)
	}

	activeSupportsCount, err := q.CountActiveSupportsEvidenceSourcesByClaimID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar suportes ativos da alegação %s: %w", id, err)
	}

	candidateID := ""
	cid, err := q.GetAdminCandidateIDByPublishedClaimID(ctx, sql.NullString{String: id, Valid: true})
	if err == nil {
		candidateID = cid
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: falha ao consultar candidato associado da alegação %s: %w", id, err)
	}

	return &AdminClaimDetail{
		Claim:               claim,
		EvidenceSources:     evidenceSources,
		ActiveSupportsCount: activeSupportsCount,
		Decisions:           decisions,
		CandidateID:         candidateID,
	}, nil
}

// GetAdminEvidenceSourceDetail carrega os detalhes completos de um uso de evidência, seu claim, dados da fonte e histórico de moderação.
func GetAdminEvidenceSourceDetail(ctx context.Context, db *sql.DB, id string) (*AdminEvidenceSourceDetail, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrNotFound
	}

	q := sqlc.New(db)
	es, err := q.GetAdminEvidenceSourceByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao carregar uso de evidência %s: %w", id, err)
	}

	activeSupportsCount, err := q.CountActiveSupportsEvidenceSourcesByClaimID(ctx, es.ClaimID)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar suportes ativos do claim %s: %w", es.ClaimID, err)
	}

	otherActiveSupportsCount, err := q.CountActiveSupportsEvidenceSourcesByClaimIDExcludingID(ctx, sqlc.CountActiveSupportsEvidenceSourcesByClaimIDExcludingIDParams{
		ClaimID:                 es.ClaimID,
		ExcludeEvidenceSourceID: es.EvidenceSourceID,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar outros suportes ativos do claim %s: %w", es.ClaimID, err)
	}

	decisions, err := q.ListAdminModerationDecisionsByEvidenceSourceID(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao carregar decisões de moderação do uso de evidência %s: %w", id, err)
	}

	candidateID := ""
	cid, err := q.GetAdminCandidateIDByPublishedClaimID(ctx, sql.NullString{String: es.ClaimID, Valid: true})
	if err == nil {
		candidateID = cid
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: falha ao consultar candidato associado do claim %s: %w", es.ClaimID, err)
	}

	return &AdminEvidenceSourceDetail{
		EvidenceSource:           es,
		ActiveSupportsCount:      activeSupportsCount,
		OtherActiveSupportsCount: otherActiveSupportsCount,
		Decisions:                decisions,
		CandidateID:              candidateID,
	}, nil
}

// ListAdminEvidenceSources pesquisa usos de evidência com filtros, busca textual, paginação e ordenação determinística.
func ListAdminEvidenceSources(ctx context.Context, db *sql.DB, rawFilter AdminEvidenceFilter) (*AdminEvidenceListResult, error) {
	filter := SanitizeAdminEvidenceFilter(rawFilter)
	q := sqlc.New(db)

	searchQuery := ""
	if filter.Search != "" {
		searchQuery = "%" + EscapeLike(filter.Search) + "%"
	}

	offset := int64((filter.Page - 1) * filter.PageSize)
	limit := int64(filter.PageSize)

	totalCount, err := q.CountAdminEvidenceSources(ctx, sqlc.CountAdminEvidenceSourcesParams{
		FilterClaimStatus:          filter.ClaimStatus,
		FilterEvidenceSourceStatus: filter.EvidenceSourceStatus,
		FilterOrigin:               filter.Origin,
		FilterGrade:                filter.Grade,
		FilterRole:                 filter.Role,
		FilterPeriodSince:          filter.PeriodSince,
		SearchQuery:                searchQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar usos de evidência administrativos: %w", err)
	}

	totalPages := int((totalCount + limit - 1) / limit)
	if totalPages < 1 {
		totalPages = 1
	}

	rows, err := q.ListAdminEvidenceSources(ctx, sqlc.ListAdminEvidenceSourcesParams{
		FilterClaimStatus:          filter.ClaimStatus,
		FilterEvidenceSourceStatus: filter.EvidenceSourceStatus,
		FilterOrigin:               filter.Origin,
		FilterGrade:                filter.Grade,
		FilterRole:                 filter.Role,
		FilterPeriodSince:          filter.PeriodSince,
		SearchQuery:                searchQuery,
		PageOffset:                 offset,
		PageLimit:                  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar usos de evidência administrativos: %w", err)
	}

	return &AdminEvidenceListResult{
		EvidenceSources: rows,
		TotalCount:      totalCount,
		Page:            filter.Page,
		PageSize:        filter.PageSize,
		TotalPages:      totalPages,
	}, nil
}

// ListAdminSources pesquisa fontes com filtros, busca textual, paginação e ordenação determinística.
func ListAdminSources(ctx context.Context, db *sql.DB, rawFilter AdminSourceFilter) (*AdminSourceListResult, error) {
	filter := SanitizeAdminSourceFilter(rawFilter)
	q := sqlc.New(db)

	searchQuery := ""
	if filter.Search != "" {
		searchQuery = "%" + EscapeLike(filter.Search) + "%"
	}

	offset := int64((filter.Page - 1) * filter.PageSize)
	limit := int64(filter.PageSize)

	totalCount, err := q.CountAdminSources(ctx, sqlc.CountAdminSourcesParams{
		FilterAccessStatus: filter.AccessStatus,
		FilterSourceType:   filter.SourceType,
		FilterPeriodSince:  filter.PeriodSince,
		SearchQuery:        searchQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar fontes administrativas: %w", err)
	}

	totalPages := int((totalCount + limit - 1) / limit)
	if totalPages < 1 {
		totalPages = 1
	}

	rows, err := q.ListAdminSources(ctx, sqlc.ListAdminSourcesParams{
		FilterAccessStatus: filter.AccessStatus,
		FilterSourceType:   filter.SourceType,
		FilterPeriodSince:  filter.PeriodSince,
		SearchQuery:        searchQuery,
		PageOffset:         offset,
		PageLimit:          limit,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar fontes administrativas: %w", err)
	}

	return &AdminSourceListResult{
		Sources:    rows,
		TotalCount: totalCount,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}
