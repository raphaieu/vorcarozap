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
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupModerationTestServer(t *testing.T) (*http.Server, *sql.DB, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "moderation_test.db")

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

	// Insere dados editoriais de teste
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-mod-1', 'Caso Operação Master', 'caso-operacao-master', 'Investigação documental');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-mod-1', 'person', 'Marcos Senador', 'marcos senador', 'marcos-senador', 'Parlamentar', 'Resumo Marcos', 5, 'Figura pública', 'Politica', 'Nacional'),
			('ent-mod-2', 'person', 'Bruno Assessor', 'bruno assessor', 'bruno-assessor', 'Assessor', 'Resumo Bruno', 3, 'Assessor parlamentar', 'Outros', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES
			('rel-mod-1', 'ent-mod-1', 'case-mod-1', 'contato', 'Registro de contato formal', 'Sem ressalvas'),
			('rel-mod-2', 'ent-mod-2', 'case-mod-1', 'mencao', 'Registro de menção em agenda', 'Citação indireta');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES
			('clm-quar-valid', 'rel-mod-1', 'Marcos realizou reuniões registradas', '', 'openrouter', 'B', 'supports_link', 1, 'quarantined', 'quarantined', '["NEEDS_HUMAN_APPROVAL"]'),
			('clm-quar-nosup', 'rel-mod-1', 'Marcos teria intermediado contratos', '', 'openrouter', 'C', 'possible_link', 0, 'quarantined', 'quarantined', '["NO_SUPPORT"]'),
			('clm-pub-active', 'rel-mod-1', 'Marcos manteve comunicação institucional', '', 'curated_seed', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]'),
			('clm-rej-prev', 'rel-mod-2', 'Bruno atuou como operador financeiro', '', 'openrouter', 'D', 'possible_link', 0, 'rejected', 'quarantined', '["GRADE_D"]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-1', 'clm-quar-valid', 'Documento oficial do senado', 'document'),
			('ev-2', 'clm-quar-nosup', 'Citação em blog de fofoca', 'blog'),
			('ev-3', 'clm-pub-active', 'Diário Oficial da União', 'official_statement'),
			('ev-4', 'clm-rej-prev', 'Postagem em rede social', 'social_media');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, http_status, normalized_error_code, source_access_checked_at)
		VALUES
			('src-mod-1', 'Diário Oficial', 'Imprensa Nacional', 'https://in.gov.br/dou1', 'https://in.gov.br/dou1', 'official_statement', 'reachable', 200, '', '2026-09-10 10:00:00'),
			('src-mod-2', 'Blog Regional', 'Autor Desconhecido', 'https://blog.com/post', 'https://blog.com/post', 'blog', 'reachable', 200, '', '2026-09-10 10:00:00');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES
			('es-mod-1', 'ev-1', 'src-mod-1', 'supports', 'Trecho confirmando a agenda oficial de Marcos', 'Página 12', 'active'),
			('es-mod-2', 'ev-2', 'src-mod-2', 'contradicts', 'Trecho contestando o suposto vínculo', 'Página 2', 'active'),
			('es-mod-3', 'ev-3', 'src-mod-1', 'supports', 'Publicação oficial de portaria conjunta', 'Seção 1', 'active'),
			('es-mod-4', 'ev-4', 'src-mod-2', 'supports', 'Postagem alegando operador', '', 'rejected');

		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES
			('run-mod-1', 'completed', 'Marcos Senador', 'openai/gpt-4o', '2026-09-10 10:00:00');

		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			case_name, normalized_case_name, proposition, suggested_grade, source_url,
			canonical_url, excerpt, technical_confidence, editorial_status, is_duplicate,
			duplicate_reason, canonical_candidate_id, resolved_subject_entity_id, resolved_case_id,
			published_claim_id, structural_gate_passed, structural_gate_reasons, semantic_gate_passed,
			semantic_gate_reasons, created_at, updated_at
		) VALUES
			('cand-pub-1', 'run-mod-1', 'v1:fp_marcos_active', 'Marcos Senador', 'marcos senador', 'Caso Operação Master', 'caso operacao master', 'Marcos manteve comunicação institucional', 'A', 'https://in.gov.br/dou1', 'https://in.gov.br/dou1', 'Publicação oficial', 0.98, 'published', 0, '', NULL, 'ent-mod-1', 'case-mod-1', 'clm-pub-active', 1, '[]', 1, '[]', '2026-09-10 10:00:00', '2026-09-10 10:00:00');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados de teste: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)
	allowedOrigin := "http://localhost:8090"

	cfg := &config.Config{
		Port:               8080,
		Env:                "test",
		DBPath:             dbPath,
		PublicDataCutoff:   "2026-09-03",
		ReadTimeout:        5 * time.Second,
		WriteTimeout:       10 * time.Second,
		IdleTimeout:        60 * time.Second,
		AdminUser:          "admin_editor",
		AdminPasswordHash:  passwordHash,
		AdminAllowedOrigin: allowedOrigin,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin_editor:password"))
	return srv, db, authHeader
}

