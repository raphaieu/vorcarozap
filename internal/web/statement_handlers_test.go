package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/contradiction"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupStatementWebTest(t *testing.T) (*http.Server, *sql.DB, string, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_statement_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de dados: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Inserir entidade
	_, err = db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-1', 'person', 'Daniel Vorcaro', 'daniel vorcaro', 'daniel-vorcaro', 'Empresário', 'Resumo', 5, 'Relevância', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir entidade: %v", err)
	}

	// Inserir caso
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description, created_at, updated_at)
		VALUES ('case-1', 'Caso Master', 'caso-master', 'Descrição', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir caso: %v", err)
	}

	// Inserir relação
	_, err = db.ExecContext(ctx, `
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, created_at, updated_at)
		VALUES ('rel-1', 'ent-1', 'case-1', 'Investigado', 'Resumo', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir relação: %v", err)
	}

	// Inserir claim publicado
	claimID := "claim-pub-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES (?, 'rel-1', 'Alegação pública sobre contrato', 'MPF', 'curated_seed', 'A', 'supports_link', 1, 'published', ?, ?)
	`, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim: %v", err)
	}

	// Inserir source e evidence
	_, err = db.ExecContext(ctx, `
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES ('src-1', 'Notícia MPF', 'Veículo', 'https://exemplo.com/noticia', 'https://exemplo.com/noticia', 'article', 'reachable', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir source: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence (id, claim_id, summary, created_at, updated_at)
		VALUES ('evi-1', ?, 'Resumo', ?, ?)
	`, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidência: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, role, status, created_at, updated_at)
		VALUES ('es-1', 'evi-1', 'src-1', 'Trecho documental', 'supports', 'active', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidence_source: %v", err)
	}

	passwordHash := getTestAdminHashCost12(t)

	cfg := &config.Config{
		Port:                  8080,
		Env:                   "testing",
		DBPath:                dbPath,
		PublicDataCutoff:      "2026-09-03",
		AdminUser:             "admin",
		AdminPasswordHash:     passwordHash,
		AdminAllowedOrigin:    "http://localhost:8080",
		AdminMFAEncryptionKey: "12345678901234567890123456789012",
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor HTTP: %v", err)
	}

	return srv, db, claimID, "daniel-vorcaro"
}

func TestManifestationForm_GET(t *testing.T) {
	srv, _, claimID, _ := setupStatementWebTest(t)

	req := httptest.NewRequest(http.MethodGet, "/manifestar?claim_id="+claimID, nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /manifestar retornou %d, esperado %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Apresentar Manifestação, Defesa ou Retificação") {
		t.Errorf("HTML não contém o título do formulário")
	}
	if !strings.Contains(body, "Alegação pública sobre contrato") {
		t.Errorf("HTML não contém a proposição do claim de contexto")
	}
}

