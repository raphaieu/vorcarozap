package web_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupTestServer(t *testing.T) *http.Server {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha nas migrations: %v", err)
	}

	cfg := &config.Config{
		Port:         8080,
		Env:          "test",
		DBPath:       dbPath,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	return srv
}

func TestHealthLiveEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"status":"live"`) {
		t.Errorf("body inesperado: %s", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type inesperado: %s", ct)
	}
}

func TestHealthReadyEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"status":"ready"`) {
		t.Errorf("body inesperado: %s", body)
	}
}

func TestHomeEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "VorcaroZAP") {
		t.Errorf("página não contém nome do projeto 'VorcaroZAP'")
	}
	if !strings.Contains(body, "Aviso Editorial e de Independência") {
		t.Errorf("página não contém aviso editorial obrigatório")
	}
	if !strings.Contains(body, "Fase 1") {
		t.Errorf("página não contém menção à Fase 1")
	}

	// Verifica headers de segurança
	if h := w.Header().Get("X-Content-Type-Options"); h != "nosniff" {
		t.Errorf("X-Content-Type-Options: esperado 'nosniff', obtido %q", h)
	}
	if h := w.Header().Get("X-Frame-Options"); h != "DENY" {
		t.Errorf("X-Frame-Options: esperado 'DENY', obtido %q", h)
	}
}

func TestStaticCSSEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Errorf("Content-Type inesperado: %s", ct)
	}
}

func setupTestServerWithData(t *testing.T) (*http.Server, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha nas migrations: %v", err)
	}

	// Inserir dados de teste conforme o schema editorial
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-master', 'Caso Banco Master', 'caso-banco-master', 'Investigação e apurações');

		-- 1. Alice: Entidade pública com claim publicado e fonte de suporte
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-1', 'person', 'Alice Santos & Cia', 'alice santos & cia', 'alice-santos', 'Senadora', 'Resumo Alice', 5, 'Figura pública', 'Politica', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-1', 'ent-1', 'case-master', 'contato', 'Registro de contato', 'Sem limites adicionais');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-1', 'rel-1', 'Alice manteve conversas documentadas', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-1', 'clm-1', 'Evidência documental de contato', 'document');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-1', 'Folha de S.Paulo', 'Redação', 'https://folha.com.br/artigo1', 'https://folha.com.br/artigo1', 'article', 'reachable');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-2', 'Nota Oficial da Assessoria', 'Assessoria', 'https://assessoria.gov.br/nota', 'https://assessoria.gov.br/nota', 'official_statement', 'reachable');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-1', 'ev-1', 'src-1', 'supports', 'Trecho comprovando contato', 'Página 2', 'active');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-2', 'ev-1', 'src-2', 'contradicts', 'Nota oficial negando', 'Parágrafo 1', 'active');

		-- 2. Bob: Entidade pública com citação vazia
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-2', 'person', 'Bob Silveira', 'bob silveira', 'bob-silveira', 'Diretor', 'Resumo Bob', 3, 'Executivo citado', 'Empresarial', 'Estadual');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-2', 'ent-2', 'case-master', 'contato', 'Contato de negócios', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-2', 'rel-2', 'Bob enviou mensagens', '', 'curated_seed', 'B', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-2', 'clm-2', 'Troca de mensagens', 'message');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-3', 'ev-2', 'src-1', 'supports', '', '', 'active');

		-- 3. Carlos: Entidade em quarentena (não deve aparecer)
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-3', 'person', 'Carlos Quarentena', 'carlos quarentena', 'carlos-quarentena', 'Assessor', 'Resumo Carlos', 2, 'Assessor', 'Outros', 'Local');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-3', 'ent-3', 'case-master', 'mencao', 'Menção indireta', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-3', 'rel-3', 'Carlos mencionado de passagem', '', 'curated_seed', 'E', 'possible_link', 0, 'quarantined', 'quarantined', '["UNKNOWN_LEGACY_GRADE"]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-3', 'clm-3', 'Evidência em quarentena', 'mention');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-4', 'ev-3', 'src-1', 'supports', 'Citação qualquer', '', 'active');

		-- 4. Diana: Entidade com claim publicado mas SEM fonte supports ativa (não deve aparecer)
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-4', 'person', 'Diana Sem Suporte', 'diana sem suporte', 'diana-sem-suporte', 'Consultora', 'Resumo Diana', 1, 'Consultora', 'Outros', 'Local');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-4', 'ent-4', 'case-master', 'contato', 'Contato não suportado', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-4', 'rel-4', 'Diana apenas contestou', '', 'curated_seed', 'C', 'contradicts_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-4', 'clm-4', 'Evidência sem suporte', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-5', 'ev-4', 'src-1', 'contradicts', 'Trecho de contestação', '', 'active');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados de teste: %v", err)
	}

	cfg := &config.Config{
		Port:         8080,
		Env:          "test",
		DBPath:       dbPath,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	return srv, db
}

