package web

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/web/pages"
)

type Handlers struct {
	db *sql.DB
}

func NewHandlers(db *sql.DB) *Handlers {
	return &Handlers{db: db}
}

// HandleHealthLive comprova que o processo HTTP está ativo e aceitando requisições.
func (h *Handlers) HandleHealthLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"live"}`))
}

// HandleHealthReady comprova que a aplicação acessa o SQLite e que as migrations estão aplicadas.
// Retorna HTTP 503 e mensagem genérica caso haja falha, sem expor detalhes internos.
func (h *Handlers) HandleHealthReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := store.CheckReady(ctx, h.db); err != nil {
		slog.Error("readiness probe failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

// HandleHome renderiza a página pública inicial SSR via templ.
func (h *Handlers) HandleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := pages.Home()
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render home template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleEntities renderiza a listagem paginada de entidades públicas com busca e filtros.
func (h *Handlers) HandleEntities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rel, _ := strconv.Atoi(q.Get("relevance"))
	page, _ := strconv.Atoi(q.Get("page"))

	filter := store.PublicEntityFilter{
		Search:      q.Get("q"),
		Category:    q.Get("category"),
		Grade:       q.Get("grade"),
		Relevance:   rel,
		PeriodSince: q.Get("period"),
		OrderBy:     q.Get("sort"),
		OrderDir:    q.Get("dir"),
		Page:        page,
	}

	res, err := store.ListPublicEntities(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list public entities", "error", err)
		http.Error(w, "Erro ao consultar entidades públicas", http.StatusInternalServerError)
		return
	}

	categories, err := store.ListPublicCategories(r.Context(), h.db)
	if err != nil {
		slog.Warn("failed to list categories for filter", "error", err)
		categories = nil
	}

	var cardVMs []pages.EntityCardVM
	for _, ent := range res.Entities {
		cardVMs = append(cardVMs, pages.ToEntityCardVM(ent))
	}

	sanitized := store.SanitizeFilter(filter)
	vm := pages.EntityListVM{
		Entities: cardVMs,
		Filter: pages.FilterParamsVM{
			Search:      sanitized.Search,
			Category:    sanitized.Category,
			Grade:       sanitized.Grade,
			Relevance:   sanitized.Relevance,
			PeriodSince: sanitized.PeriodSince,
			OrderBy:     sanitized.OrderBy,
			OrderDir:    sanitized.OrderDir,
			Page:        res.Page,
			PageSize:    res.PageSize,
			TotalPages:  res.TotalPages,
			TotalCount:  res.TotalCount,
			Categories:  categories,
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := pages.Entities(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render entities template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleEntityDetail renderiza os detalhes e alegações/fontes de uma entidade pública pelo slug.
func (h *Handlers) HandleEntityDetail(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if strings.TrimSpace(slug) == "" {
		http.NotFound(w, r)
		return
	}

	detail, err := store.GetPublicEntityDetail(r.Context(), h.db, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get entity detail", "slug", slug, "error", err)
		http.Error(w, "Erro interno ao carregar dados da entidade", http.StatusInternalServerError)
		return
	}

	vm := pages.ToEntityDetailVM(detail)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := pages.EntityDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render entity detail template", "slug", slug, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}