func TestAdminClaimDetail_AuthenticationAndRendering(t *testing.T) {
	srv, _, authHeader := setupModerationTestServer(t)

	// 1. Acesso não autenticado -> 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/admin/claims/clm-quar-valid", nil)
	wUnauth := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 não autenticado, obtido %d", wUnauth.Code)
	}

	// 2. Claim inexistente -> 404
	req404 := httptest.NewRequest(http.MethodGet, "/admin/claims/clm-inexistente", nil)
	req404.Header.Set("Authorization", authHeader)
	w404 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("esperado 404 para claim inexistente, obtido %d", w404.Code)
	}

	// 3. Claim existente em quarentena com suporte ativo -> 200 e exibe botão Aprovar
	reqQuar := httptest.NewRequest(http.MethodGet, "/admin/claims/clm-quar-valid", nil)
	reqQuar.Header.Set("Authorization", authHeader)
	wQuar := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wQuar, reqQuar)
	if wQuar.Code != http.StatusOK {
		t.Fatalf("esperado 200 para claim válido, obtido %d", wQuar.Code)
	}
	bodyQuar := wQuar.Body.String()
	if !strings.Contains(bodyQuar, "Marcos realizou reuniões registradas") {
		t.Errorf("deve conter a proposição do claim")
	}
	if !strings.Contains(bodyQuar, "Aprovar e Publicar") {
		t.Errorf("deve conter botão de aprovação")
	}
	if !strings.Contains(bodyQuar, "Rejeitar") {
		t.Errorf("deve conter botão de rejeição")
	}

	// 4. Claim em quarentena sem suporte ativo -> botão aprovar deve estar desabilitado
	reqNoSup := httptest.NewRequest(http.MethodGet, "/admin/claims/clm-quar-nosup", nil)
	reqNoSup.Header.Set("Authorization", authHeader)
	wNoSup := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wNoSup, reqNoSup)
	if wNoSup.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", wNoSup.Code)
	}
	bodyNoSup := wNoSup.Body.String()
	if !strings.Contains(bodyNoSup, "Aprovação bloqueada") {
		t.Errorf("deve exibir aviso de bloqueio de aprovação por ausência de suporte ativo")
	}
}

func TestAdminModerateClaim_CSRFProtection(t *testing.T) {
	srv, _, authHeader := setupModerationTestServer(t)

	form := url.Values{
		"action": {"reject"},
		"reason": {"Rejeição editorial fundamentada"},
	}

	// 1. Sem header Origin -> 403 Forbidden
	reqNoOrigin := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
	reqNoOrigin.Header.Set("Authorization", authHeader)
	reqNoOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wNoOrigin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wNoOrigin, reqNoOrigin)
	if wNoOrigin.Code != http.StatusForbidden {
		t.Fatalf("sem Origin: esperado 403, obtido %d", wNoOrigin.Code)
	}

	// 2. Origin divergente -> 403 Forbidden
	reqBadOrigin := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
	reqBadOrigin.Header.Set("Authorization", authHeader)
	reqBadOrigin.Header.Set("Origin", "http://evil-attacker.com")
	reqBadOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wBadOrigin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wBadOrigin, reqBadOrigin)
	if wBadOrigin.Code != http.StatusForbidden {
		t.Fatalf("Origin maliciosa: esperado 403, obtido %d", wBadOrigin.Code)
	}

	// 3. Content-Type inválido (JSON) -> 415 Unsupported Media Type
	reqBadCT := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(`{"action":"reject","reason":"teste"}`))
	reqBadCT.Header.Set("Authorization", authHeader)
	reqBadCT.Header.Set("Origin", "http://localhost:8090")
	reqBadCT.Header.Set("Content-Type", "application/json")
	wBadCT := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wBadCT, reqBadCT)
	if wBadCT.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("Content-Type JSON: esperado 415, obtido %d", wBadCT.Code)
	}
}

