package web_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
	"github.com/raphaieu/vorcarozap/internal/web"
)

// setupRBACTestServer cria o ambiente completo para testes de RBAC e autenticação.
func setupRBACTestServer(t *testing.T) (*http.Server, *sql.DB, *auth.Service, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_rbac_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de dados de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	authSvc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 5, false, "12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("falha ao inicializar auth service: %v", err)
	}

	cfg := &config.Config{
		Port:                  8080,
		Env:                   "test",
		DBPath:                dbPath,
		PublicDataCutoff:      "2026-09-03",
		AdminAllowedOrigin:    "http://example.com",
		AdminUser:             "admin",
		AdminPasswordHash:     "$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		AdminMFAEncryptionKey: "12345678901234567890123456789012",
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao inicializar web.NewServer: %v", err)
	}

	// Popula entidades básicas para teste de moderação e visualização
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description, created_at, updated_at)
		VALUES ('case-rbac-1', 'Caso RBAC', 'caso-rbac', 'Descrição do caso de teste', ?, ?);

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-rbac-1', 'person', 'Investigado RBAC', 'investigado rbac', 'investigado-rbac', 'Alvo', 'Resumo', 4, 'Justificativa', ?, ?);

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits, created_at, updated_at)
		VALUES ('rel-rbac-1', 'ent-rbac-1', 'case-rbac-1', 'contato', 'Resumo da relação', '', ?, ?);

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons, created_at, updated_at)
		VALUES ('claim-rbac-1', 'rel-rbac-1', 'Proposição para teste de moderação RBAC', 'Fonte X', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]', ?, ?);

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES ('src-rbac-1', 'Portal Noticioso RBAC', 'Editora RBAC', 'https://exemplo.com/noticia-rbac', 'https://exemplo.com/noticia-rbac', 'news_report', 'reachable', ?, ?);

		INSERT INTO evidence (id, claim_id, summary, evidence_type, created_at, updated_at)
		VALUES ('ev-rbac-1', 'claim-rbac-1', 'Resumo de evidência para RBAC', 'document', ?, ?);

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status, created_at, updated_at)
		VALUES ('es-rbac-1', 'ev-rbac-1', 'src-rbac-1', 'supports', 'Trecho factual comprobatório', 'https://exemplo.com/noticia-rbac', 'active', ?, ?);

		INSERT INTO defense_statements (id, claim_id, statement_type, title, content, source_url, contact_info, status, created_at, updated_at)
		VALUES ('stmt-rbac-1', 'claim-rbac-1', 'clarification', 'Manifestação Formal de Defesa', 'Texto formal de esclarecimento para teste de RBAC', 'https://exemplo.com/defesa.pdf', 'advogado@defesa-exemplo.com', 'quarantined', ?, ?);
	`, now, now, now, now, now, now, now, now, now, now, now, now, now, now, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir dados factuais de teste: %v", err)
	}

	return srv, db, authSvc, "http://example.com"
}

// createTestUserAndSession cria um usuário com determinado papel e retorna um cookie de sessão válido e verificado.
func createTestUserAndSession(t *testing.T, ctx context.Context, authSvc *auth.Service, db *sql.DB, username string, role domain.UserRole, status domain.UserStatus) *http.Cookie {
	t.Helper()
	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}
	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    username,
		DisplayName: "Usuário " + string(role),
		Password:    "SenhaForte123!",
		Role:        role,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário %s (role %s): %v", username, role, err)
	}

	if status != domain.UserStatusActive {
		q := sqlc.New(db)
		_, err = q.UpdateAdminUserStatus(ctx, sqlc.UpdateAdminUserStatusParams{
			ID:                user.ID,
			NewStatus:         string(status),
			ExpectedUpdatedAt: user.UpdatedAt,
		})
		if err != nil {
			t.Fatalf("falha ao atualizar status do usuário %s para %s: %v", username, status, err)
		}
	}

	rawToken, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("falha ao gerar token de sessão: %v", err)
	}

	now := time.Now().UTC()
	session := domain.AdminSession{
		ID:             rawToken,
		UserID:         user.ID,
		MFAVerified:    true,
		IPAddress:      "127.0.0.1",
		UserAgent:      "TestAgent/1.0",
		ExpiresAt:      now.Add(8 * time.Hour).Format(time.RFC3339Nano),
		LastActivityAt: now.Format(time.RFC3339Nano),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}
	if err := store.CreateAdminSession(ctx, db, session); err != nil {
		t.Fatalf("falha ao persistir sessão de teste: %v", err)
	}

	return &http.Cookie{
		Name:  web.SessionCookieName,
		Value: rawToken,
		Path:  "/admin",
	}
}

func TestRBAC_Dashboard_AccessByAllRoles(t *testing.T) {
	srv, db, authSvc, _ := setupRBACTestServer(t)
	ctx := context.Background()

	roles := []domain.UserRole{domain.RoleAdmin, domain.RoleEditor, domain.RoleReviewer, domain.RoleAuditor}

	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "user-"+string(role), role, domain.UserStatusActive)

			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("role %s esperado 200 no dashboard, obtido %d", role, w.Code)
			}
			if !strings.Contains(w.Body.String(), "Painel Administrativo") {
				t.Errorf("role %s esperado conter 'Painel Administrativo' no HTML", role)
			}
		})
	}
}

func TestRBAC_CandidatesAndEvidence_AccessByAllRoles(t *testing.T) {
	srv, db, authSvc, _ := setupRBACTestServer(t)
	ctx := context.Background()

	endpoints := []string{
		"/admin/candidatos",
		"/admin/evidencias",
		"/admin/fontes",
		"/admin/fontes/src-rbac-1",
		"/admin/evidencias/es-rbac-1",
		"/admin/claims/claim-rbac-1",
		"/admin/manifestacoes",
	}

	roles := []domain.UserRole{domain.RoleAdmin, domain.RoleEditor, domain.RoleReviewer, domain.RoleAuditor}

	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "view-"+string(role), role, domain.UserStatusActive)

			for _, ep := range endpoints {
				req := httptest.NewRequest(http.MethodGet, ep, nil)
				req.AddCookie(cookie)
				w := httptest.NewRecorder()

				srv.Handler.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					t.Errorf("role %s acessando %s: esperado 200, obtido %d", role, ep, w.Code)
				}
			}
		})
	}
}

func TestRBAC_Moderation_OnlyAdminAndEditor(t *testing.T) {
	srv, db, authSvc, origin := setupRBACTestServer(t)
	ctx := context.Background()

	testCases := []struct {
		role        domain.UserRole
		expectAllow bool
	}{
		{role: domain.RoleAdmin, expectAllow: true},
		{role: domain.RoleEditor, expectAllow: true},
		{role: domain.RoleReviewer, expectAllow: false},
		{role: domain.RoleAuditor, expectAllow: false},
	}

	// 1. Moderação de Claim
	for _, tc := range testCases {
		t.Run("Claim_"+string(tc.role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "mod-clm-"+string(tc.role), tc.role, domain.UserStatusActive)

			form := url.Values{
				"action":              {"approve"},
				"reason":              {"Validação editorial via teste RBAC"},
				"expected_updated_at": {"2026-09-16T12:00:00Z"},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/claims/claim-rbac-1/moderate", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if tc.expectAllow {
				if w.Code != http.StatusSeeOther && w.Code != http.StatusConflict {
					t.Errorf("role %s esperado 303 (ou 409 OCC), obtido %d", tc.role, w.Code)
				}
			} else {
				if w.Code != http.StatusForbidden {
					t.Errorf("role %s esperado 403 Forbidden ao moderar claim, obtido %d", tc.role, w.Code)
				}
			}
		})
	}

	// 2. Moderação de Evidence Source
	for _, tc := range testCases {
		t.Run("Evidence_"+string(tc.role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "mod-es-"+string(tc.role), tc.role, domain.UserStatusActive)

			form := url.Values{
				"action":              {"reject"},
				"reason":              {"Rejeição de fonte para teste RBAC"},
				"expected_updated_at": {"2026-09-16T12:00:00Z"},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-rbac-1/moderate", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if tc.expectAllow {
				if w.Code != http.StatusSeeOther && w.Code != http.StatusConflict {
					t.Errorf("role %s esperado 303 (ou 409 OCC), obtido %d", tc.role, w.Code)
				}
			} else {
				if w.Code != http.StatusForbidden {
					t.Errorf("role %s esperado 403 Forbidden ao moderar evidência, obtido %d", tc.role, w.Code)
				}
			}
		})
	}

	// 3. Moderação de Manifestação de Defesa
	for _, tc := range testCases {
		t.Run("Defense_"+string(tc.role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "mod-stmt-"+string(tc.role), tc.role, domain.UserStatusActive)

			form := url.Values{
				"action":              {"accept"},
				"reason":              {"Aceitação de manifestação via teste RBAC"},
				"expected_updated_at": {"2026-09-16T12:00:00Z"},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/manifestacoes/stmt-rbac-1/moderate", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if tc.expectAllow {
				if w.Code != http.StatusSeeOther && w.Code != http.StatusConflict {
					t.Errorf("role %s esperado 303 (ou 409 OCC), obtido %d", tc.role, w.Code)
				}
			} else {
				if w.Code != http.StatusForbidden {
					t.Errorf("role %s esperado 403 Forbidden ao moderar manifestação, obtido %d", tc.role, w.Code)
				}
			}
		})
	}
}

func TestRBAC_ManifestationContactRedaction(t *testing.T) {
	srv, db, authSvc, _ := setupRBACTestServer(t)
	ctx := context.Background()

	// Admin, Editor e Reviewer devem ver o email de contato
	for _, role := range []domain.UserRole{domain.RoleAdmin, domain.RoleEditor, domain.RoleReviewer} {
		t.Run("Allowed_"+string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "contact-allow-"+string(role), role, domain.UserStatusActive)

			req := httptest.NewRequest(http.MethodGet, "/admin/manifestacoes/stmt-rbac-1", nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("role %s: esperado 200, obtido %d", role, w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, "advogado@defesa-exemplo.com") {
				t.Errorf("role %s deveria ver o email de contato da defesa", role)
			}
		})
	}

	// Auditor NÃO deve ver o email (deve estar redigido)
	for _, role := range []domain.UserRole{domain.RoleAuditor} {
		t.Run("Redacted_"+string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "contact-redact-"+string(role), role, domain.UserStatusActive)

			req := httptest.NewRequest(http.MethodGet, "/admin/manifestacoes/stmt-rbac-1", nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("role %s: esperado 200, obtido %d", role, w.Code)
			}
			body := w.Body.String()
			if strings.Contains(body, "advogado@defesa-exemplo.com") {
				t.Errorf("role %s NÃO DEVERIA ver o email de contato da defesa (vazamento de PII)", role)
			}
			if !strings.Contains(body, "[Acesso Restrito: Permissão Insuficiente]") {
				t.Errorf("role %s deveria ver aviso de acesso restrito de contato", role)
			}
		})
	}
}

func TestRBAC_UserManagement_OnlyAdmin(t *testing.T) {
	srv, db, authSvc, origin := setupRBACTestServer(t)
	ctx := context.Background()

	nonAdminRoles := []domain.UserRole{domain.RoleEditor, domain.RoleReviewer, domain.RoleAuditor}

	// 1. Listagem de Usuários (/admin/usuarios)
	for _, role := range nonAdminRoles {
		t.Run("List_"+string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "usr-list-"+string(role), role, domain.UserStatusActive)

			req := httptest.NewRequest(http.MethodGet, "/admin/usuarios", nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("role %s esperado 403 em /admin/usuarios, obtido %d", role, w.Code)
			}
		})
	}

	// 2. Criação de Novo Usuário (/admin/usuarios POST)
	for _, role := range nonAdminRoles {
		t.Run("Create_"+string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "usr-create-"+string(role), role, domain.UserStatusActive)

			form := url.Values{
				"username":     {"novo-tentativa-" + string(role)},
				"display_name": {"Tentativa Não Autorizada"},
				"password":     {"SenhaForte123!"},
				"role":         {"reviewer"},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/usuarios", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("role %s esperado 403 em POST /admin/usuarios, obtido %d", role, w.Code)
			}
		})
	}

	// 3. Admin legítimo tem acesso completo
	t.Run("AdminAccess", func(t *testing.T) {
		adminCookie := createTestUserAndSession(t, ctx, authSvc, db, "super-admin", domain.RoleAdmin, domain.UserStatusActive)

		// GET list
		req := httptest.NewRequest(http.MethodGet, "/admin/usuarios", nil)
		req.AddCookie(adminCookie)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("admin: esperado 200 em /admin/usuarios, obtido %d", w.Code)
		}

		// POST create user
		form := url.Values{
			"username":     {"operador-novo"},
			"display_name": {"Novo Operador Criado"},
			"password":     {"SenhaForte123!"},
			"role":         {"editor"},
		}
		reqCreate := httptest.NewRequest(http.MethodPost, "/admin/usuarios", strings.NewReader(form.Encode()))
		reqCreate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqCreate.Header.Set("Origin", origin)
		reqCreate.AddCookie(adminCookie)
		wCreate := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wCreate, reqCreate)
		if wCreate.Code != http.StatusSeeOther {
			t.Fatalf("admin: esperado 303 ao criar usuário, obtido %d", wCreate.Code)
		}
	})
}

func TestRBAC_AuditLogs_OnlyAdminAndAuditor(t *testing.T) {
	srv, db, authSvc, _ := setupRBACTestServer(t)
	ctx := context.Background()

	// Admin e Auditor têm acesso
	for _, role := range []domain.UserRole{domain.RoleAdmin, domain.RoleAuditor} {
		t.Run("Allowed_"+string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "audit-allow-"+string(role), role, domain.UserStatusActive)

			req := httptest.NewRequest(http.MethodGet, "/admin/auditoria", nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("role %s: esperado 200 em /admin/auditoria, obtido %d", role, w.Code)
			}
			if !strings.Contains(w.Body.String(), "Trilha de Auditoria") {
				t.Errorf("role %s: esperado conter 'Trilha de Auditoria'", role)
			}
		})
	}

	// Editor e Reviewer são bloqueados
	for _, role := range []domain.UserRole{domain.RoleEditor, domain.RoleReviewer} {
		t.Run("Forbidden_"+string(role), func(t *testing.T) {
			cookie := createTestUserAndSession(t, ctx, authSvc, db, "audit-forbid-"+string(role), role, domain.UserStatusActive)

			req := httptest.NewRequest(http.MethodGet, "/admin/auditoria", nil)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("role %s: esperado 403 Forbidden em /admin/auditoria, obtido %d", role, w.Code)
			}
		})
	}
}

func TestRBAC_DisabledUser_Blocked(t *testing.T) {
	srv, db, authSvc, _ := setupRBACTestServer(t)
	ctx := context.Background()

	cookie := createTestUserAndSession(t, ctx, authSvc, db, "user-disabled", domain.RoleEditor, domain.UserStatusDisabled)

	req := httptest.NewRequest(http.MethodGet, "/admin/candidatos", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	// Usuário desativado não pode acessar nenhuma rota
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusSeeOther {
		t.Errorf("usuário desativado com cookie: esperado 401 ou 303 (redirecionamento para login), obtido %d", w.Code)
	}
}
