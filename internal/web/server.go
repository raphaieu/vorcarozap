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

	handlers := NewHandlers(db)

	// Endpoints de saúde
	r.Get("/health/live", handlers.HandleHealthLive)
	r.Get("/health/ready", handlers.HandleHealthReady)

	// Página pública inicial
	r.Get("/", handlers.HandleHome)

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
