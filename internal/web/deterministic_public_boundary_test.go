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

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupBoundaryTestServer(t *testing.T) (*http.Server, *sql.DB, string, *http.Cookie) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "boundary_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha nas migrations: %v", err)
	}

	// Popula entidades e claims com diferentes estados
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-bound-1', 'Caso Fronteira', 'caso-fronteira', 'Descrição de teste');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-pub-1', 'person', 'Pessoa Pública 1', 'pessoa publica 1', 'pessoa-publica-1', 'Diretor', 'Resumo 1', 5, 'Figura central', 'Finanças', 'Nacional'),
			('ent-pub-2', 'person', 'Pessoa Pública 2', 'pessoa publica 2', 'pessoa-publica-2', 'Sócio', 'Resumo 2', 4, 'Sócio principal', 'Empresarial', 'Nacional'),
			('ent-quar-1', 'person', 'Pessoa em Quarentena', 'pessoa em quarentena', 'pessoa-em-quarentena', 'Investigado', 'Resumo 3', 3, 'Sob análise', 'Setor Público', 'Regional'),
			('ent-rej-1', 'person', 'Pessoa Rejeitada', 'pessoa rejeitada', 'pessoa-rejeitada', 'Ex-diretor', 'Resumo 4', 2, 'Alegações refutadas', 'Finanças', 'Regional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary)
		VALUES
			('rel-pub-1', 'ent-pub-1', 'case-bound-1', 'investigado', 'Resumo vínculo 1'),
			('rel-pub-2', 'ent-pub-2', 'case-bound-1', 'investigado', 'Resumo vínculo 2'),
			('rel-quar-1', 'ent-quar-1', 'case-bound-1', 'investigado', 'Resumo vínculo 3'),
			('rel-rej-1', 'ent-rej-1', 'case-bound-1', 'investigado', 'Resumo vínculo 4');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_access_status)
		VALUES
			('src-bound-1', 'Notícia 1', 'Jornal 1', 'https://jornal1.com', 'https://jornal1.com', 'reachable'),
			('src-bound-2', 'Notícia 2', 'Jornal 2', 'https://jornal2.com', 'https://jornal2.com', 'reachable');

		-- Claim 1: Publicado
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-pub-1', 'rel-pub-1', 'Proposição Documentada 1', 'Jornal 1', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-1', 'claim-pub-1', 'Evidência 1');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-1', 'ev-1', 'src-bound-1', 'Trecho 1', 'Pág. 1', 'supports', 'active');

		-- Claim 2: Publicado (Grau B)
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-pub-2', 'rel-pub-2', 'Proposição Documentada 2', 'Jornal 2', 'curated_seed', 'B', 'supports_link', 1, 'published', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-2', 'claim-pub-2', 'Evidência 2');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-2', 'ev-2', 'src-bound-2', 'Trecho 2', 'Pág. 2', 'supports', 'active');

		-- Claim 3: Em Quarentena
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-quar-1', 'rel-quar-1', 'Proposição Sob Quarentena', 'Jornal 1', 'openrouter', 'C', 'possible_link', 1, 'quarantined', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-3', 'claim-quar-1', 'Evidência 3');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-3', 'ev-3', 'src-bound-1', 'Trecho 3', 'Pág. 3', 'supports', 'active');

		-- Claim 4: Rejeitado
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-rej-1', 'rel-rej-1', 'Proposição Rejeitada', 'Jornal 2', 'curated_seed', 'D', 'supports_link', 0, 'rejected', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-4', 'claim-rej-1', 'Evidência 4');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-4', 'ev-4', 'src-bound-2', 'Trecho 4', 'Pág. 4', 'supports', 'rejected');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	allowedOrigin := "http://localhost:8090"

	cfg := &config.Config{
		Port:                  8090,
		PublicDataCutoff:      "2026-09-03",
		AdminUser:             "admin_editor",
		AdminPasswordHash:     passwordHash,
		AdminAllowedOrigin:    allowedOrigin,
		AdminMFAEncryptionKey: "12345678901234567890123456789012",
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	sessionCookie := createSessionCookieForUser(t, db, "admin_editor", domain.RoleAdmin)
	return srv, db, allowedOrigin, sessionCookie
}