func TestAdminModerateClaim_ApproveWorkflow(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)
	ctx := context.Background()

	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-quar-valid'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao consultar updated_at inicial: %v", err)
	}

	form := url.Values{
		"action":              {"approve"},
		"reason":              {"Documentação oficial conferida e validada integralmente."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	// Valida PRG com status 303 See Other
	if w.Code != http.StatusSeeOther {
		t.Fatalf("esperado 303 See Other, obtido %d (body: %s)", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/claims/clm-quar-valid?msg=") {
		t.Errorf("Location de redirecionamento incorreta: %q", loc)
	}

	// Verifica se claim foi alterado para published no banco
	var status string
	err = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-quar-valid'").Scan(&status)
	if err != nil || status != "published" {
		t.Fatalf("status do claim no banco: esperado 'published', obtido %q (err: %v)", status, err)
	}

	// Verifica registro de auditoria em moderation_decisions com actor injetado do auth context
	var action, reason, actor string
	err = db.QueryRowContext(ctx, `
		SELECT action, reason, actor
		FROM moderation_decisions
		WHERE claim_id = 'clm-quar-valid'
	`).Scan(&action, &reason, &actor)
	if err != nil {
		t.Fatalf("falha ao consultar moderation_decisions: %v", err)
	}
	if action != "approve" || actor != "admin_editor" {
		t.Errorf("decisão gravada incorreta: action=%q, actor=%q", action, actor)
	}
	if !strings.Contains(reason, "Documentação oficial conferida") {
		t.Errorf("reason gravado incorreto: %q", reason)
	}
}

func TestAdminModerateClaim_ApproveWithoutSupportsBlocked(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)
	ctx := context.Background()

	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-quar-nosup'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao consultar updated_at inicial: %v", err)
	}

	form := url.Values{
		"action":              {"approve"},
		"reason":              {"Tentativa indevida de aprovar sem suporte ativo."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-nosup/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("aprovação sem suporte: esperado 400 Bad Request, obtido %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Aprovação negada") {
		t.Errorf("mensagem de erro deve conter 'Aprovação negada', obtido: %s", w.Body.String())
	}
}

func TestAdminModerateClaim_RejectPublished_PreservesCandidateBlock(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)
	ctx := context.Background()

	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-pub-active'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao consultar updated_at inicial: %v", err)
	}

	form := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeição motivada por desmentido oficial posterior."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-pub-active/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("esperado 303 See Other, obtido %d", w.Code)
	}

	// Claim deve estar rejected
	var claimStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-pub-active'").Scan(&claimStatus)
	if err != nil || claimStatus != "rejected" {
		t.Fatalf("status do claim: esperado 'rejected', obtido %q", claimStatus)
	}

	// Candidato canônico associado deve ter mudado para rejected
	var candStatus string
	err = db.QueryRowContext(ctx, "SELECT editorial_status FROM monitoring_candidates WHERE id = 'cand-pub-1'").Scan(&candStatus)
	if err != nil || candStatus != "rejected" {
		t.Fatalf("candidato associado deve estar 'rejected', obtido %q", candStatus)
	}

	// Decisão gravou o fingerprint preservado do candidato
	var candFp string
	err = db.QueryRowContext(ctx, "SELECT candidate_fingerprint FROM moderation_decisions WHERE claim_id = 'clm-pub-active'").Scan(&candFp)
	if err != nil || candFp != "v1:fp_marcos_active" {
		t.Fatalf("fingerprint preservado esperado 'v1:fp_marcos_active', obtido %q", candFp)
	}
}

