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
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/moderation"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupDocumentCatalogTestServer(t *testing.T) (*http.Server, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "doc_catalog_test.db")

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

	// Insere massa de teste representativa
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-cat-1', 'Operação Documental', 'operacao-documental', 'Contexto');

		-- Entidades
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-cat-1', 'person', 'Investigado Alpha', 'investigado alpha', 'investigado-alpha', 'Senador', 'Resumo Alpha', 5, 'Figura pública', 'Politica', 'Nacional'),
			('ent-cat-2', 'person', 'Executivo Beta', 'executivo beta', 'executivo-beta', 'Diretor', 'Resumo Beta', 3, 'Executivo citado', 'Empresas', 'Nacional'),
			('ent-cat-3', 'person', 'Alvo Quarentena', 'alvo quarentena', 'alvo-quarentena', 'Assessor', 'Resumo Quarentena', 2, 'Assessor', 'Outros', 'Local');

		-- Relações
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES
			('rel-cat-1', 'ent-cat-1', 'case-cat-1', 'Investigado', 'Citado em laudo pericial', ''),
			('rel-cat-2', 'ent-cat-2', 'case-cat-1', 'Testemunha', 'Depoimento formal de contexto', 'Não é investigado'),
			('rel-cat-3', 'ent-cat-3', 'case-cat-1', 'Mencionado', 'Menção em quarentena', '');

		-- Fontes:
		-- 1. src-laudo-oficial: Usada por Alpha (clm-1) e Beta (clm-2) -> Deve aparecer UMA ÚNICA VEZ com 2 citações e 2 pessoas
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, published_at, accessed_at)
		VALUES ('src-laudo-oficial', 'Laudo Pericial INC nº 50/2026', 'Polícia Federal', 'https://pf.gov.br/laudo50#secret_token_fragment', 'https://pf.gov.br/laudo50', 'police_report', 'reachable', '2026-08-15T00:00:00Z', '2026-09-01T12:00:00Z');

		-- 2. src-nota-contexto: Usada por Beta (clm-2) com metric_eligible=0 -> Deve aparecer normalmente no catálogo
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, published_at, accessed_at)
		VALUES ('src-nota-contexto', 'Nota de Esclarecimento Notarial', 'Cartório 1º Ofício', 'https://cartorio.gov.br/nota', 'https://cartorio.gov.br/nota', 'official_statement', 'reachable', '2026-08-20T00:00:00Z', '2026-09-02T10:00:00Z');

		-- 3. src-quar-exclusiva: Usada apenas pelo claim em quarentena clm-3 -> NÃO DEVE APARECER
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-quar-exclusiva', 'Boato de Blog Secreto', 'Blog Oculto Quarentena', 'https://blog.com/quar', 'https://blog.com/quar', 'article', 'unreachable');

		-- 4. src-rejeitada-exclusiva: Usada apenas em evidence_source com status='rejected' -> NÃO DEVE APARECER
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-rejeitada-exclusiva', 'Artigo de Fonte Desaprovada', 'Jornal Desaprovado', 'https://jornal.com/rej', 'https://jornal.com/rej', 'article', 'reachable');

		-- Claims:
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES
			('clm-cat-1', 'rel-cat-1', 'Alpha manteve contato documentado', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]'),
			('clm-cat-2', 'rel-cat-2', 'Beta esclareceu estrutura operacional', '', 'curated_seed', 'B', 'context_only', 0, 'published', 'contact_confirmed', '[]'),
			('clm-cat-3', 'rel-cat-3', 'Alvo em boato isolado', '', 'curated_seed', 'E', 'possible_link', 0, 'quarantined', 'quarantined', '["UNKNOWN_GRADE"]');

		-- Evidências:
		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-cat-1', 'clm-cat-1', 'Evidência Laudo Alpha', 'document'),
			('ev-cat-2', 'clm-cat-2', 'Evidência Depoimento Beta', 'document'),
			('ev-cat-3', 'clm-cat-3', 'Evidência Quarentena', 'document');

		-- Evidence Sources:
		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES
			('es-cat-1', 'ev-cat-1', 'src-laudo-oficial', 'supports', 'Trecho Alpha comprovado', 'Página 12', 'active'),
			('es-cat-2', 'ev-cat-2', 'src-laudo-oficial', 'supports', 'Trecho Beta corroborado', 'Página 24', 'active'),
			('es-cat-3', 'ev-cat-2', 'src-nota-contexto', 'supports', 'Trecho Notarial de contexto', 'Folha 3', 'active'),
			('es-cat-4', 'ev-cat-3', 'src-quar-exclusiva', 'supports', 'Trecho de Quarentena', 'Pág. 1', 'active'),
			('es-cat-5', 'ev-cat-1', 'src-rejeitada-exclusiva', 'supports', 'Trecho Rejeitado', 'Pág. 99', 'rejected');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir fixtures do catálogo de documentos: %v", err)
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
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	return srv, db
}

