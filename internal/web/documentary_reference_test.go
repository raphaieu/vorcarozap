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
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupDocumentaryReferenceTestServer(t *testing.T) (*http.Server, *sql.DB, string, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "doc_ref_test.db")

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

	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-doc-1', 'Operação Documental', 'operacao-documental', 'Caso de teste documental');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-doc-1', 'person', 'Investigado Primário', 'investigado primario', 'investigado-primario', 'Alvo Principal', 'Resumo', 5, 'R1', 'Finanças', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary)
		VALUES
			('rel-doc-1', 'ent-doc-1', 'case-doc-1', 'investigado', 'Vínculo Documental');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES
			('src-court-1', 'Decisão Judicial de Busca', '10ª Vara Federal', 'https://justica.gov.br/decisao-10', 'https://justica.gov.br/decisao-10', 'court_document', 'reachable'),
			('src-police-1', 'Laudo Pericial Contábil', 'Polícia Federal', 'https://pf.gov.br/laudo-44', 'https://pf.gov.br/laudo-44', 'police_report', 'reachable'),
			('src-news-1', 'Reportagem Especial Investigativa', 'Jornal Nacional', 'https://jornal.com/reportagem', 'https://jornal.com/reportagem', 'article', 'reachable');

		-- Claim 1: Sustentado por Decisão Judicial e Laudo Pericial (Documentos Primários com Localizadores Precisos)
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-doc-1', 'rel-doc-1', 'Movimentação atípica de R$ 50 milhões', 'Polícia Federal', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ativo');

		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-doc-1', 'claim-doc-1', 'Evidência Pericial Primária');

		-- Uso 1: Decisão judicial com pág e figura
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-doc-1', 'ev-doc-1', 'src-court-1', 'Constatou-se a emissão de notas fiscais sem lastro.', 'p. 42, figura 12', 'supports', 'active');

		-- Uso 2: Laudo pericial com intervalo de folhas
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-doc-2', 'ev-doc-1', 'src-police-1', 'A perícia contábil identificou cruzamento de dados bancários.', 'fls. 10-14, tabela 3', 'supports', 'active');

		-- Uso 3: Notícia jornalística com intervalo de páginas
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-doc-3', 'ev-doc-1', 'src-news-1', 'Veículo noticiou o desdobramento da operação.', 'págs. 15-18', 'contextualizes', 'active');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	cfg := &config.Config{
		Port:              8090,
		Env:               "test",
		DBPath:            dbPath,
		PublicDataCutoff:  "2026-09-03",
		AdminUser:         "admin_editor",
		AdminPasswordHash: passwordHash,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}
	return srv, db, "admin_editor", "password"
}

func TestDocumentaryReference_PublicEntityDetailRendering(t *testing.T) {
	srv, _, _, _ := setupDocumentaryReferenceTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/pessoas/investigado-primario", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200 na página pública da entidade, obtido: %d", rec.Code)
	}

	body := rec.Body.String()

	// 1. Verifica se os localizadores foram normalizados determinísticamente
	// "p. 42, figura 12" -> "Pág. 42, Fig. 12"
	if !strings.Contains(body, "Pág. 42, Fig. 12") {
		t.Errorf("esperava encontrar localizador normalizado 'Pág. 42, Fig. 12', corpo: %s", body)
	}

	// "fls. 10-14, tabela 3" -> "Fls. 10–14, Tabela 3" (com en-dash)
	if !strings.Contains(body, "Fls. 10–14, Tabela 3") {
		t.Errorf("esperava encontrar localizador normalizado 'Fls. 10–14, Tabela 3', corpo: %s", body)
	}

	// "págs. 15-18" -> "Págs. 15–18"
	if !strings.Contains(body, "Págs. 15–18") {
		t.Errorf("esperava encontrar localizador normalizado 'Págs. 15–18', corpo: %s", body)
	}

	// 2. Verifica a segregação entre trecho literal e caixa do localizador
	if !strings.Contains(body, "source-locator-box") {
		t.Errorf("esperava conter a classe .source-locator-box para segregação de localizadores")
	}
	if !strings.Contains(body, "source-locator-chip") {
		t.Errorf("esperava conter a classe .source-locator-chip")
	}
	if !strings.Contains(body, "source-excerpt") {
		t.Errorf("esperava conter a classe .source-excerpt para o trecho literal")
	}

	// 3. Verifica distinção de tipo de documento primário vs notícia
	if !strings.Contains(body, "Peça Judicial / Decisão") {
		t.Errorf("esperava rótulo 'Peça Judicial / Decisão' para src-court-1")
	}
	if !strings.Contains(body, "Relatório Policial / Pericial") {
		t.Errorf("esperava rótulo 'Relatório Policial / Pericial' para src-police-1")
	}
	if !strings.Contains(body, "Artigo / Notícia") {
		t.Errorf("esperava rótulo 'Artigo / Notícia' para src-news-1")
	}

	// Verifica aplicação de badge de documento primário
	if !strings.Contains(body, "source-type-primary") {
		t.Errorf("esperava classe .source-type-primary para documentos primários oficiais")
	}
}

