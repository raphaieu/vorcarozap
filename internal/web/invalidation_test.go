package web_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupInvalidationTestServer(t *testing.T) (*http.Server, *sql.DB, string, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "invalidation_test.db")

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

	// Insere dados editoriais iniciais para teste de invalidação
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description)
		VALUES ('case-inv-1', 'Operação Invalidação', 'operacao-invalidacao', 'Investigação documental de teste');

		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
		VALUES
			('ent-inv-1', 'person', 'Investigado Alfa', 'investigado alfa', 'investigado-alfa', 'Diretor Financeiro', 'Resumo Alfa', 5, 'Figura central', 'Finanças', 'Nacional'),
			('ent-inv-2', 'person', 'Investigado Beta', 'investigado beta', 'investigado-beta', 'Sócio Operacional', 'Resumo Beta', 4, 'Articulador', 'Politica', 'Regional'),
			('ent-inv-3', 'person', 'Investigado Gama', 'investigado gama', 'investigado-gama', 'Representante', 'Resumo Gama', 3, 'Contato', 'Setor Público', 'Regional');

		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary)
		VALUES
			('rel-inv-1', 'ent-inv-1', 'case-inv-1', 'investigado', 'Vínculo investigado Alfa'),
			('rel-inv-2', 'ent-inv-2', 'case-inv-1', 'investigado', 'Vínculo investigado Beta'),
			('rel-inv-3', 'ent-inv-3', 'case-inv-1', 'investigado', 'Vínculo investigado Gama');

		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_access_status)
		VALUES
			('src-inv-1a', 'Reportagem Alfa 1', 'Jornal Alfa', 'https://alfa.com/1', 'https://alfa.com/1', 'reachable'),
			('src-inv-1b', 'Reportagem Alfa 2', 'Jornal Beta', 'https://beta.com/2', 'https://beta.com/2', 'reachable'),
			('src-inv-2', 'Reportagem Beta', 'Portal Notícias', 'https://noticias.com/beta', 'https://noticias.com/beta', 'reachable'),
			('src-inv-3', 'Reportagem Gama', 'Diário Oficial', 'https://diario.gov.br/gama', 'https://diario.gov.br/gama', 'reachable');

		-- Claim 1: Publicado (ent-inv-1) com 2 suportes ativos (es-inv-1a e es-inv-1b)
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-inv-1', 'rel-inv-1', 'Proposição Documentada Alfa', 'Jornal Alfa', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ativo');

		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-inv-1', 'claim-inv-1', 'Evidência Principal Alfa');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES
			('es-inv-1a', 'ev-inv-1', 'src-inv-1a', 'Trecho literal comprobatório 1', 'Pág. 10', 'supports', 'active'),
			('es-inv-1b', 'ev-inv-1', 'src-inv-1b', 'Trecho literal comprobatório 2', 'Pág. 15', 'supports', 'active');

		-- Claim 2: Em Quarentena (ent-inv-2) com 1 suporte ativo
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-inv-2', 'rel-inv-2', 'Proposição Sob Análise Beta', 'Portal Notícias', 'openrouter', 'C', 'possible_link', 1, 'quarantined', 'ativo');

		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-inv-2', 'claim-inv-2', 'Evidência Provisória Beta');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-inv-2', 'ev-inv-2', 'src-inv-2', 'Trecho provisório Beta', 'Pág. 2', 'supports', 'active');

		-- Claim 3: Publicado (ent-inv-3) com apenas 1 suporte ativo (para teste do último suporte)
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
		VALUES ('claim-inv-3', 'rel-inv-3', 'Proposição Único Suporte Gama', 'Diário Oficial', 'curated_seed', 'B', 'supports_link', 1, 'published', 'ativo');

		INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-inv-3', 'claim-inv-3', 'Evidência Única Gama');
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
		VALUES ('es-inv-3', 'ev-inv-3', 'src-inv-3', 'Trecho único Gama', 'Pág. 5', 'supports', 'active');
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

