package web_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/observability"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupObservabilityTestServer(t *testing.T) (*http.Server, *sql.DB, *auth.Service, *observability.MetricsRegistry) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_obs_test.db")

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

	reg := observability.GetRegistry()

	cfg := &config.Config{
		Port:                       8080,
		Env:                        "test",
		DBPath:                     dbPath,
		PublicDataCutoff:           "2026-09-03",
		AdminAllowedOrigin:         "http://example.com",
		AdminUser:                  "admin",
		AdminPasswordHash:          "$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		AdminMFAEncryptionKey:      "12345678901234567890123456789012",
		BackupRemoteEnabled:        true,
		BackupRemoteProvider:       "s3",
		BackupRemoteBucket:         "test-bucket",
		BackupRemotePrefix:         "backups",
		BackupRemoteAccessKey:      "AKIA123",
		BackupRemoteSecretKey:      "SECRET123",
		BackupRemoteRetentionCount: 14,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao inicializar web.NewServer: %v", err)
	}

	return srv, db, authSvc, reg
}

func createObsTestUserAndSession(t *testing.T, ctx context.Context, authSvc *auth.Service, db *sql.DB, username string, role domain.UserRole) *http.Cookie {
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

func TestObservabilityRoutes_RBAC(t *testing.T) {
	srv, db, authSvc, _ := setupObservabilityTestServer(t)
	ctx := context.Background()

	adminCookie := createObsTestUserAndSession(t, ctx, authSvc, db, "admin_user", domain.RoleAdmin)
	auditorCookie := createObsTestUserAndSession(t, ctx, authSvc, db, "auditor_user", domain.RoleAuditor)
	editorCookie := createObsTestUserAndSession(t, ctx, authSvc, db, "editor_user", domain.RoleEditor)
	reviewerCookie := createObsTestUserAndSession(t, ctx, authSvc, db, "reviewer_user", domain.RoleReviewer)

	routes := []struct {
		name string
		path string
	}{
		{name: "Painel SSR", path: "/admin/observabilidade"},
		{name: "API JSON", path: "/admin/api/metrics"},
	}

	for _, route := range routes {
		t.Run(route.name+" - Não autenticado navegador redireciona para login", func(t *testing.T) {
			req := httptest.NewRequest("GET", route.path, nil)
			req.Header.Set("Accept", "text/html,application/xhtml+xml")
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Errorf("esperado redirecionamento 303, obtido %d", rec.Code)
			}
			if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/admin/login") {
				t.Errorf("esperado redirect para /admin/login, obtido %q", loc)
			}
		})

		t.Run(route.name+" - Não autenticado API retorna 401", func(t *testing.T) {
			req := httptest.NewRequest("GET", route.path, nil)
			req.Header.Set("Accept", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("esperado 401 Unauthorized, obtido %d", rec.Code)
			}
		})

		t.Run(route.name+" - Admin autorizado (200 OK)", func(t *testing.T) {
			req := httptest.NewRequest("GET", route.path, nil)
			req.AddCookie(adminCookie)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("esperado status 200 para Admin, obtido %d: %s", rec.Code, rec.Body.String())
			}
		})

		t.Run(route.name+" - Auditor autorizado (200 OK)", func(t *testing.T) {
			req := httptest.NewRequest("GET", route.path, nil)
			req.AddCookie(auditorCookie)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("esperado status 200 para Auditor, obtido %d: %s", rec.Code, rec.Body.String())
			}
		})

		t.Run(route.name+" - Editor proibido (403 Forbidden)", func(t *testing.T) {
			req := httptest.NewRequest("GET", route.path, nil)
			req.AddCookie(editorCookie)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("esperado status 403 para Editor, obtido %d", rec.Code)
			}
		})

		t.Run(route.name+" - Reviewer proibido (403 Forbidden)", func(t *testing.T) {
			req := httptest.NewRequest("GET", route.path, nil)
			req.AddCookie(reviewerCookie)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("esperado status 403 para Reviewer, obtido %d", rec.Code)
			}
		})
	}
}

func TestObservability_APIMetricsContent(t *testing.T) {
	srv, db, authSvc, reg := setupObservabilityTestServer(t)
	ctx := context.Background()

	adminCookie := createObsTestUserAndSession(t, ctx, authSvc, db, "admin_user_2", domain.RoleAdmin)

	// Registra telemetria de backup no registry
	reg.RecordLocalBackup(1024*1024*3, 45*time.Millisecond, 11, nil)
	reg.RecordRemoteBackup(1024*1024*3, 120*time.Millisecond, "s3", "test-bucket", "backups/vorcarozap-test.db", "abcd1234sha256", nil)

	req := httptest.NewRequest("GET", "/admin/api/metrics", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtido %d: %s", rec.Code, rec.Body.String())
	}

	var snapshot observability.MetricsSnapshot
	if err := json.NewDecoder(rec.Body).Decode(&snapshot); err != nil {
		t.Fatalf("falha ao decodificar JSON de métricas: %v", err)
	}

	if snapshot.Environment != "test" {
		t.Errorf("esperado Environment 'test', obtido %q", snapshot.Environment)
	}
	if snapshot.Runtime.NumGoroutines <= 0 {
		t.Errorf("esperado NumGoroutines > 0")
	}
	if snapshot.SQLite.PageCount <= 0 {
		t.Errorf("esperado PageCount > 0, obtido %d", snapshot.SQLite.PageCount)
	}
	if snapshot.Backup.LocalStatus != "ok" {
		t.Errorf("esperado LocalStatus 'ok', obtido %q", snapshot.Backup.LocalStatus)
	}
	if snapshot.Backup.RemoteStatus != "ok" {
		t.Errorf("esperado RemoteStatus 'ok', obtido %q", snapshot.Backup.RemoteStatus)
	}
	if snapshot.Backup.RemoteSHA256 != "abcd1234sha256" {
		t.Errorf("esperado SHA256 'abcd1234sha256', obtido %q", snapshot.Backup.RemoteSHA256)
	}
}

func TestObservability_SSRPageRendering(t *testing.T) {
	srv, db, authSvc, reg := setupObservabilityTestServer(t)
	ctx := context.Background()

	adminCookie := createObsTestUserAndSession(t, ctx, authSvc, db, "admin_user_3", domain.RoleAdmin)

	reg.RecordLocalBackup(5000000, 50*time.Millisecond, 11, nil)

	req := httptest.NewRequest("GET", "/admin/observabilidade", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtido %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Observabilidade Operacional") {
		t.Errorf("esperado título da página no HTML renderizado")
	}
	if !strings.Contains(body, "Saúde do SQLite WAL") {
		t.Errorf("esperado bloco SQLite WAL no HTML")
	}
	if !strings.Contains(body, "Telemetria de Backups") {
		t.Errorf("esperado bloco de Backups no HTML")
	}
}