func TestPublicDocumentCatalog(t *testing.T) {
	srv, _ := setupDocumentCatalogTestServer(t)

	t.Run("GET /documentos renderiza catálogo com status 200 e cabeçalhos corretos", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/documentos", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("Content-Type: esperado text/html, obtido %q", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
			t.Errorf("Cache-Control: esperado 'no-cache, no-store, must-revalidate', obtido %q", cc)
		}

		body := w.Body.String()

		// Valida título da página
		if !strings.Contains(body, "Catálogo de Documentos") && !strings.Contains(body, "Documentos e Fontes") {
			t.Errorf("título do catálogo ausente no HTML")
		}

		// Fonte compartilhada (Laudo Pericial INC nº 50/2026) deve estar presente
		if !strings.Contains(body, "Laudo Pericial INC nº 50/2026") {
			t.Errorf("fonte pública compartilhada ausente do catálogo")
		}

		// Fonte de contexto (Nota de Esclarecimento Notarial) com metric_eligible=false deve estar presente
		if !strings.Contains(body, "Nota de Esclarecimento Notarial") {
			t.Errorf("fonte de contexto com metric_eligible=false ausente do catálogo")
		}

		// Fonte de quarentena NÃO pode estar presente
		if strings.Contains(body, "Boato de Blog Secreto") || strings.Contains(body, "Blog Oculto Quarentena") {
			t.Errorf("vazamento de quarentena: fonte de quarentena apareceu no catálogo público")
		}

		// Fonte rejeitada NÃO pode estar presente
		if strings.Contains(body, "Artigo de Fonte Desaprovada") || strings.Contains(body, "Jornal Desaprovado") {
			t.Errorf("vazamento de fonte rejeitada: fonte sem suporte ativo apareceu no catálogo")
		}

		// Valida que o link para o viewer existe no HTML
		if !strings.Contains(body, "/documentos/src-laudo-oficial") {
			t.Errorf("link para viewer /documentos/src-laudo-oficial ausente no HTML")
		}

		// Valida renderização do primeiro trecho público contextualizado
		if !strings.Contains(body, "Trecho Alpha comprovado") {
			t.Errorf("primeiro trecho público da fonte ausente no cartão do catálogo")
		}
		if !strings.Contains(body, "Pág. 12") {
			t.Errorf("localizador do primeiro trecho público ausente no cartão do catálogo")
		}

		// Valida rótulos precisos
		if !strings.Contains(body, "Entidades vinculadas:") {
			t.Errorf("rótulo 'Entidades vinculadas:' ausente no cartão")
		}
		if !strings.Contains(body, "Grau da alegação:") {
			t.Errorf("rótulo 'Grau da alegação:' ausente no cartão")
		}

		// Valida navegação no header
		if !strings.Contains(body, `href="/documentos"`) {
			t.Errorf("link /documentos ausente no header de navegação")
		}
	})

	t.Run("Verifica que link do cartão leva ao viewer público existente", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/documentos/src-laudo-oficial", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("viewer GET /documentos/src-laudo-oficial: esperado 200, obtido %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "Laudo Pericial INC nº 50/2026") {
			t.Errorf("conteúdo do documento ausente no viewer")
		}
		if !strings.Contains(body, "Trecho Alpha comprovado") || !strings.Contains(body, "Trecho Beta corroborado") {
			t.Errorf("trechos da sequência documental ausentes no viewer")
		}
	})

	t.Run("Busca textual por título e publicador/autor", func(t *testing.T) {
		// Busca por termo no título
		reqTitle := httptest.NewRequest(http.MethodGet, "/documentos?q=Laudo+Pericial", nil)
		wTitle := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wTitle, reqTitle)

		bodyTitle := wTitle.Body.String()
		if !strings.Contains(bodyTitle, "Laudo Pericial INC nº 50/2026") {
			t.Errorf("busca por título falhou: documento não encontrado")
		}
		if strings.Contains(bodyTitle, "Nota de Esclarecimento Notarial") {
			t.Errorf("busca por título retornou documento não correspondente")
		}

		// Busca por publicador/autor
		reqAuthor := httptest.NewRequest(http.MethodGet, "/documentos?q=Cartório", nil)
		wAuthor := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wAuthor, reqAuthor)

		bodyAuthor := wAuthor.Body.String()
		if !strings.Contains(bodyAuthor, "Nota de Esclarecimento Notarial") {
			t.Errorf("busca por autor falhou: documento não encontrado")
		}
		if strings.Contains(bodyAuthor, "Laudo Pericial INC nº 50/2026") {
			t.Errorf("busca por autor retornou documento não correspondente")
		}
	})

	t.Run("Filtros por source_type e access_status", func(t *testing.T) {
		// Filtro por tipo police_report
		reqType := httptest.NewRequest(http.MethodGet, "/documentos?source_type=police_report", nil)
		wType := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wType, reqType)

		bodyType := wType.Body.String()
		if !strings.Contains(bodyType, "Laudo Pericial INC nº 50/2026") {
			t.Errorf("filtro por source_type falhou: laudo não encontrado")
		}
		if strings.Contains(bodyType, "Nota de Esclarecimento Notarial") {
			t.Errorf("filtro por source_type retornou nota oficial indevidamente")
		}

		// Filtro por access_status unreachable (deve retornar vazio pois a única unreachable está em quarentena)
		reqUnreach := httptest.NewRequest(http.MethodGet, "/documentos?access_status=unreachable", nil)
		wUnreach := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wUnreach, reqUnreach)

		bodyUnreach := wUnreach.Body.String()
		if strings.Contains(bodyUnreach, "Boato de Blog Secreto") {
			t.Errorf("filtro por access_status vazou fonte de quarentena")
		}
		if !strings.Contains(bodyUnreach, "Nenhum documento encontrado") {
			t.Errorf("esperado estado vazio para access_status=unreachable sem fontes públicas")
		}
	})

	t.Run("Ordenação determinística e paginação", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/documentos?sort=title&dir=asc&page=1&page_size=1", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "Página 1 de 2") {
			t.Errorf("indicador de paginação incorreto: %s", body)
		}
	})

	t.Run("Garantia de segurança: ausência de fragmentos brutos, segredos ou dados internos", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/documentos", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		body := w.Body.String()
		forbidden := []string{
			"secret_token_fragment",
			"alvo-quarentena",
			"UNKNOWN_GRADE",
			"admin_editor",
			"password",
			"bcrypt",
		}

		for _, term := range forbidden {
			if strings.Contains(body, term) {
				t.Errorf("vazamento de segurança: termo confidencial/interno %q encontrado no HTML de /documentos", term)
			}
		}
	})
}