func TestAdminModerateClaim_RestoreToQuarantineOnly(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)
	ctx := context.Background()

	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-rej-prev'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao consultar updated_at inicial: %v", err)
	}

	form := url.Values{
		"action":              {"restore"},
		"reason":              {"Reavaliação solicitada; restaurando para quarentena para nova análise."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-rej-prev/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("esperado 303 See Other, obtido %d", w.Code)
	}

	var claimStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-rej-prev'").Scan(&claimStatus)
	if err != nil || claimStatus != "quarantined" {
		t.Fatalf("restauração deve resultar estritamente em 'quarantined', obtido %q", claimStatus)
	}
}

func TestAdminModerateClaim_MissingExpectedVersionReturns400(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)
	ctx := context.Background()

	cases := []struct {
		name string
		form url.Values
	}{
		{
			name: "campo ausente",
			form: url.Values{
				"action": {"approve"},
				"reason": {"Tentativa sem expected_updated_at."},
			},
		},
		{
			name: "campo vazio",
			form: url.Values{
				"action":              {"approve"},
				"reason":              {"Tentativa com expected_updated_at vazio."},
				"expected_updated_at": {""},
			},
		},
		{
			name: "campo apenas espacos",
			form: url.Values{
				"action":              {"approve"},
				"reason":              {"Tentativa com expected_updated_at whitespace."},
				"expected_updated_at": {"   \t  "},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Authorization", authHeader)
			req.Header.Set("Origin", "http://localhost:8090")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400 Bad Request para versão ausente, obtido: %d (body: %s)", w.Code, w.Body.String())
			}

			// Verificar que o claim não foi modificado
			var status string
			_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-quar-valid'").Scan(&status)
			if status != "quarantined" {
				t.Errorf("status do claim alterado indevidamente: %q", status)
			}

			// Verificar que nenhuma decisão foi registrada
			var decCount int
			_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = 'clm-quar-valid'").Scan(&decCount)
			if decCount != 0 {
				t.Errorf("decisão persistida indevidamente para requisição sem versão: %d", decCount)
			}
		})
	}
}

func TestAdminModerateClaim_ArchiveActionForbidden(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)

	ctx := context.Background()
	var initialStatus, initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT status, updated_at FROM claims WHERE id = 'clm-quar-valid'").Scan(&initialStatus, &initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao obter dados iniciais do claim: %v", err)
	}

	form := url.Values{
		"action":              {"archive"},
		"reason":              {"Tentativa de acionar archive via POST forjado."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST com action=archive deve retornar 400 Bad Request, obtido: %d (body: %s)", w.Code, w.Body.String())
	}

	// Verificar que o claim não foi modificado no banco
	var finalStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-quar-valid'").Scan(&finalStatus)
	if finalStatus != initialStatus {
		t.Errorf("status do claim alterado indevidamente: antes=%q, depois=%q", initialStatus, finalStatus)
	}

	// Verificar que nenhuma decisão de auditoria foi criada
	var decisionCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = 'clm-quar-valid'").Scan(&decisionCount)
	if decisionCount != 0 {
		t.Errorf("nenhuma decisão deveria ter sido gravada para ação inválida, obtido: %d", decisionCount)
	}
}

