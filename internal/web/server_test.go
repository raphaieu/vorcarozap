package web_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
	"github.com/xuri/excelize/v2"
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
		Port:             8080,
		Env:              "test",
		DBPath:           dbPath,
		PublicDataCutoff: "2026-09-03",
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     10 * time.Second,
		IdleTimeout:      60 * time.Second,
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
	if !strings.Contains(body, "Métricas da Rede Documental") {
		t.Errorf("página não contém título de métricas da rede documental")
	}
	if !strings.Contains(body, "Monitoramento automático ainda não ativado") {
		t.Errorf("página não contém aviso neutro de monitoramento ainda não ativado")
	}
	if !strings.Contains(body, "03/09/2026") {
		t.Errorf("página não contém data de corte formatada")
	}

	// Verifica headers de segurança
	if h := w.Header().Get("X-Content-Type-Options"); h != "nosniff" {
		t.Errorf("X-Content-Type-Options: esperado 'nosniff', obtido %q", h)
	}
	if h := w.Header().Get("X-Frame-Options"); h != "DENY" {
		t.Errorf("X-Frame-Options: esperado 'DENY', obtido %q", h)
	}
}

func TestMethodologyEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/metodologia", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Metodologia e Critérios Editoriais") {
		t.Errorf("título da metodologia não encontrado")
	}
	if !strings.Contains(body, "Aviso Editorial e de Independência") {
		t.Errorf("aviso editorial não encontrado na metodologia")
	}
	// Escala A–E
	for _, g := range []string{"Grau A", "Grau B", "Grau C", "Grau D", "Grau E"} {
		if !strings.Contains(body, g) {
			t.Errorf("menção ao %s não encontrada na metodologia", g)
		}
	}
	// Relevância 1–5
	if !strings.Contains(body, "1 — Local ou circunstancial") || !strings.Contains(body, "5 — Estratégica ou internacional") {
		t.Errorf("níveis de relevância 1 a 5 não encontrados na metodologia")
	}
	// Regras de métricas e status de fontes
	if !strings.Contains(body, "metric_eligible") {
		t.Errorf("explicação de elegibilidade métrica não encontrada")
	}
	if !strings.Contains(body, "not_checked") {
		t.Errorf("explicação de status de fonte not_checked não encontrada")
	}
	// Política de contraditório
	if !strings.Contains(body, "Direito de Resposta") {
		t.Errorf("seção de direito de resposta não encontrada")
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
		Port:             8080,
		Env:              "test",
		DBPath:           dbPath,
		PublicDataCutoff: "2026-09-03",
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     10 * time.Second,
		IdleTimeout:      60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	return srv, db
}