func TestManifestationSubmit_SuccessQuarantine(t *testing.T) {
	srv, db, claimID, _ := setupStatementWebTest(t)

	form := url.Values{}
	form.Set("claim_id", claimID)
	form.Set("statement_type", "rebuttal")
	form.Set("title", "Contestação Oficial da Defesa")
	form.Set("content", "Texto explicativo detalhado informando que a operação foi regular.")
	form.Set("source_url", "https://defesa.com/parecer.pdf")
	form.Set("contact_info", "advogado_privado@defesa.com")

	req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /manifestar retornou %d, esperado 303 See Other", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "success=1") {
		t.Errorf("redirecionamento deve conter success=1, obtido: %q", loc)
	}

	// Verificar se foi persistida em quarentena no SQLite
	q := sqlc.New(db)
	stmts, err := q.ListAdminDefenseStatements(context.Background(), sqlc.ListAdminDefenseStatementsParams{
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("falha ao consultar statements no admin: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("esperado 1 statement no banco, obtido %d", len(stmts))
	}
	if stmts[0].Status != "quarantined" {
		t.Errorf("status deve ser quarantined, obtido %q", stmts[0].Status)
	}
	if stmts[0].ContactInfo != "advogado_privado@defesa.com" {
		t.Errorf("contact_info incorreto: %q", stmts[0].ContactInfo)
	}
}

func TestManifestationSubmit_RateLimit(t *testing.T) {
	srv, _, claimID, _ := setupStatementWebTest(t)

	// Criar handlers com rate limiter restrito para teste
	for i := 0; i < 5; i++ {
		form := url.Values{}
		form.Set("claim_id", claimID)
		form.Set("statement_type", "correction")
		form.Set("title", "Título Válido")
		form.Set("content", "Conteúdo com tamanho suficiente para submissão.")

		req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.168.1.50:12345"
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusSeeOther {
			t.Fatalf("requisição %d deveria ter sucesso, obtido %d", i+1, w.Code)
		}
	}

	// 6ª requisição deve retornar 429 Too Many Requests
	form := url.Values{}
	form.Set("claim_id", claimID)
	form.Set("statement_type", "correction")
	form.Set("title", "Título Válido")
	form.Set("content", "Conteúdo com tamanho suficiente para submissão.")

	req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.50:12345"
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("6ª requisição deveria retornar 429 Too Many Requests, obtido %d", w.Code)
	}
}

func TestManifestationSubmit_HTMLInjectionRejected(t *testing.T) {
	srv, _, claimID, _ := setupStatementWebTest(t)

	form := url.Values{}
	form.Set("claim_id", claimID)
	form.Set("statement_type", "rebuttal")
	form.Set("title", "<script>alert('xss')</script>")
	form.Set("content", "Conteúdo com texto e <script>malicioso</script>")

	req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("submissão com script injection deveria retornar 400 Bad Request, obtido %d", w.Code)
	}
}

func TestPublicEntityDetail_PrivacyAndAcceptedDisplay(t *testing.T) {
	srv, db, claimID, entitySlug := setupStatementWebTest(t)
	svc := contradiction.NewService(db)

	// Submeter manifestação
	subRes, err := svc.SubmitStatement(context.Background(), contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: domain.StatementTypeRebuttal,
		Title:         "Manifestação Pública Aceita",
		Content:       "Texto da defesa que foi formalmente aprovado pela equipe.",
		SourceURL:     "https://defesa.com/comunicado.pdf",
		ContactInfo:   "dado_estritamente_confidencial@advocacia.com",
	})
	if err != nil {
		t.Fatalf("falha ao submeter statement: %v", err)
	}

	// 1. Enquanto estiver em quarentena: NÃO deve aparecer na página pública
	req := httptest.NewRequest(http.MethodGet, "/pessoas/"+entitySlug, nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /pessoas/%s retornou %d", entitySlug, w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "Manifestação Pública Aceita") {
		t.Fatalf("manifestação em quarentena NÃO pode aparecer no HTML público")
	}
	if strings.Contains(body, "dado_estritamente_confidencial@advocacia.com") {
		t.Fatalf("dado de contato NÃO pode vazar no HTML público")
	}

	// 2. Moderar e aceitar a manifestação
	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementForModeration(context.Background(), subRes.ID)
	if err != nil {
		t.Fatalf("falha ao buscar statement: %v", err)
	}

	_, err = svc.ModerateStatement(context.Background(), contradiction.ModerateStatementParams{
		StatementID:       subRes.ID,
		ExpectedUpdatedAt: stmt.UpdatedAt,
		Action:            domain.StatementActionAccept,
		Reason:            "Defesa fundamentada aceita.",
		Actor:             "admin_tester",
	})
	if err != nil {
		t.Fatalf("moderação falhou: %v", err)
	}

	// 3. Após aceitação: deve aparecer o conteúdo da manifestação, mas NUNCA o dado de contato nem o nome do moderador
	w2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w2, req)

	if w2.Code != http.StatusOK {
		t.Fatalf("GET /pessoas/%s pós-aceite retornou %d", entitySlug, w2.Code)
	}
	body2 := w2.Body.String()
	if !strings.Contains(body2, "Manifestações Formais da Defesa / Contraditório") {
		t.Errorf("seção de manifestações deve aparecer após aceite")
	}
	if !strings.Contains(body2, "Manifestação Pública Aceita") {
		t.Errorf("título da manifestação deve aparecer no HTML público")
	}
	if !strings.Contains(body2, "Texto da defesa que foi formalmente aprovado pela equipe.") {
		t.Errorf("texto da manifestação deve aparecer no HTML público")
	}

	// Verificação estrita de ausência de vazamento de dados privados
	if strings.Contains(body2, "dado_estritamente_confidencial@advocacia.com") {
		t.Fatalf("VAZAMENTO: dado confidencial de contato vazou no HTML público!")
	}
	if strings.Contains(body2, "admin_tester") {
		t.Fatalf("VAZAMENTO: identificador do operador vazou no HTML público!")
	}
}