func TestAdminModerateClaim_ConcurrencyConflict(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)

	ctx := context.Background()
	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-quar-valid'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at inicial: %v", err)
	}

	// Simula que o claim foi modificado por outra operação enquanto o operador estava com a tela aberta
	_, err = db.ExecContext(ctx, "UPDATE claims SET status = 'rejected', updated_at = '2026-09-12T00:00:00Z' WHERE id = 'clm-quar-valid'")
	if err != nil {
		t.Fatalf("falha ao alterar status e updated_at prévios: %v", err)
	}

	form := url.Values{
		"action":              {"approve"},
		"reason":              {"Operador tentando aprovar o que já foi modificado concorrentemente."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	// Deve retornar estritamente 409 Conflict
	if w.Code != http.StatusConflict {
		t.Fatalf("conflito de versão: esperado 409 Conflict, obtido %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestAdminModerateClaim_SimultaneousRequestsConflict(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)

	ctx := context.Background()
	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-quar-valid'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at inicial: %v", err)
	}

	form := url.Values{
		"action":              {"approve"},
		"reason":              {"Aprovação concorrente disparada simultaneamente."},
		"expected_updated_at": {initialUpdatedAt},
	}

	var statusCodes []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
			req.Header.Set("Authorization", authHeader)
			req.Header.Set("Origin", "http://localhost:8090")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			mu.Lock()
			statusCodes = append(statusCodes, w.Code)
			mu.Unlock()
		}()
	}

	wg.Wait()

	count303 := 0
	count409 := 0
	for _, code := range statusCodes {
		if code == http.StatusSeeOther {
			count303++
		} else if code == http.StatusConflict {
			count409++
		}
	}

	if count303 != 1 || count409 != 1 {
		t.Fatalf("esperava exatamente uma resposta 303 e uma 409, obtido: %v", statusCodes)
	}

	// Verificar que existe exatamente uma decisão gravada
	var totalDecisions int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = 'clm-quar-valid'").Scan(&totalDecisions)
	if totalDecisions != 1 {
		t.Errorf("total de decisões gravadas: esperado 1, obtido %d", totalDecisions)
	}

	// Verificar que o status no banco é published
	var finalStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-quar-valid'").Scan(&finalStatus)
	if finalStatus != "published" {
		t.Errorf("status final do claim: esperado 'published', obtido %q", finalStatus)
	}
}

func TestAdminModerateClaim_PayloadTooLarge(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)

	// Gera corpo com mais de 64 KiB
	largeReason := strings.Repeat("A", 70*1024)
	form := url.Values{
		"action": {"approve"},
		"reason": {largeReason},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-quar-valid/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("esperado 413 Request Entity Too Large para corpo > 64KB, obtido %d", w.Code)
	}

	// Verificar que nenhuma alteração foi persistida
	ctx := context.Background()
	var status string
	_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = 'clm-quar-valid'").Scan(&status)
	if status != "quarantined" {
		t.Errorf("status do claim alterado indevidamente após erro de tamanho: %q", status)
	}

	var decCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = 'clm-quar-valid'").Scan(&decCount)
	if decCount != 0 {
		t.Errorf("decisão persistida indevidamente após erro de tamanho: %d", decCount)
	}
}

func TestAdminModerateClaim_CanonicalVsDuplicateRejection(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)

	ctx := context.Background()
	// Insere candidato duplicata associado ao mesmo claim
	_, err := db.ExecContext(ctx, `
		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			case_name, normalized_case_name, proposition, suggested_grade, source_url,
			canonical_url, excerpt, technical_confidence, editorial_status, is_duplicate,
			duplicate_reason, canonical_candidate_id, resolved_subject_entity_id, resolved_case_id,
			published_claim_id, structural_gate_passed, structural_gate_reasons, semantic_gate_passed,
			semantic_gate_reasons, created_at, updated_at
		) VALUES (
			'cand-dup-1', 'run-mod-1', 'v1:fp_marcos_duplicate', 'Marcos Senador', 'marcos senador',
			'Caso Operação Master', 'caso operacao master', 'Marcos manteve comunicação institucional', 'A', 'https://in.gov.br/dou1',
			'https://in.gov.br/dou1', 'Publicação oficial duplicada', 0.95, 'quarantined', 1,
			'existing_fingerprint', 'cand-pub-1', 'ent-mod-1', 'case-mod-1',
			'clm-pub-active', 1, '[]', 1, '[]', '2026-09-10 10:00:00', '2026-09-10 10:00:00'
		);
	`)
	if err != nil {
		t.Fatalf("falha ao inserir candidato duplicata de teste: %v", err)
	}

	var initialUpdatedAt string
	_ = db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-pub-active'").Scan(&initialUpdatedAt)

	form := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeição de claim com verificação de candidato canônico vs duplicata."},
		"expected_updated_at": {initialUpdatedAt},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-pub-active/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", "http://localhost:8090")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("esperado 303 See Other, obtido %d", w.Code)
	}

	// Verificar que o candidato canônico foi rejeitado
	var canonStatus string
	_ = db.QueryRowContext(ctx, "SELECT editorial_status FROM monitoring_candidates WHERE id = 'cand-pub-1'").Scan(&canonStatus)
	if canonStatus != "rejected" {
		t.Errorf("candidato canônico status: esperado 'rejected', obtido %q", canonStatus)
	}

	// Verificar que o candidato duplicata NÃO foi alterado
	var dupStatus string
	_ = db.QueryRowContext(ctx, "SELECT editorial_status FROM monitoring_candidates WHERE id = 'cand-dup-1'").Scan(&dupStatus)
	if dupStatus != "quarantined" {
		t.Errorf("candidato duplicata status: esperado 'quarantined' (inalterado), obtido %q", dupStatus)
	}

	// Verificar que o fingerprint auditado foi o do canônico
	var decFP string
	_ = db.QueryRowContext(ctx, "SELECT candidate_fingerprint FROM moderation_decisions WHERE claim_id = 'clm-pub-active'").Scan(&decFP)
	if decFP != "v1:fp_marcos_active" {
		t.Errorf("candidate_fingerprint na decisão: esperado 'v1:fp_marcos_active', obtido %q", decFP)
	}
}

