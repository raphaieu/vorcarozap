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
		Port:               8080,
		Env:                "test",
		DBPath:             dbPath,
		PublicDataCutoff:   "2026-09-03",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        60 * time.Second,
		AdminUser:          user,
		AdminPasswordHash:  passwordHash,
		AdminAllowedOrigin: "http://example.com",
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
	paths := []string{"/admin", "/admin/", "/admin/candidatos", "/admin/candidatos/c-1", "/admin/evidencias", "/admin/fontes", "/admin/inexistente"}

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
	paths := []string{"/admin", "/admin/", "/admin/candidatos", "/admin/candidatos/c-1", "/admin/evidencias", "/admin/fontes", "/admin/rota-inexistente"}

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
	if !strings.Contains(body, "Dashboard Administrativo") {
		t.Errorf("HTML deve conter 'Dashboard Administrativo', obtido: %s", body)
	}
	if !strings.Contains(body, "Autenticado") {
		t.Errorf("HTML deve conter 'Autenticado', obtido: %s", body)
	}
	if !strings.Contains(body, "VZ-015") {
		t.Errorf("HTML deve mencionar a fase VZ-015, obtido: %s", body)
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

func setupAdminTestServerWithData(t *testing.T, user string) (*http.Server, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_admin_data_test.db")

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

	// Popula banco com dados administrativos de teste
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-1', 'Caso Banco Master', 'caso-banco-master', 'Investigação e apurações');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-pub-1', 'person', 'Alice Santos & Cia', 'alice santos & cia', 'alice-santos', 'Senadora', 'Resumo Alice', 5, 'Figura pública', 'Politica', 'Nacional'),
			('ent-quar-1', 'person', 'Carlos Quarentena', 'carlos quarentena', 'carlos-quarentena', 'Assessor', 'Resumo Carlos', 2, 'Assessor', 'Outros', 'Local');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES
			('rel-pub-1', 'ent-pub-1', 'case-1', 'contato', 'Registro de contato', 'Sem limites adicionais'),
			('rel-quar-1', 'ent-quar-1', 'case-1', 'mencao', 'Menção indireta', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES
			('clm-pub-1', 'rel-pub-1', 'Alice manteve conversas documentadas', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]'),
			('clm-quar-1', 'rel-quar-1', 'Carlos citado em relatório preliminar', '', 'openrouter', 'C', 'possible_link', 0, 'quarantined', 'quarantined', '["NEEDS_REVIEW"]'),
			('clm-rej-1', 'rel-quar-1', 'Carlos teria participado de reunião', '', 'openrouter', 'E', 'possible_link', 0, 'rejected', 'quarantined', '["REJECTED_GRADE_E"]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-pub-1', 'clm-pub-1', 'Evidência documental de contato', 'document'),
			('ev-quar-1', 'clm-quar-1', 'Menção em artigo', 'mention'),
			('ev-rej-1', 'clm-rej-1', 'Postagem de rede', 'social_media');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, http_status, normalized_error_code, source_access_checked_at)
		VALUES
			('src-1', 'Folha de S.Paulo', 'Redação Folha', 'https://folha.com.br/artigo1', 'https://folha.com.br/artigo1', 'article', 'reachable', 200, '', '2026-09-10 12:00:00'),
			('src-2', 'Site Indisponível', 'Autor Desconhecido', 'https://indisponivel.com/artigo', 'https://indisponivel.com/artigo', 'blog', 'unreachable', 503, 'ERR_HTTP_503', '2026-09-09 10:00:00'),
			('src-3', 'Fonte Não Verificada', 'Assessoria', 'https://naoverificada.com.br/doc', 'https://naoverificada.com.br/doc', 'official_statement', 'not_checked', NULL, '', NULL);

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES
			('es-1', 'ev-pub-1', 'src-1', 'supports', 'Trecho confirmando o encontro entre as partes', 'Página 4', 'active'),
			('es-2', 'ev-pub-1', 'src-2', 'contradicts', 'Nota oficial negando que tenha ocorrido reunião', 'Parágrafo 2', 'active'),
			('es-3', 'ev-quar-1', 'src-1', 'supports', 'Trecho citando Carlos de passagem', 'Pág 10', 'active'),
			('es-4', 'ev-rej-1', 'src-3', 'supports', 'Trecho rejeitado', '', 'rejected');

		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES
			('run-1', 'completed', 'Alice Santos', 'openai/gpt-4o', '2026-09-10 10:00:00'),
			('run-2', 'running', 'Banco Master', 'openai/gpt-4o', '2026-09-11 08:00:00');

		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			case_name, normalized_case_name, proposition, suggested_grade, source_url,
			canonical_url, excerpt, technical_confidence, editorial_status, is_duplicate,
			duplicate_reason, canonical_candidate_id, resolved_subject_entity_id, resolved_case_id,
			published_claim_id, structural_gate_passed, structural_gate_reasons, semantic_gate_passed,
			semantic_gate_reasons, created_at, updated_at
		) VALUES
			('cand-1', 'run-1', 'v1:fp1', 'Alice Santos', 'alice santos', 'Caso Banco Master', 'caso banco master', 'Alice manteve contato com diretoria', 'A', 'https://folha.com.br/artigo1', 'https://folha.com.br/artigo1', 'Trecho extraído comprovando contato de Alice', 0.95, 'published', 0, '', NULL, 'ent-pub-1', 'case-1', 'clm-pub-1', 1, '[]', 1, '[]', '2026-09-10 10:01:00', '2026-09-10 10:01:00'),
			('cand-2', 'run-1', 'v1:fp2', 'Carlos Quarentena', 'carlos quarentena', 'Caso Banco Master', 'caso banco master', 'Carlos citado em relatório preliminar', 'C', 'https://folha.com.br/artigo1', 'https://folha.com.br/artigo1', 'Trecho extraído sobre Carlos', 0.70, 'quarantined', 0, '', NULL, 'ent-quar-1', 'case-1', NULL, 1, '[]', 0, '["UNCERTAIN_LINK"]', '2026-09-10 10:02:00', '2026-09-10 10:02:00'),
			('cand-3', 'run-2', 'v1:fp1', 'Alice Santos Duplicate', 'alice santos duplicate', 'Caso Banco Master', 'caso banco master', 'Alice manteve contato com diretoria', 'A', 'https://folha.com.br/artigo1', 'https://folha.com.br/artigo1', 'Mesmo trecho da Alice em outro run', 0.95, 'quarantined', 1, 'same_run_duplicate', 'cand-1', 'ent-pub-1', 'case-1', NULL, 1, '[]', 1, '[]', '2026-09-11 08:01:00', '2026-09-11 08:01:00');

		INSERT INTO semantic_evaluations (
			id, monitoring_candidate_id, provider, model, identity_match, claim_supported,
			claim_overstates_source, attribution_explicit, grade_compatible, contains_illicit_inference,
			uncertainties, recommended_action, raw_response, prompt_tokens, completion_tokens, total_tokens, cost_microusd, cost, created_at
		) VALUES
			('semeval-1', 'cand-1', 'openrouter', 'openai/gpt-4o', 1, 1, 0, 1, 1, 0, '[]', 'publish', '{"secret_prompt":"DO_NOT_LEAK_RAW_PROMPT_KEY_1"}', 850, 120, 970, 4850, 0.00485, '2026-09-10 10:01:30'),
			('semeval-2', 'cand-2', 'openrouter', 'openai/gpt-4o', 1, 0, 1, 0, 0, 0, '["UNCERTAIN_LINK"]', 'quarantine', '{"secret_prompt":"DO_NOT_LEAK_RAW_PROMPT_KEY_2"}', 400, 80, 480, 2400, 0.00240, '2026-09-10 10:02:30');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados administrativos de teste: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)

	cfg := &config.Config{
		Port:               8080,
		Env:                "test",
		DBPath:             dbPath,
		PublicDataCutoff:   "2026-09-03",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        60 * time.Second,
		AdminUser:          user,
		AdminPasswordHash:  passwordHash,
		AdminAllowedOrigin: "http://example.com",
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web com admin e dados: %v", err)
	}

	return srv, db
}

