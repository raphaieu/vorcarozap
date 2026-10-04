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

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/moderation"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
	"github.com/raphaieu/vorcarozap/internal/web"
)

// setupAPITestServer cria uma instância isolada do servidor para testes da API v1.
func setupAPITestServer(t *testing.T) (*http.Server, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "api_v1_test.db")

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

	// Popula massa de dados de teste
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-master', 'Caso Banco Master', 'caso-banco-master', 'Investigação e apurações');

		-- Inserção de entidades
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES ('ent-1', 'person', 'Alice Santos & Cia', 'alice santos & cia', 'alice-santos', 'Senadora', 'Resumo Alice', 5, 'Figura pública', 'Politica', 'Nacional'),
		       ('ent-2', 'person', 'Bob Silveira', 'bob silveira', 'bob-silveira', 'Diretor', 'Resumo Bob', 3, 'Executivo citado', 'Empresarial', 'Estadual'),
		       ('ent-3', 'person', 'Carlos Quarentena', 'carlos quarentena', 'carlos-quarentena', 'Assessor', 'Resumo Carlos', 2, 'Assessor', 'Outros', 'Local'),
		       ('ent-4', 'person', 'Diana Sem Suporte', 'diana sem suporte', 'diana-sem-suporte', 'Consultora', 'Resumo Diana', 1, 'Consultora', 'Outros', 'Local'),
		       ('ent-5', 'person', 'Eduardo Contexto', 'eduardo contexto', 'eduardo-contexto', 'Testemunha', 'Resumo Eduardo', 2, 'Contexto', 'Jurídico', 'Nacional');

		-- 1. Alice: Entidade pública ativa (Grau A, Relevância 5, Politica) com relação a Carlos
		INSERT INTO relationships (id, subject_entity_id, target_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-1', 'ent-1', 'ent-3', NULL, 'contato', 'Registro de contato', 'Sem limites adicionais');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-1', 'rel-1', 'Alice manteve conversas documentadas', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-1', 'clm-1', 'Evidência documental de contato', 'document');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status)
		VALUES ('src-1', 'Folha de S.Paulo', 'Redação', 'https://folha.com.br/artigo1#access_token=secret_fake_token_xyz', 'https://folha.com.br/artigo1', 'article', 'reachable'),
		       ('src-2', 'Nota Oficial da Assessoria', 'Assessoria', 'https://assessoria.gov.br/nota', 'https://assessoria.gov.br/nota', 'official_statement', 'reachable');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-1', 'ev-1', 'src-1', 'supports', 'Trecho comprovando contato', 'Página 2', 'active'),
		       ('es-2', 'ev-1', 'src-2', 'contradicts', 'Nota oficial negando', 'Parágrafo 1', 'active');

		-- 2. Bob: Entidade pública ativa (Grau B, Relevância 3, Empresarial) com relação a Caso
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-2', 'ent-2', 'case-master', 'contato', 'Contato de negócios', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-2', 'rel-2', 'Bob enviou mensagens', '', 'curated_seed', 'B', 'supports_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-2', 'clm-2', 'Troca de mensagens', 'message');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-3', 'ev-2', 'src-1', 'supports', 'Mensagens confirmadas', 'Pág. 10', 'active');

		-- 3. Carlos: Entidade em quarentena (NÃO deve aparecer)
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-3', 'ent-3', 'case-master', 'mencao', 'Menção indireta', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-3', 'rel-3', 'Carlos mencionado de passagem', '', 'curated_seed', 'E', 'possible_link', 0, 'quarantined', 'quarantined', '["UNKNOWN_LEGACY_GRADE"]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-3', 'clm-3', 'Evidência em quarentena', 'mention');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-4', 'ev-3', 'src-1', 'supports', 'Citação qualquer', '', 'active');

		-- 4. Diana: Entidade com claim publicado mas SEM suporte ativo (NÃO deve aparecer)
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-4', 'ent-4', 'case-master', 'contato', 'Contato não suportado', '');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-4', 'rel-4', 'Diana apenas contestou', '', 'curated_seed', 'C', 'contradicts_link', 1, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-4', 'clm-4', 'Evidência sem suporte', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-5', 'ev-4', 'src-1', 'contradicts', 'Trecho de contestação', '', 'active');

		-- 5. Eduardo: Entidade pública com claim de contexto e metric_eligible = 0
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES ('rel-5', 'ent-5', 'case-master', 'depoimento', 'Depoimento como testemunha', 'Não é investigado');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES ('clm-5', 'rel-5', 'Eduardo prestou esclarecimentos técnicos', '', 'curated_seed', 'B', 'context_only', 0, 'published', 'contact_confirmed', '[]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES ('ev-5', 'clm-5', 'Termo de depoimento', 'document');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES ('es-6', 'ev-5', 'src-1', 'supports', 'Depoimento prestado à comissão', 'Folha 12', 'active');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir dados de teste da API: %v", err)
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
		t.Fatalf("falha ao instanciar servidor web: %v", err)
	}

	return srv, db
}