// TestDeterministic_PublicBoundaryAndHeaders comprova todos os requisitos de cabeçalhos de cache,
// isolamento de rotas públicas e proteção do admin.
func TestDeterministic_PublicBoundaryAndHeaders(t *testing.T) {
	srv, _, origin, sessionCookie := setupBoundaryTestServer(t)

	t.Run("Rotas públicas dinâmicas SSR e XLSX contêm Cache-Control restritivo sem Vary: Authorization", func(t *testing.T) {
		publicPaths := []struct {
			path         string
			expectedCode int
		}{
			{"/", http.StatusOK},
			{"/pessoas", http.StatusOK},
			{"/pessoas/pessoa-publica-1", http.StatusOK},
			{"/pessoas/pessoa-publica-2", http.StatusOK},
			{"/pessoas/pessoa-em-quarentena", http.StatusNotFound}, // Entidade sem claims públicos => 404
			{"/pessoas/pessoa-rejeitada", http.StatusNotFound},     // Entidade com claim rejeitado => 404
			{"/metodologia", http.StatusOK},
			{"/exportar/base.xlsx", http.StatusOK},
		}

		for _, p := range publicPaths {
			req := httptest.NewRequest(http.MethodGet, p.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != p.expectedCode {
				t.Errorf("GET %s: esperado %d, obtido %d", p.path, p.expectedCode, rec.Code)
			}

			// Validação de Cache-Control
			cc := rec.Header().Get("Cache-Control")
			if cc != "no-cache, no-store, must-revalidate" {
				t.Errorf("GET %s: Cache-Control esperado 'no-cache, no-store, must-revalidate', obtido %q", p.path, cc)
			}

			// Rotas públicas NÃO devem ter Vary: Authorization
			vary := rec.Header().Get("Vary")
			if strings.Contains(vary, "Authorization") {
				t.Errorf("GET %s pública NÃO deve conter Vary: Authorization, obtido %q", p.path, vary)
			}

			// Se for exportação XLSX, valida Content-Disposition
			if p.path == "/exportar/base.xlsx" {
				disp := rec.Header().Get("Content-Disposition")
				if !strings.Contains(disp, "attachment") {
					t.Errorf("GET /exportar/base.xlsx: Content-Disposition esperado conter 'attachment', obtido %q", disp)
				}
			}
		}
	})

	t.Run("Área Administrativa contém Cache-Control: no-store e Vary", func(t *testing.T) {
		adminPaths := []string{
			"/admin",
			"/admin/candidatos",
			"/admin/evidencias",
			"/admin/fontes",
		}

		for _, p := range adminPaths {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			req.AddCookie(sessionCookie)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("GET %s: esperado 200, obtido %d", p, rec.Code)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("GET %s: Cache-Control esperado 'no-store', obtido %q", p, cc)
			}
			if vary := rec.Header().Get("Vary"); vary != "Authorization" && !strings.Contains(vary, "Cookie") {
				t.Errorf("GET %s: Vary esperado 'Authorization' ou conter 'Cookie', obtido %q", p, vary)
			}
		}
	})

	t.Run("Proteção CSRF estrita no /admin: POST sem Origin ou com Origin divergente retorna 403", func(t *testing.T) {
		moderateURL := "/admin/claims/claim-pub-1/moderate"

		// 1. Sem Origin
		form := url.Values{
			"action":              {"reject"},
			"reason":              {"Teste CSRF sem origin"},
			"expected_updated_at": {"2026-09-03T00:00:00Z"},
		}
		reqNoOrigin := httptest.NewRequest(http.MethodPost, moderateURL, strings.NewReader(form.Encode()))
		reqNoOrigin.AddCookie(sessionCookie)
		reqNoOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recNoOrigin := httptest.NewRecorder()
		srv.Handler.ServeHTTP(recNoOrigin, reqNoOrigin)

		if recNoOrigin.Code != http.StatusForbidden {
			t.Errorf("POST sem Origin esperado 403 Forbidden, obtido %d", recNoOrigin.Code)
		}

		// 2. Com Origin divergente
		reqEvilOrigin := httptest.NewRequest(http.MethodPost, moderateURL, strings.NewReader(form.Encode()))
		reqEvilOrigin.AddCookie(sessionCookie)
		reqEvilOrigin.Header.Set("Origin", "http://atacante.com")
		reqEvilOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recEvilOrigin := httptest.NewRecorder()
		srv.Handler.ServeHTTP(recEvilOrigin, reqEvilOrigin)

		if recEvilOrigin.Code != http.StatusForbidden {
			t.Errorf("POST com Origin divergente esperado 403 Forbidden, obtido %d", recEvilOrigin.Code)
		}

		// 3. Com Origin autorizado
		reqValidOrigin := httptest.NewRequest(http.MethodPost, moderateURL, strings.NewReader(form.Encode()))
		reqValidOrigin.AddCookie(sessionCookie)
		reqValidOrigin.Header.Set("Origin", origin)
		reqValidOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recValidOrigin := httptest.NewRecorder()
		srv.Handler.ServeHTTP(recValidOrigin, reqValidOrigin)

		// Não deve ser 403 Forbidden
		if recValidOrigin.Code == http.StatusForbidden {
			t.Errorf("POST com Origin válido foi indevidamente rejeitado com 403 Forbidden")
		}
	})
}