func TestAdminDefenseStatements_AuthAndModeration(t *testing.T) {
	srv, db, claimID, _ := setupStatementWebTest(t)
	svc := contradiction.NewService(db)

	subRes, err := svc.SubmitStatement(context.Background(), contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: domain.StatementTypeCorrection,
		Title:         "Retificação para Moderação Admin",
		Content:       "Texto detalhado com esclarecimentos para moderação.",
		ContactInfo:   "contato_admin@defesa.com",
	})
	if err != nil {
		t.Fatalf("falha ao submeter: %v", err)
	}

	// 1. Acesso sem autenticação no /admin/manifestacoes deve retornar 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "/admin/manifestacoes", nil)
	wUnauth := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 Unauthorized sem auth, obtido %d", wUnauth.Code)
	}

	// 1b. Tentativa com Basic Auth deve retornar 401 (Basic Auth não contorna MFA/sessão)
	reqBasic := httptest.NewRequest(http.MethodGet, "/admin/manifestacoes", nil)
	reqBasic.SetBasicAuth("admin", "password")
	wBasic := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wBasic, reqBasic)
	if wBasic.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 para Basic Auth em rota protegida, obtido %d", wBasic.Code)
	}

	// Cria sessão admin válida com MFA verificado
	uAdmin, err := store.GetUserByUsername(context.Background(), db, "admin")
	if err != nil {
		t.Fatalf("falha ao buscar admin: %v", err)
	}
	sessionToken, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("falha ao gerar token de sessão: %v", err)
	}
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.CreateAdminSession(context.Background(), db, domain.AdminSession{
		ID:             sessionToken,
		UserID:         uAdmin.User.ID,
		MFAVerified:    true,
		IPAddress:      "127.0.0.1",
		UserAgent:      "TestAgent",
		ExpiresAt:      time.Now().UTC().Add(8 * time.Hour).Format(time.RFC3339Nano),
		LastActivityAt: nowStr,
		CreatedAt:      nowStr,
	}); err != nil {
		t.Fatalf("falha ao criar sessão: %v", err)
	}
	sessionCookie := &http.Cookie{
		Name:  web.SessionCookieName,
		Value: sessionToken,
		Path:  "/admin",
	}

	// 2. Acesso autenticado ao detalhe da manifestação
	reqAuth := httptest.NewRequest(http.MethodGet, "/admin/manifestacoes/"+subRes.ID, nil)
	reqAuth.AddCookie(sessionCookie)
	wAuth := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAuth, reqAuth)

	if wAuth.Code != http.StatusOK {
		t.Fatalf("GET /admin/manifestacoes/%s retornou %d", subRes.ID, wAuth.Code)
	}
	detailBody := wAuth.Body.String()
	if !strings.Contains(detailBody, "contato_admin@defesa.com") {
		t.Errorf("contato privado DEVE estar visível no painel administrativo para operadores")
	}

	// 3. Moderação com CSRF válido e OCC
	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementForModeration(context.Background(), subRes.ID)
	if err != nil {
		t.Fatalf("falha ao buscar statement: %v", err)
	}

	modForm := url.Values{}
	modForm.Set("action", "accept")
	modForm.Set("reason", "Manifestação conferida e aprovada pela equipe editorial.")
	modForm.Set("expected_updated_at", stmt.UpdatedAt)

	reqMod := httptest.NewRequest(http.MethodPost, "/admin/manifestacoes/"+subRes.ID+"/moderate", strings.NewReader(modForm.Encode()))
	reqMod.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqMod.Header.Set("Origin", "http://localhost:8080")
	reqMod.AddCookie(sessionCookie)
	wMod := httptest.NewRecorder()

	srv.Handler.ServeHTTP(wMod, reqMod)

	if wMod.Code != http.StatusSeeOther {
		t.Fatalf("POST moderação retornou %d, esperado 303 See Other", wMod.Code)
	}

	// 4. Moderação concorrente com versão desatualizada (OCC) deve retornar 409 Conflict
	wModConflict := httptest.NewRecorder()
	reqModConflict := httptest.NewRequest(http.MethodPost, "/admin/manifestacoes/"+subRes.ID+"/moderate", strings.NewReader(modForm.Encode()))
	reqModConflict.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqModConflict.Header.Set("Origin", "http://localhost:8080")
	reqModConflict.AddCookie(sessionCookie)
	srv.Handler.ServeHTTP(wModConflict, reqModConflict)

	if wModConflict.Code != http.StatusConflict {
		t.Fatalf("moderação concorrente deve retornar 409 Conflict, obtido %d", wModConflict.Code)
	}
}