func TestPublicDocumentCatalogImmediateModerationInvalidation(t *testing.T) {
	srv, db := setupDocumentCatalogTestServer(t)
	ctx := context.Background()

	// Rejeita o único claim que utiliza src-nota-contexto (clm-cat-2)
	// Após a rejeição, src-nota-contexto não possui mais nenhum suporte ativo em public_claims_view
	// Logo, deve sumir imediatamente do catálogo /documentos e /documentos/src-nota-contexto deve retornar 404
	t.Run("rejeição de claim remove a fonte do catálogo imediatamente", func(t *testing.T) {
		modSvc := moderation.NewService(db)
		queries := sqlc.New(db)

		clm, err := queries.GetClaimByID(ctx, "clm-cat-2")
		if err != nil {
			t.Fatalf("falha ao obter claim: %v", err)
		}

		_, err = modSvc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           "clm-cat-2",
			Action:            domain.ModerationActionReject,
			Reason:            "Desaprovado em teste de catálogo",
			Actor:             "admin_editor",
			ExpectedUpdatedAt: clm.UpdatedAt,
		})
		if err != nil {
			t.Fatalf("falha ao moderar claim: %v", err)
		}

		// 1. Catálogo /documentos não deve mais conter a nota de contexto
		reqCat := httptest.NewRequest(http.MethodGet, "/documentos", nil)
		wCat := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wCat, reqCat)

		bodyCat := wCat.Body.String()
		if strings.Contains(bodyCat, "Nota de Esclarecimento Notarial") {
			t.Errorf("invalidação imediata falhou: fonte ainda consta em /documentos após rejeição de claim")
		}

		// 2. Viewer /documentos/src-nota-contexto deve responder 404 imediatamente
		reqDoc := httptest.NewRequest(http.MethodGet, "/documentos/src-nota-contexto", nil)
		wDoc := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wDoc, reqDoc)

		if wDoc.Code != http.StatusNotFound {
			t.Errorf("invalidação imediata falhou: viewer retornou status %d em vez de 404", wDoc.Code)
		}
	})
}