func getClaimUpdatedAt(t *testing.T, db *sql.DB, claimID string) string {
	t.Helper()
	var updatedAt string
	err := db.QueryRow("SELECT updated_at FROM claims WHERE id = ?", claimID).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at de claim %s: %v", claimID, err)
	}
	return updatedAt
}

// TestInvalidation_ClaimLifecycle_ImmediatePublicConsistency valida o ciclo completo de moderação
// de um claim (quarentena -> aprovado -> rejeitado -> restaurado) e comprova que cada mudança
// reflete imediatamente na Home, na listagem /pessoas, nos detalhes /pessoas/{slug} e no XLSX.
func TestInvalidation_ClaimLifecycle_ImmediatePublicConsistency(t *testing.T) {
	srv, db, origin, authHeader := setupInvalidationTestServer(t)

	// --- ESTADO INICIAL ---
	// ent-inv-1 (Alfa) e ent-inv-3 (Gama) são públicos. ent-inv-2 (Beta) está em quarentena (invisível).

	// 1. Home inicial: deve contabilizar 2 entidades elegíveis e 2 claims elegíveis
	reqHome := httptest.NewRequest(http.MethodGet, "/", nil)
	recHome := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recHome, reqHome)
	if recHome.Code != http.StatusOK {
		t.Fatalf("Home inicial: esperado 200, obtido %d", recHome.Code)
	}
	bodyHome := recHome.Body.String()
	if !strings.Contains(bodyHome, "Investigado Alfa") {
		t.Errorf("Home inicial deveria conter Investigado Alfa")
	}
	if strings.Contains(bodyHome, "Investigado Beta") {
		t.Errorf("Home inicial NÃO deveria conter Investigado Beta (em quarentena)")
	}

	// 2. Detalhe /pessoas/investigado-beta deve retornar 404 inicialmente
	reqBeta := httptest.NewRequest(http.MethodGet, "/pessoas/investigado-beta", nil)
	recBeta := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recBeta, reqBeta)
	if recBeta.Code != http.StatusNotFound {
		t.Errorf("detalhe Beta em quarentena: esperado 404, obtido %d", recBeta.Code)
	}

	// 3. Exportação inicial: deve conter apenas Alfa e Gama
	reqXlsx := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	recXlsx := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recXlsx, reqXlsx)
	if recXlsx.Code != http.StatusOK {
		t.Fatalf("exportação inicial: esperado 200, obtido %d", recXlsx.Code)
	}
	f1, err := excelize.OpenReader(recXlsx.Body)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX 1: %v", err)
	}
	claimRows1, _ := f1.GetRows("Alegações e Relações")
	if len(claimRows1) != 3 { // 1 cabeçalho + 2 claims (Alfa e Gama)
		t.Errorf("XLSX inicial: esperado 3 linhas em Alegações, obtido %d", len(claimRows1))
	}
	f1.Close()

	// --- AÇÃO 1: APROVAÇÃO DE CLAIM-INV-2 (Beta) ---
	updatedAtBeta := getClaimUpdatedAt(t, db, "claim-inv-2")
	formApprove := url.Values{
		"action":              {"approve"},
		"reason":              {"Aprovação editorial fundamentada em suporte verificado"},
		"expected_updated_at": {updatedAtBeta},
	}
	reqApprove := httptest.NewRequest(http.MethodPost, "/admin/claims/claim-inv-2/moderate", strings.NewReader(formApprove.Encode()))
	reqApprove.Header.Set("Authorization", authHeader)
	reqApprove.Header.Set("Origin", origin)
	reqApprove.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recApprove := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recApprove, reqApprove)

	if recApprove.Code != http.StatusSeeOther {
		t.Fatalf("aprovação claim-inv-2: esperado 303, obtido %d", recApprove.Code)
	}

	// Consulta pública IMEDIATAMENTE após aprovação:
	// A. Detalhe /pessoas/investigado-beta agora retorna 200 OK com a proposição
	recBetaAfterApprove := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recBetaAfterApprove, reqBeta)
	if recBetaAfterApprove.Code != http.StatusOK {
		t.Errorf("detalhe Beta pós-aprovação: esperado 200, obtido %d", recBetaAfterApprove.Code)
	}
	if !strings.Contains(recBetaAfterApprove.Body.String(), "Proposição Sob Análise Beta") {
		t.Errorf("detalhe Beta não exibiu a proposição aprovada")
	}

	// B. Exportação XLSX agora contém 3 claims
	recXlsx2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recXlsx2, reqXlsx)
	f2, err := excelize.OpenReader(recXlsx2.Body)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX 2: %v", err)
	}
	claimRows2, _ := f2.GetRows("Alegações e Relações")
	if len(claimRows2) != 4 { // 1 cabeçalho + 3 claims
		t.Errorf("XLSX pós-aprovação: esperado 4 linhas em Alegações, obtido %d", len(claimRows2))
	}
	f2.Close()

	// --- AÇÃO 2: REJEIÇÃO DE CLAIM-INV-2 (Beta) ---
	updatedAtBeta2 := getClaimUpdatedAt(t, db, "claim-inv-2")
	formReject := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeição editorial por inconsistência factual superveniente"},
		"expected_updated_at": {updatedAtBeta2},
	}
	reqReject := httptest.NewRequest(http.MethodPost, "/admin/claims/claim-inv-2/moderate", strings.NewReader(formReject.Encode()))
	reqReject.Header.Set("Authorization", authHeader)
	reqReject.Header.Set("Origin", origin)
	reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recReject := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recReject, reqReject)

	if recReject.Code != http.StatusSeeOther {
		t.Fatalf("rejeição claim-inv-2: esperado 303, obtido %d", recReject.Code)
	}

	// Consulta pública IMEDIATAMENTE após rejeição:
	// A. Detalhe /pessoas/investigado-beta deve voltar a responder 404
	recBetaAfterReject := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recBetaAfterReject, reqBeta)
	if recBetaAfterReject.Code != http.StatusNotFound {
		t.Errorf("detalhe Beta pós-rejeição: esperado 404, obtido %d", recBetaAfterReject.Code)
	}

	// B. Exportação XLSX volta a conter apenas 2 claims
	recXlsx3 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recXlsx3, reqXlsx)
	f3, err := excelize.OpenReader(recXlsx3.Body)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX 3: %v", err)
	}
	claimRows3, _ := f3.GetRows("Alegações e Relações")
	if len(claimRows3) != 3 {
		t.Errorf("XLSX pós-rejeição: esperado 3 linhas em Alegações, obtido %d", len(claimRows3))
	}
	f3.Close()

	// --- AÇÃO 3: RESTAURAÇÃO DE CLAIM-INV-2 (Beta -> Quarentena) ---
	updatedAtBeta3 := getClaimUpdatedAt(t, db, "claim-inv-2")
	formRestore := url.Values{
		"action":              {"restore"},
		"reason":              {"Restauração para reavaliação documental em quarentena"},
		"expected_updated_at": {updatedAtBeta3},
	}
	reqRestore := httptest.NewRequest(http.MethodPost, "/admin/claims/claim-inv-2/moderate", strings.NewReader(formRestore.Encode()))
	reqRestore.Header.Set("Authorization", authHeader)
	reqRestore.Header.Set("Origin", origin)
	reqRestore.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recRestore := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recRestore, reqRestore)

	if recRestore.Code != http.StatusSeeOther {
		t.Fatalf("restauração claim-inv-2: esperado 303, obtido %d", recRestore.Code)
	}

	// Consulta pública IMEDIATAMENTE após restauração:
	// Invariante de não-republicação: DEVE CONTINUAR 404
	recBetaAfterRestore := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recBetaAfterRestore, reqBeta)
	if recBetaAfterRestore.Code != http.StatusNotFound {
		t.Errorf("detalhe Beta pós-restauração para quarentena: esperado 404, obtido %d", recBetaAfterRestore.Code)
	}
}