func TestAPIV1EntitiesList(t *testing.T) {
	srv, _ := setupAPITestServer(t)

	t.Run("listagem geral de entidades públicas ativas", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d (body: %s)", w.Code, w.Body.String())
		}

		// Valida cabeçalhos obrigatórios
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type: esperado application/json, obtido %q", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
			t.Errorf("Cache-Control: esperado 'no-cache, no-store, must-revalidate', obtido %q", cc)
		}

		var resp web.APIEntitiesListResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("falha ao decodificar JSON: %v", err)
		}

		// Alice, Bob e Eduardo devem constar (3 entidades públicas ativas)
		if len(resp.Data) != 3 {
			t.Fatalf("esperado 3 entidades públicas, obtido %d", len(resp.Data))
		}
		if resp.Pagination.TotalItems != 3 {
			t.Errorf("total_items: esperado 3, obtido %d", resp.Pagination.TotalItems)
		}
		if resp.Pagination.Page != 1 || resp.Pagination.PageSize != 15 || resp.Pagination.TotalPages != 1 {
			t.Errorf("paginação inesperada: %+v", resp.Pagination)
		}

		// Verifica que entidades não-públicas NÃO constam
		for _, item := range resp.Data {
			if item.Slug == "carlos-quarentena" {
				t.Errorf("vazamento de quarentena: carlos-quarentena apareceu na listagem da API")
			}
			if item.Slug == "diana-sem-suporte" {
				t.Errorf("vazamento sem suporte: diana-sem-suporte apareceu na listagem da API")
			}
		}
	})

	t.Run("busca textual por q", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas?q=Alice", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntitiesListResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data) != 1 || resp.Data[0].Slug != "alice-santos" {
			t.Fatalf("esperado apenas alice-santos na busca, obtido: %+v", resp.Data)
		}
	})

	t.Run("filtro por category", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas?category=Empresarial", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntitiesListResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data) != 1 || resp.Data[0].Slug != "bob-silveira" {
			t.Fatalf("esperado apenas bob-silveira no filtro empresarial, obtido: %+v", resp.Data)
		}
	})

	t.Run("filtro por grade", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas?grade=A", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntitiesListResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data) != 1 || resp.Data[0].Slug != "alice-santos" {
			t.Fatalf("esperado apenas alice-santos no filtro de Grau A, obtido: %+v", resp.Data)
		}
	})

	t.Run("filtro por relevance", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas?relevance=3", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntitiesListResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data) != 1 || resp.Data[0].Slug != "bob-silveira" {
			t.Fatalf("esperado apenas bob-silveira no filtro de relevância 3, obtido: %+v", resp.Data)
		}
	})

	t.Run("paginação e ordenação", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas?page=1&page_size=2&sort=name&dir=asc", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntitiesListResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data) != 2 {
			t.Fatalf("esperado 2 itens na página 1, obtido %d", len(resp.Data))
		}
		if resp.Pagination.TotalItems != 3 || resp.Pagination.TotalPages != 2 {
			t.Errorf("paginação incorreta: %+v", resp.Pagination)
		}

		// Segunda página
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas?page=2&page_size=2&sort=name&dir=asc", nil)
		w2 := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w2, req2)

		var resp2 web.APIEntitiesListResponse
		_ = json.NewDecoder(w2.Body).Decode(&resp2)
		if len(resp2.Data) != 1 {
			t.Fatalf("esperado 1 item na página 2, obtido %d", len(resp2.Data))
		}
	})
}