func TestAdminDashboard_AuthenticatedData(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: esperado 200, obtido %d", w.Code)
	}

	body := w.Body.String()

	// Valida seções principais e contagens operacionais
	if !strings.Contains(body, "Dashboard Administrativo") {
		t.Errorf("título do dashboard não encontrado")
	}
	if !strings.Contains(body, "Candidatos de Monitoramento") || !strings.Contains(body, "Usos de Evidência") || !strings.Contains(body, "Fontes Documentais") {
		t.Errorf("seções de contagem operacional não encontradas")
	}

	// Valida presença de links para as subrotas
	if !strings.Contains(body, `href="/admin/candidatos"`) {
		t.Errorf("link para /admin/candidatos não encontrado")
	}
	if !strings.Contains(body, `href="/admin/evidencias"`) {
		t.Errorf("link para /admin/evidencias não encontrado")
	}
	if !strings.Contains(body, `href="/admin/fontes"`) {
		t.Errorf("link para /admin/fontes não encontrado")
	}

	// Headers de segurança e cache
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control: esperado 'no-store', obtido %q", cc)
	}
	if vary := w.Header().Get("Vary"); vary != "Authorization" {
		t.Errorf("Vary: esperado 'Authorization', obtido %q", vary)
	}
}

func TestAdminCandidates_ListingAndFiltering(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. Listagem completa sem filtros
	reqAll := httptest.NewRequest(http.MethodGet, "/admin/candidatos", nil)
	reqAll.Header.Set("Authorization", authHeader)
	wAll := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAll, reqAll)

	if wAll.Code != http.StatusOK {
		t.Fatalf("listagem geral: esperado 200, obtido %d", wAll.Code)
	}
	bodyAll := wAll.Body.String()
	if !strings.Contains(bodyAll, "cand-1") || !strings.Contains(bodyAll, "cand-2") || !strings.Contains(bodyAll, "cand-3") {
		t.Errorf("todos os candidatos esperados na listagem geral")
	}
	if !strings.Contains(bodyAll, "Duplicata de cand-1") {
		t.Errorf("indicação de duplicata com link para o canônico não encontrada no cand-3")
	}

	// 2. Filtro por status=quarantined
	reqQuar := httptest.NewRequest(http.MethodGet, "/admin/candidatos?status=quarantined", nil)
	reqQuar.Header.Set("Authorization", authHeader)
	wQuar := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wQuar, reqQuar)

	if wQuar.Code != http.StatusOK {
		t.Fatalf("filtro status=quarantined: esperado 200, obtido %d", wQuar.Code)
	}
	bodyQuar := wQuar.Body.String()
	if !strings.Contains(bodyQuar, "cand-2") {
		t.Errorf("cand-2 esperado no filtro quarantined")
	}
	if strings.Contains(bodyQuar, "Trecho extraído comprovando contato de Alice") {
		t.Errorf("cand-1 (publicado) não deveria aparecer na listagem do filtro quarantined")
	}

	// 3. Filtro por grade=A
	reqGradeA := httptest.NewRequest(http.MethodGet, "/admin/candidatos?grade=A", nil)
	reqGradeA.Header.Set("Authorization", authHeader)
	wGradeA := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wGradeA, reqGradeA)

	if wGradeA.Code != http.StatusOK {
		t.Fatalf("filtro grade=A: esperado 200, obtido %d", wGradeA.Code)
	}
	bodyGradeA := wGradeA.Body.String()
	if !strings.Contains(bodyGradeA, "cand-1") || !strings.Contains(bodyGradeA, "cand-3") {
		t.Errorf("cand-1 e cand-3 esperados no filtro grade=A")
	}
	if strings.Contains(bodyGradeA, "cand-2") {
		t.Errorf("cand-2 (grau C) não deveria aparecer no filtro grade=A")
	}

	// 4. Busca por termo de texto
	reqSearch := httptest.NewRequest(http.MethodGet, "/admin/candidatos?q=Carlos", nil)
	reqSearch.Header.Set("Authorization", authHeader)
	wSearch := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSearch, reqSearch)

	if wSearch.Code != http.StatusOK {
		t.Fatalf("busca q=Carlos: esperado 200, obtido %d", wSearch.Code)
	}
	bodySearch := wSearch.Body.String()
	if !strings.Contains(bodySearch, "cand-2") {
		t.Errorf("cand-2 esperado na busca por Carlos")
	}
	if strings.Contains(bodySearch, "Trecho extraído comprovando contato de Alice") {
		t.Errorf("cand-1 não deveria aparecer na busca por Carlos")
	}
}

