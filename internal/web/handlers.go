package web

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

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