func TestAPIV1ParameterValidation(t *testing.T) {
	srv, _ := setupAPITestServer(t)

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCode   string
	}{
		{"page menor que 1", "?page=0", http.StatusBadRequest, "bad_request"},
		{"page acima do limite máximo (10000)", "?page=10001", http.StatusBadRequest, "bad_request"},
		{"page com valor astronômico", "?page=999999999999999", http.StatusBadRequest, "bad_request"},
		{"page não-numérico", "?page=abc", http.StatusBadRequest, "bad_request"},
		{"page_size menor que 1", "?page_size=0", http.StatusBadRequest, "bad_request"},
		{"page_size acima do limite máximo (50)", "?page_size=100", http.StatusBadRequest, "bad_request"},
		{"page_size não-numérico", "?page_size=xyz", http.StatusBadRequest, "bad_request"},
		{"grade inválido", "?grade=Z", http.StatusBadRequest, "bad_request"},
		{"relevance fora da faixa 1-5", "?relevance=10", http.StatusBadRequest, "bad_request"},
		{"relevance não-numérico", "?relevance=alto", http.StatusBadRequest, "bad_request"},
		{"period inválido", "?period=ontem", http.StatusBadRequest, "bad_request"},
		{"sort fora da allowlist", "?sort=cartao_credito", http.StatusBadRequest, "bad_request"},
		{"dir inválido", "?dir=lateral", http.StatusBadRequest, "bad_request"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas"+tc.query, nil)
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status: esperado %d, obtido %d (body: %s)", tc.wantStatus, w.Code, w.Body.String())
			}

			var errResp web.APIErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("falha ao decodificar JSON de erro: %v", err)
			}
			if errResp.Error.Code != tc.wantCode {
				t.Errorf("código de erro: esperado %q, obtido %q", tc.wantCode, errResp.Error.Code)
			}
			if errResp.Error.Message == "" {
				t.Errorf("mensagem de erro vazia")
			}
		})
	}
}

func TestAPIV1EntityDetail(t *testing.T) {
	srv, _ := setupAPITestServer(t)

	t.Run("detalhe com sucesso de entidade pública (alice-santos)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas/alice-santos", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d (body: %s)", w.Code, w.Body.String())
		}

		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type incorreto: %s", ct)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
			t.Errorf("Cache-Control incorreto: %s", cc)
		}

		var resp web.APIEntityDetailResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("falha ao decodificar JSON: %v", err)
		}

		ent := resp.Data
		if ent.Slug != "alice-santos" || ent.Name != "Alice Santos & Cia" {
			t.Errorf("dados da entidade incorretos: %+v", ent)
		}
		if ent.Relevance != 5 || ent.RoleOrContext != "Senadora" {
			t.Errorf("atributos da entidade incorretos: %+v", ent)
		}

		if len(ent.Claims) != 1 {
			t.Fatalf("esperado 1 claim público, obtido %d", len(ent.Claims))
		}

		clm := ent.Claims[0]
		if clm.ID != "clm-1" || clm.Grade != "A" || clm.Disposition != "supports_link" || !clm.MetricEligible {
			t.Errorf("claim incorreto: %+v", clm)
		}
		if clm.TargetEntityName != "Carlos Quarentena" {
			t.Errorf("target_entity_name esperado 'Carlos Quarentena', obtido %q", clm.TargetEntityName)
		}
		if clm.CaseName != "" {
			t.Errorf("case_name esperado vazio para relação com entidade, obtido %q", clm.CaseName)
		}

		// Fontes associadas ao claim
		if len(clm.Sources) != 2 {
			t.Fatalf("esperado 2 fontes (supports e contradicts), obtido %d", len(clm.Sources))
		}

		var supFound, contrFound bool
		for _, s := range clm.Sources {
			if s.Role == "supports" {
				supFound = true
				if s.Locator != "Pág. 2" {
					t.Errorf("locator de suporte incorreto: %s", s.Locator)
				}
				if s.Excerpt != "Trecho comprovando contato" {
					t.Errorf("excerpt de suporte incorreto: %s", s.Excerpt)
				}
				if s.CanonicalURL != "https://folha.com.br/artigo1" {
					t.Errorf("canonical_url incorreto: %s", s.CanonicalURL)
				}
			}
			if s.Role == "contradicts" {
				contrFound = true
				if s.Excerpt != "Nota oficial negando" {
					t.Errorf("excerpt de contraditório incorreto: %s", s.Excerpt)
				}
			}
		}
		if !supFound || !contrFound {
			t.Errorf("esperado fontes supports e contradicts presentes: sup=%v, contr=%v", supFound, contrFound)
		}
	})

	t.Run("detalhe com case_name (bob-silveira)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas/bob-silveira", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntityDetailResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("falha ao decodificar JSON: %v", err)
		}

		if len(resp.Data.Claims) != 1 {
			t.Fatalf("esperado 1 claim para Bob, obtido %d", len(resp.Data.Claims))
		}
		if resp.Data.Claims[0].CaseName != "Caso Banco Master" {
			t.Errorf("case_name: esperado 'Caso Banco Master', obtido %q", resp.Data.Claims[0].CaseName)
		}
		if resp.Data.Claims[0].TargetEntityName != "" {
			t.Errorf("target_entity_name esperado vazio para relação com caso, obtido %q", resp.Data.Claims[0].TargetEntityName)
		}
	})

	t.Run("claim público de contexto com metric_eligible = false (eduardo-contexto)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas/eduardo-contexto", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status: esperado 200, obtido %d", w.Code)
		}

		var resp web.APIEntityDetailResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if len(resp.Data.Claims) != 1 {
			t.Fatalf("esperado 1 claim de contexto, obtido %d", len(resp.Data.Claims))
		}

		clm := resp.Data.Claims[0]
		if clm.MetricEligible {
			t.Errorf("metric_eligible: esperado false para claim de contexto, obtido true")
		}
		if clm.Disposition != "context_only" {
			t.Errorf("disposition: esperado 'context_only', obtido %q", clm.Disposition)
		}
	})

	t.Run("404 idêntico e controlado para slug inexistente, quarentena e sem suporte", func(t *testing.T) {
		slugs := []string{
			"inexistente-slug",
			"carlos-quarentena", // em quarentena
			"diana-sem-suporte", // sem suporte ativo
		}

		for _, slug := range slugs {
			t.Run(slug, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas/"+slug, nil)
				w := httptest.NewRecorder()
				srv.Handler.ServeHTTP(w, req)

				if w.Code != http.StatusNotFound {
					t.Fatalf("slug %q: esperado 404, obtido %d (body: %s)", slug, w.Code, w.Body.String())
				}

				var errResp web.APIErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
					t.Fatalf("falha ao decodificar JSON de erro: %v", err)
				}
				if errResp.Error.Code != "not_found" {
					t.Errorf("código de erro: esperado 'not_found', obtido %q", errResp.Error.Code)
				}
				if errResp.Error.Message != "Entidade pública não encontrada." {
					t.Errorf("mensagem de erro: esperado 'Entidade pública não encontrada.', obtido %q", errResp.Error.Message)
				}
			})
		}
	})
}