func TestAdminCandidateDetail_Inspection(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. Detalhe de cand-1 existente
	req := httptest.NewRequest(http.MethodGet, "/admin/candidatos/cand-1", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("detalhe cand-1: esperado 200, obtido %d", w.Code)
	}

	body := w.Body.String()

	// Valida campos extraídos e resolução
	if !strings.Contains(body, "Alice Santos") {
		t.Errorf("nome extraído não encontrado")
	}
	if !strings.Contains(body, "Caso Banco Master") {
		t.Errorf("caso não encontrado")
	}
	if !strings.Contains(body, "https://folha.com.br/artigo1") {
		t.Errorf("url da fonte não encontrada")
	}
	if !strings.Contains(body, "Grau A") {
		t.Errorf("grau proposto A não encontrado")
	}
	if !strings.Contains(body, "clm-pub-1") {
		t.Errorf("link para claim publicado não encontrado")
	}

	// Valida histórico de avaliação semântica
	if !strings.Contains(body, "openai/gpt-4o") || !strings.Contains(body, "openrouter") {
		t.Errorf("avaliação semântica não encontrada nos detalhes")
	}
	if !strings.Contains(body, "Correspondência de Identidade") {
		t.Errorf("critérios de avaliação semântica não encontrados")
	}

	// TESTE DE BLINDAGEM: raw_response nunca deve aparecer no HTML
	if strings.Contains(body, "DO_NOT_LEAK_RAW_PROMPT_KEY_1") || strings.Contains(body, "secret_prompt") {
		t.Errorf("vazamento de segurança: payload bruto raw_response exposto no corpo da resposta")
	}

	// TESTE DE ESCOPO EDITORIAL / TELEMETRIA: Tokens e custo não devem aparecer no HTML do detalhe VZ-015
	forbidWords := []string{
		"Tokens Prompt",
		"Tokens Resposta",
		"Tokens Totais",
		"Custo",
		"µUSD",
		"&mu;USD",
		"micro-USD",
		"cost_microusd",
		"850",
		"970",
		"4850",
	}
	for _, word := range forbidWords {
		if strings.Contains(body, word) {
			t.Errorf("violação de escopo VZ-015: termo ou valor de telemetria operacional %q encontrado no HTML", word)
		}
	}

	// 2. Detalhe de cand-3 (duplicata)
	reqDup := httptest.NewRequest(http.MethodGet, "/admin/candidatos/cand-3", nil)
	reqDup.Header.Set("Authorization", authHeader)
	wDup := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wDup, reqDup)

	if wDup.Code != http.StatusOK {
		t.Fatalf("detalhe cand-3: esperado 200, obtido %d", wDup.Code)
	}
	bodyDup := wDup.Body.String()
	if !strings.Contains(bodyDup, "cand-1") || !strings.Contains(bodyDup, "Duplicata") {
		t.Errorf("referência ao candidato canônico não encontrada no detalhe da duplicata")
	}

	// 3. Candidato inexistente -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/admin/candidatos/candidato-fantasma", nil)
	req404.Header.Set("Authorization", authHeader)
	w404 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Errorf("candidato inexistente: esperado 404, obtido %d", w404.Code)
	}
	if cc := w404.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control em 404: esperado 'no-store', obtido %q", cc)
	}
}