func TestManifestationSubmit_RateLimit_SpoofedHeadersIgnored(t *testing.T) {
	srv, _, claimID, _ := setupStatementWebTest(t)

	// Simular atacante enviando 5 requisições do mesmo IP real, mas alterando X-Forwarded-For e X-Real-IP
	for i := 0; i < 5; i++ {
		form := url.Values{}
		form.Set("claim_id", claimID)
		form.Set("statement_type", "correction")
		form.Set("title", "Título Válido Anti-Spoof")
		form.Set("content", "Conteúdo com tamanho suficiente para passar nas validações básicas.")

		req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('1'+i)))
		req.Header.Set("X-Real-IP", "198.51.100."+string(rune('1'+i)))
		req.RemoteAddr = "192.168.1.99:54321"
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)

		if w.Code != http.StatusSeeOther {
			t.Fatalf("requisição %d deveria ter sido aceita, obtido %d", i+1, w.Code)
		}
	}

	// 6ª requisição com IP spoofed diferente ainda deve ser bloqueada pelo RemoteAddr
	form := url.Values{}
	form.Set("claim_id", claimID)
	form.Set("statement_type", "correction")
	form.Set("title", "Título Válido Anti-Spoof")
	form.Set("content", "Conteúdo com tamanho suficiente para passar nas validações básicas.")

	req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	req.Header.Set("X-Real-IP", "198.51.100.99")
	req.RemoteAddr = "192.168.1.99:54321"
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("6ª requisição com headers forjados deveria receber 429 Too Many Requests, obtido %d", w.Code)
	}
}

