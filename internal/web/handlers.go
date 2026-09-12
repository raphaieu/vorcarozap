package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/raphaieu/vorcarozap/internal/metrics"
	"github.com/raphaieu/vorcarozap/internal/moderation"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
	"github.com/raphaieu/vorcarozap/web/pages"
)

type Handlers struct {
	db               *sql.DB
	queries          *sqlc.Queries
	publicDataCutoff string
	moderation       *moderation.Service
}

func NewHandlers(db *sql.DB, cutoff string) *Handlers {
	return &Handlers{
		db:               db,
		queries:          sqlc.New(db),
		publicDataCutoff: cutoff,
		moderation:       moderation.NewService(db),
	}
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

// HandleHome renderiza a página pública inicial SSR via templ com as métricas calculadas.
func (h *Handlers) HandleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	pubMetrics, err := metrics.GetPublicMetrics(r.Context(), h.queries, h.publicDataCutoff, 5)
	if err != nil {
		slog.Error("failed to get public metrics for home", "error", err)
		// Fallback gracioso com valores zerados
		pubMetrics = &metrics.PublicMetrics{
			CutoffDate: h.publicDataCutoff,
		}
	}

	vm := pages.ToHomeVM(pubMetrics)
	component := pages.Home(vm)
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
		Search:    q.Get("q"),
		Category:  q.Get("category"),
		Grade:     q.Get("grade"),
		Relevance: rel,
		Period:    q.Get("period"),
		OrderBy:   q.Get("sort"),
		OrderDir:  q.Get("dir"),
		Page:      page,
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
			Period:      sanitized.Period,
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

// HandleMethodology renderiza a página pública de metodologia e critérios editoriais.
func (h *Handlers) HandleMethodology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := pages.Methodology()
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render methodology template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleExportXLSX gera dinamicamente e serve o arquivo XLSX derivado exclusivamente da base pública atual.
func (h *Handlers) HandleExportXLSX(w http.ResponseWriter, r *http.Request) {
	exp := exporter.New(h.db, h.publicDataCutoff)

	var buf bytes.Buffer
	if err := exp.WriteTo(r.Context(), &buf); err != nil {
		slog.Error("failed to generate public xlsx export", "error", err)
		http.Error(w, "Erro interno ao gerar planilha de exportação", http.StatusInternalServerError)
		return
	}

	filename := "vorcarozap-dados-publicos-" + time.Now().UTC().Format("2006-01-02") + ".xlsx"

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// HandleAdmin renderiza o dashboard administrativo protegido SSR via templ.
func (h *Handlers) HandleAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	counts, err := store.GetAdminOverviewCounts(r.Context(), h.db)
	if err != nil {
		slog.Error("failed to get admin overview counts", "error", err)
		http.Error(w, "Erro interno ao carregar estatísticas do painel", http.StatusInternalServerError)
		return
	}

	vm := pages.ToAdminDashboardVM(counts)
	component := pages.Admin(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin dashboard template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminCandidates renderiza a listagem paginada e filtrável de candidatos de monitoramento.
func (h *Handlers) HandleAdminCandidates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	filter := store.AdminCandidateFilter{
		Status:   q.Get("status"),
		Grade:    q.Get("grade"),
		RunID:    q.Get("run_id"),
		Period:   q.Get("period"),
		Search:   q.Get("q"),
		Page:     page,
		PageSize: pageSize,
	}

	res, err := store.ListAdminCandidates(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list admin candidates", "error", err)
		http.Error(w, "Erro interno ao consultar candidatos de monitoramento", http.StatusInternalServerError)
		return
	}

	var candidateVMs []pages.AdminCandidateItemVM
	for _, c := range res.Candidates {
		candidateVMs = append(candidateVMs, pages.ToAdminCandidateItemVM(c))
	}

	sanitized := store.SanitizeAdminCandidateFilter(filter)
	vm := pages.AdminCandidateListVM{
		Candidates: candidateVMs,
		Filter: pages.AdminCandidateFilterVM{
			Status:      sanitized.Status,
			Grade:       sanitized.Grade,
			RunID:       sanitized.RunID,
			Period:      sanitized.Period,
			PeriodSince: sanitized.PeriodSince,
			Search:      sanitized.Search,
			Page:        res.Page,
			PageSize:    res.PageSize,
			TotalPages:  res.TotalPages,
			TotalCount:  res.TotalCount,
		},
	}

	component := pages.AdminCandidates(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin candidates template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminCandidateDetail renderiza a inspeção individual de um candidato e seu histórico de avaliações.
func (h *Handlers) HandleAdminCandidateDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	detail, err := store.GetAdminCandidateDetail(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get admin candidate detail", "id", id, "error", err)
		http.Error(w, "Erro interno ao carregar detalhes do candidato", http.StatusInternalServerError)
		return
	}

	vm := pages.ToAdminCandidateDetailVM(detail)
	component := pages.AdminCandidateDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin candidate detail template", "id", id, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminEvidences renderiza a listagem paginada e filtrável de usos de evidência (evidence_sources).
func (h *Handlers) HandleAdminEvidences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	filter := store.AdminEvidenceFilter{
		ClaimStatus:          q.Get("claim_status"),
		EvidenceSourceStatus: q.Get("es_status"),
		Origin:               q.Get("origin"),
		Grade:                q.Get("grade"),
		Role:                 q.Get("role"),
		Period:               q.Get("period"),
		Search:               q.Get("q"),
		Page:                 page,
		PageSize:             pageSize,
	}

	res, err := store.ListAdminEvidenceSources(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list admin evidence sources", "error", err)
		http.Error(w, "Erro interno ao consultar usos de evidência", http.StatusInternalServerError)
		return
	}

	var esVMs []pages.AdminEvidenceItemVM
	for _, es := range res.EvidenceSources {
		esVMs = append(esVMs, pages.ToAdminEvidenceItemVM(es))
	}

	sanitized := store.SanitizeAdminEvidenceFilter(filter)
	vm := pages.AdminEvidenceListVM{
		EvidenceSources: esVMs,
		Filter: pages.AdminEvidenceFilterVM{
			ClaimStatus:          sanitized.ClaimStatus,
			EvidenceSourceStatus: sanitized.EvidenceSourceStatus,
			Origin:               sanitized.Origin,
			Grade:                sanitized.Grade,
			Role:                 sanitized.Role,
			Period:               sanitized.Period,
			PeriodSince:          sanitized.PeriodSince,
			Search:               sanitized.Search,
			Page:                 res.Page,
			PageSize:             res.PageSize,
			TotalPages:           res.TotalPages,
			TotalCount:           res.TotalCount,
		},
	}

	component := pages.AdminEvidences(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin evidences template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminSources renderiza a listagem paginada e filtrável de fontes documentais (sources).
func (h *Handlers) HandleAdminSources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	filter := store.AdminSourceFilter{
		AccessStatus: q.Get("access_status"),
		SourceType:   q.Get("source_type"),
		Period:       q.Get("period"),
		Search:       q.Get("q"),
		Page:         page,
		PageSize:     pageSize,
	}

	res, err := store.ListAdminSources(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list admin sources", "error", err)
		http.Error(w, "Erro interno ao consultar fontes documentais", http.StatusInternalServerError)
		return
	}

	var sourceVMs []pages.AdminSourceItemVM
	for _, s := range res.Sources {
		sourceVMs = append(sourceVMs, pages.ToAdminSourceItemVM(s))
	}

	sanitized := store.SanitizeAdminSourceFilter(filter)
	vm := pages.AdminSourceListVM{
		Sources: sourceVMs,
		Filter: pages.AdminSourceFilterVM{
			AccessStatus: sanitized.AccessStatus,
			SourceType:   sanitized.SourceType,
			Period:       sanitized.Period,
			PeriodSince:  sanitized.PeriodSince,
			Search:       sanitized.Search,
			Page:         res.Page,
			PageSize:     res.PageSize,
			TotalPages:   res.TotalPages,
			TotalCount:   res.TotalCount,
		},
		SourceTypeOptions: pages.GetAdminSourceTypeOptionsVM(),
	}

	component := pages.AdminSources(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin sources template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminClaimDetail renderiza a inspeção detalhada de um claim, suas evidências e o histórico de moderação.
func (h *Handlers) HandleAdminClaimDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	detail, err := store.GetAdminClaimDetail(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get admin claim detail", "id", id, "error", err)
		http.Error(w, "Erro interno ao carregar detalhes da alegação", http.StatusInternalServerError)
		return
	}

	flashMsg := r.URL.Query().Get("msg")
	flashErr := r.URL.Query().Get("err")

	vm := pages.ToAdminClaimDetailVM(detail, flashMsg, flashErr)
	component := pages.AdminClaimDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin claim detail template", "id", id, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminModerateClaim processa a ação humana de moderação sobre uma alegação via POST.
// Segue o padrão PRG (303 See Other), validação estrita de ator, concorrência atômica e proteção CSRF.
func (h *Handlers) HandleAdminModerateClaim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	actor, ok := AuthenticatedUserFromContext(r.Context())
	if !ok || strings.TrimSpace(actor) == "" {
		sendUnauthorized(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Corpo da requisição excede o limite máximo permitido de 64 KiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Requisição inválida: formulário corrompido", http.StatusBadRequest)
		return
	}

	actionStr := strings.TrimSpace(r.FormValue("action"))
	reason := strings.TrimSpace(r.FormValue("reason"))
	expectedUpdatedAt := strings.TrimSpace(r.FormValue("expected_updated_at"))

	action := domain.ModerationAction(actionStr)
	if !action.IsValid() {
		http.Error(w, fmt.Sprintf("Ação de moderação inválida: %q", actionStr), http.StatusBadRequest)
		return
	}

	res, err := h.moderation.ModerateClaim(r.Context(), moderation.ModerateClaimParams{
		ClaimID:           id,
		ExpectedUpdatedAt: expectedUpdatedAt,
		Action:            action,
		Reason:            reason,
		Actor:             actor,
	})
	if err != nil {
		if errors.Is(err, moderation.ErrClaimNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, moderation.ErrConflict) {
			http.Error(w, "Conflito de concorrência: o estado da alegação foi modificado por outra operação.", http.StatusConflict)
			return
		}
		if errors.Is(err, moderation.ErrMissingExpectedVersion) {
			http.Error(w, "Versão esperada (expected_updated_at) é obrigatória para moderação.", http.StatusBadRequest)
			return
		}
		if errors.Is(err, moderation.ErrInvalidTransition) {
			http.Error(w, fmt.Sprintf("Transição de estado inválida: %v", err), http.StatusBadRequest)
			return
		}
		if errors.Is(err, moderation.ErrNoActiveSupport) {
			http.Error(w, "Aprovação negada: a alegação não possui nenhum uso de evidência ativo com papel 'supports'.", http.StatusBadRequest)
			return
		}
		if errors.Is(err, moderation.ErrInvalidReason) || errors.Is(err, moderation.ErrInvalidAction) || errors.Is(err, moderation.ErrInvalidActor) {
			http.Error(w, fmt.Sprintf("Dados de moderação inválidos: %v", err), http.StatusBadRequest)
			return
		}

		slog.Error("failed to moderate claim", "claim_id", id, "action", actionStr, "error", err)
		http.Error(w, "Erro interno ao processar moderação", http.StatusInternalServerError)
		return
	}

	successMsg := fmt.Sprintf("Alegação moderada com sucesso: ação '%s' aplicada (novo status: %s).", actionStr, res.NewStatus)
	redirectURL := fmt.Sprintf("/admin/claims/%s?msg=%s", id, url.QueryEscape(successMsg))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
