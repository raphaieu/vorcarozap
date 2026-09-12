package web

import (
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/web/static"
)

// SecurityHeadersMiddleware adiciona cabeçalhos essenciais de proteção HTTP.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// NewServer inicializa e configura o servidor HTTP da aplicação.
func NewServer(cfg *config.Config, db *sql.DB) (*http.Server, error) {
	r := chi.NewRouter()

	// Middlewares padrão
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(SecurityHeadersMiddleware)

	handlers := NewHandlers(db, cfg.PublicDataCutoff)

	// Endpoints de saúde
	r.Get("/health/live", handlers.HandleHealthLive)
	r.Get("/health/ready", handlers.HandleHealthReady)

	// Página pública inicial e navegação pública (VZ-006, VZ-007)
	r.Get("/", handlers.HandleHome)
	r.Get("/pessoas", handlers.HandleEntities)
	r.Get("/pessoas/{slug}", handlers.HandleEntityDetail)
	r.Get("/metodologia", handlers.HandleMethodology)

	// Exportação pública de dados XLSX (VZ-009)
	r.Get("/exportar/base.xlsx", handlers.HandleExportXLSX)
	r.Get("/exportar", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/exportar/base.xlsx", http.StatusTemporaryRedirect)
	})

	// Área administrativa protegida (VZ-014)
	if cfg.IsAdminEnabled() {
		authMiddleware := BasicAuthMiddleware(cfg.AdminUser, cfg.AdminPasswordHash)
		r.Mount("/admin", newAdminRouter(authMiddleware, handlers))
	}

	// Arquivos estáticos embutidos (/static/*)
	staticSubFS, err := fs.Sub(static.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("web: falha ao montar subfilesystem estático: %w", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS))))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return srv, nil
}

// newAdminRouter cria o roteador dedicado à área administrativa.
// O middleware de autenticação é executado incondicionalmente no topo de toda a árvore /admin,
// garantindo que qualquer requisição (qualquer método HTTP ou subrota) seja autenticada antes
// de qualquer decisão de handler, 404 ou 405.
func newAdminRouter(authMiddleware func(http.Handler) http.Handler, handlers *Handlers) http.Handler {
	adminRouter := chi.NewRouter()
	adminRouter.Use(authMiddleware)
	adminRouter.Get("/", handlers.HandleAdmin)
	adminRouter.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Vary", "Authorization")
		http.NotFound(w, r)
	})
	adminRouter.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Vary", "Authorization")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
	return adminRouter
}