func TestManifestation_NonPublicOrInexistentClaim_NeutralBoundary(t *testing.T) {
	srv, db, _, _ := setupStatementWebTest(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)

	// Inserir entidade privada e claim em quarentena (não público)
	_, err := db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-priv', 'person', 'Alvo Secreto Privado', 'alvo secreto privado', 'alvo-secreto-privado', 'Contexto', 'Resumo', 3, 'Razao', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir entidade privada: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, created_at, updated_at)
		VALUES ('rel-priv', 'ent-priv', 'case-1', 'Investigado', 'Resumo', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir relação privada: %v", err)
	}

	quarantinedClaimID := "claim-quarantine-secret"
	secretProposition := "Proposição ultrassecreta em quarentena investigativa"
	_, err = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES (?, 'rel-priv', ?, 'Polícia Federal', 'openrouter', 'D', 'supports_link', 0, 'quarantined', ?, ?)
	`, quarantinedClaimID, secretProposition, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim em quarentena: %v", err)
	}

	// 1. GET /manifestar com claim em quarentena -> não pode vazar dados do claim
	reqGetQuarantine := httptest.NewRequest(http.MethodGet, "/manifestar?claim_id="+quarantinedClaimID, nil)
	wGetQuarantine := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wGetQuarantine, reqGetQuarantine)

	if wGetQuarantine.Code != http.StatusOK {
		t.Fatalf("GET /manifestar com claim não-público retornou %d, esperado %d", wGetQuarantine.Code, http.StatusOK)
	}
	bodyGetQuarantine := wGetQuarantine.Body.String()
	if strings.Contains(bodyGetQuarantine, secretProposition) {
		t.Fatalf("VAZAMENTO: GET /manifestar vazou a proposição do claim em quarentena!")
	}
	if strings.Contains(bodyGetQuarantine, "Alvo Secreto Privado") {
		t.Fatalf("VAZAMENTO: GET /manifestar vazou a entidade do claim em quarentena!")
	}

	// 2. POST /manifestar com claim em quarentena -> deve ser rejeitado com 400 Bad Request de forma neutra
	formQuarantine := url.Values{}
	formQuarantine.Set("claim_id", quarantinedClaimID)
	formQuarantine.Set("statement_type", "rebuttal")
	formQuarantine.Set("title", "Tentativa de manifestar em claim privado")
	formQuarantine.Set("content", "Conteúdo com tamanho suficiente para submissão.")

	reqPostQuarantine := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(formQuarantine.Encode()))
	reqPostQuarantine.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wPostQuarantine := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPostQuarantine, reqPostQuarantine)

	if wPostQuarantine.Code != http.StatusBadRequest {
		t.Fatalf("POST /manifestar para claim em quarentena retornou %d, esperado 400 Bad Request", wPostQuarantine.Code)
	}
	bodyPostQuarantine := wPostQuarantine.Body.String()
	if strings.Contains(bodyPostQuarantine, secretProposition) {
		t.Fatalf("VAZAMENTO: POST /manifestar vazou a proposição do claim em quarentena!")
	}

	// 3. POST /manifestar com claim inexistente -> 400 Bad Request neutro
	formInexistent := url.Values{}
	formInexistent.Set("claim_id", "claim-inexistente-12345")
	formInexistent.Set("statement_type", "rebuttal")
	formInexistent.Set("title", "Tentativa em claim inexistente")
	formInexistent.Set("content", "Conteúdo com tamanho suficiente para submissão.")

	reqPostInexistent := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(formInexistent.Encode()))
	reqPostInexistent.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wPostInexistent := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wPostInexistent, reqPostInexistent)

	if wPostInexistent.Code != http.StatusBadRequest {
		t.Fatalf("POST /manifestar para claim inexistente retornou %d, esperado 400 Bad Request", wPostInexistent.Code)
	}
}

func TestManifestationSubmit_InvalidURLRejected(t *testing.T) {
	srv, _, claimID, _ := setupStatementWebTest(t)

	invalidURLs := []string{
		"http://user:pass@exemplo.com/parecer.pdf",
		"javascript:alert(1)",
		"ftp://exemplo.com/documento.pdf",
		"http://",
		"https://exemplo.com/documento\x00.pdf",
		"http://-invalid-host/doc.pdf",
	}

	for idx, badURL := range invalidURLs {
		t.Run("URL_"+badURL, func(t *testing.T) {
			form := url.Values{}
			form.Set("claim_id", claimID)
			form.Set("statement_type", "rebuttal")
			form.Set("title", "Manifestação com URL Inválida")
			form.Set("content", "Texto explicativo válido com mais de vinte caracteres.")
			form.Set("source_url", badURL)

			req := httptest.NewRequest(http.MethodPost, "/manifestar", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", idx+10)
			w := httptest.NewRecorder()

			srv.Handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("submissão com URL inválida %q retornou %d, esperado 400", badURL, w.Code)
			}
		})
	}
}