func TestPublicEntitiesList(t *testing.T) {
	srv, _ := setupTestServerWithData(t)

	// GET /pessoas lista pública
	req := httptest.NewRequest(http.MethodGet, "/pessoas", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", w.Code)
	}

	body := w.Body.String()
	// Alice Santos e Bob Silveira devem aparecer (possuem claims publicados com suporte)
	// Deve escapar HTML (&amp;)
	if !strings.Contains(body, "Alice Santos &amp; Cia") {
		t.Errorf("Alice Santos & Cia não encontrada com escape na listagem")
	}
	if !strings.Contains(body, "Bob Silveira") {
		t.Errorf("Bob Silveira não encontrado na listagem")
	}

	// Carlos Quarentena (quarantined) NÃO deve aparecer
	if strings.Contains(body, "Carlos Quarentena") {
		t.Errorf("Carlos Quarentena (quarantined) não deveria aparecer na listagem pública")
	}

	// Diana Sem Suporte (sem supports ativo) NÃO deve aparecer
	if strings.Contains(body, "Diana Sem Suporte") {
		t.Errorf("Diana Sem Suporte não deveria aparecer na listagem pública")
	}

	// Teste com busca
	reqSearch := httptest.NewRequest(http.MethodGet, "/pessoas?q=Alice", nil)
	wSearch := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSearch, reqSearch)
	if wSearch.Code != http.StatusOK {
		t.Fatalf("busca status: esperado 200, obtido %d", wSearch.Code)
	}
	bodySearch := wSearch.Body.String()
	if !strings.Contains(bodySearch, "Alice Santos &amp; Cia") {
		t.Errorf("Alice Santos não encontrada na busca")
	}
	if strings.Contains(bodySearch, "Bob Silveira") {
		t.Errorf("Bob Silveira não deveria aparecer na busca por Alice")
	}

	// Teste com filtro de categoria
	reqCat := httptest.NewRequest(http.MethodGet, "/pessoas?category=Empresarial", nil)
	wCat := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wCat, reqCat)
	bodyCat := wCat.Body.String()
	if !strings.Contains(bodyCat, "Bob Silveira") {
		t.Errorf("Bob Silveira não encontrado no filtro Empresarial")
	}
	if strings.Contains(bodyCat, "Alice Santos") {
		t.Errorf("Alice Santos não deveria aparecer no filtro Empresarial")
	}

	// Teste com busca sem resultados
	reqEmpty := httptest.NewRequest(http.MethodGet, "/pessoas?q=NaoExisteNome", nil)
	wEmpty := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wEmpty, reqEmpty)
	bodyEmpty := wEmpty.Body.String()
	if !strings.Contains(bodyEmpty, "Nenhum resultado encontrado") {
		t.Errorf("esperado aviso de nenhum resultado encontrado")
	}
}

func TestPublicEntityDetail(t *testing.T) {
	srv, _ := setupTestServerWithData(t)

	// 1. Sucesso: /pessoas/alice-santos
	req := httptest.NewRequest(http.MethodGet, "/pessoas/alice-santos", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Alice Santos &amp; Cia") {
		t.Errorf("Nome de Alice não renderizado com escape")
	}
	if !strings.Contains(body, "Senadora") {
		t.Errorf("Papel 'Senadora' não renderizado")
	}
	if !strings.Contains(body, "Trecho comprovando contato") {
		t.Errorf("Citação de suporte não encontrada")
	}
	// Defesa / Contraditório segregado
	if !strings.Contains(body, "Defesa, Contestação ou Contexto") {
		t.Errorf("Seção de defesa/contestação não renderizada")
	}
	if !strings.Contains(body, "Nota oficial negando") {
		t.Errorf("Citação de defesa não encontrada")
	}

	// 2. Detalhe de Bob: citação vazia não deve renderizar <blockquote>
	reqBob := httptest.NewRequest(http.MethodGet, "/pessoas/bob-silveira", nil)
	wBob := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wBob, reqBob)

	if wBob.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", wBob.Code)
	}
	bodyBob := wBob.Body.String()
	if !strings.Contains(bodyBob, "Bob Silveira") {
		t.Errorf("Bob Silveira não encontrado")
	}
	// Não deve haver <blockquote> na citação vazia
	if strings.Contains(bodyBob, "source-quote") {
		t.Errorf("quote vazio de Bob não deveria renderizar elemento source-quote")
	}

	// 3. Entidade em quarentena deve retornar 404
	reqQuarantine := httptest.NewRequest(http.MethodGet, "/pessoas/carlos-quarentena", nil)
	wQuarantine := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wQuarantine, reqQuarantine)

	if wQuarantine.Code != http.StatusNotFound {
		t.Errorf("carlos-quarentena esperado 404, obtido %d", wQuarantine.Code)
	}

	// 4. Entidade inexistente deve retornar 404
	req404 := httptest.NewRequest(http.MethodGet, "/pessoas/inexistente", nil)
	w404 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Errorf("inexistente esperado 404, obtido %d", w404.Code)
	}
}