func TestHomeMetricsRendering(t *testing.T) {
	srv, _ := setupTestServerWithData(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", w.Code)
	}

	body := w.Body.String()

	// 1. Visão Geral e Rede Elegível
	if !strings.Contains(body, "Entidades na Rede") || !strings.Contains(body, "Alegações Elegíveis") {
		t.Errorf("cards de rede qualificada não renderizados")
	}
	if !strings.Contains(body, "Fontes Citadas") {
		t.Errorf("card de fontes citadas não renderizado")
	}
	if !strings.Contains(body, "Total de Entidades") || !strings.Contains(body, "Total de Alegações") {
		t.Errorf("cards de totais públicos não renderizados")
	}

	// 2. Distribuições estatísticas com links de filtro
	if !strings.Contains(body, "Alegações por Grau Probatório (A–E)") {
		t.Errorf("seção de graus não renderizada")
	}
	if !strings.Contains(body, `href="/pessoas?grade=A"`) {
		t.Errorf("link para filtro de Grau A não encontrado: %s", body)
	}
	if !strings.Contains(body, `href="/pessoas?grade=B"`) {
		t.Errorf("link para filtro de Grau B não encontrado: %s", body)
	}

	// Relevância
	if !strings.Contains(body, "Entidades por Relevância Pública (1–5)") {
		t.Errorf("seção de relevância não renderizada")
	}
	if !strings.Contains(body, `href="/pessoas?relevance=5"`) {
		t.Errorf("link para relevância 5 não encontrado: %s", body)
	}

	// Categoria
	if !strings.Contains(body, "Entidades por Categoria") {
		t.Errorf("seção de categorias não renderizada")
	}
	if !strings.Contains(body, `href="/pessoas?category=Politica"`) {
		t.Errorf("link para categoria Politica não encontrado: %s", body)
	}

	// 3. Alegações Recentes
	if !strings.Contains(body, "Alegações Recentes na Rede") {
		t.Errorf("seção de alegações recentes não renderizada")
	}
	if !strings.Contains(body, "Alice Santos &amp; Cia") && !strings.Contains(body, "Alice Santos") {
		t.Errorf("recente da Alice não encontrada na home")
	}
	if !strings.Contains(body, "Bob Silveira") {
		t.Errorf("recente do Bob não encontrada na home")
	}
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

func TestPublicPaginationAndQueryPreservation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_pagination_test.db")

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

	// Criar caso e 18 entidades públicas (DefaultPageSize = 15 => 2 páginas)
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-master', 'Caso Banco Master', 'caso-banco-master', 'Investigação');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-main', 'Fonte Principal', 'Editora', 'https://noticias.com/art', 'https://noticias.com/art', 'article', 'reachable');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir base: %v", err)
	}

	for i := 1; i <= 18; i++ {
		entID := fmt.Sprintf("ent-pag-%02d", i)
		slug := fmt.Sprintf("pessoa-pag-%02d", i)
		name := fmt.Sprintf("Pessoa Paginação %02d", i)
		relID := fmt.Sprintf("rel-pag-%02d", i)
		clmID := fmt.Sprintf("clm-pag-%02d", i)
		evID := fmt.Sprintf("ev-pag-%02d", i)
		esID := fmt.Sprintf("es-pag-%02d", i)

		_, err = db.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
			VALUES ('%s', 'person', '%s', '%s', '%s', 'Função', 'Resumo', 3, 'Justificativa', 'Politica', 'Nacional');

			INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
			VALUES ('%s', '%s', 'case-master', 'contato', 'Resumo', '');

			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
			VALUES ('%s', '%s', 'Proposição', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

			INSERT INTO evidence (id, claim_id, summary, evidence_type)
			VALUES ('%s', '%s', 'Evidência', 'document');

			INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
			VALUES ('%s', '%s', 'src-main', 'supports', 'Citação suporte', '', 'active');
		`, entID, name, strings.ToLower(name), slug, relID, entID, clmID, relID, evID, clmID, esID, evID))
		if err != nil {
			t.Fatalf("falha ao inserir pessoa %d: %v", i, err)
		}
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
		t.Fatalf("falha ao criar servidor: %v", err)
	}

	// 1. Página 1 com filtro preservado
	req := httptest.NewRequest(http.MethodGet, "/pessoas?category=Politica&grade=A&sort=name", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status página 1: esperado 200, obtido %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Página 1 de 2") {
		t.Errorf("status de paginação esperado 'Página 1 de 2', obtido no body")
	}
	// O link da próxima página deve preservar os filtros: category, grade, sort e ter page=2
	if !strings.Contains(body, `href="/pessoas?category=Politica&amp;grade=A&amp;page=2&amp;sort=name"`) &&
		!strings.Contains(body, `href="/pessoas?category=Politica&grade=A&page=2&sort=name"`) {
		// Pode estar codificado com &amp; ou &
		if !strings.Contains(body, "page=2") || !strings.Contains(body, "category=Politica") {
			t.Errorf("link para próxima página não preservou query params: %s", body)
		}
	}

	// 2. Página 2
	req2 := httptest.NewRequest(http.MethodGet, "/pessoas?category=Politica&grade=A&sort=name&page=2", nil)
	w2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("status página 2: esperado 200, obtido %d", w2.Code)
	}
	body2 := w2.Body.String()
	if !strings.Contains(body2, "Página 2 de 2") {
		t.Errorf("status de paginação esperado 'Página 2 de 2'")
	}
	if !strings.Contains(body2, "Anterior") {
		t.Errorf("botão Anterior ausente na página 2")
	}
}

func TestSecurityAndXSSProtection(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_xss_test.db")

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

	// Inserir payload com potencial XSS
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-xss', 'Caso <script>alert(1)</script>', 'caso-xss', 'Desc');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-xss', 'person', 'Malicious <script>alert("xss")</script>', 'malicious xss', 'malicious-xss', 'Cargo <b>audacioso</b>', 'Resumo', 4, 'Justificativa', 'Politica', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-xss', 'ent-xss', 'case-xss', 'contato', 'Resumo', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-xss', 'rel-xss', 'Proposição com <img src=x onerror=alert(2)>', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-xss', 'clm-xss', 'Evidência', 'document');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-xss', 'Fonte <iframe src="evil.com">', 'Autor', 'https://safe.com', 'https://safe.com', 'article', 'reachable');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-xss', 'ev-xss', 'src-xss', 'supports', 'Citação <script>alert(3)</script>', '', 'active');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir dados maliciosos: %v", err)
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
		t.Fatalf("falha ao criar servidor: %v", err)
	}

	// 1. Verificar listagem /pessoas
	reqList := httptest.NewRequest(http.MethodGet, "/pessoas", nil)
	wList := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wList, reqList)

	bodyList := wList.Body.String()
	if strings.Contains(bodyList, "<script>alert") {
		t.Errorf("XSS detectado sem escape na listagem /pessoas: %s", bodyList)
	}
	if !strings.Contains(bodyList, "&lt;script&gt;alert") {
		t.Errorf("Esperado HTML escape de script tag na listagem")
	}

	// 2. Verificar detalhe /pessoas/malicious-xss
	reqDetail := httptest.NewRequest(http.MethodGet, "/pessoas/malicious-xss", nil)
	wDetail := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wDetail, reqDetail)

	bodyDetail := wDetail.Body.String()
	if strings.Contains(bodyDetail, "<script>") || strings.Contains(bodyDetail, "<iframe") || strings.Contains(bodyDetail, "<img src=x") {
		t.Errorf("Tags perigosas não escapadas no detalhe da entidade: %s", bodyDetail)
	}

	// 3. Verificar headers de segurança em todas as páginas
	endpoints := []string{"/", "/pessoas", "/pessoas/malicious-xss", "/metodologia"}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if h := w.Header().Get("X-Content-Type-Options"); h != "nosniff" {
			t.Errorf("[%s] X-Content-Type-Options: esperado 'nosniff', obtido %q", ep, h)
		}
		if h := w.Header().Get("X-Frame-Options"); h != "DENY" {
			t.Errorf("[%s] X-Frame-Options: esperado 'DENY', obtido %q", ep, h)
		}
	}
}

func TestCanonicalVisibilityRules(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_visibility_test.db")

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

	// Inserir entidades testando diferentes regras de visibilidade
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-v', 'Caso Visibilidade', 'caso-visibilidade', 'Desc');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-v', 'Fonte V', 'Autor', 'https://v.com', 'https://v.com', 'article', 'reachable');

		-- 1. Entidade com claim 'rejected' (não deve aparecer)
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-rej', 'person', 'Pessoa Rejeitada', 'pessoa rejeitada', 'pessoa-rejeitada', 'Papel', 'Resumo', 3, 'Just', 'Politica', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-rej', 'ent-rej', 'case-v', 'contato', 'Resumo', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-rej', 'rel-rej', 'Proposição', '', 'curated_seed', 'A', 'supports_link', 1, 'rejected', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-rej', 'clm-rej', 'Evidência', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-rej', 'ev-rej', 'src-v', 'supports', 'Citação', '', 'active');

		-- 2. Entidade com claim 'archived' (não deve aparecer)
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-arc', 'person', 'Pessoa Arquivada', 'pessoa arquivada', 'pessoa-arquivada', 'Papel', 'Resumo', 3, 'Just', 'Politica', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-arc', 'ent-arc', 'case-v', 'contato', 'Resumo', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-arc', 'rel-arc', 'Proposição', '', 'curated_seed', 'B', 'supports_link', 1, 'archived', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-arc', 'clm-arc', 'Evidência', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-arc', 'ev-arc', 'src-v', 'supports', 'Citação', '', 'active');

		-- 3. Entidade com evidence_source com status 'rejected' (não é active, não deve aparecer)
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-es-rej', 'person', 'Pessoa Fonte Rejeitada', 'pessoa fonte rejeitada', 'pessoa-fonte-rejeitada', 'Papel', 'Resumo', 3, 'Just', 'Politica', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-es-rej', 'ent-es-rej', 'case-v', 'contato', 'Resumo', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-es-rej', 'rel-es-rej', 'Proposição', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-es-rej', 'clm-es-rej', 'Evidência', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-es-rej', 'ev-es-rej', 'src-v', 'supports', 'Citação', '', 'rejected');

		-- 4. Entidade legítima publicada (deve aparecer)
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-ok', 'person', 'Pessoa Visível OK', 'pessoa visivel ok', 'pessoa-visivel-ok', 'Papel', 'Resumo', 4, 'Just', 'Empresarial', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-ok', 'ent-ok', 'case-v', 'contato', 'Resumo', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-ok', 'rel-ok', 'Proposição legítima', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-ok', 'clm-ok', 'Evidência', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-ok', 'ev-ok', 'src-v', 'supports', 'Citação de suporte ativa', '', 'active');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir dados: %v", err)
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
		t.Fatalf("falha ao criar servidor: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/pessoas", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", w.Code)
	}
	body := w.Body.String()

	// Pessoa OK deve aparecer
	if !strings.Contains(body, "Pessoa Visível OK") {
		t.Errorf("Pessoa Visível OK não encontrada na listagem")
	}

	// Não devem aparecer:
	if strings.Contains(body, "Pessoa Rejeitada") {
		t.Errorf("Pessoa Rejeitada (status=rejected) não deveria aparecer")
	}
	if strings.Contains(body, "Pessoa Arquivada") {
		t.Errorf("Pessoa Arquivada (status=archived) não deveria aparecer")
	}
	if strings.Contains(body, "Pessoa Fonte Rejeitada") {
		t.Errorf("Pessoa Fonte Rejeitada (evidence_source status=rejected) não deveria aparecer")
	}

	// Teste detalhe direto: 404 para entidades não públicas
	for _, nonPublicSlug := range []string{"pessoa-rejeitada", "pessoa-arquivada", "pessoa-fonte-rejeitada"} {
		reqSlug := httptest.NewRequest(http.MethodGet, "/pessoas/"+nonPublicSlug, nil)
		wSlug := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wSlug, reqSlug)

		if wSlug.Code != http.StatusNotFound {
			t.Errorf("[%s] esperado 404 para entidade sem alegações válidas, obtido %d", nonPublicSlug, wSlug.Code)
		}
	}
}

func TestExportEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	// 1. Testa download do XLSX em /exportar/base.xlsx
	req := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status /exportar/base.xlsx: esperado 200, obtido %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	expectedCT := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if contentType != expectedCT {
		t.Errorf("Content-Type: esperado %q, obtido %q", expectedCT, contentType)
	}

	contentDisp := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisp, "attachment; filename=\"vorcarozap-dados-publicos-") {
		t.Errorf("Content-Disposition inesperado: %q", contentDisp)
	}

	// Valida que o payload retornado é um arquivo XLSX válido
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("payload retornado por /exportar/base.xlsx não é um XLSX válido: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) < 4 {
		t.Errorf("esperava pelo menos 4 abas no XLSX exportado, obteve %d (%v)", len(sheets), sheets)
	}

	// 2. Testa redirect de /exportar para /exportar/base.xlsx
	reqRedir := httptest.NewRequest(http.MethodGet, "/exportar", nil)
	wRedir := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wRedir, reqRedir)

	if wRedir.Code != http.StatusTemporaryRedirect {
		t.Errorf("status /exportar: esperado 307, obtido %d", wRedir.Code)
	}
	loc := wRedir.Header().Get("Location")
	if loc != "/exportar/base.xlsx" {
		t.Errorf("Location de redirect: esperado '/exportar/base.xlsx', obtido %q", loc)
	}

	// 3. Testa presença do link de exportação em /pessoas
	reqPessoas := httptest.NewRequest(http.MethodGet, "/pessoas", nil)
	wPessoas := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPessoas, reqPessoas)

	if wPessoas.Code != http.StatusOK {
		t.Fatalf("status /pessoas: esperado 200, obtido %d", wPessoas.Code)
	}
	if !strings.Contains(wPessoas.Body.String(), "/exportar/base.xlsx") {
		t.Errorf("link para /exportar/base.xlsx não encontrado no HTML de /pessoas")
	}

	// 4. Testa presença do link de exportação na Home
	reqHome := httptest.NewRequest(http.MethodGet, "/", nil)
	wHome := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wHome, reqHome)

	if wHome.Code != http.StatusOK {
		t.Fatalf("status /: esperado 200, obtido %d", wHome.Code)
	}
	if !strings.Contains(wHome.Body.String(), "/exportar/base.xlsx") {
		t.Errorf("link para /exportar/base.xlsx não encontrado no HTML da Home")
	}
}

func TestExportEndpoint_ErrorHandling(t *testing.T) {
	// Cria servidor de teste com banco válido inicial
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("falha ao abrir sqlite em memória: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao rodar migrations: %v", err)
	}

	cfg := &config.Config{
		Port:             8080,
		Env:              "test",
		PublicDataCutoff: "2026-09-03",
	}
	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	// Fecha intencionalmente o banco para forçar falha no início da transação de exportação
	_ = db.Close()

	req := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	// Valida que retornou HTTP 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status de falha: esperado 500, obtido %d", w.Code)
	}

	// Valida que NÃO enviou cabeçalho de anexo
	if cd := w.Header().Get("Content-Disposition"); cd != "" {
		t.Errorf("Content-Disposition deve ser vazio em caso de erro, obtido %q", cd)
	}

	// Valida mensagem de erro no corpo
	if !strings.Contains(w.Body.String(), "Erro interno ao gerar planilha de exportação") {
		t.Errorf("corpo da resposta de erro inesperado: %s", w.Body.String())
	}
}

var (
	testAdminHashCost12Once sync.Once
	testAdminHashCost12     string
)

func getTestAdminHashCost12(t *testing.T) string {
	t.Helper()
	testAdminHashCost12Once.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("password"), 12)
		if err != nil {
			panic(fmt.Sprintf("falha ao gerar hash bcrypt no teste: %v", err))
		}
		testAdminHashCost12 = string(h)
	})
	return testAdminHashCost12
}

func setupAdminTestServer(t *testing.T, user string) (*http.Server, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_admin_test.db")

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

	passwordHash := getTestAdminHashCost12(t)

	cfg := &config.Config{
		Port:              8080,
		Env:               "test",
		DBPath:            dbPath,
		PublicDataCutoff:  "2026-09-03",
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		AdminUser:         user,
		AdminPasswordHash: passwordHash,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web com admin: %v", err)
	}

	return srv, passwordHash
}

func TestAdminDisabled_Returns404(t *testing.T) {
	// Servidor padrão sem AdminUser e AdminPasswordHash configurados
	srv := setupTestServer(t)

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
	paths := []string{"/admin", "/admin/", "/admin/inexistente"}

	for _, method := range methods {
		for _, p := range paths {
			t.Run(method+" "+p, func(t *testing.T) {
				req := httptest.NewRequest(method, p, nil)
				w := httptest.NewRecorder()
				srv.Handler.ServeHTTP(w, req)

				if w.Code != http.StatusNotFound {
					t.Errorf("rota %s %s com admin desabilitado: esperado status 404, obtido %d", method, p, w.Code)
				}
				// Não deve emitir cabeçalho WWW-Authenticate quando a rota administrativa está desabilitada
				if auth := w.Header().Get("WWW-Authenticate"); auth != "" {
					t.Errorf("WWW-Authenticate não deve estar presente quando admin está desabilitado, obtido %q", auth)
				}
			})
		}
	}

	// Rotas públicas continuam funcionando normalmente
	reqHome := httptest.NewRequest(http.MethodGet, "/", nil)
	wHome := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wHome, reqHome)
	if wHome.Code != http.StatusOK {
		t.Errorf("status / público: esperado 200, obtido %d", wHome.Code)
	}
}

func TestAdminEnabled_BasicAuth_Unauthenticated_AllMethodsAndPaths(t *testing.T) {
	srv, _ := setupAdminTestServer(t, "admin")

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
	paths := []string{"/admin", "/admin/", "/admin/rota-inexistente"}

	for _, method := range methods {
		for _, p := range paths {
			t.Run(method+" "+p, func(t *testing.T) {
				req := httptest.NewRequest(method, p, nil)
				w := httptest.NewRecorder()
				srv.Handler.ServeHTTP(w, req)

				if w.Code != http.StatusUnauthorized {
					t.Errorf("status para %s %s sem credenciais: esperado 401, obtido %d", method, p, w.Code)
				}

				expectedWWWAuth := `Basic realm="VorcaroZAP admin", charset="UTF-8"`
				if auth := w.Header().Get("WWW-Authenticate"); auth != expectedWWWAuth {
					t.Errorf("WWW-Authenticate: esperado %q, obtido %q", expectedWWWAuth, auth)
				}

				if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
					t.Errorf("Cache-Control: esperado 'no-store', obtido %q", cc)
				}

				if vary := w.Header().Get("Vary"); vary != "Authorization" {
					t.Errorf("Vary: esperado 'Authorization', obtido %q", vary)
				}

				if !strings.Contains(w.Body.String(), "Unauthorized") {
					t.Errorf("corpo da resposta de não autorizado deve conter 'Unauthorized', obtido %q", w.Body.String())
				}
			})
		}
	}
}

func TestAdminEnabled_BasicAuth_EqualityBetweenFailures(t *testing.T) {
	srv, _ := setupAdminTestServer(t, "admin")

	// 1. Falha por usuário incorreto
	reqWrongUser := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqWrongUser.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("wronguser:password")))
	wWrongUser := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wWrongUser, reqWrongUser)

	// 2. Falha por senha incorreta
	reqWrongPass := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqWrongPass.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:wrongpassword")))
	wWrongPass := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wWrongPass, reqWrongPass)

	// 3. Falha por ambos incorretos
	reqBothWrong := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqBothWrong.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("wronguser:wrongpassword")))
	wBothWrong := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wBothWrong, reqBothWrong)

	// Validação de paridade estrita: status, corpo e cabeçalhos idênticos
	if wWrongUser.Code != http.StatusUnauthorized || wWrongPass.Code != http.StatusUnauthorized || wBothWrong.Code != http.StatusUnauthorized {
		t.Fatalf("todos devem retornar 401: obtidos %d, %d, %d", wWrongUser.Code, wWrongPass.Code, wBothWrong.Code)
	}

	if wWrongUser.Body.String() != wWrongPass.Body.String() || wWrongUser.Body.String() != wBothWrong.Body.String() {
		t.Errorf("corpo da resposta difere entre falhas de autenticação: wrongUser=%q, wrongPass=%q, bothWrong=%q",
			wWrongUser.Body.String(), wWrongPass.Body.String(), wBothWrong.Body.String())
	}

	if wWrongUser.Header().Get("WWW-Authenticate") != wWrongPass.Header().Get("WWW-Authenticate") {
		t.Errorf("cabeçalho WWW-Authenticate difere entre usuário errado (%q) e senha errada (%q)",
			wWrongUser.Header().Get("WWW-Authenticate"), wWrongPass.Header().Get("WWW-Authenticate"))
	}
	if wWrongUser.Header().Get("Cache-Control") != wWrongPass.Header().Get("Cache-Control") {
		t.Errorf("cabeçalho Cache-Control difere")
	}
	if wWrongUser.Header().Get("Vary") != wWrongPass.Header().Get("Vary") {
		t.Errorf("cabeçalho Vary difere")
	}

	// 4. Testes com outros formatos malformados
	malformedHeaders := []struct {
		name       string
		authHeader string
	}{
		{"esquema Bearer não permitido", "Bearer secret-token"},
		{"base64 malformado", "Basic !!!invalid_base64!!!"},
		{"base64 sem separador de dois pontos", "Basic " + base64.StdEncoding.EncodeToString([]byte("adminpassword"))},
		{"usuário vazio", "Basic " + base64.StdEncoding.EncodeToString([]byte(":password"))},
	}

	for _, tt := range malformedHeaders {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("status para %s: esperado 401, obtido %d", tt.name, w.Code)
			}
			if auth := w.Header().Get("WWW-Authenticate"); auth != `Basic realm="VorcaroZAP admin", charset="UTF-8"` {
				t.Errorf("WWW-Authenticate incorreto: %q", auth)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control: esperado 'no-store', obtido %q", cc)
			}
			if vary := w.Header().Get("Vary"); vary != "Authorization" {
				t.Errorf("Vary: esperado 'Authorization', obtido %q", vary)
			}
		})
	}
}

func TestAdminEnabled_BasicAuth_ValidCredentials(t *testing.T) {
	srv, validHash := setupAdminTestServer(t, "admin")

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. Testa GET /admin
	reqAdmin := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqAdmin.Header.Set("Authorization", authHeader)
	wAdmin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAdmin, reqAdmin)

	if wAdmin.Code != http.StatusOK {
		t.Fatalf("status /admin com credencial válida: esperado 200, obtido %d", wAdmin.Code)
	}

	if cc := wAdmin.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control em resposta autenticada: esperado 'no-store', obtido %q", cc)
	}
	if vary := wAdmin.Header().Get("Vary"); vary != "Authorization" {
		t.Errorf("Vary em resposta autenticada: esperado 'Authorization', obtido %q", vary)
	}
	if ct := wAdmin.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: esperado 'text/html', obtido %q", ct)
	}

	body := wAdmin.Body.String()
	if !strings.Contains(body, "Painel Administrativo") {
		t.Errorf("HTML deve conter 'Painel Administrativo', obtido: %s", body)
	}
	if !strings.Contains(body, "Autenticado") {
		t.Errorf("HTML deve conter 'Autenticado', obtido: %s", body)
	}
	if !strings.Contains(body, "VZ-015") || !strings.Contains(body, "VZ-016") {
		t.Errorf("HTML deve mencionar as fases VZ-015 e VZ-016, obtido: %s", body)
	}

	// Garante que nenhum hash ou dado sensível aparece no corpo
	if strings.Contains(body, validHash) {
		t.Errorf("vazamento de segurança: hash da senha encontrado no corpo da resposta HTML")
	}
	if strings.Contains(body, "password") {
		t.Errorf("vazamento de segurança: senha encontrada no corpo da resposta HTML")
	}

	// 2. Testa GET /admin/
	reqAdminSlash := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	reqAdminSlash.Header.Set("Authorization", authHeader)
	wAdminSlash := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAdminSlash, reqAdminSlash)

	if wAdminSlash.Code != http.StatusOK {
		t.Fatalf("status /admin/ com credencial válida: esperado 200, obtido %d", wAdminSlash.Code)
	}

	// 3. Testa subrota inexistente autenticada /admin/inexistente -> 404 protegido
	reqInexistente := httptest.NewRequest(http.MethodGet, "/admin/inexistente", nil)
	reqInexistente.Header.Set("Authorization", authHeader)
	wInexistente := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wInexistente, reqInexistente)

	if wInexistente.Code != http.StatusNotFound {
		t.Errorf("status /admin/inexistente autenticado: esperado 404, obtido %d", wInexistente.Code)
	}
	if cc := wInexistente.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control em subrota 404 autenticada: esperado 'no-store', obtido %q", cc)
	}
	if vary := wInexistente.Header().Get("Vary"); vary != "Authorization" {
		t.Errorf("Vary em subrota 404 autenticada: esperado 'Authorization', obtido %q", vary)
	}

	// 4. Testa métodos não implementados autenticados (POST, PUT, PATCH, DELETE, OPTIONS)
	unimplementedMethods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}
	for _, m := range unimplementedMethods {
		t.Run("autenticado "+m+" /admin", func(t *testing.T) {
			reqM := httptest.NewRequest(m, "/admin", nil)
			reqM.Header.Set("Authorization", authHeader)
			wM := httptest.NewRecorder()
			srv.Handler.ServeHTTP(wM, reqM)

			if wM.Code != http.StatusNotFound && wM.Code != http.StatusMethodNotAllowed {
				t.Errorf("status para %s /admin autenticado: esperado 404/405, obtido %d", m, wM.Code)
			}
			if cc := wM.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control: esperado 'no-store', obtido %q", cc)
			}
			if vary := wM.Header().Get("Vary"); vary != "Authorization" {
				t.Errorf("Vary: esperado 'Authorization', obtido %q", vary)
			}
			// Não pode retornar o HTML do painel administrativo
			if strings.Contains(wM.Body.String(), "Painel Administrativo") {
				t.Errorf("método %s não implementado não pode renderizar o painel administrativo", m)
			}
		})
	}
}

func TestAdminEnabled_NoBypass(t *testing.T) {
	srv, _ := setupAdminTestServer(t, "admin")

	tests := []struct {
		name    string
		method  string
		url     string
		headers map[string]string
	}{
		{
			name:   "bypass por cabeçalho X-Forwarded-For",
			method: http.MethodGet,
			url:    "/admin",
			headers: map[string]string{
				"X-Forwarded-For": "127.0.0.1",
			},
		},
		{
			name:   "bypass por cabeçalho X-Real-IP",
			method: http.MethodGet,
			url:    "/admin",
			headers: map[string]string{
				"X-Real-IP": "127.0.0.1",
			},
		},
		{
			name:   "bypass por cabeçalho X-Remote-User",
			method: http.MethodGet,
			url:    "/admin",
			headers: map[string]string{
				"X-Remote-User": "admin",
			},
		},
		{
			name:   "bypass por cabeçalho X-Forwarded-User",
			method: http.MethodGet,
			url:    "/admin",
			headers: map[string]string{
				"X-Forwarded-User": "admin",
			},
		},
		{
			name:   "bypass por cabeçalho X-Admin",
			method: http.MethodGet,
			url:    "/admin",
			headers: map[string]string{
				"X-Admin": "true",
			},
		},
		{
			name:   "bypass por query params",
			method: http.MethodGet,
			url:    "/admin?user=admin&pass=password",
		},
		{
			name:   "método POST não autenticado",
			method: http.MethodPost,
			url:    "/admin",
		},
		{
			name:   "método PUT não autenticado",
			method: http.MethodPut,
			url:    "/admin",
		},
		{
			name:   "método PATCH não autenticado",
			method: http.MethodPatch,
			url:    "/admin",
		},
		{
			name:   "método DELETE não autenticado",
			method: http.MethodDelete,
			url:    "/admin",
		},
		{
			name:   "método OPTIONS não autenticado",
			method: http.MethodOptions,
			url:    "/admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			// Nunca deve autorizar (status != 200)
			if w.Code == http.StatusOK {
				t.Errorf("vulnerabilidade de segurança: bypass bem-sucedido em %s (status 200 retornado)", tt.name)
			}
			// Todas as requisições sob /admin sem auth devem receber 401
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status esperado 401 em tentativa de bypass %s: obtido %d", tt.name, w.Code)
			}
			if auth := w.Header().Get("WWW-Authenticate"); auth != `Basic realm="VorcaroZAP admin", charset="UTF-8"` {
				t.Errorf("WWW-Authenticate esperado em tentativa de bypass: obtido %q", auth)
			}
		})
	}
}