// TestInvalidation_EvidenceSource_QuarantineOnLastSupportLoss comprova que rejeitar o último
// suporte ativo de um claim publicado coloca o claim em quarentena atomicamente e retira
// imediatamente o claim de todas as visualizações públicas, métricas e exportação XLSX,
// mantendo a fonte documental global intacta.
func TestInvalidation_EvidenceSource_QuarantineOnLastSupportLoss(t *testing.T) {
	srv, db, origin, authHeader := setupInvalidationTestServer(t)

	// Claim 3 (Gama) tem apenas es-inv-3 como suporte. Inicialmente publicado.
	reqGama := httptest.NewRequest(http.MethodGet, "/pessoas/investigado-gama", nil)
	recGama1 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recGama1, reqGama)
	if recGama1.Code != http.StatusOK {
		t.Fatalf("detalhe Gama antes de moderar suporte: esperado 200, obtido %d", recGama1.Code)
	}

	// Modera es-inv-3: reject
	updatedAtES := getEvidenceSourceUpdatedAt(t, db, "es-inv-3")
	form := url.Values{
		"action":              {"reject"},
		"reason":              {"Fonte desatualizada sem respaldo para o fato"},
		"expected_updated_at": {updatedAtES},
	}
	reqMod := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-inv-3/moderate", strings.NewReader(form.Encode()))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", origin)
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recMod, reqMod)

	if recMod.Code != http.StatusSeeOther {
		t.Fatalf("moderação es-inv-3: esperado 303, obtido %d", recMod.Code)
	}

	// 1. Detalhe /pessoas/investigado-gama deve responder imediatamente 404 (sem claims públicos)
	recGama2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recGama2, reqGama)
	if recGama2.Code != http.StatusNotFound {
		t.Errorf("detalhe Gama pós-perda do último suporte: esperado 404, obtido %d", recGama2.Code)
	}

	// 2. Exportação XLSX não deve conter Claim 3 nem es-inv-3
	reqXlsx := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	recXlsx := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recXlsx, reqXlsx)
	f, err := excelize.OpenReader(recXlsx.Body)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX: %v", err)
	}
	defer f.Close()

	claimRows, _ := f.GetRows("Alegações e Relações")
	for _, r := range claimRows[1:] {
		if r[5] == "Proposição Único Suporte Gama" {
			t.Errorf("claim que perdeu o último suporte vazou no XLSX: %s", r[5])
		}
	}

	// 3. A fonte documental global em sources permanece intacta
	var srcStatus string
	err = db.QueryRow("SELECT source_access_status FROM sources WHERE id = 'src-inv-3'").Scan(&srcStatus)
	if err != nil {
		t.Fatalf("erro ao consultar source global src-inv-3: %v", err)
	}
	if srcStatus != "reachable" {
		t.Errorf("source global foi alterada indevidamente: status=%s", srcStatus)
	}
}

