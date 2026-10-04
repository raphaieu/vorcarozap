package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/store"
)

// =============================================================================
// DTOs da API Pública v1 (VZ-029)
// =============================================================================

// APIErrorResponse representa o formato canônico de erro retornado pela API.
type APIErrorResponse struct {
	Error APIErrorDetail `json:"error"`
}

// APIErrorDetail encapsula o código da falha e a mensagem explicativa para o consumidor.
type APIErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIPaginationDTO encapsula os metadados de paginação das respostas da API.
type APIPaginationDTO struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

// APIEntityItemDTO representa uma entidade pública na listagem da API v1.
type APIEntityItemDTO struct {
	ID                  string `json:"id"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	Category            string `json:"category"`
	RoleOrContext       string `json:"role_or_context"`
	Reach               string `json:"reach"`
	Relevance           int64  `json:"relevance"`
	RelevanceRationale  string `json:"relevance_rationale"`
	PublicClaimsCount   int64  `json:"public_claims_count"`
	HighestGrade        string `json:"highest_grade"`
	LastPublicUpdatedAt string `json:"last_public_updated_at"`
	ShortSynthesis      string `json:"short_synthesis"`
}

// APIEntitiesListResponse representa o contrato de resposta de listagem da API v1.
type APIEntitiesListResponse struct {
	Data       []APIEntityItemDTO `json:"data"`
	Pagination APIPaginationDTO   `json:"pagination"`
}

// APISourceDTO representa uma fonte documental pública associada a uma alegação.
type APISourceDTO struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	PublisherOrAuthor  string `json:"publisher_or_author"`
	CanonicalURL       string `json:"canonical_url"`
	PublishedAt        string `json:"published_at,omitempty"`
	AccessedAt         string `json:"accessed_at,omitempty"`
	SourceType         string `json:"source_type"`
	Excerpt            string `json:"excerpt"`
	Locator            string `json:"locator"`
	Role               string `json:"role"`
	SourceAccessStatus string `json:"source_access_status"`
}

// APIDefenseStatementDTO representa uma manifestação de defesa aceita pela moderação.
type APIDefenseStatementDTO struct {
	ID            string `json:"id"`
	StatementType string `json:"statement_type"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	SourceURL     string `json:"source_url,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// APIClaimDTO representa uma alegação pública ativa com suas respectivas fontes e defesas.
type APIClaimDTO struct {
	ID                  string                   `json:"id"`
	RelationshipType    string                   `json:"relationship_type"`
	RelationshipSummary string                   `json:"relationship_summary"`
	ContextLimits       string                   `json:"context_limits"`
	TargetEntityName    string                   `json:"target_entity_name,omitempty"`
	CaseName            string                   `json:"case_name,omitempty"`
	Proposition         string                   `json:"proposition"`
	Attribution         string                   `json:"attribution"`
	Grade               string                   `json:"grade"`
	Disposition         string                   `json:"disposition"`
	MetricEligible      bool                     `json:"metric_eligible"`
	ContextStatus       string                   `json:"context_status"`
	UpdatedAt           string                   `json:"updated_at"`
	Sources             []APISourceDTO           `json:"sources"`
	DefenseStatements   []APIDefenseStatementDTO `json:"defense_statements,omitempty"`
}

// APIEditorialEventDTO representa um evento auditável do histórico público redigido.
type APIEditorialEventDTO struct {
	ID             string `json:"id"`
	CreatedAt      string `json:"created_at"`
	Action         string `json:"action"`
	TargetType     string `json:"target_type"`
	Summary        string `json:"summary"`
	IsTargetPublic bool   `json:"is_target_public"`
	ClaimID        string `json:"claim_id,omitempty"`
	ClaimGrade     string `json:"claim_grade,omitempty"`
	SourceID       string `json:"source_id,omitempty"`
	SourceTitle    string `json:"source_title,omitempty"`
	Locator        string `json:"locator,omitempty"`
}

// APIEntityDetailDTO representa o detalhe completo de uma entidade pública.
type APIEntityDetailDTO struct {
	ID                 string                 `json:"id"`
	Slug               string                 `json:"slug"`
	Name               string                 `json:"name"`
	Category           string                 `json:"category"`
	RoleOrContext      string                 `json:"role_or_context"`
	Reach              string                 `json:"reach"`
	Summary            string                 `json:"summary"`
	Relevance          int64                  `json:"relevance"`
	RelevanceRationale string                 `json:"relevance_rationale"`
	UpdatedAt          string                 `json:"updated_at"`
	Claims             []APIClaimDTO          `json:"claims"`
	EditorialHistory   []APIEditorialEventDTO `json:"editorial_history,omitempty"`
}

// APIEntityDetailResponse representa o contrato de resposta de detalhe da API v1.
type APIEntityDetailResponse struct {
	Data APIEntityDetailDTO `json:"data"`
}

// =============================================================================
// Roteador e Handlers da API Pública v1
// =============================================================================

// writeAPIError padroniza o envio de erros em JSON com cabeçalhos de segurança e cache dinâmico.
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIErrorResponse{
		Error: APIErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// newAPIV1Router cria e configura o roteador dedicado à API v1 somente leitura.
func newAPIV1Router(handlers *Handlers) http.Handler {
	r := chi.NewRouter()

	r.Get("/pessoas", handlers.HandleAPIV1Entities)
	r.Get("/pessoas/{slug}", handlers.HandleAPIV1EntityDetail)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusNotFound, "not_found", "Endpoint não encontrado na API v1.")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método HTTP não permitido. A API v1 é estritamente somente leitura (GET).")
	})

	return r
}

// HandleAPIV1Entities atende GET /api/v1/pessoas com listagem paginada, busca e filtros.
func (h *Handlers) HandleAPIV1Entities(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))

	gradeParam := strings.TrimSpace(r.URL.Query().Get("grade"))
	if gradeParam != "" {
		gradeUpper := strings.ToUpper(gradeParam)
		if gradeUpper != "A" && gradeUpper != "B" && gradeUpper != "C" && gradeUpper != "D" && gradeUpper != "E" {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Parâmetro 'grade' inválido. Valores permitidos: A, B, C, D, E.")
			return
		}
		gradeParam = gradeUpper
	}

	relParam := strings.TrimSpace(r.URL.Query().Get("relevance"))
	relevance := 0
	if relParam != "" {
		val, err := strconv.Atoi(relParam)
		if err != nil || val < 1 || val > 5 {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Parâmetro 'relevance' inválido. O valor deve ser um número inteiro entre 1 e 5.")
			return
		}
		relevance = val
	}

	periodParam := strings.TrimSpace(r.URL.Query().Get("period"))
	if periodParam != "" {
		periodLower := strings.ToLower(periodParam)
		if periodLower != "7d" && periodLower != "30d" && periodLower != "90d" && periodLower != "all" {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Parâmetro 'period' inválido. Valores permitidos: 7d, 30d, 90d, all.")
			return
		}
		periodParam = periodLower
	}

	sortParam := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortParam == "" {
		sortParam = strings.TrimSpace(r.URL.Query().Get("order_by"))
	}
	if sortParam != "" {
		sortLower := strings.ToLower(sortParam)
		if sortLower != store.OrderFieldName && sortLower != store.OrderFieldRelevance && sortLower != store.OrderFieldUpdated {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Parâmetro 'sort' inválido. Valores permitidos: name, relevance, updated.")
			return
		}
		sortParam = sortLower
	}

	dirParam := strings.TrimSpace(r.URL.Query().Get("dir"))
	if dirParam == "" {
		dirParam = strings.TrimSpace(r.URL.Query().Get("order_dir"))
	}
	if dirParam != "" {
		dirLower := strings.ToLower(dirParam)
		if dirLower != store.OrderDirAsc && dirLower != store.OrderDirDesc {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "Parâmetro 'dir' inválido. Valores permitidos: asc, desc.")
			return
		}
		dirParam = dirLower
	}

	pageParam := strings.TrimSpace(r.URL.Query().Get("page"))
	page := 1
	if pageParam != "" {
		val, err := strconv.Atoi(pageParam)
		if err != nil || val < 1 || val > store.MaxPage {
			writeAPIError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("Parâmetro 'page' inválido. O valor deve ser um número inteiro entre 1 e %d.", store.MaxPage))
			return
		}
		page = val
	}

	pageSizeParam := strings.TrimSpace(r.URL.Query().Get("page_size"))
	pageSize := store.DefaultPageSize
	if pageSizeParam != "" {
		val, err := strconv.Atoi(pageSizeParam)
		if err != nil || val < 1 || val > store.MaxPageSize {
			writeAPIError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("Parâmetro 'page_size' inválido. O valor deve ser um número inteiro entre 1 e %d.", store.MaxPageSize))
			return
		}
		pageSize = val
	}

	filter := store.PublicEntityFilter{
		Search:    q,
		Category:  category,
		Grade:     gradeParam,
		Relevance: relevance,
		Period:    periodParam,
		OrderBy:   sortParam,
		OrderDir:  dirParam,
		Page:      page,
		PageSize:  pageSize,
	}

	res, err := store.ListPublicEntities(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("falha ao listar entidades públicas para API v1", "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "Erro interno ao consultar entidades públicas.")
		return
	}

	items := make([]APIEntityItemDTO, len(res.Entities))
	for i, ent := range res.Entities {
		items[i] = APIEntityItemDTO{
			ID:                  ent.ID,
			Slug:                ent.Slug,
			Name:                ent.Name,
			Category:            ent.Category,
			RoleOrContext:       ent.RoleOrContext,
			Reach:               ent.Reach,
			Relevance:           ent.Relevance,
			RelevanceRationale:  ent.RelevanceRationale,
			PublicClaimsCount:   ent.PublicClaimsCount,
			HighestGrade:        ent.HighestGrade,
			LastPublicUpdatedAt: ent.LastPublicUpdatedAt,
			ShortSynthesis:      ent.ShortSynthesis,
		}
	}

	resp := APIEntitiesListResponse{
		Data: items,
		Pagination: APIPaginationDTO{
			Page:       res.Page,
			PageSize:   res.PageSize,
			TotalItems: res.TotalCount,
			TotalPages: res.TotalPages,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("falha ao serializar resposta da API v1", "error", err)
	}
}

// HandleAPIV1EntityDetail atende GET /api/v1/pessoas/{slug} expondo o detalhe completo da entidade pública.
func (h *Handlers) HandleAPIV1EntityDetail(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		writeAPIError(w, http.StatusNotFound, "not_found", "Entidade pública não encontrada.")
		return
	}

	detail, err := store.GetPublicEntityDetail(r.Context(), h.db, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "not_found", "Entidade pública não encontrada.")
			return
		}
		slog.Error("falha ao carregar detalhe da entidade para API v1", "slug", slug, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "Erro interno ao carregar dados da entidade.")
		return
	}

	lastUpdated := detail.Entity.UpdatedAt
	claimsDTO := make([]APIClaimDTO, 0, len(detail.Claims))

	for _, c := range detail.Claims {
		if c.Claim.UpdatedAt > lastUpdated {
			lastUpdated = c.Claim.UpdatedAt
		}

		sourcesDTO := make([]APISourceDTO, 0, len(c.Sources))
		for _, s := range c.Sources {
			pubAt := ""
			if s.PublishedAt.Valid {
				pubAt = s.PublishedAt.String
			}
			accAt := ""
			if s.AccessedAt.Valid {
				accAt = s.AccessedAt.String
			}
			loc := strings.TrimSpace(s.Locator)
			if formattedLoc, err := normalize.Locator(loc); err == nil && formattedLoc != "" {
				loc = formattedLoc
			}

			sourcesDTO = append(sourcesDTO, APISourceDTO{
				ID:                 s.SourceID,
				Title:              strings.TrimSpace(s.Title),
				PublisherOrAuthor:  strings.TrimSpace(s.PublisherOrAuthor),
				CanonicalURL:       s.CanonicalUrl,
				PublishedAt:        pubAt,
				AccessedAt:         accAt,
				SourceType:         s.SourceType,
				Excerpt:            strings.TrimSpace(s.Excerpt),
				Locator:            loc,
				Role:               s.Role,
				SourceAccessStatus: s.SourceAccessStatus,
			})
		}

		stmtsDTO := make([]APIDefenseStatementDTO, 0, len(c.Statements))
		for _, stmt := range c.Statements {
			stmtsDTO = append(stmtsDTO, APIDefenseStatementDTO{
				ID:            stmt.ID,
				StatementType: string(stmt.StatementType),
				Title:         stmt.Title,
				Content:       stmt.Content,
				SourceURL:     stmt.SourceURL,
				CreatedAt:     stmt.CreatedAt,
			})
		}

		targetName := ""
		if c.Claim.TargetEntityName.Valid {
			targetName = c.Claim.TargetEntityName.String
		}

		caseName := ""
		if c.Claim.CaseName.Valid {
			caseName = c.Claim.CaseName.String
		}

		claimsDTO = append(claimsDTO, APIClaimDTO{
			ID:                  c.Claim.ClaimID,
			RelationshipType:    c.Claim.RelationshipType,
			RelationshipSummary: c.Claim.RelationshipSummary,
			ContextLimits:       c.Claim.ContextLimits,
			TargetEntityName:    targetName,
			CaseName:            caseName,
			Proposition:         c.Claim.Proposition,
			Attribution:         c.Claim.Attribution,
			Grade:               c.Claim.Grade,
			Disposition:         c.Claim.Disposition,
			MetricEligible:      c.Claim.MetricEligible == 1,
			ContextStatus:       c.Claim.ContextStatus,
			UpdatedAt:           c.Claim.UpdatedAt,
			Sources:             sourcesDTO,
			DefenseStatements:   stmtsDTO,
		})
	}

	historyDTO := make([]APIEditorialEventDTO, 0, len(detail.EditorialHistory))
	for _, ev := range detail.EditorialHistory {
		historyDTO = append(historyDTO, APIEditorialEventDTO{
			ID:             ev.ID,
			CreatedAt:      ev.CreatedAt,
			Action:         string(ev.Action),
			TargetType:     string(ev.TargetType),
			Summary:        ev.Summary,
			IsTargetPublic: ev.IsTargetPublic,
			ClaimID:        ev.ClaimID,
			ClaimGrade:     string(ev.ClaimGrade),
			SourceID:       ev.SourceID,
			SourceTitle:    ev.SourceTitle,
			Locator:        ev.Locator,
		})
	}

	resp := APIEntityDetailResponse{
		Data: APIEntityDetailDTO{
			ID:                 detail.Entity.ID,
			Slug:               detail.Entity.Slug,
			Name:               detail.Entity.Name,
			Category:           detail.Entity.Category,
			RoleOrContext:      detail.Entity.RoleOrContext,
			Reach:              detail.Entity.Reach,
			Summary:            detail.Entity.Summary,
			Relevance:          detail.Entity.Relevance,
			RelevanceRationale: detail.Entity.RelevanceRationale,
			UpdatedAt:          lastUpdated,
			Claims:             claimsDTO,
			EditorialHistory:   historyDTO,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("falha ao serializar detalhe da entidade na API v1", "slug", slug, "error", err)
	}
}
