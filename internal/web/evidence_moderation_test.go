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

func setupEvidenceModerationTestServer(t *testing.T) (*http.Server, *sql.DB, string, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "evidence_moderation_test.db")

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
		VALUES ('case-es-1', 'Operação Master Fake', 'operacao-master-fake', 'Investigação documental de teste');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-es-1', 'person', 'Marcos Investigado', 'marcos investigado', 'marcos-investigado', 'Parlamentar', 'Resumo Marcos', 5, 'Figura pública', 'Politica', 'Nacional'),
			('ent-es-2', 'person', 'Bruno Assessor', 'bruno assessor', 'bruno-assessor', 'Assessor', 'Resumo Bruno', 3, 'Assessor', 'Outros', 'Nacional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
		VALUES
			('rel-es-1', 'ent-es-1', 'case-es-1', 'contato', 'Registro de contato formal', 'Sem ressalvas'),
			('rel-es-2', 'ent-es-2', 'case-es-1', 'mencao', 'Registro de menção', 'Citação indireta');

		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons)
		VALUES
			('clm-single-sup', 'rel-es-1', 'Marcos participou de reunião decisória', '', 'openrouter', 'A', 'supports_link', 1, 'published', 'contact_confirmed', '[]'),
			('clm-multi-sup', 'rel-es-1', 'Marcos manteve comunicação frequente', '', 'openrouter', 'B', 'supports_link', 1, 'published', 'contact_confirmed', '[]'),
			('clm-quar-sup', 'rel-es-2', 'Bruno atuou como intermediário financeiro', '', 'openrouter', 'C', 'possible_link', 0, 'quarantined', 'quarantined', '["NEEDS_HUMAN_APPROVAL"]');

		INSERT INTO evidence (id, claim_id, summary, evidence_type)
		VALUES
			('ev-single-1', 'clm-single-sup', 'Ata oficial de reunião', 'document'),
			('ev-multi-1', 'clm-multi-sup', 'Registro de ligações', 'document'),
			('ev-multi-2', 'clm-multi-sup', 'E-mails corporativos', 'document'),
			('ev-quar-1', 'clm-quar-sup', 'Relatório policial', 'police_report');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, http_status, normalized_error_code, source_access_checked_at)
		VALUES
			('src-test-1', 'Diário Oficial da União', 'Imprensa Nacional', 'https://in.gov.br/dou1', 'https://in.gov.br/dou1', 'official_statement', 'reachable', 200, '', '2026-09-10 10:00:00'),
			('src-test-2', 'Portal de Notícias', 'Redação Central', 'https://noticias.com/artigo1', 'https://noticias.com/artigo1', 'article', 'reachable', 200, '', '2026-09-10 10:00:00');

		INSERT INTO evidence_sources (id, evidence_id, source_id, role, excerpt, locator, status)
		VALUES
			('es-single-1', 'ev-single-1', 'src-test-1', 'supports', 'Trecho literal comprovatório da reunião', 'Página 10', 'active'),
			('es-multi-1', 'ev-multi-1', 'src-test-1', 'supports', 'Trecho com registros de chamadas telefônicas', 'Página 25', 'active'),
			('es-multi-2', 'ev-multi-2', 'src-test-2', 'supports', 'Trecho com mensagens trocadas por correio eletrônico', 'Anexo B', 'active'),
			('es-quar-1', 'ev-quar-1', 'src-test-2', 'supports', 'Trecho do relatório policial citando Bruno', 'Folha 4', 'rejected');

		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES ('run-es-1', 'completed', 'Marcos Investigado', 'openai/gpt-4o', '2026-09-10 10:00:00');

		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			case_name, normalized_case_name, proposition, suggested_grade, source_url,
			canonical_url, excerpt, technical_confidence, editorial_status, is_duplicate,
			duplicate_reason, canonical_candidate_id, resolved_subject_entity_id, resolved_case_id,
			published_claim_id, structural_gate_passed, structural_gate_reasons, semantic_gate_passed,
			semantic_gate_reasons, created_at, updated_at
		) VALUES
			('cand-es-1', 'run-es-1', 'v1:fp_marcos_single', 'Marcos Investigado', 'marcos investigado', 'Operação Master Fake', 'operacao master fake', 'Marcos participou de reunião decisória', 'A', 'https://in.gov.br/dou1', 'https://in.gov.br/dou1', 'Trecho reunião', 0.98, 'published', 0, '', NULL, 'ent-es-1', 'case-es-1', 'clm-single-sup', 1, '[]', 1, '[]', '2026-09-10 10:00:00', '2026-09-10 10:00:00');
	`)
	if err != nil {
		t.Fatalf("falha ao popular dados de teste: %v", err)
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
		t.Fatalf("falha ao instanciar web server: %v", err)
	}

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin_editor:password"))
	return srv, db, allowedOrigin, authHeader
}

func getEvidenceSourceUpdatedAt(t *testing.T, db *sql.DB, esID string) string {
	t.Helper()
	var updatedAt string
	err := db.QueryRow("SELECT updated_at FROM evidence_sources WHERE id = ?", esID).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at de evidence_source %s: %v", esID, err)
	}
	return updatedAt
}

func TestAdminEvidenceSourceDetail_AuthAndRender(t *testing.T) {
	srv, _, _, authHeader := setupEvidenceModerationTestServer(t)

	// 1. Acesso não autenticado retorna 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-single-1", nil)
	recUnauth := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("acesso sem auth: esperado 401, obtido %d", recUnauth.Code)
	}

	// 2. Acesso autenticado a ID existente retorna 200 OK com HTML completo
	req := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-single-1", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/evidencias/es-single-1: esperado 200 OK, obtido %d (corpo: %s)", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "es-single-1") {
		t.Errorf("HTML deve conter o ID do evidence_source 'es-single-1'")
	}
	if !strings.Contains(body, "Trecho literal comprovatório da reunião") {
		t.Errorf("HTML deve conter o trecho literal da evidência")
	}
	if !strings.Contains(body, "Diário Oficial da União") {
		t.Errorf("HTML deve conter o título da fonte documental")
	}
	if !strings.Contains(body, "Marcos participou de reunião decisória") {
		t.Errorf("HTML deve conter a proposição do claim associado")
	}
	if !strings.Contains(body, `name="expected_updated_at"`) {
		t.Errorf("HTML deve conter o campo oculto expected_updated_at para controle otimista de versão")
	}
	if !strings.Contains(body, "Desaprovar / Rejeitar Uso") {
		t.Errorf("HTML deve conter a ação de rejeição permitida para uso ativo")
	}

	// 3. ID inexistente retorna 404 Not Found
	req404 := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-inexistente", nil)
	req404.Header.Set("Authorization", authHeader)
	rec404 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusNotFound {
		t.Errorf("GET /admin/evidencias/es-inexistente: esperado 404, obtido %d", rec404.Code)
	}
}

func TestAdminEvidenceSourceDetail_WarningOnLastSupport(t *testing.T) {
	srv, _, _, authHeader := setupEvidenceModerationTestServer(t)

	// 1. es-single-1 é o único suporte ativo de clm-single-sup (published) -> deve exibir alerta de último suporte
	req1 := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-single-1", nil)
	req1.Header.Set("Authorization", authHeader)
	rec1 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("GET es-single-1 falhou: %d", rec1.Code)
	}
	body1 := rec1.Body.String()
	if !strings.Contains(body1, "Aviso Editorial Crítico") || !strings.Contains(body1, "ÚLTIMO suporte ativo") {
		t.Errorf("es-single-1 deveria exibir o alerta de último suporte ativo")
	}

	// 2. es-multi-1 tem outro suporte ativo (es-multi-2) -> NÃO deve exibir alerta de último suporte
	req2 := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-multi-1", nil)
	req2.Header.Set("Authorization", authHeader)
	rec2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("GET es-multi-1 falhou: %d", rec2.Code)
	}
	body2 := rec2.Body.String()
	if strings.Contains(body2, "Aviso Editorial Crítico") {
		t.Errorf("es-multi-1 não deveria exibir o alerta de último suporte ativo, pois es-multi-2 também está ativo")
	}
}

func TestAdminModerateEvidenceSource_CSRFProtection(t *testing.T) {
	srv, db, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)
	esUpdatedAt := getEvidenceSourceUpdatedAt(t, db, "es-single-1")

	formData := url.Values{
		"action":              {"reject"},
		"reason":              {"Justificativa válida para teste de CSRF."},
		"expected_updated_at": {esUpdatedAt},
	}.Encode()

	// 1. Requisição sem cabeçalho Origin -> 403 Forbidden
	reqNoOrigin := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formData))
	reqNoOrigin.Header.Set("Authorization", authHeader)
	reqNoOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recNoOrigin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recNoOrigin, reqNoOrigin)

	if recNoOrigin.Code != http.StatusForbidden {
		t.Errorf("sem Origin: esperado 403 Forbidden, obtido %d", recNoOrigin.Code)
	}

	// 2. Requisição com Origin divergente -> 403 Forbidden
	reqBadOrigin := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formData))
	reqBadOrigin.Header.Set("Authorization", authHeader)
	reqBadOrigin.Header.Set("Origin", "http://evil.com")
	reqBadOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recBadOrigin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recBadOrigin, reqBadOrigin)

	if recBadOrigin.Code != http.StatusForbidden {
		t.Errorf("Origin divergente: esperado 403 Forbidden, obtido %d", recBadOrigin.Code)
	}

	// 3. Requisição com Content-Type inválido -> 415 Unsupported Media Type
	reqBadCT := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formData))
	reqBadCT.Header.Set("Authorization", authHeader)
	reqBadCT.Header.Set("Origin", allowedOrigin)
	reqBadCT.Header.Set("Content-Type", "application/json")
	recBadCT := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recBadCT, reqBadCT)

	if recBadCT.Code != http.StatusUnsupportedMediaType {
		t.Errorf("Content-Type incorreto: esperado 415, obtido %d", recBadCT.Code)
	}

	// 4. Requisição com payload > 64 KiB -> 413 Request Entity Too Large
	largeReason := strings.Repeat("A", 65*1024)
	largeData := url.Values{
		"action":              {"reject"},
		"reason":              {largeReason},
		"expected_updated_at": {esUpdatedAt},
	}.Encode()

	reqLarge := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(largeData))
	reqLarge.Header.Set("Authorization", authHeader)
	reqLarge.Header.Set("Origin", allowedOrigin)
	reqLarge.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLarge := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recLarge, reqLarge)

	if recLarge.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("payload excessivo: esperado 413, obtido %d", recLarge.Code)
	}
}

func TestAdminModerateEvidenceSource_RejectLastSupport_QuarantinesClaimAndIsolatesPublicBoundary(t *testing.T) {
	srv, db, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)

	// Verificar visibilidade pública antes da moderação
	reqPubBefore := httptest.NewRequest(http.MethodGet, "/pessoas/marcos-investigado", nil)
	recPubBefore := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recPubBefore, reqPubBefore)
	if recPubBefore.Code != http.StatusOK {
		t.Fatalf("entidade antes da moderação: esperado 200 OK, obtido %d", recPubBefore.Code)
	}
	if !strings.Contains(recPubBefore.Body.String(), "Marcos participou de reunião decisória") {
		t.Fatalf("página pública da entidade deveria exibir o claim antes da moderação")
	}

	// Obter versão inicial de es-single-1
	esUpdatedAt := getEvidenceSourceUpdatedAt(t, db, "es-single-1")

	formData := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeitando uso de evidência truncado e fora de contexto."},
		"expected_updated_at": {esUpdatedAt},
	}.Encode()

	reqMod := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formData))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", allowedOrigin)
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recMod, reqMod)

	// Resposta deve ser 303 See Other (PRG)
	if recMod.Code != http.StatusSeeOther {
		t.Fatalf("POST /admin/evidencias/es-single-1/moderate: esperado 303 See Other, obtido %d (corpo: %s)", recMod.Code, recMod.Body.String())
	}

	loc := recMod.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/evidencias/es-single-1?msg=") {
		t.Errorf("Location = %q, esperado redirecionamento para /admin/evidencias/es-single-1 com msg", loc)
	}

	// Verificar estado no SQLite
	var esStatus string
	err := db.QueryRow("SELECT status FROM evidence_sources WHERE id = 'es-single-1'").Scan(&esStatus)
	if err != nil {
		t.Fatalf("falha ao consultar evidence_sources: %v", err)
	}
	if esStatus != "rejected" {
		t.Errorf("evidence_sources status = %q, esperado 'rejected'", esStatus)
	}

	var claimStatus string
	var metricEligible int
	err = db.QueryRow("SELECT status, metric_eligible FROM claims WHERE id = 'clm-single-sup'").Scan(&claimStatus, &metricEligible)
	if err != nil {
		t.Fatalf("falha ao consultar claims: %v", err)
	}
	if claimStatus != "quarantined" {
		t.Errorf("claims status = %q, esperado 'quarantined' (perdeu o último suporte)", claimStatus)
	}
	if metricEligible != 0 {
		t.Errorf("claims metric_eligible = %d, esperado 0", metricEligible)
	}

	// Verificar auditoria em moderation_decisions (com XOR)
	var decClaimID sql.NullString
	var decESID sql.NullString
	var decActor, decReason string
	err = db.QueryRow(`
		SELECT claim_id, evidence_source_id, actor, reason
		FROM moderation_decisions
		WHERE evidence_source_id = 'es-single-1'
	`).Scan(&decClaimID, &decESID, &decActor, &decReason)
	if err != nil {
		t.Fatalf("falha ao consultar decisão de moderação: %v", err)
	}
	if decClaimID.Valid {
		t.Errorf("claim_id na decisão deve ser nulo (XOR), obtido %q", decClaimID.String)
	}
	if !decESID.Valid || decESID.String != "es-single-1" {
		t.Errorf("evidence_source_id na decisão = %+v, esperado 'es-single-1'", decESID)
	}
	if decActor != "admin_editor" {
		t.Errorf("actor na decisão = %q, esperado 'admin_editor'", decActor)
	}

	// Verificar isolamento da fronteira pública pós-moderação: o claim não pode aparecer
	reqPubAfter := httptest.NewRequest(http.MethodGet, "/pessoas/marcos-investigado", nil)
	recPubAfter := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recPubAfter, reqPubAfter)

	// marcos-investigado ainda tem clm-multi-sup publicado, então a página responde 200, mas clm-single-sup sumiu!
	if strings.Contains(recPubAfter.Body.String(), "Marcos participou de reunião decisória") {
		t.Errorf("claim colocado em quarentena NÃO deve aparecer na página pública da entidade!")
	}
}

func TestAdminModerateEvidenceSource_RejectNonLastSupport_ClaimRemainsPublished(t *testing.T) {
	srv, db, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)

	es1UpdatedAt := getEvidenceSourceUpdatedAt(t, db, "es-multi-1")

	formData := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeitando primeiro suporte; claim ainda possui segundo suporte ativo."},
		"expected_updated_at": {es1UpdatedAt},
	}.Encode()

	reqMod := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-multi-1/moderate", strings.NewReader(formData))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", allowedOrigin)
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recMod, reqMod)

	if recMod.Code != http.StatusSeeOther {
		t.Fatalf("POST falhou: esperado 303, obtido %d", recMod.Code)
	}

	// clm-multi-sup deve continuar publicado
	var claimStatus string
	var metricEligible int
	err := db.QueryRow("SELECT status, metric_eligible FROM claims WHERE id = 'clm-multi-sup'").Scan(&claimStatus, &metricEligible)
	if err != nil {
		t.Fatalf("falha ao consultar claims: %v", err)
	}
	if claimStatus != "published" {
		t.Errorf("claim status = %q, esperado 'published' (ainda possui es-multi-2 ativo)", claimStatus)
	}
	if metricEligible != 1 {
		t.Errorf("claim metric_eligible = %d, esperado 1", metricEligible)
	}
}

func TestAdminModerateEvidenceSource_Restore_KeepsClaimInQuarantine(t *testing.T) {
	srv, db, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)

	esQuarUpdatedAt := getEvidenceSourceUpdatedAt(t, db, "es-quar-1")

	formData := url.Values{
		"action":              {"restore"},
		"reason":              {"Restaurando uso de evidência para ativo."},
		"expected_updated_at": {esQuarUpdatedAt},
	}.Encode()

	reqMod := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-quar-1/moderate", strings.NewReader(formData))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", allowedOrigin)
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recMod, reqMod)

	if recMod.Code != http.StatusSeeOther {
		t.Fatalf("POST restore falhou: esperado 303, obtido %d", recMod.Code)
	}

	// es-quar-1 deve ter virado active
	var esStatus string
	err := db.QueryRow("SELECT status FROM evidence_sources WHERE id = 'es-quar-1'").Scan(&esStatus)
	if err != nil {
		t.Fatalf("falha ao consultar evidence_source: %v", err)
	}
	if esStatus != "active" {
		t.Errorf("evidence_source status = %q, esperado 'active'", esStatus)
	}

	// clm-quar-sup DEVE continuar em quarentena (invariante: restauração de evidência nunca republica claim)
	var claimStatus string
	err = db.QueryRow("SELECT status FROM claims WHERE id = 'clm-quar-sup'").Scan(&claimStatus)
	if err != nil {
		t.Fatalf("falha ao consultar claim: %v", err)
	}
	if claimStatus != "quarantined" {
		t.Errorf("claim status = %q, esperado 'quarantined'", claimStatus)
	}
}

func TestAdminModerateEvidenceSource_OCC_Conflict(t *testing.T) {
	srv, _, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)

	// 1. expected_updated_at ausente -> 400 Bad Request
	formNoVersion := url.Values{
		"action": {"reject"},
		"reason": {"Justificativa sem versão"},
	}.Encode()

	reqNoVersion := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formNoVersion))
	reqNoVersion.Header.Set("Authorization", authHeader)
	reqNoVersion.Header.Set("Origin", allowedOrigin)
	reqNoVersion.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recNoVersion := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recNoVersion, reqNoVersion)

	if recNoVersion.Code != http.StatusBadRequest {
		t.Errorf("versão ausente: esperado 400 Bad Request, obtido %d", recNoVersion.Code)
	}

	// 2. expected_updated_at divergente -> 409 Conflict
	formStale := url.Values{
		"action":              {"reject"},
		"reason":              {"Justificativa versão antiga"},
		"expected_updated_at": {"2020-01-01T00:00:00Z"},
	}.Encode()

	reqStale := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formStale))
	reqStale.Header.Set("Authorization", authHeader)
	reqStale.Header.Set("Origin", allowedOrigin)
	reqStale.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recStale := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recStale, reqStale)

	if recStale.Code != http.StatusConflict {
		t.Errorf("versão stale: esperado 409 Conflict, obtido %d", recStale.Code)
	}
}

func TestAdminModerateEvidenceSource_ValidationErrors(t *testing.T) {
	srv, db, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)
	esUpdatedAt := getEvidenceSourceUpdatedAt(t, db, "es-single-1")

	tests := []struct {
		name       string
		action     string
		reason     string
		version    string
		targetID   string
		expectCode int
	}{
		{
			name:       "motivo vazio",
			action:     "reject",
			reason:     "   ",
			version:    esUpdatedAt,
			targetID:   "es-single-1",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "motivo com html",
			action:     "reject",
			reason:     "Rejeição com <script>alert(1)</script>",
			version:    esUpdatedAt,
			targetID:   "es-single-1",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "ação inválida",
			action:     "acao_desconhecida",
			reason:     "Justificativa válida",
			version:    esUpdatedAt,
			targetID:   "es-single-1",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "transição inválida (restore em uso ativo)",
			action:     "restore",
			reason:     "Justificativa válida",
			version:    esUpdatedAt,
			targetID:   "es-single-1",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "evidence_source inexistente",
			action:     "reject",
			reason:     "Justificativa válida",
			version:    esUpdatedAt,
			targetID:   "es-inexistente",
			expectCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{
				"action":              {tt.action},
				"reason":              {tt.reason},
				"expected_updated_at": {tt.version},
			}.Encode()

			req := httptest.NewRequest(http.MethodPost, "/admin/evidencias/"+tt.targetID+"/moderate", strings.NewReader(formData))
			req.Header.Set("Authorization", authHeader)
			req.Header.Set("Origin", allowedOrigin)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectCode {
				t.Errorf("POST /admin/evidencias/%s/moderate (%s): esperado %d, obtido %d (corpo: %s)", tt.targetID, tt.name, tt.expectCode, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdminModerateEvidenceSource_GlobalSourceIntact(t *testing.T) {
	srv, db, allowedOrigin, authHeader := setupEvidenceModerationTestServer(t)
	esUpdatedAt := getEvidenceSourceUpdatedAt(t, db, "es-single-1")

	// Capturar dados da source src-test-1 antes
	var titleBefore, publisherBefore, accessStatusBefore, urlBefore string
	err := db.QueryRow("SELECT title, publisher_or_author, source_access_status, original_url FROM sources WHERE id = 'src-test-1'").
		Scan(&titleBefore, &publisherBefore, &accessStatusBefore, &urlBefore)
	if err != nil {
		t.Fatalf("falha ao consultar source antes: %v", err)
	}

	formData := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeitando uso específico da evidência."},
		"expected_updated_at": {esUpdatedAt},
	}.Encode()

	reqMod := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-single-1/moderate", strings.NewReader(formData))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", allowedOrigin)
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recMod, reqMod)

	if recMod.Code != http.StatusSeeOther {
		t.Fatalf("moderação falhou: %d", recMod.Code)
	}

	// Capturar dados da source src-test-1 depois
	var titleAfter, publisherAfter, accessStatusAfter, urlAfter string
	err = db.QueryRow("SELECT title, publisher_or_author, source_access_status, original_url FROM sources WHERE id = 'src-test-1'").
		Scan(&titleAfter, &publisherAfter, &accessStatusAfter, &urlAfter)
	if err != nil {
		t.Fatalf("falha ao consultar source depois: %v", err)
	}

	if titleBefore != titleAfter || publisherBefore != publisherAfter || accessStatusBefore != accessStatusAfter || urlBefore != urlAfter {
		t.Errorf("a source documental global NUNCA deve ser alterada durante moderação de evidence_source")
	}
}

func TestAdminModerateEvidenceSource_NoSecretLeakage(t *testing.T) {
	srv, _, _, authHeader := setupEvidenceModerationTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/evidencias/es-single-1", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "raw_response") {
		t.Errorf("HTML da página não deve vazar raw_response")
	}
	if strings.Contains(body, "$2a$") || strings.Contains(body, "$2b$") {
		t.Errorf("HTML da página não deve vazar hash de senhas")
	}
}