func TestAdminEvidences_ListingAndFiltering(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. Listagem geral e validação do aviso editorial
	reqAll := httptest.NewRequest(http.MethodGet, "/admin/evidencias", nil)
	reqAll.Header.Set("Authorization", authHeader)
	wAll := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAll, reqAll)

	if wAll.Code != http.StatusOK {
		t.Fatalf("listagem geral de evidências: esperado 200, obtido %d", wAll.Code)
	}

	bodyAll := wAll.Body.String()
	// Valida aviso editorial obrigatório de moderação no evidence_source
	if !strings.Contains(bodyAll, "Unidade de Moderação") ||
		!strings.Contains(bodyAll, "jamais rejeita uma fonte documental globalmente") {
		t.Errorf("aviso de escopo de moderação em evidence_source não encontrado")
	}

	if !strings.Contains(bodyAll, "es-1") || !strings.Contains(bodyAll, "es-2") || !strings.Contains(bodyAll, "es-3") || !strings.Contains(bodyAll, "es-4") {
		t.Errorf("todos os evidence_sources esperados na listagem geral")
	}

	// 2. Filtro por papel (role=contradicts)
	reqContra := httptest.NewRequest(http.MethodGet, "/admin/evidencias?role=contradicts", nil)
	reqContra.Header.Set("Authorization", authHeader)
	wContra := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wContra, reqContra)

	if wContra.Code != http.StatusOK {
		t.Fatalf("filtro role=contradicts: esperado 200, obtido %d", wContra.Code)
	}
	bodyContra := wContra.Body.String()
	if !strings.Contains(bodyContra, "es-2") {
		t.Errorf("es-2 esperado no filtro contradicts")
	}
	if strings.Contains(bodyContra, "es-1") {
		t.Errorf("es-1 (supports) não deveria aparecer no filtro contradicts")
	}

	// 3. Filtro por status do claim (claim_status=quarantined)
	reqClaimQuar := httptest.NewRequest(http.MethodGet, "/admin/evidencias?claim_status=quarantined", nil)
	reqClaimQuar.Header.Set("Authorization", authHeader)
	wClaimQuar := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wClaimQuar, reqClaimQuar)

	if wClaimQuar.Code != http.StatusOK {
		t.Fatalf("filtro claim_status=quarantined: esperado 200, obtido %d", wClaimQuar.Code)
	}
	bodyClaimQuar := wClaimQuar.Body.String()
	if !strings.Contains(bodyClaimQuar, "es-3") {
		t.Errorf("es-3 esperado no filtro claim_status=quarantined")
	}
	if strings.Contains(bodyClaimQuar, "es-1") {
		t.Errorf("es-1 (published) não deveria aparecer no filtro claim_status=quarantined")
	}

	// 4. Filtro por status da evidência (es_status=rejected)
	reqEsRej := httptest.NewRequest(http.MethodGet, "/admin/evidencias?es_status=rejected", nil)
	reqEsRej.Header.Set("Authorization", authHeader)
	wEsRej := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wEsRej, reqEsRej)

	if wEsRej.Code != http.StatusOK {
		t.Fatalf("filtro es_status=rejected: esperado 200, obtido %d", wEsRej.Code)
	}
	bodyEsRej := wEsRej.Body.String()
	if !strings.Contains(bodyEsRej, "es-4") {
		t.Errorf("es-4 esperado no filtro es_status=rejected")
	}
	if strings.Contains(bodyEsRej, "es-1") {
		t.Errorf("es-1 (active) não deveria aparecer no filtro es_status=rejected")
	}
}

