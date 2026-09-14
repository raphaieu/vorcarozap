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
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupEditorialHistoryTestServer(t *testing.T) (*http.Server, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "editorial_history_test.db")

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

	// Insere fixtures para teste de histórico editorial
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-hist-1', 'Caso Transparência', 'caso-transparencia', 'Contexto');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-hist-1', 'person', 'Autoridade Auditada', 'autoridade auditada', 'autoridade-auditada', 'Diretor', 'Resumo', 4, 'Relevância alta', 'Poder Público', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES
			('rel-hist-1', 'ent-hist-1', 'case-hist-1', 'Investigado', 'Vínculo documental', 'Limites');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, http_status, normalized_error_code, source_access_checked_at)
		VALUES
			('src-hist-1', 'Diário Oficial da União nº 50', 'Imprensa Nacional', 'https://in.gov.br/dou/50', 'https://in.gov.br/dou/50', 'official_statement', 'reachable', 200, '', '2026-09-10T10:00:00Z');

		-- Claims
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons, created_at, updated_at)
		VALUES
			('clm-hist-pub', 'rel-hist-1', 'Alegação publicada originalmente no seed', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ok', '[]', '2026-09-01T10:00:00Z', '2026-09-01T10:00:00Z'),
			('clm-hist-mod', 'rel-hist-1', 'Alegação que foi aprovada por moderação', '', 'openrouter', 'B', 'supports_link', 1, 'published', 'ok', '[]', '2026-09-02T11:00:00Z', '2026-09-03T12:00:00Z');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-hist-1', 'clm-hist-pub', 'Evidência 1', 'document'),
			('ev-hist-2', 'clm-hist-mod', 'Evidência 2', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status, created_at, updated_at)
		VALUES
			('es-hist-1', 'ev-hist-1', 'src-hist-1', 'supports', 'Trecho da página 10', 'Pág. 10', 'active', '2026-09-01T10:00:00Z', '2026-09-01T10:00:00Z'),
			('es-hist-2', 'ev-hist-2', 'src-hist-1', 'supports', 'Trecho da página 25', 'Pág. 25', 'active', '2026-09-02T11:00:00Z', '2026-09-02T11:00:00Z');

		-- Decisões de moderação com dados administrativos confidenciais
		INSERT INTO moderation_decisions (id, claim_id, evidence_source_id, action, reason, actor, candidate_fingerprint, created_at)
		VALUES
			('dec-app-1', 'clm-hist-mod', NULL, 'approve', 'Aprovado internamente pelo operador com dado secreto 99999', 'operador_admin_confidencial', 'fp_sha256_abcdef1234567890', '2026-09-03T12:00:00Z'),
			('dec-rej-es2', NULL, 'es-hist-2', 'reject', 'Desativação de trecho por inadequação', 'operador_admin_confidencial', '', '2026-09-04T16:00:00Z'),
			('dec-rst-es2', NULL, 'es-hist-2', 'restore', 'Reativação de trecho após confirmação documental', 'operador_admin_confidencial', '', '2026-09-05T08:00:00Z');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir fixtures de histórico: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	cfg := &config.Config{
		Port:               8080,
		Env:                "test",
		PublicDataCutoff:   "2026-09-03",
		AdminUser:          "admin_editor",
		AdminPasswordHash:  passwordHash,
		AdminAllowedOrigin: "http://example.com",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao inicializar servidor: %v", err)
	}

	return srv, db
}

func TestPublicEntityDetail_EditorialHistory_RenderingAndRedaction(t *testing.T) {
	srv, _ := setupEditorialHistoryTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/pessoas/autoridade-auditada", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /pessoas/autoridade-auditada retornou status %d, esperado %d", w.Code, http.StatusOK)
	}

	// 1. Headers de cache
	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") || !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control inválido: %q", cc)
	}

	body := w.Body.String()

	// 2. Seção de Histórico Editorial
	if !strings.Contains(body, "Histórico de Alterações Editoriais") {
		t.Errorf("seção 'Histórico de Alterações Editoriais' ausente no HTML")
	}
	if !strings.Contains(body, "Trilha pública de transparência editorial") {
		t.Errorf("subtítulo explicativo de transparência ausente no HTML")
	}

	// 3. Ações esperadas no histórico
	if !strings.Contains(body, "Aprovação e Publicação") {
		t.Errorf("ação 'Aprovação e Publicação' ausente no histórico")
	}
	if !strings.Contains(body, "Reativação de Suporte") {
		t.Errorf("ação 'Reativação de Suporte' ausente no histórico")
	}

	// 4. Redação rigorosa de dados administrativos
	if strings.Contains(body, "operador_admin_confidencial") {
		t.Errorf("vazamento de username administrativo (actor) na página pública")
	}
	if strings.Contains(body, "fp_sha256_abcdef1234567890") {
		t.Errorf("vazamento de fingerprint técnico na página pública")
	}
	if strings.Contains(body, "dado secreto 99999") {
		t.Errorf("vazamento de justificativa administrativa interna (reason) na página pública")
	}
}

