package web

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
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
	if cfg.IsAdminEnabled() {
		_, _ = store.BootstrapAdminUser(context.Background(), db, cfg.AdminUser, cfg.AdminPasswordHash, "Administrador Principal")
	}

	r := chi.NewRouter()

	// Middlewares padrão
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(SecurityHeadersMiddleware)

	var authSvc *auth.Service
	if cfg.AdminMFAEncryptionKey != "" {
		var err error
		authSvc, err = auth.NewService(db, cfg.AdminSessionTTL, cfg.AdminLockoutDuration, cfg.AdminMaxLoginAttempts, cfg.AdminMFARequired, cfg.AdminMFAEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("web: falha ao inicializar serviço de autenticação: %w", err)
		}
	} else if cfg.IsAdminEnabled() {
		return nil, fmt.Errorf("web: ADMIN_MFA_ENCRYPTION_KEY é obrigatório quando a área administrativa está habilitada")
	}
	handlers := NewHandlers(db, cfg.PublicDataCutoff, authSvc)

	// Endpoints de saúde
	r.Get("/health/live", handlers.HandleHealthLive)
	r.Get("/health/ready", handlers.HandleHealthReady)

	// Página pública inicial e navegação pública (VZ-006, VZ-007, VZ-023)
	r.Get("/", handlers.HandleHome)
	r.Get("/pessoas", handlers.HandleEntities)
	r.Get("/pessoas/{slug}", handlers.HandleEntityDetail)
	r.Get("/documentos/{id}", handlers.HandleDocumentDetail)
	r.Get("/metodologia", handlers.HandleMethodology)

	// Canal público de contraditório e submissão de manifestações (VZ-025)
	r.Get("/manifestar", handlers.HandleManifestationForm)
	r.Post("/manifestar", handlers.HandleSubmitManifestation)

	// Exportação pública de dados XLSX (VZ-009)
	r.Get("/exportar/base.xlsx", handlers.HandleExportXLSX)
	r.Get("/exportar", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/exportar/base.xlsx", http.StatusTemporaryRedirect)
	})

	// Área administrativa protegida (VZ-014, VZ-016, VZ-023, VZ-025, VZ-026)
	if cfg.IsAdminEnabled() {
		authMiddleware := SessionAuthMiddleware(authSvc)
		csrfMiddleware := AdminCSRFMiddleware(cfg.AdminAllowedOrigin)
		mfaMiddleware := RequireMFAVerified()
		r.Mount("/admin", newAdminRouter(authMiddleware, csrfMiddleware, mfaMiddleware, handlers))
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
// de qualquer decisão de handler, 404 ou 405 (com exceção de /admin/login que é permitida para não-autenticados).
func newAdminRouter(
	authMiddleware func(http.Handler) http.Handler,
	csrfMiddleware func(http.Handler) http.Handler,
	mfaMiddleware func(http.Handler) http.Handler,
	handlers *Handlers,
) http.Handler {
	adminRouter := chi.NewRouter()
	adminRouter.Use(authMiddleware)

	// 1. Rotas de autenticação (não exigem sessão prévia)
	adminRouter.Get("/login", handlers.HandleAdminLogin)
	adminRouter.With(csrfMiddleware).Post("/login", handlers.HandleAdminLoginSubmit)

	// 2. Rotas de desafio/setup de MFA e logout (exigem sessão autenticada, mas permitem MFA pendente)
	adminRouter.Get("/mfa/challenge", handlers.HandleAdminMFAChallenge)
	adminRouter.With(csrfMiddleware).Post("/mfa/challenge", handlers.HandleAdminMFAChallengeSubmit)
	adminRouter.Get("/mfa/setup", handlers.HandleAdminMFASetup)
	adminRouter.With(csrfMiddleware).Post("/mfa/setup", handlers.HandleAdminMFASetupSubmit)
	adminRouter.With(csrfMiddleware).Post("/logout", handlers.HandleAdminLogout)

	// 3. Rotas administrativas protegidas (exigem sessão autenticada + MFA verificado)
	adminRouter.Group(func(protected chi.Router) {
		protected.Use(mfaMiddleware)

		// Dashboard
		protected.With(RequirePermission(domain.PermViewDashboard)).Get("/", handlers.HandleAdmin)

		// Candidatos e Monitoramento
		protected.With(RequirePermission(domain.PermViewCandidates)).Get("/candidatos", handlers.HandleAdminCandidates)
		protected.With(RequirePermission(domain.PermViewCandidates)).Get("/candidatos/{id}", handlers.HandleAdminCandidateDetail)

		// Evidências e Fontes
		protected.With(RequirePermission(domain.PermViewEvidences)).Get("/evidencias", handlers.HandleAdminEvidences)
		protected.With(RequirePermission(domain.PermViewSources)).Get("/fontes", handlers.HandleAdminSources)
		protected.With(RequirePermission(domain.PermViewSources)).Get("/fontes/{id}", handlers.HandleAdminSourceDetail)
		protected.With(RequirePermission(domain.PermViewEvidences)).Get("/evidencias/{id}", handlers.HandleAdminEvidenceSourceDetail)

		// Moderação de Evidências (Admin e Editor)
		protected.With(RequirePermission(domain.PermModerateEvidenceSources), csrfMiddleware).Post("/evidencias/{id}/moderate", handlers.HandleAdminModerateEvidenceSource)

		// Alegações e Moderação de Claims (PermViewClaims / PermModerateClaims)
		protected.With(RequirePermission(domain.PermViewClaims)).Get("/claims/{id}", handlers.HandleAdminClaimDetail)
		protected.With(RequirePermission(domain.PermModerateClaims), csrfMiddleware).Post("/claims/{id}/moderate", handlers.HandleAdminModerateClaim)

		// Manifestações de Defesa e Contraditório (PermViewManifestations / PermModerateManifestations)
		protected.With(RequirePermission(domain.PermViewManifestations)).Get("/manifestacoes", handlers.HandleAdminDefenseStatements)
		protected.With(RequirePermission(domain.PermViewManifestations)).Get("/manifestacoes/{id}", handlers.HandleAdminDefenseStatementDetail)
		protected.With(RequirePermission(domain.PermModerateManifestations), csrfMiddleware).Post("/manifestacoes/{id}/moderate", handlers.HandleAdminModerateDefenseStatement)

		// Perfil do Usuário Autenticado
		protected.Get("/perfil", handlers.HandleAdminProfile)
		protected.With(csrfMiddleware).Post("/perfil/senha", handlers.HandleAdminChangePassword)

		// Gestão de Usuários (Apenas Admin)
		protected.With(RequirePermission(domain.PermManageUsers)).Get("/usuarios", handlers.HandleAdminUsersList)
		protected.With(RequirePermission(domain.PermManageUsers)).Get("/usuarios/novo", handlers.HandleAdminUserNew)
		protected.With(RequirePermission(domain.PermManageUsers), csrfMiddleware).Post("/usuarios", handlers.HandleAdminUserCreate)
		protected.With(RequirePermission(domain.PermManageUsers)).Get("/usuarios/{id}", handlers.HandleAdminUserDetail)
		protected.With(RequirePermission(domain.PermManageUsers), csrfMiddleware).Post("/usuarios/{id}/role", handlers.HandleAdminUserUpdateRole)
		protected.With(RequirePermission(domain.PermManageUsers), csrfMiddleware).Post("/usuarios/{id}/status", handlers.HandleAdminUserUpdateStatus)
		protected.With(RequirePermission(domain.PermManageUsers), csrfMiddleware).Post("/usuarios/{id}/reset-password", handlers.HandleAdminUserResetPassword)
		protected.With(RequirePermission(domain.PermManageUsers), csrfMiddleware).Post("/usuarios/{id}/disable-mfa", handlers.HandleAdminUserDisableMFA)

		// Trilha de Auditoria (Admin e Auditor)
		protected.With(RequirePermission(domain.PermViewAuditLogs)).Get("/auditoria", handlers.HandleAdminAuditLogs)
	})

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