func TestAdminSources_ListingAndFiltering(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. Listagem geral
	reqAll := httptest.NewRequest(http.MethodGet, "/admin/fontes", nil)
	reqAll.Header.Set("Authorization", authHeader)
	wAll := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAll, reqAll)

	if wAll.Code != http.StatusOK {
		t.Fatalf("listagem de fontes: esperado 200, obtido %d", wAll.Code)
	}

	bodyAll := wAll.Body.String()
	if !strings.Contains(bodyAll, "Folha de S.Paulo") || !strings.Contains(bodyAll, "Site Indisponível") || !strings.Contains(bodyAll, "Fonte Não Verificada") {
		t.Errorf("todas as fontes esperadas na listagem")
	}

	// Valida segurança nos links externos: target="_blank" e rel="noopener noreferrer"
	if !strings.Contains(bodyAll, `target="_blank"`) || !strings.Contains(bodyAll, `rel="noopener noreferrer"`) {
		t.Errorf("links externos de fontes devem conter target='_blank' e rel='noopener noreferrer'")
	}

	// 2. Filtro por status de acessibilidade (access_status=unreachable)
	reqUnreach := httptest.NewRequest(http.MethodGet, "/admin/fontes?access_status=unreachable", nil)
	reqUnreach.Header.Set("Authorization", authHeader)
	wUnreach := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wUnreach, reqUnreach)

	if wUnreach.Code != http.StatusOK {
		t.Fatalf("filtro access_status=unreachable: esperado 200, obtido %d", wUnreach.Code)
	}
	bodyUnreach := wUnreach.Body.String()
	if !strings.Contains(bodyUnreach, "Site Indisponível") {
		t.Errorf("Site Indisponível esperado no filtro unreachable")
	}
	if strings.Contains(bodyUnreach, "Folha de S.Paulo") {
		t.Errorf("Folha de S.Paulo (reachable) não deveria aparecer no filtro unreachable")
	}
	if !strings.Contains(bodyUnreach, "503") || !strings.Contains(bodyUnreach, "ERR_HTTP_503") {
		t.Errorf("código HTTP 503 e código de erro esperados para fonte inacessível")
	}
}