func TestAPIV1ImmediateReflectionAfterModeration(t *testing.T) {
	srv, db := setupAPITestServer(t)
	ctx := context.Background()

	// 1. Desaprovar / rejeitar claim da Alice (clm-1)
	// Alice não possui outros claims públicos, logo /api/v1/pessoas/alice-santos deve passar a retornar 404
	// e Alice deve sumir da listagem /api/v1/pessoas imediatamente
	t.Run("rejeição de claim remove entidade da API imediatamente", func(t *testing.T) {
		modSvc := moderation.NewService(db)

		queries := sqlc.New(db)
		clm, err := queries.GetClaimByID(ctx, "clm-1")
		if err != nil {
			t.Fatalf("falha ao obter claim: %v", err)
		}

		_, err = modSvc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           "clm-1",
			Action:            domain.ModerationActionReject,
			Reason:            "Desaprovado em teste de API",
			Actor:             "admin_tester",
			ExpectedUpdatedAt: clm.UpdatedAt,
		})
		if err != nil {
			t.Fatalf("falha ao moderar claim: %v", err)
		}

		// Imediatamente: GET /api/v1/pessoas/alice-santos deve retornar 404
		reqDetail := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas/alice-santos", nil)
		wDetail := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wDetail, reqDetail)
		if wDetail.Code != http.StatusNotFound {
			t.Errorf("invalidação imediata: esperado 404 após rejeição, obtido %d", wDetail.Code)
		}

		// Imediatamente: GET /api/v1/pessoas não deve conter Alice
		reqList := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas", nil)
		wList := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wList, reqList)

		var listResp web.APIEntitiesListResponse
		_ = json.NewDecoder(wList.Body).Decode(&listResp)
		for _, item := range listResp.Data {
			if item.Slug == "alice-santos" {
				t.Errorf("invalidação imediata: alice-santos ainda consta na listagem após rejeição de claim")
			}
		}
	})

	// 2. Rejeitar o único evidence_source ativo do Bob (es-3)
	// Bob possui apenas es-3 com role='supports'. Ao desativá-lo, o claim clm-2 entra em quarentena automaticamente
	// e Bob desaparece imediatamente da API pública.
	t.Run("rejeição do último suporte documental coloca claim em quarentena e remove da API", func(t *testing.T) {
		modSvc := moderation.NewService(db)

		queries := sqlc.New(db)
		es, err := queries.GetAdminEvidenceSourceByID(ctx, "es-3")
		if err != nil {
			t.Fatalf("falha ao obter evidence_source: %v", err)
		}

		_, err = modSvc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
			EvidenceSourceID:  "es-3",
			Action:            domain.ModerationActionReject,
			Reason:            "Trecho documental desaprovado",
			Actor:             "admin_tester",
			ExpectedUpdatedAt: es.EvidenceSourceUpdatedAt,
		})
		if err != nil {
			t.Fatalf("falha ao moderar evidence_source: %v", err)
		}

		// Imediatamente: GET /api/v1/pessoas/bob-silveira deve retornar 404
		reqBob := httptest.NewRequest(http.MethodGet, "/api/v1/pessoas/bob-silveira", nil)
		wBob := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wBob, reqBob)
		if wBob.Code != http.StatusNotFound {
			t.Errorf("invalidação imediata: esperado 404 para Bob após perda de suporte, obtido %d", wBob.Code)
		}
	})
}