func TestPublicDocumentDetail_EditorialHistory_Rendering(t *testing.T) {
	srv, _ := setupEditorialHistoryTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/documentos/src-hist-1", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /documentos/src-hist-1 retornou status %d, esperado %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	if !strings.Contains(body, "Histórico de Alterações Editoriais do Documento") {
		t.Errorf("seção 'Histórico de Alterações Editoriais do Documento' ausente no HTML")
	}
	if !strings.Contains(body, "Reativação de Trecho") {
		t.Errorf("ação 'Reativação de Trecho' ausente no histórico do documento")
	}

	// Redação de actor e fingerprint
	if strings.Contains(body, "operador_admin_confidencial") || strings.Contains(body, "fp_sha256_") {
		t.Errorf("vazamento de dados administrativos no documento público")
	}
}

func TestPublicEditorialHistory_ImmediateConsistencyAfterModeration(t *testing.T) {
	srv, _ := setupEditorialHistoryTestServer(t)

	// 1. Executa rejeição de claim via POST no admin (clm-hist-mod possui aprovação prévia)
	form := url.Values{}
	form.Set("expected_updated_at", "2026-09-03T12:00:00Z")
	form.Set("action", "reject")
	form.Set("reason", "Alegação rejeitada por erro factual comprovado")

	reqMod := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-hist-mod/moderate", strings.NewReader(form.Encode()))
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqMod.Header.Set("Origin", "http://example.com")
	reqMod.Header.Set("Authorization", basicAuthHeader("admin_editor", "password"))

	wMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMod, reqMod)

	if wMod.Code != http.StatusSeeOther {
		t.Fatalf("POST /admin/claims/... retornou status %d, esperado 303 See Other", wMod.Code)
	}

	// 2. Consulta imediatamente a página pública da entidade
	reqPub := httptest.NewRequest(http.MethodGet, "/pessoas/autoridade-auditada", nil)
	wPub := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPub, reqPub)

	if wPub.Code != http.StatusOK {
		t.Fatalf("GET /pessoas/autoridade-auditada retornou status %d", wPub.Code)
	}

	bodyPub := wPub.Body.String()

	// A alegação rejeitada não deve mais constar como proposição ativa
	if strings.Contains(bodyPub, "Alegação que foi aprovada por moderação") {
		t.Errorf("alegação rejeitada ainda aparece no corpo de alegações ativas da entidade")
	}

	// O evento de 'Retirada Editorial' deve aparecer no histórico
	if !strings.Contains(bodyPub, "Retirada Editorial") {
		t.Errorf("evento de 'Retirada Editorial' não apareceu no histórico público da entidade")
	}
}

func TestAdminDetail_VisualDifferentiationOfAdminAudit(t *testing.T) {
	srv, _ := setupEditorialHistoryTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/claims/clm-hist-mod", nil)
	req.Header.Set("Authorization", basicAuthHeader("admin_editor", "password"))
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/claims/clm-hist-mod retornou status %d", w.Code)
	}

	body := w.Body.String()

	// No painel administrativo, o bloco deve destacar que se trata da auditoria administrativa privada
	if !strings.Contains(body, "Auditoria Administrativa Completa (Privada)") {
		t.Errorf("destaque visual 'Auditoria Administrativa Completa (Privada)' ausente no admin")
	}

	// E os dados completos (actor, fingerprint, reason) DEVEM estar presentes no admin
	if !strings.Contains(body, "operador_admin_confidencial") {
		t.Errorf("actor ausente na visualização administrativa")
	}
	if !strings.Contains(body, "fp_sha256_abcdef1234567890") {
		t.Errorf("fingerprint ausente na visualização administrativa")
	}
	if !strings.Contains(body, "dado secreto 99999") {
		t.Errorf("reason ausente na visualização administrativa")
	}
}