func TestAdminQuarantineIsolationFromPublicArea(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. No Admin: Carlos Quarentena e cand-2 aparecem normalmente
	reqAdmin := httptest.NewRequest(http.MethodGet, "/admin/candidatos?q=Carlos", nil)
	reqAdmin.Header.Set("Authorization", authHeader)
	wAdmin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAdmin, reqAdmin)

	if wAdmin.Code != http.StatusOK || !strings.Contains(wAdmin.Body.String(), "cand-2") {
		t.Fatalf("cand-2 deve estar visível para consulta no admin")
	}

	// 2. Na Área Pública: Carlos Quarentena NÃO deve aparecer em /pessoas
	reqPublicList := httptest.NewRequest(http.MethodGet, "/pessoas", nil)
	wPublicList := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPublicList, reqPublicList)

	if wPublicList.Code != http.StatusOK {
		t.Fatalf("status /pessoas: esperado 200, obtido %d", wPublicList.Code)
	}
	if strings.Contains(wPublicList.Body.String(), "Carlos Quarentena") {
		t.Errorf("vazamento: entidade em quarentena apareceu na listagem pública /pessoas")
	}

	// 3. Na Área Pública: /pessoas/carlos-quarentena deve retornar rigorosamente 404
	reqPublicDetail := httptest.NewRequest(http.MethodGet, "/pessoas/carlos-quarentena", nil)
	wPublicDetail := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPublicDetail, reqPublicDetail)

	if wPublicDetail.Code != http.StatusNotFound {
		t.Errorf("vazamento: /pessoas/carlos-quarentena deve retornar 404, obtido %d", wPublicDetail.Code)
	}
}

func TestAdminInvalidQueryParams(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	invalidPaths := []string{
		"/admin/candidatos?page=-10&page_size=9999&grade=INVALID&status=UNKNOWN_STATUS&period=bizarre",
		"/admin/evidencias?page=abc&page_size=-5&role=unknown&claim_status=xyz&es_status=foo&period=bar",
		"/admin/fontes?page=0&page_size=10000&access_status=fake&source_type=aliens&period=future",
	}

	for _, p := range invalidPaths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			req.Header.Set("Authorization", authHeader)
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			// Nunca deve dar pânico ou erro 500
			if w.Code != http.StatusOK {
				t.Errorf("parâmetros inválidos em %s: esperado 200 com sanitização defensiva, obtido %d", p, w.Code)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control: esperado 'no-store', obtido %q", cc)
			}
		})
	}
}

