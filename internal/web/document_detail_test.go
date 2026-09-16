package web_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupDocumentDetailTestServer(t *testing.T) (*http.Server, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "doc_detail_test.db")

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

	// Insere dados de teste para documentos e sequências contextuais
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-doc-1', 'Operação Factual', 'operacao-factual', 'Contexto investigativo');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-doc-1', 'person', 'Investigado Alpha', 'investigado alpha', 'investigado-alpha', 'Executivo', 'Resumo Alpha', 4, 'Alvo da apuração', 'Empresas', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES
			('rel-doc-1', 'ent-doc-1', 'case-doc-1', 'Investigado', 'Citado em laudo pericial oficial', 'Limites documentais');

		-- Fonte 1: Laudo pericial com alegação publicada (visível publicamente)
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, http_status, normalized_error_code, source_access_checked_at)
		VALUES
			('src-laudo-pub', 'Laudo Pericial INC nº 100/2026', 'Polícia Federal', 'https://pf.gov.br/laudo100.pdf', 'https://pf.gov.br/laudo100.pdf', 'police_report', 'reachable', 200, '', '2026-09-12T10:00:00Z'),
			('src-quar-only', 'Fonte em Quarentena', 'Veículo Blog', 'https://blog.com/quar', 'https://blog.com/quar', 'article', 'not_checked', 0, '', ''),
			('src-xss-test', 'Fonte Insegura <script>alert(1)</script>', 'Autor <img src=x onerror=alert(1)>', 'javascript:alert(1)', 'javascript:alert(1)', 'article', 'not_checked', 0, '', '');

		-- Claims: 1 publicado e 1 em quarentena
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES
			('clm-doc-pub', 'rel-doc-1', 'Constatação técnica de mensagens no relatório', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]'),
			('clm-doc-quar', 'rel-doc-1', 'Alegação em quarentena sem aprovação', '', 'openrouter', 'D', 'supports_link', 0, 'quarantined', 'quarantined', '["GRADE_D"]');

		-- Evidências
		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-doc-pub', 'clm-doc-pub', 'Laudo pericial da Polícia Federal', 'document'),
			('ev-doc-quar', 'clm-doc-quar', 'Menção em blog sem corroboração', 'document');

		-- Evidence Sources no laudo público (criados fora de ordem propositalmente)
		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status, created_at)
		VALUES
			('es-doc-42', 'ev-doc-pub', 'src-laudo-pub', 'supports', 'Trecho pericial da página 42', 'Pág. 42, Fig. 12', 'active', '2026-09-01T10:00:00Z'),
			('es-doc-5', 'ev-doc-pub', 'src-laudo-pub', 'supports', 'Trecho pericial inicial da página 5', 'p. 5', 'active', '2026-09-01T10:05:00Z'),
			('es-doc-12', 'ev-doc-pub', 'src-laudo-pub', 'supports', 'Trecho intermediário das páginas 12 a 14', 'págs. 12-14', 'active', '2026-09-01T10:10:00Z'),
			('es-doc-rej', 'ev-doc-pub', 'src-laudo-pub', 'supports', 'Trecho desaprovado na página 1', 'Pág. 1', 'rejected', '2026-09-01T10:15:00Z');

		-- Evidence Source na fonte em quarentena
		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status, created_at)
		VALUES
			('es-quar-item', 'ev-doc-quar', 'src-quar-only', 'supports', 'Trecho sob quarentena', 'Pág. 99', 'active', '2026-09-01T10:20:00Z');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir fixtures: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	cfg := &config.Config{
		Port:                  8080,
		Env:                   "test",
		PublicDataCutoff:      "2026-09-03",
		AdminUser:             "admin_editor",
		AdminPasswordHash:     passwordHash,
		AdminAllowedOrigin:    "http://example.com",
		AdminMFAEncryptionKey: "12345678901234567890123456789012",
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao inicializar servidor: %v", err)
	}

	return srv, db
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestPublicDocumentDetail_SuccessAndOrdering(t *testing.T) {
	srv, _ := setupDocumentDetailTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/documentos/src-laudo-pub", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /documentos/src-laudo-pub retornou status %d, esperado %d", w.Code, http.StatusOK)
	}

	// 1. Cabeçalhos de segurança e cache
	cacheControl := w.Header().Get("Cache-Control")
	if !strings.Contains(cacheControl, "no-cache") || !strings.Contains(cacheControl, "no-store") {
		t.Errorf("Cache-Control inválido: %q, esperado no-cache, no-store", cacheControl)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options ausente ou incorreto: %q", w.Header().Get("X-Content-Type-Options"))
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options ausente ou incorreto: %q", w.Header().Get("X-Frame-Options"))
	}

	body := w.Body.String()

	// 2. Metadados do documento
	if !strings.Contains(body, "Laudo Pericial INC nº 100/2026") {
		t.Errorf("título do documento ausente na resposta")
	}
	if !strings.Contains(body, "Polícia Federal") {
		t.Errorf("veículo/autor ausente na resposta")
	}
	if !strings.Contains(body, "Relatório Policial / Pericial") {
		t.Errorf("tipo documental ausente na resposta")
	}
	if !strings.Contains(body, "Documento Oficial / Primário") {
		t.Errorf("classificação de natureza primária ausente na resposta")
	}

	// 3. Verificação de ordenação determinística no HTML
	// Pág. 5 deve aparecer ANTES de Págs. 12–14, que por sua vez deve aparecer ANTES de Pág. 42
	idxP5 := strings.Index(body, "Pág. 5")
	idxP12 := strings.Index(body, "Págs. 12–14")
	idxP42 := strings.Index(body, "Pág. 42, Fig. 12")

	if idxP5 == -1 || idxP12 == -1 || idxP42 == -1 {
		t.Fatalf("localizadores esperados não encontrados no HTML: p5=%d, p12=%d, p42=%d", idxP5, idxP12, idxP42)
	}

	if !(idxP5 < idxP12 && idxP12 < idxP42) {
		t.Errorf("ordenação dos itens no HTML incorreta: Pág.5 (pos %d), Págs.12-14 (pos %d), Pág.42 (pos %d)", idxP5, idxP12, idxP42)
	}

	// 4. Trecho rejeitado ('Trecho desaprovado') NÃO deve aparecer na página pública
	if strings.Contains(body, "Trecho desaprovado") {
		t.Errorf("trecho rejeitado vazou para a página pública")
	}

	// 5. Trecho em quarentena NÃO deve aparecer na página pública
	if strings.Contains(body, "Trecho sob quarentena") {
		t.Errorf("trecho em quarentena vazou para a página pública")
	}

	// 6. Link externo seguro
	if !strings.Contains(body, `href="https://pf.gov.br/laudo100.pdf"`) || !strings.Contains(body, `target="_blank"`) || !strings.Contains(body, `rel="noopener noreferrer"`) {
		t.Errorf("link externo canônico ausente ou sem atributos de segurança")
	}
}