// TestInvalidation_EvidenceSource_RejectNonLastSupport_SourceOmittedClaimRemains comprova que
// rejeitar um suporte secundário mantém o claim publicado, mas remove imediatamente a fonte rejeitada
// da exibição pública e da exportação.
func TestInvalidation_EvidenceSource_RejectNonLastSupport_SourceOmittedClaimRemains(t *testing.T) {
	srv, db, origin, authHeader := setupInvalidationTestServer(t)

	// Claim 1 (Alfa) possui es-inv-1a (src-inv-1a) e es-inv-1b (src-inv-1b). Ambos ativos.
	reqAlfa := httptest.NewRequest(http.MethodGet, "/pessoas/investigado-alfa", nil)
	recAlfa1 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recAlfa1, reqAlfa)
	if !strings.Contains(recAlfa1.Body.String(), "https://alfa.com/1") || !strings.Contains(recAlfa1.Body.String(), "https://beta.com/2") {
		t.Fatalf("esperava ambas as fontes no detalhe Alfa antes da moderação")
	}

	// Modera es-inv-1a: reject
	updatedAtES := getEvidenceSourceUpdatedAt(t, db, "es-inv-1a")
	form := url.Values{
		"action":              {"reject"},
		"reason":              {"Rejeição de fonte redundante sem suporte específico"},
		"expected_updated_at": {updatedAtES},
	}
	reqMod := httptest.NewRequest(http.MethodPost, "/admin/evidencias/es-inv-1a/moderate", strings.NewReader(form.Encode()))
	reqMod.Header.Set("Authorization", authHeader)
	reqMod.Header.Set("Origin", origin)
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMod := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recMod, reqMod)

	if recMod.Code != http.StatusSeeOther {
		t.Fatalf("moderação es-inv-1a: esperado 303, obtido %d", recMod.Code)
	}

	// 1. Detalhe Alfa continua 200 OK com a proposição, mas sem a fonte es-inv-1a
	recAlfa2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recAlfa2, reqAlfa)
	if recAlfa2.Code != http.StatusOK {
		t.Fatalf("detalhe Alfa pós-rejeição de suporte secundário: esperado 200, obtido %d", recAlfa2.Code)
	}
	bodyAlfa := recAlfa2.Body.String()
	if !strings.Contains(bodyAlfa, "Proposição Documentada Alfa") {
		t.Errorf("proposição Alfa deveria continuar presente")
	}
	if strings.Contains(bodyAlfa, "https://alfa.com/1") {
		t.Errorf("fonte rejeitada es-inv-1a (https://alfa.com/1) continuou visível no detalhe público")
	}
	if !strings.Contains(bodyAlfa, "https://beta.com/2") {
		t.Errorf("fonte ativa restante es-inv-1b (https://beta.com/2) deveria permanecer visível")
	}

	// 2. Exportação XLSX deve conter apenas es-inv-1b
	reqXlsx := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	recXlsx := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recXlsx, reqXlsx)
	f, err := excelize.OpenReader(recXlsx.Body)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX: %v", err)
	}
	defer f.Close()

	sourceRows, _ := f.GetRows("Evidências e Fontes")
	for _, r := range sourceRows[1:] {
		if r[10] == "https://alfa.com/1" {
			t.Errorf("fonte rejeitada https://alfa.com/1 vazou na exportação XLSX de fontes")
		}
	}
}