func TestAdminSources_SourceTypeAllowlistAndSanitization(t *testing.T) {
	srv, _ := setupAdminTestServerWithData(t, "admin")
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	// 1. Validar que o select renderiza exatamente todas as opções canônicas de store.GetAdminSourceTypeOptions()
	reqAll := httptest.NewRequest(http.MethodGet, "/admin/fontes", nil)
	reqAll.Header.Set("Authorization", authHeader)
	wAll := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAll, reqAll)

	if wAll.Code != http.StatusOK {
		t.Fatalf("listagem geral status: esperado 200, obtido %d", wAll.Code)
	}
	bodyAll := wAll.Body.String()

	canonicalOptions := store.GetAdminSourceTypeOptions()
	for _, opt := range canonicalOptions {
		expectedOptionSub := fmt.Sprintf(`value="%s"`, opt.Value)
		if !strings.Contains(bodyAll, expectedOptionSub) {
			t.Errorf("opção canônica com valor %q não renderizada no select", opt.Value)
		}
		if !strings.Contains(bodyAll, opt.Label) {
			t.Errorf("label canônico %q não renderizado no select", opt.Label)
		}
	}

	// 2. Tipo permitido (article) deve filtrar corretamente, marcar selected e retornar 200
	reqAllowed := httptest.NewRequest(http.MethodGet, "/admin/fontes?source_type=article", nil)
	reqAllowed.Header.Set("Authorization", authHeader)
	wAllowed := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAllowed, reqAllowed)

	if wAllowed.Code != http.StatusOK {
		t.Fatalf("tipo permitido status: esperado 200, obtido %d", wAllowed.Code)
	}
	bodyAllowed := wAllowed.Body.String()
	if !strings.Contains(bodyAllowed, "Folha de S.Paulo") {
		t.Errorf("Folha de S.Paulo esperada no filtro source_type=article")
	}
	if !strings.Contains(bodyAllowed, `<option value="article" selected>`) {
		t.Errorf("opção 'article' deve estar selecionada no select")
	}

	// 3. Tipo desconhecido não deve causar 500 nem ser preservado no select
	reqUnknown := httptest.NewRequest(http.MethodGet, "/admin/fontes?source_type=unknown_arbitrary_type", nil)
	reqUnknown.Header.Set("Authorization", authHeader)
	wUnknown := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wUnknown, reqUnknown)

	if wUnknown.Code != http.StatusOK {
		t.Fatalf("tipo desconhecido status: esperado 200 (sanitizado), obtido %d", wUnknown.Code)
	}
	bodyUnknown := wUnknown.Body.String()
	// Como foi sanitizado para vazio, todas as fontes aparecem
	if !strings.Contains(bodyUnknown, "Folha de S.Paulo") || !strings.Contains(bodyUnknown, "Site Indisponível") {
		t.Errorf("tipo desconhecido deve retornar busca geral segura sem quebrar")
	}
	if strings.Contains(bodyUnknown, "unknown_arbitrary_type") {
		t.Errorf("tipo desconhecido não deve ser refletido/preservado na interface/select")
	}

	// 4. Tipo excessivamente longo não causa 500 nem aparece no HTML
	longType := strings.Repeat("evil_type_", 50)
	reqLong := httptest.NewRequest(http.MethodGet, "/admin/fontes?source_type="+longType, nil)
	reqLong.Header.Set("Authorization", authHeader)
	wLong := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLong, reqLong)

	if wLong.Code != http.StatusOK {
		t.Fatalf("tipo longo status: esperado 200, obtido %d", wLong.Code)
	}
	bodyLong := wLong.Body.String()
	if strings.Contains(bodyLong, longType) {
		t.Errorf("tipo excessivamente longo não deve ser refletido no HTML")
	}
}