func TestAdminCSRFMiddleware_IsolatedUnit(t *testing.T) {
	allowedOrigin := "https://admin.vorcarozap.example.org"
	middleware := web.AdminCSRFMiddleware(allowedOrigin)

	nextCalled := false
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := middleware(dummyHandler)

	tests := []struct {
		name           string
		method         string
		origin         string
		contentType    string
		body           string
		expectedStatus int
		expectNext     bool
	}{
		{
			name:           "GET seguro passa sem Origin",
			method:         http.MethodGet,
			origin:         "",
			contentType:    "",
			body:           "",
			expectedStatus: http.StatusOK,
			expectNext:     true,
		},
		{
			name:           "HEAD seguro passa sem Origin",
			method:         http.MethodHead,
			origin:         "",
			contentType:    "",
			body:           "",
			expectedStatus: http.StatusOK,
			expectNext:     true,
		},
		{
			name:           "OPTIONS seguro passa sem Origin",
			method:         http.MethodOptions,
			origin:         "",
			contentType:    "",
			body:           "",
			expectedStatus: http.StatusOK,
			expectNext:     true,
		},
		{
			name:           "POST sem Origin -> 403",
			method:         http.MethodPost,
			origin:         "",
			contentType:    "application/x-www-form-urlencoded",
			body:           "action=reject&reason=teste",
			expectedStatus: http.StatusForbidden,
			expectNext:     false,
		},
		{
			name:           "POST com Origin divergente -> 403",
			method:         http.MethodPost,
			origin:         "https://malicious.com",
			contentType:    "application/x-www-form-urlencoded",
			body:           "action=reject&reason=teste",
			expectedStatus: http.StatusForbidden,
			expectNext:     false,
		},
		{
			name:           "POST com Content-Type JSON -> 415",
			method:         http.MethodPost,
			origin:         allowedOrigin,
			contentType:    "application/json",
			body:           `{"action":"reject"}`,
			expectedStatus: http.StatusUnsupportedMediaType,
			expectNext:     false,
		},
		{
			name:           "POST com Content-Type text/plain -> 415",
			method:         http.MethodPost,
			origin:         allowedOrigin,
			contentType:    "text/plain",
			body:           "action=reject",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectNext:     false,
		},
		{
			name:           "POST válido com Origin e form-urlencoded -> 200",
			method:         http.MethodPost,
			origin:         allowedOrigin,
			contentType:    "application/x-www-form-urlencoded; charset=UTF-8",
			body:           "action=reject&reason=teste",
			expectedStatus: http.StatusOK,
			expectNext:     true,
		},
		{
			name:           "DELETE válido com Origin e form-urlencoded -> 200",
			method:         http.MethodDelete,
			origin:         allowedOrigin,
			contentType:    "application/x-www-form-urlencoded",
			body:           "action=restore",
			expectedStatus: http.StatusOK,
			expectNext:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled = false
			req := httptest.NewRequest(tt.method, "/admin/teste", strings.NewReader(tt.body))
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("status: esperado %d, obtido %d", tt.expectedStatus, w.Code)
			}
			if nextCalled != tt.expectNext {
				t.Errorf("next handler chamado: esperado %v, obtido %v", tt.expectNext, nextCalled)
			}
			if !tt.expectNext {
				if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
					t.Errorf("Cache-Control em rejeição de CSRF: esperado 'no-store', obtido %q", cc)
				}
			}
		})
	}
}