func TestAPIV1HTTPMethodsAndSecurity(t *testing.T) {
	srv, _ := setupAPITestServer(t)

	t.Run("métodos não-GET recebem 405 Method Not Allowed em JSON", func(t *testing.T) {
		methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

		for _, method := range methods {
			req := httptest.NewRequest(method, "/api/v1/pessoas", nil)
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("método %s em /api/v1/pessoas: esperado 405, obtido %d", method, w.Code)
			}

			var errResp web.APIErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("falha ao decodificar erro 405: %v", err)
			}
			if errResp.Error.Code != "method_not_allowed" {
				t.Errorf("código de erro: esperado 'method_not_allowed', obtido %q", errResp.Error.Code)
			}
		}
	})

	t.Run("rota inexistente sob /api/v1 retorna 404 em JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/rota-inexistente", nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("esperado 404, obtido %d", w.Code)
		}

		var errResp web.APIErrorResponse
		_ = json.NewDecoder(w.Body).Decode(&errResp)
		if errResp.Error.Code != "not_found" {
			t.Errorf("código de erro: esperado 'not_found', obtido %q", errResp.Error.Code)
		}
	})

	t.Run("garantia estrita de ausência de segredos, custos ou dados administrativos no JSON", func(t *testing.T) {
		endpoints := []string{
			"/api/v1/pessoas",
			"/api/v1/pessoas/alice-santos",
		}

		forbiddenKeys := []string{
			"password",
			"hash",
			"bcrypt",
			"secret",
			"session",
			"mfa",
			"token",
			"cost_micro_usd",
			"raw_response",
			"fingerprint",
			"quarantine_reasons",
			"actor",
			"moderation_reason",
			"original_url",
			"target_entity_slug",
			"case_slug",
			"secret_fake_token_xyz",
			"carlos-quarentena", // slug do alvo em quarentena não pode vazar
		}

		for _, ep := range endpoints {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			bodyLower := strings.ToLower(w.Body.String())
			for _, key := range forbiddenKeys {
				if strings.Contains(bodyLower, `"`+key+`"`) || strings.Contains(bodyLower, `"`+key+`_`) || strings.Contains(bodyLower, key) {
					t.Errorf("vazamento de dado confidencial/interno no endpoint %s: chave/termo proibido %q encontrado no JSON: %s", ep, key, w.Body.String())
				}
			}
		}
	})
}

func TestStorePaginationBoundsProtection(t *testing.T) {
	_, db := setupAPITestServer(t)
	ctx := context.Background()

	// Testa chamada direta ao store com Page extremo
	res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{
		Page:     999999999,
		PageSize: 15,
	})
	if err != nil {
		t.Fatalf("ListPublicEntities falhou com página astronômica: %v", err)
	}
	if res.Page != store.MaxPage {
		t.Errorf("Page não foi limitado a MaxPage (%d), obtido %d", store.MaxPage, res.Page)
	}
}