func TestAdminExternalLinks_SanitizationAndMaliciousURLDefense(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "admin_url_defense_test.db")

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

	now := time.Now().UTC().Format(time.RFC3339)

	// Inserir fontes e candidatos com URLs maliciosas e válidas
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-sec', 'Caso Segurança', 'caso-seguranca', 'Auditoria');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-sec', 'person', 'Alvo Seguro', 'alvo seguro', 'alvo-seguro', 'Função', 'Resumo', 3, 'Justificativa', 'Outros', 'Local');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary)
		VALUES ('rel-sec', 'ent-sec', 'case-sec', 'contato', 'Resumo');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-sec', 'rel-sec', 'Proposição defensiva', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-sec', 'clm-sec', 'Evidência URL', 'document');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES
			('src-malicious', 'Fonte Maliciosa', 'Hacker', 'javascript:alert(1)', 'javascript:alert(1)', 'article', 'reachable'),
			('src-dataurl', 'Fonte Data URL', 'Hacker', 'data:text/html,<script>alert(2)</script>', 'data:text/html,<script>alert(2)</script>', 'article', 'reachable'),
			('src-valid', 'Fonte Válida', 'Jornal', 'https://seguro.com.br/materia', 'https://seguro.com.br/materia', 'article', 'reachable');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES
			('es-malicious', 'ev-sec', 'src-malicious', 'supports', 'Citação maliciosa', '', 'active'),
			('es-valid', 'ev-sec', 'src-valid', 'supports', 'Citação válida', '', 'active');

		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES ('run-sec', 'completed', 'Auditoria Links', 'openai/gpt-4o', ?);

		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			proposition, suggested_grade, source_url, canonical_url, technical_confidence,
			editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id, created_at, updated_at
		) VALUES
			('cand-malicious', 'run-sec', 'v1:sec-1', 'Alvo Malicioso', 'alvo malicioso', 'Prop maliciosa', 'B', 'javascript:alert(3)', 'javascript:alert(3)', 0.90, 'published', 0, '', NULL, ?, ?),
			('cand-valid', 'run-sec', 'v1:sec-2', 'Alvo Válido', 'alvo valido', 'Prop válida', 'A', 'https://noticias-validas.com/art', 'https://noticias-validas.com/art', 0.95, 'published', 0, '', NULL, ?, ?);
	`, now, now, now, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir dados de teste de segurança de URL: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	cfg := &config.Config{
		Port:              8080,
		Env:               "test",
		DBPath:            dbPath,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		AdminUser:         "admin",
		AdminPasswordHash: passwordHash,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor: %v", err)
	}

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:password"))

	endpoints := []struct {
		name string
		path string
	}{
		{"Fontes", "/admin/fontes"},
		{"Candidatos", "/admin/candidatos"},
		{"Candidato Detalhe", "/admin/candidatos/cand-malicious"},
		{"Evidências", "/admin/evidencias"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep.path, nil)
			req.Header.Set("Authorization", authHeader)
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("%s status: esperado 200, obtido %d", ep.name, w.Code)
			}
			body := w.Body.String()

			// 1. NÃO DEVE existir nenhum href com javascript: ou data:
			if strings.Contains(body, `href="javascript:`) || strings.Contains(body, `href='javascript:`) ||
				strings.Contains(body, `href="data:`) || strings.Contains(body, `href='data:`) {
				t.Errorf("%s: link executável inseguro renderizado no atributo href: %s", ep.name, body)
			}

			// 2. Deve conter indicador neutro de URL inválida/insegura
			if !strings.Contains(body, "URL inválida ou insegura") {
				t.Errorf("%s: esperado aviso textual neutro de URL inválida/insegura", ep.name)
			}

			// 3. O link malicioso só pode aparecer como texto escapado seguro
			if !strings.Contains(body, "javascript:alert") {
				t.Errorf("%s: texto bruto original deveria estar presente de forma segura/escapada", ep.name)
			}
		})
	}

	// Validar que link válido contém target="_blank" e rel="noopener noreferrer"
	reqCandValid := httptest.NewRequest(http.MethodGet, "/admin/candidatos/cand-valid", nil)
	reqCandValid.Header.Set("Authorization", authHeader)
	wCandValid := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wCandValid, reqCandValid)
	bodyCandValid := wCandValid.Body.String()

	if !strings.Contains(bodyCandValid, `href="https://noticias-validas.com/art"`) {
		t.Errorf("link válido esperado com href correto")
	}
	if !strings.Contains(bodyCandValid, `target="_blank"`) || !strings.Contains(bodyCandValid, `rel="noopener noreferrer"`) {
		t.Errorf("link válido deve conter target='_blank' e rel='noopener noreferrer'")
	}
}
