package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/contradiction"
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
	contradiction    *contradiction.Service
	rateLimiter      *contradiction.RateLimiter
	authService      *auth.Service
}

func NewHandlers(db *sql.DB, cutoff string, authSvc *auth.Service) *Handlers {
	return &Handlers{
		db:               db,
		queries:          sqlc.New(db),
		publicDataCutoff: cutoff,
		moderation:       moderation.NewService(db),
		contradiction:    contradiction.NewService(db),
		rateLimiter:      contradiction.NewRateLimiter(5, 10*time.Minute),
		authService:      authSvc,
	}
}

// SetRateLimiter permite injetar um limitador customizado para testes.
func (h *Handlers) SetRateLimiter(limiter *contradiction.RateLimiter) {
	h.rateLimiter = limiter
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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

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

	component := pages.Entities(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render entities template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleEntityDetail renderiza os detalhes e alegações/fontes de uma entidade pública pelo slug.
func (h *Handlers) HandleEntityDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

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

	component := pages.EntityDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render entity detail template", "slug", slug, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleDocumentDetail renderiza a página pública SSR de um documento e sua sequência contextual.
func (h *Handlers) HandleDocumentDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	doc, err := store.GetPublicDocumentDetail(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get public document detail", "id", id, "error", err)
		http.Error(w, "Erro interno ao carregar dados do documento", http.StatusInternalServerError)
		return
	}

	vm := pages.ToDocumentDetailVM(doc)
	component := pages.DocumentDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render document detail template", "id", id, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleMethodology renderiza a página pública de metodologia e critérios editoriais.
func (h *Handlers) HandleMethodology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
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

// HandleAdminSourceDetail renderiza o detalhe administrativo protegido SSR de uma fonte e sua sequência completa de usos.
func (h *Handlers) HandleAdminSourceDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	res, err := store.GetAdminSourceDetail(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get admin source detail", "id", id, "error", err)
		http.Error(w, "Erro interno ao carregar dados da fonte", http.StatusInternalServerError)
		return
	}

	vm := pages.ToAdminSourceDetailVM(res)
	component := pages.AdminSourceDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin source detail template", "id", id, "error", err)
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

	if currentUser, ok := CurrentUserFromContext(r.Context()); ok && currentUser != nil {
		if !currentUser.Role.HasPermission(domain.PermModerateClaims) {
			http.Error(w, "Forbidden: permissão insuficiente para moderar alegações", http.StatusForbidden)
			return
		}
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

// HandleAdminEvidenceSourceDetail renderiza a inspeção detalhada de um uso de evidência, o claim associado, fonte e histórico de moderação.
func (h *Handlers) HandleAdminEvidenceSourceDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	detail, err := store.GetAdminEvidenceSourceDetail(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get admin evidence source detail", "id", id, "error", err)
		http.Error(w, "Erro interno ao carregar detalhes do uso de evidência", http.StatusInternalServerError)
		return
	}

	flashMsg := r.URL.Query().Get("msg")
	flashErr := r.URL.Query().Get("err")

	vm := pages.ToAdminEvidenceSourceDetailVM(detail, flashMsg, flashErr)
	component := pages.AdminEvidenceDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin evidence source detail template", "id", id, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminModerateEvidenceSource processa a ação humana de moderação sobre um uso de evidência via POST.
// Segue o padrão PRG (303 See Other), validação estrita de ator, concorrência atômica, quarentena se perder último suporte e proteção CSRF.
func (h *Handlers) HandleAdminModerateEvidenceSource(w http.ResponseWriter, r *http.Request) {
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

	if currentUser, ok := CurrentUserFromContext(r.Context()); ok && currentUser != nil {
		if !currentUser.Role.HasPermission(domain.PermModerateEvidenceSources) {
			http.Error(w, "Forbidden: permissão insuficiente para moderar usos de evidência", http.StatusForbidden)
			return
		}
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

	res, err := h.moderation.ModerateEvidenceSource(r.Context(), moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  id,
		ExpectedUpdatedAt: expectedUpdatedAt,
		Action:            action,
		Reason:            reason,
		Actor:             actor,
	})
	if err != nil {
		if errors.Is(err, moderation.ErrEvidenceSourceNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, moderation.ErrConflict) {
			http.Error(w, "Conflito de concorrência: o estado do uso de evidência foi modificado por outra operação.", http.StatusConflict)
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
		if errors.Is(err, moderation.ErrInvalidReason) || errors.Is(err, moderation.ErrInvalidAction) || errors.Is(err, moderation.ErrInvalidActor) {
			http.Error(w, fmt.Sprintf("Dados de moderação inválidos: %v", err), http.StatusBadRequest)
			return
		}

		slog.Error("failed to moderate evidence source", "evidence_source_id", id, "action", actionStr, "error", err)
		http.Error(w, "Erro interno ao processar moderação", http.StatusInternalServerError)
		return
	}

	successMsg := fmt.Sprintf("Uso de evidência moderado com sucesso: ação '%s' aplicada (novo status: %s).", actionStr, res.NewStatus)
	if res.ClaimQuarantined {
		successMsg += " A alegação associada perdeu o último suporte ativo e foi movida para QUARENTENA."
	}
	redirectURL := fmt.Sprintf("/admin/evidencias/%s?msg=%s", id, url.QueryEscape(successMsg))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// getClientIP extrai o IP de origem da conexão a partir de RemoteAddr.
// Não confia em cabeçalhos de proxy controláveis pelo cliente (como X-Forwarded-For ou X-Real-IP).
func getClientIP(r *http.Request) string {
	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if remoteAddr == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

// HandleManifestationForm renderiza a página pública SSR para submissão de manifestação/contraditório.
func (h *Handlers) HandleManifestationForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	claimID := strings.TrimSpace(r.URL.Query().Get("claim_id"))
	isSuccess := r.URL.Query().Get("success") == "1"

	var claimProposition, entityName, entitySlug string
	var flashErr string

	if claimID != "" {
		q := sqlc.New(h.db)
		pubClaim, err := q.GetPublicClaimForManifestation(r.Context(), claimID)
		if err == nil {
			claimProposition = pubClaim.Proposition
			entityName = pubClaim.EntityName
			entitySlug = pubClaim.EntitySlug
		} else {
			// Se o claim não está na fronteira pública, não vazar proposição nem entidade
			if !isSuccess {
				flashErr = "Alegação vinculada não encontrada ou indisponível para manifestação pública."
			}
		}
	}

	statementTypes := []pages.StatementTypeOptionVM{
		{Value: "rebuttal", Label: "Contestação / Defesa Formal", Description: "Contesta o vínculo ou traz a versão oficial da defesa."},
		{Value: "correction", Label: "Correção / Retificação Factual", Description: "Corrige dado específico (data, cargo, número, valor, etc.)."},
		{Value: "clarification", Label: "Esclarecimento Institucional", Description: "Explica o contexto sem necessariamente contestar o fato."},
		{Value: "additional_context", Label: "Contexto Adicional / Complemento", Description: "Acrescenta informações documentais relevantes."},
	}

	flashMsg := ""
	if isSuccess {
		flashMsg = "Sua manifestação foi registrada com sucesso e encaminhada para a quarentena editorial."
	}

	vm := pages.ManifestationFormVM{
		ClaimID:          claimID,
		ClaimProposition: claimProposition,
		EntityName:       entityName,
		EntitySlug:       entitySlug,
		StatementTypes:   statementTypes,
		FlashMessage:     flashMsg,
		FlashError:       flashErr,
		IsSuccess:        isSuccess,
	}

	component := pages.ManifestationForm(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render manifestation form template", "error", err)
		http.Error(w, "Erro interno ao renderizar formulário", http.StatusInternalServerError)
	}
}

// HandleSubmitManifestation processa o envio público de uma nova manifestação via POST com rate limit e quarentena compulsória.
func (h *Handlers) HandleSubmitManifestation(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	if !h.rateLimiter.Allow(clientIP) {
		http.Error(w, "Limite de submissões excedido. Por favor, aguarde alguns minutos antes de enviar nova manifestação.", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if err := r.ParseForm(); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Corpo da requisição excede o limite máximo permitido de 64 KiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Requisição inválida: formulário corrompido", http.StatusBadRequest)
		return
	}

	claimID := strings.TrimSpace(r.FormValue("claim_id"))
	statementType := domain.StatementType(strings.TrimSpace(r.FormValue("statement_type")))
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	contactInfo := strings.TrimSpace(r.FormValue("contact_info"))

	_, err := h.contradiction.SubmitStatement(r.Context(), contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: statementType,
		Title:         title,
		Content:       content,
		SourceURL:     sourceURL,
		ContactInfo:   contactInfo,
	})
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)

		statementTypes := []pages.StatementTypeOptionVM{
			{Value: "rebuttal", Label: "Contestação / Defesa Formal", Description: "Contesta o vínculo ou traz a versão oficial da defesa."},
			{Value: "correction", Label: "Correção / Retificação Factual", Description: "Corrige dado específico (data, cargo, número, valor, etc.)."},
			{Value: "clarification", Label: "Esclarecimento Institucional", Description: "Explica o contexto sem necessariamente contestar o fato."},
			{Value: "additional_context", Label: "Contexto Adicional / Complemento", Description: "Acrescenta informações documentais relevantes."},
		}

		errMsg := err.Error()
		if errors.Is(err, contradiction.ErrClaimNotFound) {
			errMsg = "A alegação vinculada informada não foi encontrada ou não está disponível para manifestação pública."
		}

		vm := pages.ManifestationFormVM{
			ClaimID:        claimID,
			StatementTypes: statementTypes,
			FlashError:     errMsg,
			IsSuccess:      false,
		}
		_ = pages.ManifestationForm(vm).Render(r.Context(), w)
		return
	}

	redirectURL := fmt.Sprintf("/manifestar?claim_id=%s&success=1", url.QueryEscape(claimID))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// HandleAdminDefenseStatements renderiza a listagem administrativa de manifestações recebidas.
func (h *Handlers) HandleAdminDefenseStatements(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))

	filter := store.AdminDefenseStatementFilter{
		Status:   q.Get("status"),
		Type:     q.Get("type"),
		ClaimID:  q.Get("claim_id"),
		Period:   q.Get("period"),
		Page:     page,
		PageSize: 20,
	}

	if filter.Period != "" {
		filter.PeriodSince = store.ResolvePeriod(filter.Period, time.Now().UTC())
	}

	res, err := store.ListAdminDefenseStatements(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list admin defense statements", "error", err)
		http.Error(w, "Erro ao consultar manifestações", http.StatusInternalServerError)
		return
	}

	flashMsg := q.Get("msg")
	flashErr := q.Get("err")

	vm := pages.ToAdminDefenseStatementsListVM(res, filter, flashMsg, flashErr)
	component := pages.AdminManifestations(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin manifestations template", "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminDefenseStatementDetail renderiza a inspeção detalhada de uma manifestação e dados de deliberação.
func (h *Handlers) HandleAdminDefenseStatementDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	detail, err := store.GetAdminDefenseStatementDetail(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get admin defense statement detail", "id", id, "error", err)
		http.Error(w, "Erro interno ao carregar detalhes da manifestação", http.StatusInternalServerError)
		return
	}

	flashMsg := r.URL.Query().Get("msg")
	flashErr := r.URL.Query().Get("err")

	vm := pages.ToAdminDefenseStatementDetailVM(detail, flashMsg, flashErr)

	if currentUser, ok := CurrentUserFromContext(r.Context()); ok && currentUser != nil {
		if !currentUser.Role.HasPermission(domain.PermViewManifestationContact) {
			vm.ContactInfo = "[Acesso Restrito: Permissão Insuficiente]"
		}
	}

	component := pages.AdminManifestationDetail(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin defense statement detail template", "id", id, "error", err)
		http.Error(w, "Erro interno ao renderizar página", http.StatusInternalServerError)
	}
}

// HandleAdminModerateDefenseStatement processa a deliberação de moderação humana sobre uma manifestação via POST com OCC.
func (h *Handlers) HandleAdminModerateDefenseStatement(w http.ResponseWriter, r *http.Request) {
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

	if currentUser, ok := CurrentUserFromContext(r.Context()); ok && currentUser != nil {
		if !currentUser.Role.HasPermission(domain.PermModerateManifestations) {
			http.Error(w, "Forbidden: permissão insuficiente para moderar manifestações", http.StatusForbidden)
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
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

	action := domain.StatementModerationAction(actionStr)
	if !action.IsValid() {
		http.Error(w, fmt.Sprintf("Ação de moderação inválida: %q", actionStr), http.StatusBadRequest)
		return
	}

	res, err := h.contradiction.ModerateStatement(r.Context(), contradiction.ModerateStatementParams{
		StatementID:       id,
		ExpectedUpdatedAt: expectedUpdatedAt,
		Action:            action,
		Reason:            reason,
		Actor:             actor,
	})
	if err != nil {
		if errors.Is(err, contradiction.ErrStatementNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, contradiction.ErrConflict) {
			http.Error(w, "Conflito de concorrência: o estado da manifestação foi modificado por outra operação.", http.StatusConflict)
			return
		}
		if errors.Is(err, contradiction.ErrMissingExpectedVersion) {
			http.Error(w, "Versão esperada (expected_updated_at) é obrigatória para moderação.", http.StatusBadRequest)
			return
		}
		if errors.Is(err, contradiction.ErrInvalidTransition) {
			http.Error(w, fmt.Sprintf("Transição de estado inválida: %v", err), http.StatusBadRequest)
			return
		}
		if errors.Is(err, contradiction.ErrInvalidReason) || errors.Is(err, contradiction.ErrInvalidAction) || errors.Is(err, contradiction.ErrInvalidActor) {
			http.Error(w, fmt.Sprintf("Dados de moderação inválidos: %v", err), http.StatusBadRequest)
			return
		}

		slog.Error("failed to moderate defense statement", "statement_id", id, "action", actionStr, "error", err)
		http.Error(w, "Erro interno ao processar moderação", http.StatusInternalServerError)
		return
	}

	successMsg := fmt.Sprintf("Manifestação moderada com sucesso: ação '%s' aplicada (novo status: %s).", actionStr, res.NewStatus)
	redirectURL := fmt.Sprintf("/admin/manifestacoes/%s?msg=%s", id, url.QueryEscape(successMsg))
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// HandleAdminLogin exibe o formulário SSR de login administrativo.
func (h *Handlers) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	// Se o usuário já tiver uma sessão válida e verificada, redireciona para o dashboard
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		if _, session, err := h.authService.ValidateSession(r.Context(), cookie.Value); err == nil && session.MFAVerified {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
	}

	returnTo := r.URL.Query().Get("return_to")
	flashErr := r.URL.Query().Get("err")

	vm := pages.AdminLoginVM{
		ReturnTo:   returnTo,
		FlashError: flashErr,
	}

	component := pages.AdminLogin(vm)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin login template", "error", err)
		http.Error(w, "Erro ao carregar página de login", http.StatusInternalServerError)
	}
}

// HandleAdminLoginSubmit processa a autenticação de login via POST com senha.
func (h *Handlers) HandleAdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	returnTo := r.FormValue("return_to")
	clientIP := getClientIP(r)

	user, session, needsMFA, err := h.authService.AuthenticatePassword(r.Context(), username, password, clientIP, r.UserAgent())
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		errMsg := "Credenciais inválidas ou acesso não autorizado."
		if errors.Is(err, auth.ErrAccountLocked) {
			errMsg = "Conta temporariamente bloqueada por excesso de tentativas incorretas. Tente novamente mais tarde."
		} else if errors.Is(err, auth.ErrAccountDisabled) {
			errMsg = "Conta de usuário desativada. Contate o administrador."
		}
		vm := pages.AdminLoginVM{
			ReturnTo:   returnTo,
			FlashError: errMsg,
		}
		_ = pages.AdminLogin(vm).Render(r.Context(), w)
		return
	}

	if needsMFA {
		// Define cookie temporário para o passo do MFA
		expiresAt := time.Now().UTC().Add(15 * time.Minute)
		SetSessionCookie(w, r, session.ID, expiresAt)

		if user.MFAEnabled {
			challengeURL := "/admin/mfa/challenge"
			if returnTo != "" {
				challengeURL += "?return_to=" + url.QueryEscape(returnTo)
			}
			http.Redirect(w, r, challengeURL, http.StatusSeeOther)
			return
		}

		// Se a conta exige MFA mas ainda não configurou, direciona para setup
		http.Redirect(w, r, "/admin/mfa/setup", http.StatusSeeOther)
		return
	}

	// Login direto sem MFA (se permitido por papel)
	expiresAt, _ := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	SetSessionCookie(w, r, session.ID, expiresAt)

	dest := "/admin"
	if returnTo != "" && strings.HasPrefix(returnTo, "/admin") {
		dest = returnTo
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// HandleAdminMFAChallenge exibe a tela para entrada do código TOTP.
func (h *Handlers) HandleAdminMFAChallenge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	returnTo := r.URL.Query().Get("return_to")
	flashErr := r.URL.Query().Get("err")

	vm := pages.AdminMFAChallengeVM{
		ReturnTo:   returnTo,
		FlashError: flashErr,
	}

	if err := pages.AdminMFAChallenge(vm).Render(r.Context(), w); err != nil {
		slog.Error("failed to render mfa challenge template", "error", err)
		http.Error(w, "Erro ao carregar página de verificação MFA", http.StatusInternalServerError)
	}
}

// HandleAdminMFAChallengeSubmit valida o código TOTP no fluxo de login.
func (h *Handlers) HandleAdminMFAChallengeSubmit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	returnTo := r.FormValue("return_to")
	clientIP := getClientIP(r)

	_, newSession, err := h.authService.VerifyMFALogin(r.Context(), cookie.Value, code, clientIP)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		errMsg := "Código de autenticação inválido ou expirado. Verifique o relógio do seu aplicativo e tente novamente."
		if errors.Is(err, auth.ErrAccountLocked) {
			ClearSessionCookie(w, r)
			errMsg = "Conta temporariamente bloqueada por excesso de tentativas de autenticação inválidas. Tente novamente mais tarde."
			vm := pages.AdminLoginVM{
				ReturnTo:   returnTo,
				FlashError: errMsg,
			}
			_ = pages.AdminLogin(vm).Render(r.Context(), w)
			return
		}
		if errors.Is(err, auth.ErrAccountDisabled) {
			ClearSessionCookie(w, r)
			errMsg = "Conta de usuário desativada. Contate o administrador."
			vm := pages.AdminLoginVM{
				ReturnTo:   returnTo,
				FlashError: errMsg,
			}
			_ = pages.AdminLogin(vm).Render(r.Context(), w)
			return
		}
		if errors.Is(err, auth.ErrSessionNotFound) {
			ClearSessionCookie(w, r)
			http.Redirect(w, r, "/admin/login?err="+url.QueryEscape("Sessão expirada. Faça login novamente."), http.StatusSeeOther)
			return
		}
		vm := pages.AdminMFAChallengeVM{
			ReturnTo:   returnTo,
			FlashError: errMsg,
		}
		_ = pages.AdminMFAChallenge(vm).Render(r.Context(), w)
		return
	}

	expiresAt, _ := time.Parse(time.RFC3339Nano, newSession.ExpiresAt)
	SetSessionCookie(w, r, newSession.ID, expiresAt)

	dest := "/admin"
	if returnTo != "" && strings.HasPrefix(returnTo, "/admin") {
		dest = returnTo
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// HandleAdminMFASetup exibe a tela de configuração de novo MFA TOTP.
func (h *Handlers) HandleAdminMFASetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	swu, err := store.GetAdminSessionWithUser(r.Context(), h.db, cookie.Value)
	if err != nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	if swu.User.MFAEnabled {
		http.Redirect(w, r, "/admin/perfil?msg="+url.QueryEscape("MFA já está configurado e ativo para esta conta."), http.StatusSeeOther)
		return
	}

	secret, uri, err := h.authService.GenerateMFASetup(r.Context(), swu.User.ID)
	if err != nil {
		slog.Error("failed to generate mfa setup", "error", err)
		if errors.Is(err, auth.ErrAccountLocked) {
			ClearSessionCookie(w, r)
			http.Redirect(w, r, "/admin/login?err="+url.QueryEscape("Conta temporariamente bloqueada por excesso de tentativas. Tente novamente mais tarde."), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/perfil?err="+url.QueryEscape("Não foi possível iniciar a configuração de MFA."), http.StatusSeeOther)
		return
	}

	flashErr := r.URL.Query().Get("err")
	vm := pages.AdminMFASetupVM{
		Secret:     secret,
		URI:        uri,
		FlashError: flashErr,
	}

	if err := pages.AdminMFASetup(vm).Render(r.Context(), w); err != nil {
		slog.Error("failed to render mfa setup template", "error", err)
		http.Error(w, "Erro ao carregar página de configuração MFA", http.StatusInternalServerError)
	}
}

// HandleAdminMFASetupSubmit confirma a ativação de MFA validando o código inicial contra o segredo pendente server-side.
func (h *Handlers) HandleAdminMFASetupSubmit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	code := strings.TrimSpace(r.FormValue("code"))
	clientIP := getClientIP(r)

	_, newSession, err := h.authService.ConfirmMFASetup(r.Context(), cookie.Value, code, clientIP)
	if err != nil {
		if errors.Is(err, auth.ErrAccountLocked) {
			ClearSessionCookie(w, r)
			http.Redirect(w, r, "/admin/login?err="+url.QueryEscape("Conta temporariamente bloqueada por excesso de tentativas incorretas. Tente novamente mais tarde."), http.StatusSeeOther)
			return
		}
		if errors.Is(err, auth.ErrAccountDisabled) {
			ClearSessionCookie(w, r)
			http.Redirect(w, r, "/admin/login?err="+url.QueryEscape("Conta de usuário desativada. Contate o administrador."), http.StatusSeeOther)
			return
		}
		if errors.Is(err, auth.ErrSessionNotFound) {
			ClearSessionCookie(w, r)
			http.Redirect(w, r, "/admin/login?err="+url.QueryEscape("Sessão expirada. Faça login novamente."), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin/mfa/setup?err="+url.QueryEscape("Código incorreto ou expirado. Certifique-se de digitar o código de 6 dígitos gerado pelo seu app autenticador."), http.StatusSeeOther)
		return
	}

	expiresAt, _ := time.Parse(time.RFC3339Nano, newSession.ExpiresAt)
	SetSessionCookie(w, r, newSession.ID, expiresAt)
	http.Redirect(w, r, "/admin?msg="+url.QueryEscape("Autenticação em dois fatores (MFA) ativada com sucesso!"), http.StatusSeeOther)
}

// HandleAdminLogout encerra a sessão ativa do operador.
func (h *Handlers) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		_ = h.authService.RevokeSession(r.Context(), cookie.Value, getClientIP(r))
	}
	ClearSessionCookie(w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// HandleAdminProfile renderiza a página de perfil do usuário conectado.
func (h *Handlers) HandleAdminProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	user, ok := CurrentUserFromContext(r.Context())
	if !ok || user == nil {
		sendUnauthorized(w)
		return
	}

	flashMsg := r.URL.Query().Get("msg")
	flashErr := r.URL.Query().Get("err")

	vm := pages.AdminProfileVM{
		User:         pages.ToAdminUserItemVM(*user),
		FlashMessage: flashMsg,
		FlashError:   flashErr,
	}

	if err := pages.AdminProfile(vm).Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin profile template", "error", err)
		http.Error(w, "Erro ao carregar página de perfil", http.StatusInternalServerError)
	}
}

// HandleAdminChangePassword processa a alteração de senha pelo próprio usuário conectado.
func (h *Handlers) HandleAdminChangePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	user, ok := CurrentUserFromContext(r.Context())
	if !ok || user == nil {
		sendUnauthorized(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	clientIP := getClientIP(r)

	if err := h.authService.ChangeOwnPassword(r.Context(), user.ID, currentPassword, newPassword, clientIP); err != nil {
		errMsg := "Falha ao alterar senha: senha atual incorreta ou nova senha fora dos padrões."
		http.Redirect(w, r, "/admin/perfil?err="+url.QueryEscape(errMsg), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/perfil?msg="+url.QueryEscape("Sua senha foi alterada com sucesso!"), http.StatusSeeOther)
}

// HandleAdminUsersList renderiza a listagem de usuários administrativos.
func (h *Handlers) HandleAdminUsersList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	filter := store.AdminUserFilter{
		Role:     q.Get("role"),
		Status:   q.Get("status"),
		Search:   q.Get("q"),
		Page:     page,
		PageSize: pageSize,
	}

	res, err := store.ListAdminUsers(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list admin users", "error", err)
		http.Error(w, "Erro ao consultar usuários", http.StatusInternalServerError)
		return
	}

	var userVMs []pages.AdminUserItemVM
	for _, u := range res.Users {
		userVMs = append(userVMs, pages.ToAdminUserItemVM(u))
	}

	sanitized := store.SanitizeUserFilter(filter)
	currentUser, _ := CurrentUserFromContext(r.Context())
	canCreate := currentUser != nil && currentUser.Role.HasPermission(domain.PermManageUsers)

	vm := pages.AdminUserListVM{
		Users: userVMs,
		Filter: pages.AdminUserFilterVM{
			Role:       sanitized.Role,
			Status:     sanitized.Status,
			Search:     sanitized.Search,
			Page:       res.Page,
			PageSize:   res.PageSize,
			TotalPages: res.TotalPages,
			TotalCount: res.TotalCount,
		},
		CanCreateUser: canCreate,
		FlashMessage:  q.Get("msg"),
		FlashError:    q.Get("err"),
	}

	if err := pages.AdminUsers(vm).Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin users template", "error", err)
		http.Error(w, "Erro ao renderizar lista de usuários", http.StatusInternalServerError)
	}
}

// HandleAdminUserNew exibe o formulário para criação de novo usuário.
func (h *Handlers) HandleAdminUserNew(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	flashErr := r.URL.Query().Get("err")
	if err := pages.AdminUserNew(flashErr).Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin user new template", "error", err)
		http.Error(w, "Erro ao renderizar formulário de cadastro", http.StatusInternalServerError)
	}
}

// HandleAdminUserCreate processa a criação de um novo usuário via POST.
func (h *Handlers) HandleAdminUserCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	actor, ok := CurrentUserFromContext(r.Context())
	if !ok || actor == nil {
		sendUnauthorized(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	displayName := r.FormValue("display_name")
	roleStr := r.FormValue("role")
	password := r.FormValue("password")
	clientIP := getClientIP(r)

	role, err := domain.ValidateUserRole(roleStr)
	if err != nil {
		http.Redirect(w, r, "/admin/usuarios/novo?err="+url.QueryEscape("Papel de usuário inválido."), http.StatusSeeOther)
		return
	}

	created, err := h.authService.CreateUser(r.Context(), *actor, auth.CreateUserParams{
		Username:    username,
		DisplayName: displayName,
		Password:    password,
		Role:        role,
		Status:      domain.UserStatusActive,
	}, clientIP)
	if err != nil {
		http.Redirect(w, r, "/admin/usuarios/novo?err="+url.QueryEscape("Falha ao criar usuário: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/usuarios/"+created.ID+"?msg="+url.QueryEscape("Usuário cadastrado com sucesso!"), http.StatusSeeOther)
}

// HandleAdminUserDetail exibe os detalhes e opções de gestão de um usuário.
func (h *Handlers) HandleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}

	uWithCreds, err := store.GetUserByID(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		slog.Error("failed to get admin user detail", "id", id, "error", err)
		http.Error(w, "Erro ao carregar dados do usuário", http.StatusInternalServerError)
		return
	}

	actor, _ := CurrentUserFromContext(r.Context())
	canManage := actor != nil && actor.Role.HasPermission(domain.PermManageUsers)

	vm := pages.AdminUserDetailVM{
		User:             pages.ToAdminUserItemVM(uWithCreds.User),
		CanEditRole:      canManage,
		CanEditStatus:    canManage && actor.ID != uWithCreds.User.ID,
		CanResetPassword: canManage,
		CanDisableMFA:    canManage && uWithCreds.User.MFAEnabled,
		FlashMessage:     r.URL.Query().Get("msg"),
		FlashError:       r.URL.Query().Get("err"),
	}

	if err := pages.AdminUserDetail(vm).Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin user detail template", "error", err)
		http.Error(w, "Erro ao renderizar detalhes do usuário", http.StatusInternalServerError)
	}
}

// HandleAdminUserUpdateRole processa a alteração de papel de um usuário via POST com OCC.
func (h *Handlers) HandleAdminUserUpdateRole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	actor, ok := CurrentUserFromContext(r.Context())
	if !ok || actor == nil {
		sendUnauthorized(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	expectedUpdatedAt := r.FormValue("expected_updated_at")
	newRole, err := domain.ValidateUserRole(r.FormValue("role"))
	if err != nil {
		http.Error(w, "Papel de usuário inválido", http.StatusBadRequest)
		return
	}

	clientIP := getClientIP(r)
	err = h.authService.UpdateUserRole(r.Context(), *actor, id, expectedUpdatedAt, newRole, clientIP)
	if err != nil {
		if errors.Is(err, auth.ErrConflict) {
			http.Error(w, "Conflito de concorrência: o usuário foi modificado por outra operação.", http.StatusConflict)
			return
		}
		if errors.Is(err, auth.ErrForbidden) {
			http.Error(w, "Forbidden: permissão insuficiente para alterar papéis", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/admin/usuarios/"+id+"?err="+url.QueryEscape("Falha ao atualizar papel: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/usuarios/"+id+"?msg="+url.QueryEscape("Papel do usuário atualizado com sucesso!"), http.StatusSeeOther)
}

// HandleAdminUserUpdateStatus processa a alteração de status de um usuário via POST com OCC.
func (h *Handlers) HandleAdminUserUpdateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	actor, ok := CurrentUserFromContext(r.Context())
	if !ok || actor == nil {
		sendUnauthorized(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	expectedUpdatedAt := r.FormValue("expected_updated_at")
	newStatus, err := domain.ValidateUserStatus(r.FormValue("status"))
	if err != nil {
		http.Error(w, "Status de usuário inválido", http.StatusBadRequest)
		return
	}

	clientIP := getClientIP(r)
	err = h.authService.UpdateUserStatus(r.Context(), *actor, id, expectedUpdatedAt, newStatus, clientIP)
	if err != nil {
		if errors.Is(err, auth.ErrConflict) {
			http.Error(w, "Conflito de concorrência: o usuário foi modificado por outra operação.", http.StatusConflict)
			return
		}
		if errors.Is(err, auth.ErrForbidden) {
			http.Error(w, "Forbidden: permissão insuficiente para alterar status", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/admin/usuarios/"+id+"?err="+url.QueryEscape("Falha ao atualizar status: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/usuarios/"+id+"?msg="+url.QueryEscape("Status do usuário atualizado com sucesso!"), http.StatusSeeOther)
}

// HandleAdminUserResetPassword processa a redefinição de senha de um usuário via POST.
func (h *Handlers) HandleAdminUserResetPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	actor, ok := CurrentUserFromContext(r.Context())
	if !ok || actor == nil {
		sendUnauthorized(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Requisição inválida", http.StatusBadRequest)
		return
	}

	newPassword := r.FormValue("new_password")
	clientIP := getClientIP(r)

	err := h.authService.ResetUserPassword(r.Context(), *actor, id, newPassword, clientIP)
	if err != nil {
		if errors.Is(err, auth.ErrForbidden) {
			http.Error(w, "Forbidden: permissão insuficiente para redefinir senhas", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/admin/usuarios/"+id+"?err="+url.QueryEscape("Falha ao redefinir senha: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/usuarios/"+id+"?msg="+url.QueryEscape("Senha redefinida com sucesso!"), http.StatusSeeOther)
}

// HandleAdminUserDisableMFA desativa o MFA de um usuário por ação administrativa.
func (h *Handlers) HandleAdminUserDisableMFA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	actor, ok := CurrentUserFromContext(r.Context())
	if !ok || actor == nil {
		sendUnauthorized(w)
		return
	}

	clientIP := getClientIP(r)
	err := h.authService.DisableMFA(r.Context(), *actor, id, clientIP)
	if err != nil {
		if errors.Is(err, auth.ErrForbidden) {
			http.Error(w, "Forbidden: permissão insuficiente para desativar MFA", http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/admin/usuarios/"+id+"?err="+url.QueryEscape("Falha ao desativar MFA: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/usuarios/"+id+"?msg="+url.QueryEscape("MFA desativado com sucesso! As sessões do usuário foram revogadas."), http.StatusSeeOther)
}

// HandleAdminAuditLogs renderiza a listagem paginada da trilha de auditoria administrativa.
func (h *Handlers) HandleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))

	filter := store.AdminAuditFilter{
		Action:   q.Get("action"),
		Actor:    q.Get("actor"),
		Period:   q.Get("period"),
		Search:   q.Get("q"),
		Page:     page,
		PageSize: pageSize,
	}

	res, err := store.ListAdminAuditLogs(r.Context(), h.db, filter)
	if err != nil {
		slog.Error("failed to list admin audit logs", "error", err)
		http.Error(w, "Erro ao consultar trilha de auditoria", http.StatusInternalServerError)
		return
	}

	var logVMs []pages.AdminAuditItemVM
	for _, l := range res.Logs {
		logVMs = append(logVMs, pages.ToAdminAuditItemVM(l))
	}

	sanitized := store.SanitizeAuditFilter(filter)
	vm := pages.AdminAuditListVM{
		Logs: logVMs,
		Filter: pages.AdminAuditFilterVM{
			Action:     sanitized.Action,
			Actor:      sanitized.Actor,
			Period:     sanitized.Period,
			Search:     sanitized.Search,
			Page:       res.Page,
			PageSize:   res.PageSize,
			TotalPages: res.TotalPages,
			TotalCount: res.TotalCount,
		},
	}

	if err := pages.AdminAudit(vm).Render(r.Context(), w); err != nil {
		slog.Error("failed to render admin audit template", "error", err)
		http.Error(w, "Erro ao renderizar trilha de auditoria", http.StatusInternalServerError)
	}
}