func TestPublicDocumentDetail_NotFoundForQuarantinedOnlyOrMissing(t *testing.T) {
	srv, _ := setupDocumentDetailTestServer(t)

	// Fonte que só tem alegações em quarentena não deve ser exposta publicamente
	req := httptest.NewRequest(http.MethodGet, "/documentos/src-quar-only", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /documentos/src-quar-only retornou %d, esperado %d", w.Code, http.StatusNotFound)
	}

	// Fonte inexistente
	req2 := httptest.NewRequest(http.MethodGet, "/documentos/src-inexistente-xyz", nil)
	w2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("GET /documentos/src-inexistente-xyz retornou %d, esperado %d", w2.Code, http.StatusNotFound)
	}
}

func TestPublicDocumentDetail_SecuritySanitization(t *testing.T) {
	srv, db := setupDocumentDetailTestServer(t)

	// Insere alegação pública para src-xss-test para testar sanitização
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-xss', 'rel-doc-1', 'Proposição normal', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ok', '[]');
		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-xss', 'clm-xss', 'Sumário', 'document');
		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-xss', 'ev-xss', 'src-xss-test', 'supports', 'Trecho com <script>alert("xss")</script>', 'Pág. <script>1</script>', 'active');
	`)

	req := httptest.NewRequest(http.MethodGet, "/documentos/src-xss-test", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /documentos/src-xss-test retornou status %d", w.Code)
	}

	body := w.Body.String()

	// Não pode conter tags executáveis de script
	if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, `<script>alert("xss")</script>`) {
		t.Errorf("HTML não escapou tags de script maliciosas")
	}

	// Link malicioso com javascript:alert(1) não deve ser renderizado como link clicável seguro
	if strings.Contains(body, `href="javascript:alert(1)"`) {
		t.Errorf("link com esquema javascript: não foi neutralizado")
	}
}

func TestAdminSourceDetail_AccessAndExhibition(t *testing.T) {
	srv, db := setupDocumentDetailTestServer(t)
	sessionCookie := createSessionCookieForUser(t, db, "admin_editor", domain.RoleAdmin)

	// 1. Sem autenticação deve responder 401 Unauthorized
	reqUnauth := httptest.NewRequest(http.MethodGet, "/admin/fontes/src-laudo-pub", nil)
	wUnauth := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wUnauth, reqUnauth)

	if wUnauth.Code != http.StatusUnauthorized {
		t.Errorf("GET /admin/fontes/... não autenticado retornou %d, esperado 401", wUnauth.Code)
	}

	// 2. Autenticado deve responder 200 OK
	reqAuth := httptest.NewRequest(http.MethodGet, "/admin/fontes/src-laudo-pub", nil)
	reqAuth.AddCookie(sessionCookie)
	wAuth := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAuth, reqAuth)

	if wAuth.Code != http.StatusOK {
		t.Fatalf("GET /admin/fontes/src-laudo-pub autenticado retornou %d, esperado 200", wAuth.Code)
	}

	// Cabeçalhos de segurança do admin
	if wAuth.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Admin Cache-Control = %q, esperado no-store", wAuth.Header().Get("Cache-Control"))
	}
	if vary := wAuth.Header().Get("Vary"); vary != "Authorization" && !strings.Contains(vary, "Cookie") {
		t.Errorf("Admin Vary = %q, esperado Authorization ou Cookie", vary)
	}

	body := wAuth.Body.String()

	// Deve conter tanto os itens ativos quanto os rejeitados no painel
	if !strings.Contains(body, "Trecho pericial da página 42") {
		t.Errorf("trecho ativo ausente no painel admin")
	}
	if !strings.Contains(body, "Trecho desaprovado na página 1") {
		t.Errorf("trecho rejeitado ausente no painel admin")
	}
	if !strings.Contains(body, "Uso Desaprovado") {
		t.Errorf("badge de uso desaprovado ausente no painel admin")
	}

	// 3. Inspeção de fonte que só possui itens em quarentena deve funcionar normalmente no admin
	reqQuar := httptest.NewRequest(http.MethodGet, "/admin/fontes/src-quar-only", nil)
	reqQuar.AddCookie(sessionCookie)
	wQuar := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wQuar, reqQuar)

	if wQuar.Code != http.StatusOK {
		t.Fatalf("GET /admin/fontes/src-quar-only autenticado retornou %d, esperado 200", wQuar.Code)
	}
	if !strings.Contains(wQuar.Body.String(), "Trecho sob quarentena") {
		t.Errorf("trecho sob quarentena ausente na inspeção administrativa")
	}
}