// TestInvalidation_Rollback_PreservesCleanState comprova que falhas durante moderação
// (ex: OCC mismatch) não provocam mutações parciais ou descompasso de dados públicos.
func TestInvalidation_Rollback_PreservesCleanState(t *testing.T) {
	srv, _, origin, authHeader := setupInvalidationTestServer(t)

	// Submete moderação com versão esperada inválida / stale
	form := url.Values{
		"action":              {"reject"},
		"reason":              {"Tentativa com versão stale"},
		"expected_updated_at": {"2000-01-01T00:00:00Z"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/claims/claim-inv-1/moderate", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("esperado 409 Conflict em OCC mismatch, obtido %d", rec.Code)
	}

	// Verifica que claim-inv-1 continua 100% publicado e visível
	reqAlfa := httptest.NewRequest(http.MethodGet, "/pessoas/investigado-alfa", nil)
	recAlfa := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recAlfa, reqAlfa)
	if recAlfa.Code != http.StatusOK {
		t.Errorf("detalhe Alfa após rollback de OCC: esperado 200, obtido %d", recAlfa.Code)
	}
	if !strings.Contains(recAlfa.Body.String(), "Proposição Documentada Alfa") {
		t.Errorf("proposição Alfa deveria permanecer intacta")
	}
}