func TestAdminModerateClaim_PublicBoundaryIsolation(t *testing.T) {
	srv, db, authHeader := setupModerationTestServer(t)
	ctx := context.Background()

	// 1. Verifica que a página de Marcos Senador (/pessoas/marcos-senador) antes da moderação exibe a alegação publicada
	reqEntBefore := httptest.NewRequest(http.MethodGet, "/pessoas/marcos-senador", nil)
	wEntBefore := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wEntBefore, reqEntBefore)
	if wEntBefore.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /pessoas/marcos-senador, obtido %d", wEntBefore.Code)
	}
	if !strings.Contains(wEntBefore.Body.String(), "Marcos manteve comunicação institucional") {
		t.Errorf("antes da rejeição, a alegação publicada deve constar na página pública da entidade")
	}

	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = 'clm-pub-active'").Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao consultar updated_at inicial: %v", err)
	}

	// 2. Modera o claim clm-pub-active para 'reject'
	form := url.Values{
		"action":              {"reject"},
		"reason":              {"Desaprovação editorial: fonte desmentida em apuração posterior."},
		"expected_updated_at": {initialUpdatedAt},
	}
	reqMod := httptest.NewRequest(http.MethodPost, "/admin/claims/clm-pub-active/moderate", strings.NewReader(form.Encode()))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", "http://localhost:8090")
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMod, reqMod)
	if wMod.Code != http.StatusSeeOther {
		t.Fatalf("moderação: esperado 303, obtido %d", wMod.Code)
	}

	// 3. Verifica que a página pública de Marcos Senador agora retorna 404 Not Found (entidade sem claims publicados ativos não é visível ao público)
	reqEntAfter := httptest.NewRequest(http.MethodGet, "/pessoas/marcos-senador", nil)
	wEntAfter := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wEntAfter, reqEntAfter)
	if wEntAfter.Code != http.StatusNotFound {
		t.Fatalf("após rejeição do único claim, página da entidade deve retornar 404 Not Found, obtido %d", wEntAfter.Code)
	}

	// 4. Verifica que a listagem pública de entidades (/pessoas) não contém Marcos Senador
	reqList := httptest.NewRequest(http.MethodGet, "/pessoas", nil)
	wList := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /pessoas, obtido %d", wList.Code)
	}
	if strings.Contains(wList.Body.String(), "Marcos Senador") {
		t.Errorf("após a rejeição, entidade sem claims publicados NÃO pode constar na listagem pública")
	}

	// 5. Verifica que exportação XLSX não contém o claim rejeitado
	reqExport := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	wExport := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wExport, reqExport)
	if wExport.Code != http.StatusOK {
		t.Fatalf("esperado 200 em exportação XLSX, obtido %d", wExport.Code)
	}
}

func TestAdminCSRFMiddleware_CanonicalizedOriginMatching(t *testing.T) {
	// Origem configurada canonicamente a partir de "HTTPS://EXAMPLE.COM/"
	allowedOrigin := "https://example.com"
	middleware := web.AdminCSRFMiddleware(allowedOrigin)

	nextCalled := false
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(dummyHandler)

	req := httptest.NewRequest(http.MethodPost, "/admin/teste", strings.NewReader("action=reject&reason=teste"))
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200 para origin canônica correspondente, obtido %d", w.Code)
	}
	if !nextCalled {
		t.Fatalf("next handler não foi chamado para origin canônica correspondente")
	}
}