func TestPublicEditorialHistory_PrivateQuarantinedItems_NeverLeakedInHTML(t *testing.T) {
	srv, db := setupEditorialHistoryTestServer(t)
	ctx := context.Background()

	// Insere uma alegação e evidência privadas em quarentena (nunca públicas) com marcadores de texto únicos
	_, err := db.ExecContext(ctx, `
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, http_status, normalized_error_code, source_access_checked_at)
		VALUES
			('src-priv-secret-99', 'Documento Sigiloso do Gabinete Secreto 7777', 'Órgão Secreto', 'https://example.gov.br/secreto/99', 'https://example.gov.br/secreto/99', 'court_document', 'reachable', 200, '', '2026-09-10T10:00:00Z');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons, created_at, updated_at)
		VALUES
			('clm-priv-secret-88', 'rel-hist-1', 'Proposição altamente confidencial de quarentena 8888', '', 'curated_seed', 'E', 'context_only', 0, 'quarantined', 'Quarentena', '["mapped_initial_state"]', '2026-09-01T10:00:00Z', '2026-09-01T10:00:00Z');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-priv-secret-77', 'clm-priv-secret-88', 'Evidência Secreta', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status, created_at, updated_at)
		VALUES
			('es-priv-secret-66', 'ev-priv-secret-77', 'src-priv-secret-99', 'supports', 'Trecho ultra secreto', 'Pág. 9999_LOCATOR_SECRETO', 'rejected', '2026-09-01T10:00:00Z', '2026-09-01T10:00:00Z');

		INSERT INTO moderation_decisions (id, claim_id, evidence_source_id, action, reason, actor, candidate_fingerprint, created_at)
		VALUES
			('dec-priv-claim-rej-1', 'clm-priv-secret-88', NULL, 'reject', 'Motivo confidencial de rejeição em quarentena 6666', 'operador_quarentena_5555', '', '2026-09-06T10:00:00Z'),
			('dec-priv-claim-rst-1', 'clm-priv-secret-88', NULL, 'restore', 'Motivo confidencial de restauração em quarentena 6666', 'operador_quarentena_5555', '', '2026-09-06T11:00:00Z'),
			('dec-priv-es-rej-1', NULL, 'es-priv-secret-66', 'reject', 'Desativação confidencial de trecho em quarentena 6666', 'operador_quarentena_5555', '', '2026-09-06T12:00:00Z'),
			('dec-priv-es-rst-1', NULL, 'es-priv-secret-66', 'restore', 'Reativação confidencial de trecho em quarentena 6666', 'operador_quarentena_5555', '', '2026-09-06T13:00:00Z');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir fixtures de quarentena privada: %v", err)
	}

	// 1. Consulta GET na página da pessoa
	req := httptest.NewRequest(http.MethodGet, "/pessoas/autoridade-auditada", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /pessoas/autoridade-auditada retornou status %d, esperado %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()

	// 2. Validação rigorosa de ausência total de metadados, identificadores, localizadores e justificativas
	forbiddenSubstrings := []string{
		"8888",
		"Proposição altamente confidencial",
		"9999_LOCATOR_SECRETO",
		"Gabinete Secreto 7777",
		"src-priv-secret-99",
		"clm-priv-secret-88",
		"es-priv-secret-66",
		"dec-priv-claim-rej-1",
		"dec-priv-claim-rst-1",
		"dec-priv-es-rej-1",
		"dec-priv-es-rst-1",
		"6666",
		"operador_quarentena_5555",
	}

	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(body, forbidden) {
			t.Errorf("VIOLAÇÃO DE PRIVACIDADE: elemento privado de quarentena %q vazou no HTML público de /pessoas/{slug}", forbidden)
		}
	}

	// 3. Consulta GET no documento privado (não possui alegações públicas ativas)
	reqDoc := httptest.NewRequest(http.MethodGet, "/documentos/src-priv-secret-99", nil)
	wDoc := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wDoc, reqDoc)

	// Documento privado deve retornar 404 Not Found para visitantes públicos
	if wDoc.Code != http.StatusNotFound {
		t.Errorf("GET /documentos/src-priv-secret-99 retornou status %d, esperado 404 Not Found", wDoc.Code)
	}
}