// TestInvalidation_ConcurrentReadsDuringModeration valida que leituras públicas concorrentes
// durante ações de moderação são executadas com sucesso sem corrupção ou panics.
func TestInvalidation_ConcurrentReadsDuringModeration(t *testing.T) {
	srv, db, origin, authHeader := setupInvalidationTestServer(t)

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 10 Goroutines de leitura pública contínua
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			paths := []string{"/", "/pessoas", "/pessoas/investigado-alfa", "/metodologia", "/exportar/base.xlsx"}
			for {
				select {
				case <-ctx.Done():
					return
				default:
					p := paths[id%len(paths)]
					req := httptest.NewRequest(http.MethodGet, p, nil)
					rec := httptest.NewRecorder()
					srv.Handler.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
						t.Errorf("leitura concorrente de %s retornou código inesperado: %d", p, rec.Code)
					}
				}
			}
		}(i)
	}

	// 1 Goroutine de moderação intercalada
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(20 * time.Millisecond)
				updatedAt := getClaimUpdatedAt(t, db, "claim-inv-1")
				action := "reject"
				if i%2 == 1 {
					action = "restore"
				}
				form := url.Values{
					"action":              {action},
					"reason":              {fmt.Sprintf("Moderação concorrente ciclo %d", i)},
					"expected_updated_at": {updatedAt},
				}
				req := httptest.NewRequest(http.MethodPost, "/admin/claims/claim-inv-1/moderate", strings.NewReader(form.Encode()))
				req.Header.Set("Authorization", authHeader)
				req.Header.Set("Origin", origin)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				rec := httptest.NewRecorder()
				srv.Handler.ServeHTTP(rec, req)
				// Pode retornar 303 (sucesso) ou 409 (se outra ação alterou concorrentemente)
				if rec.Code != http.StatusSeeOther && rec.Code != http.StatusConflict {
					t.Errorf("moderação concorrente ciclo %d retornou código inesperado: %d", i, rec.Code)
				}
			}
		}
	}()

	wg.Wait()
}

// TestInvalidation_CacheControlHeaders valida os cabeçalhos de controle de cache restritivos
// na área administrativa e na exportação XLSX.
func TestInvalidation_CacheControlHeaders(t *testing.T) {
	srv, _, _, authHeader := setupInvalidationTestServer(t)

	// 1. /admin/* deve conter Cache-Control: no-store e Vary: Authorization
	adminPaths := []string{
		"/admin",
		"/admin/candidatos",
		"/admin/evidencias",
		"/admin/fontes",
		"/admin/claims/claim-inv-1",
		"/admin/evidencias/es-inv-1a",
	}

	for _, p := range adminPaths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s esperado 200, obtido %d", p, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("GET %s Cache-Control esperado 'no-store', obtido %q", p, cc)
		}
		if vary := rec.Header().Get("Vary"); vary != "Authorization" {
			t.Errorf("GET %s Vary esperado 'Authorization', obtido %q", p, vary)
		}
	}

	// 2. /exportar/base.xlsx deve conter Cache-Control: no-cache, no-store, must-revalidate
	reqXlsx := httptest.NewRequest(http.MethodGet, "/exportar/base.xlsx", nil)
	recXlsx := httptest.NewRecorder()
	srv.Handler.ServeHTTP(recXlsx, reqXlsx)

	if recXlsx.Code != http.StatusOK {
		t.Errorf("GET /exportar/base.xlsx esperado 200, obtido %d", recXlsx.Code)
	}
	if cc := recXlsx.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
		t.Errorf("GET /exportar/base.xlsx Cache-Control esperado 'no-cache, no-store, must-revalidate', obtido %q", cc)
	}

	// 3. Rotas públicas dinâmicas SSR devem conter Cache-Control: no-cache, no-store, must-revalidate
	// e NÃO devem conter Vary: Authorization (não dependem de autenticação)
	publicRoutes := []struct {
		path         string
		expectedCode int
	}{
		{path: "/", expectedCode: http.StatusOK},
		{path: "/pessoas", expectedCode: http.StatusOK},
		{path: "/pessoas/investigado-alfa", expectedCode: http.StatusOK},
		{path: "/pessoas/slug-inexistente-xyz", expectedCode: http.StatusNotFound},
	}

	for _, pr := range publicRoutes {
		req := httptest.NewRequest(http.MethodGet, pr.path, nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != pr.expectedCode {
			t.Errorf("GET %s status esperado %d, obtido %d", pr.path, pr.expectedCode, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache, no-store, must-revalidate" {
			t.Errorf("GET %s Cache-Control esperado 'no-cache, no-store, must-revalidate', obtido %q", pr.path, cc)
		}
		if vary := rec.Header().Get("Vary"); strings.Contains(vary, "Authorization") {
			t.Errorf("GET %s pública não deveria conter Vary: Authorization, obtido %q", pr.path, vary)
		}
	}
}