func TestDocumentaryReference_AdminViewsRendering(t *testing.T) {
	srv, _, username, password := setupDocumentaryReferenceTestServer(t)

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))

	// 1. Detalhe de uso de evidência (admin_evidence_detail)
	req := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-doc-1", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200 no detalhe de evidência admin, obtido: %d", rec.Code)
	}

	evidenceBody := rec.Body.String()
	if !strings.Contains(evidenceBody, "Localizador Cirúrgico:") {
		t.Errorf("esperava rótulo 'Localizador Cirúrgico:' no detalhe de evidência admin")
	}
	if !strings.Contains(evidenceBody, "Pág. 42, Fig. 12") {
		t.Errorf("esperava localizador normalizado 'Pág. 42, Fig. 12' no detalhe de evidência admin")
	}
	if !strings.Contains(evidenceBody, "Peça Judicial / Decisão") {
		t.Errorf("esperava tipo 'Peça Judicial / Decisão' no detalhe de evidência admin")
	}

	// 2. Detalhe de claim (admin_claim_detail)
	reqClaim := httptest.NewRequest(http.MethodGet, "/admin/claims/claim-doc-1", nil)
	reqClaim.Header.Set("Authorization", authHeader)
	recClaim := httptest.NewRecorder()

	srv.Handler.ServeHTTP(recClaim, reqClaim)

	if recClaim.Code != http.StatusOK {
		t.Fatalf("esperado status 200 no detalhe de claim admin, obtido: %d", recClaim.Code)
	}

	claimBody := recClaim.Body.String()
	if !strings.Contains(claimBody, "Pág. 42, Fig. 12") {
		t.Errorf("esperava localizador 'Pág. 42, Fig. 12' no detalhe de claim admin")
	}
	if !strings.Contains(claimBody, "Fls. 10–14, Tabela 3") {
		t.Errorf("esperava localizador 'Fls. 10–14, Tabela 3' no detalhe de claim admin")
	}

	// 3. Listagem de fontes (admin_sources)
	reqSources := httptest.NewRequest(http.MethodGet, "/admin/fontes", nil)
	reqSources.Header.Set("Authorization", authHeader)
	recSources := httptest.NewRecorder()

	srv.Handler.ServeHTTP(recSources, reqSources)

	if recSources.Code != http.StatusOK {
		t.Fatalf("esperado status 200 na listagem de fontes admin, obtido: %d", recSources.Code)
	}

	sourcesBody := recSources.Body.String()
	if !strings.Contains(sourcesBody, "Peça Judicial / Decisão") {
		t.Errorf("esperava rótulo humano 'Peça Judicial / Decisão' na listagem de fontes")
	}
	if !strings.Contains(sourcesBody, "Relatório Policial / Pericial") {
		t.Errorf("esperava rótulo humano 'Relatório Policial / Pericial' na listagem de fontes")
	}
}

func TestDocumentaryReference_InsecureLocatorSanitization(t *testing.T) {
	// Garante que mesmo que um locator malicioso estivesse em dados legados ou mockados,
	// a camada de viewmodel e SafeLocator o sanitiza completamente para string vazia
	maliciousInputs := []string{
		"<script>alert(1)</script>",
		"javascript:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"../../etc/passwd",
		"p. 42\x00malicious",
		strings.Repeat("a", 205),
	}

	for _, mal := range maliciousInputs {
		safe := normalize.SafeLocator(mal)
		if safe != "" {
			t.Errorf("SafeLocator(%q) = %q, esperava string vazia", mal, safe)
		}
	}
}

func TestSourceType_UnifiedSourceOfTruth(t *testing.T) {
	// Garante que GetAdminSourceTypeOptions usa a mesma fonte e ordem de domain.CanonicalSourceTypes
	storeOpts := store.GetAdminSourceTypeOptions()
	domainTypes := domain.CanonicalSourceTypes

	if len(storeOpts) != len(domainTypes) {
		t.Fatalf("discrepância no tamanho das opções: store=%d, domain=%d", len(storeOpts), len(domainTypes))
	}

	for i, opt := range storeOpts {
		dt := domainTypes[i]
		if opt.Value != string(dt) {
			t.Errorf("opção[%d] valor divergente: store=%q, domain=%q", i, opt.Value, dt)
		}
		if opt.Label != dt.Label() {
			t.Errorf("opção[%d] label divergente: store=%q, domain=%q", i, opt.Label, dt.Label())
		}
		if !store.IsAllowedAdminSourceType(opt.Value) {
			t.Errorf("tipo %q deveria ser permitido em IsAllowedAdminSourceType", opt.Value)
		}
	}

	// Tipos desconhecidos devem ser rejeitados e não classificados como primários
	unknown := "unsupported_type"
	if store.IsAllowedAdminSourceType(unknown) {
		t.Errorf("tipo desconhecido %q não deveria ser permitido", unknown)
	}
	if domain.SourceType(unknown).IsPrimaryDocument() {
		t.Errorf("tipo desconhecido %q não deve ser documento primário", unknown)
	}
}
