package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

// PublicEntityFilter encapsula os parâmetros de consulta pública.
type PublicEntityFilter struct {
	Search      string
	Category    string
	Grade       string
	Relevance   int
	PeriodSince string
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

// SanitizeFilter valida parâmetros e aplica defaults seguros e conservadores.
func SanitizeFilter(f PublicEntityFilter) PublicEntityFilter {
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

	// 4. Período
	f.PeriodSince = strings.TrimSpace(f.PeriodSince)

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
		searchQuery = "%" + filter.Search + "%"
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
