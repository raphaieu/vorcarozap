package web_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupBoundaryTestServer(t *testing.T) (*http.Server, *sql.DB, string, string) {
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

	// 1. Popula banco com diferentes estados editoriais
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-bnd-1', 'Operação Fronteira', 'operacao-fronteira', 'Caso de teste de fronteira pública');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-pub-1', 'person', 'Pessoa Publica 1', 'pessoa publica 1', 'pessoa-publica-1', 'Diretor', 'Resumo', 5, 'R1', 'Finanças', 'Nacional'),
			('ent-pub-2', 'person', 'Pessoa Publica 2', 'pessoa publica 2', 'pessoa-publica-2', 'Sócio', 'Resumo', 4, 'R2', 'Politica', 'Nacional'),
			('ent-quar-1', 'person', 'Pessoa Em Quarentena', 'pessoa em quarentena', 'pessoa-em-quarentena', 'Contato', 'Resumo', 3, 'R3', 'Setor Público', 'Regional'),
			('ent-rej-1', 'person', 'Pessoa Rejeitada', 'pessoa rejeitada', 'pessoa-rejeitada', 'Representante', 'Resumo', 2, 'R4', 'Outros', 'Regional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary)
		VALUES
			('rel-pub-1', 'ent-pub-1', 'case-bnd-1', 'investigado', 'Vínculo 1'),
			('rel-pub-2', 'ent-pub-2', 'case-bnd-1', 'investigado', 'Vínculo 2'),
			('rel-quar-1', 'ent-quar-1', 'case-bnd-1', 'investigado', 'Vínculo 3'),
			('rel-rej-1', 'ent-rej-1', 'case-bnd-1', 'investigado', 'Vínculo 4');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_access_status)
		VALUES
			('src-bnd-1', 'Notícia 1', 'Jornal 1', 'https://bnd1.com', 'https://bnd1.com', 'reachable'),
			('src-bnd-2', 'Notícia 2', 'Jornal 2', 'https://bnd2.com', 'https://bnd2.com', 'reachable');

		-- Claim 1: Publicado
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-pub-1', 'rel-pub-1', 'Proposição Pública 1', 'Jornal 1', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-bnd-1', 'claim-pub-1', 'Ev 1');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-bnd-1', 'ev-bnd-1', 'src-bnd-1', 'Trecho 1', 'P.1', 'supports', 'active');

		-- Claim 2: Publicado de Contexto
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-pub-2', 'rel-pub-2', 'Proposição Pública 2', 'Jornal 2', 'curated_seed', 'C', 'context_only', 0, 'published', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-bnd-2', 'claim-pub-2', 'Ev 2');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-bnd-2', 'ev-bnd-2', 'src-bnd-2', 'Trecho 2', 'P.2', 'supports', 'active');

		-- Claim 3: Em Quarentena
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-quar-1', 'rel-quar-1', 'Proposição Quarentenada', 'Jornal 1', 'openrouter', 'D', 'possible_link', 1, 'quarantined', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-bnd-3', 'claim-quar-1', 'Ev 3');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-bnd-3', 'ev-bnd-3', 'src-bnd-1', 'Trecho 3', 'P.3', 'supports', 'active');

		-- Claim 4: Rejeitado
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-rej-1', 'rel-rej-1', 'Proposição Rejeitada', 'Jornal 2', 'admin', 'E', 'possible_link', 1, 'rejected', 'ativo');
		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-bnd-4', 'claim-rej-1', 'Ev 4');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-bnd-4', 'ev-bnd-4', 'src-bnd-2', 'Trecho 4', 'P.4', 'supports', 'active');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	allowedOrigin := "http://localhost:8090"

	cfg := &config.Config{
		Port:               8090,
		PublicDataCutoff:   "2026-09-03",
		AdminUser:          "admin_editor",
		AdminPasswordHash:  passwordHash,
		AdminAllowedOrigin: allowedOrigin,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin_editor:password"))
	return srv, db, allowedOrigin, authHeader
}

// TestDeterministic_PublicBoundaryAndHeaders comprova todos os requisitos de cabeçalhos de cache,
// isolamento de rotas públicas e proteção do admin.
func TestDeterministic_PublicBoundaryAndHeaders(t *testing.T) {
	srv, _, origin, authHeader := setupBoundaryTestServer(t)

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

	t.Run("Área Administrativa contém Cache-Control: no-store e Vary: Authorization", func(t *testing.T) {
		adminPaths := []string{
			"/admin",
			"/admin/candidatos",
			"/admin/evidencias",
			"/admin/fontes",
		}

		for _, p := range adminPaths {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			req.Header.Set("Authorization", authHeader)
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("GET %s: esperado 200, obtido %d", p, rec.Code)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("GET %s: Cache-Control esperado 'no-store', obtido %q", p, cc)
			}
			if vary := rec.Header().Get("Vary"); vary != "Authorization" {
				t.Errorf("GET %s: Vary esperado 'Authorization', obtido %q", p, vary)
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
		reqNoOrigin.Header.Set("Authorization", authHeader)
		reqNoOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recNoOrigin := httptest.NewRecorder()
		srv.Handler.ServeHTTP(recNoOrigin, reqNoOrigin)

		if recNoOrigin.Code != http.StatusForbidden {
			t.Errorf("POST sem Origin esperado 403 Forbidden, obtido %d", recNoOrigin.Code)
		}

		// 2. Com Origin divergente
		reqEvilOrigin := httptest.NewRequest(http.MethodPost, moderateURL, strings.NewReader(form.Encode()))
		reqEvilOrigin.Header.Set("Authorization", authHeader)
		reqEvilOrigin.Header.Set("Origin", "http://atacante.com")
		reqEvilOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recEvilOrigin := httptest.NewRecorder()
		srv.Handler.ServeHTTP(recEvilOrigin, reqEvilOrigin)

		if recEvilOrigin.Code != http.StatusForbidden {
			t.Errorf("POST com Origin divergente esperado 403 Forbidden, obtido %d", recEvilOrigin.Code)
		}

		// 3. Com Origin autorizado
		reqValidOrigin := httptest.NewRequest(http.MethodPost, moderateURL, strings.NewReader(form.Encode()))
		reqValidOrigin.Header.Set("Authorization", authHeader)
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
