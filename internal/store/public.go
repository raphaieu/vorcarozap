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
	Entity sqlc.GetPublicEntityBySlugRow
	Claims []PublicClaimWithSources
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

// GetPublicEntityDetail obtém a entidade pelo slug e carrega seus claims públicos com as fontes segregadas.
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

	return &PublicEntityDetail{
		Entity: ent,
		Claims: claimsWithSources,
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

