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
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupAuthFlowTestServer(t *testing.T, mfaRequired bool) (*http.Server, *sql.DB, *auth.Service, string) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_auth_flow_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao rodar migrations: %v", err)
	}

	authSvc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, mfaRequired, "12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("falha ao inicializar auth.Service: %v", err)
	}

	cfg := &config.Config{
		Port:                  8080,
		Env:                   "test",
		DBPath:                dbPath,
		PublicDataCutoff:      "2026-09-03",
		AdminAllowedOrigin:    "http://example.com",
		AdminUser:             "admin",
		AdminPasswordHash:     "$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		AdminMaxLoginAttempts: 3,
		AdminLockoutDuration:  15 * time.Minute,
		AdminMFARequired:      mfaRequired,
		AdminMFAEncryptionKey: "12345678901234567890123456789012",
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	return srv, db, authSvc, "http://example.com"
}

func TestAuthFlow_LoginPage_Rendering(t *testing.T) {
	srv, _, _, _ := setupAuthFlowTestServer(t, false)

	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200 em /admin/login, obtido %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Painel Administrativo") {
		t.Errorf("esperado 'Painel Administrativo' no HTML de login")
	}
	if !strings.Contains(body, `name="username"`) || !strings.Contains(body, `name="password"`) {
		t.Errorf("campos username e password ausentes no form de login")
	}
}

func TestAuthFlow_PasswordLogin_InvalidCredentialsAndLockout(t *testing.T) {
	srv, _, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	// Cria usuário com max 3 tentativas
	_, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "operador1",
		DisplayName: "Operador 1",
		Password:    "SenhaCorreta123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// 1. Tentativa com senha errada
	form := url.Values{
		"username": {"operador1"},
		"password": {"SenhaErrada!"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 re-renderizando login com erro, obtido %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Credenciais inválidas") {
		t.Errorf("esperado mensagem de credenciais inválidas, obtido: %s", w.Body.String())
	}

	// 2. Segunda tentativa errada
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Origin", origin)
	srv.Handler.ServeHTTP(w2, req2)

	// 3. Terceira tentativa errada -> deve disparar lockout
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.Header.Set("Origin", origin)
	srv.Handler.ServeHTTP(w3, req3)

	// 4. Quarta tentativa com a senha CORRETA -> deve estar bloqueado pelo lockout
	formCorrect := url.Values{
		"username": {"operador1"},
		"password": {"SenhaCorreta123!"},
	}
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(formCorrect.Encode()))
	req4.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req4.Header.Set("Origin", origin)
	srv.Handler.ServeHTTP(w4, req4)

	if !strings.Contains(w4.Body.String(), "bloqueada") {
		t.Errorf("esperado aviso de conta bloqueada após exceder tentativas, obtido: %s", w4.Body.String())
	}
}

func TestAuthFlow_SuccessfulLogin_NoMFA_SetsSessionCookie(t *testing.T) {
	srv, _, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	_, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "editor-nomfa",
		DisplayName: "Editor Sem MFA",
		Password:    "SenhaValida123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	form := url.Values{
		"username": {"editor-nomfa"},
		"password": {"SenhaValida123!"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("esperado redirect 303 após login bem-sucedido, obtido %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/admin" {
		t.Errorf("esperado redirect para /admin, obtido %q", loc)
	}

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == web.SessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("cookie de sessão %q não foi retornado", web.SessionCookieName)
	}
	if !sessionCookie.HttpOnly {
		t.Errorf("cookie de sessão deve ser HttpOnly")
	}
	if sessionCookie.Path != "/admin" {
		t.Errorf("cookie path esperado '/admin', obtido %q", sessionCookie.Path)
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite esperado Lax, obtido %v", sessionCookie.SameSite)
	}
}

func TestAuthFlow_MFAChallenge_AndVerification(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "usuario-mfa",
		DisplayName: "Usuário com MFA",
		Password:    "SenhaValida123!",
		Role:        domain.RoleAdmin,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// Cria uma sessão temporária para realizar o setup
	rawToken, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("falha ao gerar token: %v", err)
	}
	now := time.Now().UTC()
	setupSession := domain.AdminSession{
		ID:             rawToken,
		UserID:         user.ID,
		MFAVerified:    false,
		IPAddress:      "127.0.0.1",
		UserAgent:      "TestAgent",
		ExpiresAt:      now.Add(8 * time.Hour).Format(time.RFC3339Nano),
		LastActivityAt: now.Format(time.RFC3339Nano),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}
	if err := store.CreateAdminSession(ctx, db, setupSession); err != nil {
		t.Fatalf("falha ao criar sessão de setup: %v", err)
	}

	// Configura e ativa MFA para este usuário
	secret, _, err := authSvc.GenerateMFASetup(ctx, user.ID)
	if err != nil {
		t.Fatalf("falha ao gerar setup de MFA: %v", err)
	}
	initialCode, err := auth.GenerateCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("falha ao gerar código TOTP inicial: %v", err)
	}
	if _, _, err := authSvc.ConfirmMFASetup(ctx, setupSession.ID, initialCode, "127.0.0.1"); err != nil {
		t.Fatalf("falha ao confirmar MFA: %v", err)
	}

	// 1. Faz login com usuário e senha -> deve redirecionar para /admin/mfa/challenge
	form := url.Values{
		"username": {"usuario-mfa"},
		"password": {"SenhaValida123!"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("login com MFA habilitado: esperado redirect 303, obtido %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/mfa/challenge" {
		t.Fatalf("esperado redirect para /admin/mfa/challenge, obtido %q", loc)
	}

	// Extrai cookie da sessão pendente de MFA
	var pendingCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == web.SessionCookieName {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil {
		t.Fatalf("cookie de sessão pendente ausente")
	}

	// 2. Tentativa de acessar rota protegida antes de verificar MFA -> deve redirecionar para challenge
	reqProtected := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqProtected.Header.Set("Accept", "text/html")
	reqProtected.AddCookie(pendingCookie)
	wProt := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wProt, reqProtected)
	if wProt.Code != http.StatusSeeOther || wProt.Header().Get("Location") != "/admin/mfa/challenge" {
		t.Errorf("acesso a /admin com sessão pendente: esperado redirect para /admin/mfa/challenge, obtido code=%d loc=%q", wProt.Code, wProt.Header().Get("Location"))
	}

	// 3. Submissão de código MFA inválido
	formMFAErr := url.Values{"code": {"000000"}}
	reqMFAErr := httptest.NewRequest(http.MethodPost, "/admin/mfa/challenge", strings.NewReader(formMFAErr.Encode()))
	reqMFAErr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqMFAErr.Header.Set("Origin", origin)
	reqMFAErr.AddCookie(pendingCookie)
	wMFAErr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMFAErr, reqMFAErr)

	if wMFAErr.Code != http.StatusUnauthorized {
		t.Fatalf("código MFA inválido: esperado 401 re-renderizando desafio, obtido %d", wMFAErr.Code)
	}
	if !strings.Contains(wMFAErr.Body.String(), "Código de autenticação inválido") {
		t.Errorf("esperado mensagem de código inválido, obtido: %s", wMFAErr.Body.String())
	}

	// 4. Submissão de código MFA correto
	validCode, err := auth.GenerateCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("falha ao gerar código TOTP válido: %v", err)
	}
	formMFAOk := url.Values{"code": {validCode}}
	reqMFAOk := httptest.NewRequest(http.MethodPost, "/admin/mfa/challenge", strings.NewReader(formMFAOk.Encode()))
	reqMFAOk.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqMFAOk.Header.Set("Origin", origin)
	reqMFAOk.AddCookie(pendingCookie)
	wMFAOk := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMFAOk, reqMFAOk)

	if wMFAOk.Code != http.StatusSeeOther {
		t.Fatalf("MFA bem-sucedido: esperado redirect 303, obtido %d", wMFAOk.Code)
	}
	if loc := wMFAOk.Header().Get("Location"); loc != "/admin" {
		t.Errorf("esperado redirect para /admin, obtido %q", loc)
	}

	// Extrai cookie com sessão verificada e rotacionada
	var verifiedCookie *http.Cookie
	for _, c := range wMFAOk.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			verifiedCookie = c
			break
		}
	}
	if verifiedCookie == nil {
		t.Fatalf("novo cookie de sessão verificado ausente")
	}

	// 5. Agora pode acessar /admin com sucesso
	reqFinal := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqFinal.AddCookie(verifiedCookie)
	wFinal := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wFinal, reqFinal)
	if wFinal.Code != http.StatusOK {
		t.Errorf("acesso pós-MFA a /admin: esperado 200, obtido %d", wFinal.Code)
	}
}

func TestAuthFlow_Logout_Revocation(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	cookie := createTestUserAndSession(t, ctx, authSvc, db, "user-logout", domain.RoleEditor, domain.UserStatusActive)

	// POST /admin/logout
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("esperado redirect 303 no logout, obtido %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/login" {
		t.Errorf("esperado redirect para /admin/login, obtido %q", loc)
	}

	// Verifica se cookie foi limpo (Max-Age -1 ou expirado)
	var clearedCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == web.SessionCookieName {
			clearedCookie = c
			break
		}
	}
	if clearedCookie == nil || (clearedCookie.MaxAge > 0 && clearedCookie.Value != "") {
		t.Errorf("cookie de sessão não foi limpo corretamente no logout")
	}

	// Tentativa de reutilizar o cookie antigo em /admin deve falhar
	reqReplay := httptest.NewRequest(http.MethodGet, "/admin/candidatos", nil)
	reqReplay.AddCookie(cookie)
	wReplay := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wReplay, reqReplay)

	if wReplay.Code != http.StatusUnauthorized {
		t.Errorf("sessão revogada deve ser rejeitada com 401, obtido %d", wReplay.Code)
	}
}

func TestAuthFlow_SensitiveData_NoSecretsLeakedInResponses(t *testing.T) {
	srv, db, authSvc, _ := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminCookie := createTestUserAndSession(t, ctx, authSvc, db, "admin-leak-test", domain.RoleAdmin, domain.UserStatusActive)

	// 1. Lista de usuários (/admin/usuarios)
	req := httptest.NewRequest(http.MethodGet, "/admin/usuarios", nil)
	req.AddCookie(adminCookie)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "$2a$") || strings.Contains(body, "$2b$") {
		t.Errorf("Vazamento crítico: hash bcrypt detectado no HTML de /admin/usuarios")
	}

	// 2. Detalhe de usuário (/admin/usuarios/{id})
	uWithCreds, err := store.GetUserByUsername(ctx, db, "admin-leak-test")
	if err != nil {
		t.Fatalf("falha ao buscar usuário: %v", err)
	}
	reqDet := httptest.NewRequest(http.MethodGet, "/admin/usuarios/"+uWithCreds.User.ID, nil)
	reqDet.AddCookie(adminCookie)
	wDet := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wDet, reqDet)

	bodyDet := wDet.Body.String()
	if strings.Contains(bodyDet, "$2a$") || strings.Contains(bodyDet, "$2b$") {
		t.Errorf("Vazamento crítico: hash bcrypt detectado no HTML de /admin/usuarios/{id}")
	}
}

func TestAuthFlow_PasswordChange(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	cookie := createTestUserAndSession(t, ctx, authSvc, db, "user-pwchange", domain.RoleEditor, domain.UserStatusActive)

	// 1. Senha atual incorreta
	formErr := url.Values{
		"current_password": {"SenhaErrada123!"},
		"new_password":     {"NovaSenhaForte123!"},
	}
	reqErr := httptest.NewRequest(http.MethodPost, "/admin/perfil/senha", strings.NewReader(formErr.Encode()))
	reqErr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqErr.Header.Set("Origin", origin)
	reqErr.AddCookie(cookie)
	wErr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wErr, reqErr)

	if wErr.Code != http.StatusSeeOther {
		t.Fatalf("esperado redirect 303 no erro de alteração de senha, obtido %d", wErr.Code)
	}
	locErr := wErr.Header().Get("Location")
	if !strings.Contains(locErr, "err=") {
		t.Errorf("esperado redirect contendo query param err, obtido %q", locErr)
	}

	// 2. Senha atual correta
	formOk := url.Values{
		"current_password": {"SenhaForte123!"},
		"new_password":     {"NovaSenhaForte123!"},
	}
	reqOk := httptest.NewRequest(http.MethodPost, "/admin/perfil/senha", strings.NewReader(formOk.Encode()))
	reqOk.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqOk.Header.Set("Origin", origin)
	reqOk.AddCookie(cookie)
	wOk := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wOk, reqOk)

	if wOk.Code != http.StatusSeeOther {
		t.Fatalf("esperado redirect 303 no sucesso de alteração de senha, obtido %d", wOk.Code)
	}
	locOk := wOk.Header().Get("Location")
	if !strings.Contains(locOk, "msg=") {
		t.Errorf("esperado redirect contendo query param msg, obtido %q", locOk)
	}
}

func TestAuthFlow_MFASetup_HTTPFlow_NoHiddenSecret(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, true)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "editor-setup-test",
		DisplayName: "Editor Setup Test",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// 1. Login por senha redireciona para setup de MFA
	form := url.Values{
		"username": {"editor-setup-test"},
		"password": {"SenhaForte123!"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.Header.Set("Origin", origin)
	wLogin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusSeeOther || wLogin.Header().Get("Location") != "/admin/mfa/setup" {
		t.Fatalf("login deveria redirecionar para /admin/mfa/setup, obtido code=%d loc=%q", wLogin.Code, wLogin.Header().Get("Location"))
	}

	var pendingCookie *http.Cookie
	for _, c := range wLogin.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil {
		t.Fatalf("cookie de sessão pendente ausente")
	}

	// 2. GET /admin/mfa/setup renderiza QR code e chave manual, SEM campo hidden com o segredo
	reqSetupPage := httptest.NewRequest(http.MethodGet, "/admin/mfa/setup", nil)
	reqSetupPage.AddCookie(pendingCookie)
	wSetupPage := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSetupPage, reqSetupPage)

	if wSetupPage.Code != http.StatusOK {
		t.Fatalf("GET /admin/mfa/setup falhou com status %d", wSetupPage.Code)
	}
	setupHTML := wSetupPage.Body.String()
	if strings.Contains(setupHTML, `type="hidden" name="secret"`) || strings.Contains(setupHTML, `name="secret"`) {
		t.Fatalf("VULNERABILIDADE: HTML de setup contém campo secreto 'secret' que pode ser reenviado pelo cliente!")
	}
	if !strings.Contains(setupHTML, `name="code"`) {
		t.Errorf("formulário de setup deve conter input para o código TOTP 'code'")
	}

	// Consulta o segredo temporário que o servidor armazenou cifrado no banco
	uWithCreds, err := store.GetUserByID(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("falha ao buscar credenciais do usuário: %v", err)
	}
	if uWithCreds.MFAPendingSecretEncrypted == "" {
		t.Fatalf("mfa_pending_secret_encrypted deveria estar preenchido no servidor")
	}

	// Decriptografa segredo para simular o aplicativo autenticador do usuário
	keyBytes, err := auth.DeriveKey("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("falha ao derivar chave: %v", err)
	}
	pendingSecret, err := auth.DecryptString(uWithCreds.MFAPendingSecretEncrypted, keyBytes)
	if err != nil {
		t.Fatalf("falha ao decifrar segredo pendente para teste: %v", err)
	}
	validTOTPCode, err := auth.GenerateCode(pendingSecret, time.Now().UTC())
	if err != nil {
		t.Fatalf("falha ao gerar código TOTP: %v", err)
	}

	// 3. POST /admin/mfa/setup enviando APENAS o código TOTP de 6 dígitos
	formSetupConfirm := url.Values{
		"code": {validTOTPCode},
	}
	reqConfirm := httptest.NewRequest(http.MethodPost, "/admin/mfa/setup", strings.NewReader(formSetupConfirm.Encode()))
	reqConfirm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqConfirm.Header.Set("Origin", origin)
	reqConfirm.AddCookie(pendingCookie)
	wConfirm := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wConfirm, reqConfirm)

	if wConfirm.Code != http.StatusSeeOther || !strings.HasPrefix(wConfirm.Header().Get("Location"), "/admin") {
		t.Fatalf("confirmação de MFA esperava redirect 303 para /admin, obtido code=%d loc=%q", wConfirm.Code, wConfirm.Header().Get("Location"))
	}

	// Extrai cookie atualizado
	var verifiedCookie *http.Cookie
	for _, c := range wConfirm.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			verifiedCookie = c
			break
		}
	}
	if verifiedCookie == nil {
		t.Fatalf("cookie verificado ausente após confirmação de setup")
	}

	// 4. Acesso subsequente a /admin e demais páginas: garante que o segredo TOTP NUNCA vaza
	subsequentPaths := []string{
		"/admin",
		"/admin/perfil",
		"/admin/usuarios",
		"/admin/usuarios/" + user.ID,
		"/admin/auditoria",
	}

	for _, p := range subsequentPaths {
		reqPage := httptest.NewRequest(http.MethodGet, p, nil)
		reqPage.AddCookie(verifiedCookie)
		wPage := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wPage, reqPage)
		if wPage.Code != http.StatusOK && wPage.Code != http.StatusForbidden {
			t.Errorf("esperado 200 ou 403 em %s pós-setup, obtido %d", p, wPage.Code)
		}
		body := wPage.Body.String()
		if strings.Contains(body, pendingSecret) {
			t.Fatalf("VULNERABILIDADE: segredo TOTP vazou na página %s pós-setup!", p)
		}
		if strings.Contains(body, "otpauth://") {
			t.Fatalf("VULNERABILIDADE: URI otpauth vazou na página %s pós-setup!", p)
		}
	}

	// 5. Tentar acessar /admin/mfa/setup após confirmação deve redirecionar e NÃO exibir segredo
	reqSetupAfter := httptest.NewRequest(http.MethodGet, "/admin/mfa/setup", nil)
	reqSetupAfter.AddCookie(verifiedCookie)
	wSetupAfter := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSetupAfter, reqSetupAfter)
	if wSetupAfter.Code != http.StatusSeeOther {
		t.Fatalf("GET /admin/mfa/setup após confirmação deveria redirecionar 303, obtido %d", wSetupAfter.Code)
	}
	bodyAfter := wSetupAfter.Body.String()
	if strings.Contains(bodyAfter, pendingSecret) || strings.Contains(bodyAfter, "otpauth://") {
		t.Fatalf("VULNERABILIDADE: segredo TOTP vazou na resposta de redirect de setup!")
	}
}

func TestAuthFlow_PendingMFASetup_HTTPExpirationAndCleanup(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, true)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "editor-exp-test",
		DisplayName: "Editor Expiration Test",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// 1. Login por senha
	form := url.Values{
		"username": {"editor-exp-test"},
		"password": {"SenhaForte123!"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.Header.Set("Origin", origin)
	wLogin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLogin, reqLogin)

	var pendingCookie *http.Cookie
	for _, c := range wLogin.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil {
		t.Fatalf("cookie de sessão pendente ausente")
	}

	// 2. GET /admin/mfa/setup
	reqSetup := httptest.NewRequest(http.MethodGet, "/admin/mfa/setup", nil)
	reqSetup.AddCookie(pendingCookie)
	wSetup := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSetup, reqSetup)
	if wSetup.Code != http.StatusOK {
		t.Fatalf("setup falhou: %d", wSetup.Code)
	}

	// 3. Força expiração do segredo pendente no banco
	pastExpiresAt := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, "UPDATE admin_users SET mfa_pending_expires_at = ? WHERE id = ?", pastExpiresAt, user.ID)
	if err != nil {
		t.Fatalf("falha ao alterar expiração no banco: %v", err)
	}

	// 4. POST /admin/mfa/setup deve falhar e redirecionar com mensagem de erro
	formConfirm := url.Values{"code": {"123456"}}
	reqConfirm := httptest.NewRequest(http.MethodPost, "/admin/mfa/setup", strings.NewReader(formConfirm.Encode()))
	reqConfirm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqConfirm.Header.Set("Origin", origin)
	reqConfirm.AddCookie(pendingCookie)
	wConfirm := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wConfirm, reqConfirm)

	if wConfirm.Code != http.StatusSeeOther {
		t.Fatalf("esperava redirect 303 no erro de expiração, obtido %d", wConfirm.Code)
	}
	if !strings.Contains(wConfirm.Header().Get("Location"), "err=") {
		t.Errorf("esperava redirect para /admin/mfa/setup com param err, obtido %q", wConfirm.Header().Get("Location"))
	}

	// 5. Verifica que o segredo pendente foi limpo no SQLite
	var pendingSecretEnc string
	_ = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted FROM admin_users WHERE id = ?", user.ID).Scan(&pendingSecretEnc)
	if pendingSecretEnc != "" {
		t.Errorf("mfa_pending_secret_encrypted deveria ter sido limpo após tentativa expirada, obtido %q", pendingSecretEnc)
	}
}

func TestAuthFlow_PendingMFASetup_HTTPLogoutCleanup(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, true)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin",
		Username: "sys-admin",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "editor-logout-test",
		DisplayName: "Editor Logout Test",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// 1. Login por senha
	form := url.Values{
		"username": {"editor-logout-test"},
		"password": {"SenhaForte123!"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.Header.Set("Origin", origin)
	wLogin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLogin, reqLogin)

	var pendingCookie *http.Cookie
	for _, c := range wLogin.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil {
		t.Fatalf("cookie de sessão pendente ausente")
	}

	// 2. GET /admin/mfa/setup gera segredo pendente
	reqSetup := httptest.NewRequest(http.MethodGet, "/admin/mfa/setup", nil)
	reqSetup.AddCookie(pendingCookie)
	wSetup := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSetup, reqSetup)
	if wSetup.Code != http.StatusOK {
		t.Fatalf("setup falhou: %d", wSetup.Code)
	}

	// Verifica que segredo pendente existe
	var pendingSecretEnc string
	_ = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted FROM admin_users WHERE id = ?", user.ID).Scan(&pendingSecretEnc)
	if pendingSecretEnc == "" {
		t.Fatalf("esperava segredo pendente gerado antes do logout")
	}

	// 3. POST /admin/logout
	reqLogout := httptest.NewRequest(http.MethodPost, "/admin/logout", strings.NewReader(""))
	reqLogout.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogout.Header.Set("Origin", origin)
	reqLogout.AddCookie(pendingCookie)
	wLogout := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLogout, reqLogout)

	if wLogout.Code != http.StatusSeeOther {
		t.Fatalf("logout esperava redirect 303, obtido %d", wLogout.Code)
	}

	// 4. Verifica que o segredo pendente foi invalidado/limpo no banco
	_ = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted FROM admin_users WHERE id = ?", user.ID).Scan(&pendingSecretEnc)
	if pendingSecretEnc != "" {
		t.Errorf("mfa_pending_secret_encrypted deveria ter sido limpo no logout, obtido %q", pendingSecretEnc)
	}
}

func TestAuthFlow_BasicAuthBypassPrevention_AllAdminRoutes(t *testing.T) {
	srv, _, _, origin := setupAuthFlowTestServer(t, false)

	protectedPaths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin"},
		{http.MethodGet, "/admin/candidatos"},
		{http.MethodGet, "/admin/usuarios"},
		{http.MethodGet, "/admin/auditoria"},
		{http.MethodGet, "/admin/manifestacoes"},
		{http.MethodPost, "/admin/claims/claim-bypass-1/moderate"},
		{http.MethodPost, "/admin/evidence-sources/es-bypass-1/moderate"},
		{http.MethodPost, "/admin/usuarios"},
	}

	for _, tc := range protectedPaths {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			form := url.Values{"action": {"approve"}, "reason": {"Bypass Attempt"}}
			var bodyReader *strings.Reader
			if tc.method == http.MethodPost {
				bodyReader = strings.NewReader(form.Encode())
			} else {
				bodyReader = strings.NewReader("")
			}

			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
			// Envia cabeçalho HTTP Basic Auth válido do admin padrão
			req.SetBasicAuth("admin", "SenhaPadraoAdmin123!")

			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			// Basic Auth NÃO pode conceder acesso; deve ser rejeitado com 401 Unauthorized
			if w.Code != http.StatusUnauthorized {
				t.Errorf("rota %s %s com Basic Auth esperava 401 Unauthorized, mas retornou %d", tc.method, tc.path, w.Code)
			}
		})
	}
}

func TestAuthFlow_Security_NoPlaintextSecretsOrTokensInAuditAndViews(t *testing.T) {
	srv, db, authSvc, _ := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminCookie := createTestUserAndSession(t, ctx, authSvc, db, "admin-sec-audit", domain.RoleAdmin, domain.UserStatusActive)

	// Cria usuário com MFA
	editor, err := authSvc.CreateUser(ctx, domain.AdminUser{
		ID:       "sys-admin",
		Username: "admin-sec-audit",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}, auth.CreateUserParams{
		Username:    "editor-sec",
		DisplayName: "Editor Sec",
		Password:    "SenhaSuperSecreta999!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar editor: %v", err)
	}

	// Gera MFA
	secret, _, err := authSvc.GenerateMFASetup(ctx, editor.ID)
	if err != nil {
		t.Fatalf("falha ao gerar MFA: %v", err)
	}

	// 1. Verifica no SQLite se o segredo plano aparece em alguma tabela de auditoria ou credenciais
	rows, err := db.QueryContext(ctx, "SELECT action, details FROM admin_audit_logs")
	if err != nil {
		t.Fatalf("falha ao ler logs de auditoria: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var action, details string
		if err := rows.Scan(&action, &details); err != nil {
			t.Fatalf("scan audit error: %v", err)
		}
		if strings.Contains(details, secret) {
			t.Errorf("VULNERABILIDADE: segredo TOTP em texto plano encontrado no log de auditoria (%s): %s", action, details)
		}
		if strings.Contains(details, "SenhaSuperSecreta999!") {
			t.Errorf("VULNERABILIDADE: senha em texto plano encontrada no log de auditoria (%s): %s", action, details)
		}
	}

	// 2. Consulta página de auditoria via HTML
	reqAudit := httptest.NewRequest(http.MethodGet, "/admin/auditoria", nil)
	reqAudit.AddCookie(adminCookie)
	wAudit := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAudit, reqAudit)

	if wAudit.Code != http.StatusOK {
		t.Fatalf("GET /admin/auditoria retornou %d", wAudit.Code)
	}
	auditHTML := wAudit.Body.String()
	if strings.Contains(auditHTML, secret) {
		t.Errorf("VULNERABILIDADE: segredo TOTP vazado no HTML de /admin/auditoria")
	}
	if strings.Contains(auditHTML, "SenhaSuperSecreta999!") {
		t.Errorf("VULNERABILIDADE: senha vazada no HTML de /admin/auditoria")
	}
}

func TestAuthFlow_MFAChallenge_LockoutAndSessionInvalidation(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin-challenge-lockout",
		Username: "sys-admin-challenge-lockout",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "editor-challenge-lockout",
		DisplayName: "Editor Challenge Lockout",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("falha ao gerar segredo: %v", err)
	}
	keyBytes, _ := auth.DeriveKey("12345678901234567890123456789012")
	encSecret, _ := auth.EncryptString(secret, keyBytes)
	_, err = db.ExecContext(ctx, "UPDATE admin_users SET mfa_enabled = 1, mfa_secret_encrypted = ? WHERE id = ?", encSecret, user.ID)
	if err != nil {
		t.Fatalf("falha ao ativar MFA: %v", err)
	}

	// 1. Login por senha -> redireciona para /admin/mfa/challenge com cookie de sessão pendente
	formLogin := url.Values{
		"username": {"editor-challenge-lockout"},
		"password": {"SenhaForte123!"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(formLogin.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.Header.Set("Origin", origin)
	wLogin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLogin, reqLogin)

	var pendingCookie *http.Cookie
	for _, c := range wLogin.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil {
		t.Fatalf("cookie de sessão pendente ausente")
	}

	// 2. Envia 2 tentativas com código errado -> 401 Unauthorized e mensagem de erro
	for i := 1; i <= 2; i++ {
		formMFA := url.Values{"code": {fmt.Sprintf("00000%d", i)}}
		reqMFA := httptest.NewRequest(http.MethodPost, "/admin/mfa/challenge", strings.NewReader(formMFA.Encode()))
		reqMFA.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqMFA.Header.Set("Origin", origin)
		reqMFA.AddCookie(pendingCookie)
		wMFA := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wMFA, reqMFA)

		if wMFA.Code != http.StatusUnauthorized {
			t.Errorf("tentativa %d esperava 401, obtido %d", i, wMFA.Code)
		}
	}

	// 3. Terceira tentativa (limite atingido) -> 401 com mensagem de bloqueio e limpeza de cookie
	formMFA3 := url.Values{"code": {"000003"}}
	reqMFA3 := httptest.NewRequest(http.MethodPost, "/admin/mfa/challenge", strings.NewReader(formMFA3.Encode()))
	reqMFA3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqMFA3.Header.Set("Origin", origin)
	reqMFA3.AddCookie(pendingCookie)
	wMFA3 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMFA3, reqMFA3)

	if wMFA3.Code != http.StatusUnauthorized {
		t.Errorf("3a tentativa esperava 401, obtido %d", wMFA3.Code)
	}
	if !strings.Contains(wMFA3.Body.String(), "bloqueada") {
		t.Errorf("esperava mensagem de conta bloqueada no HTML da resposta: %s", wMFA3.Body.String())
	}

	// 4. Verifica no banco que a conta está bloqueada
	uCheck, err := store.GetUserByID(ctx, db, user.ID)
	if err != nil || !uCheck.User.IsLocked() {
		t.Errorf("usuário deveria estar bloqueado no SQLite após 3 falhas de MFA")
	}

	// 5. Tentativa subsequente de acesso a /admin com o cookie anterior deve falhar com 401
	reqAfter := httptest.NewRequest(http.MethodGet, "/admin", nil)
	reqAfter.AddCookie(pendingCookie)
	wAfter := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAfter, reqAfter)

	if wAfter.Code != http.StatusUnauthorized && wAfter.Code != http.StatusSeeOther {
		t.Errorf("acesso com sessão invalidada esperava 401 ou 303, obtido %d", wAfter.Code)
	}
}

func TestAuthFlow_MFASetup_LockoutAndSessionInvalidation(t *testing.T) {
	srv, db, authSvc, origin := setupAuthFlowTestServer(t, false)
	ctx := context.Background()

	adminActor := domain.AdminUser{
		ID:       "sys-admin-setup-lockout",
		Username: "sys-admin-setup-lockout",
		Role:     domain.RoleAdmin,
		Status:   domain.UserStatusActive,
	}

	user, err := authSvc.CreateUser(ctx, adminActor, auth.CreateUserParams{
		Username:    "editor-setup-lockout",
		DisplayName: "Editor Setup Lockout",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// Login
	formLogin := url.Values{
		"username": {"editor-setup-lockout"},
		"password": {"SenhaForte123!"},
	}
	reqLogin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(formLogin.Encode()))
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqLogin.Header.Set("Origin", origin)
	wLogin := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wLogin, reqLogin)

	var pendingCookie *http.Cookie
	for _, c := range wLogin.Result().Cookies() {
		if c.Name == web.SessionCookieName && c.Value != "" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil {
		t.Fatalf("cookie de sessão pendente ausente")
	}

	// Inicia setup
	reqSetup := httptest.NewRequest(http.MethodGet, "/admin/mfa/setup", nil)
	reqSetup.AddCookie(pendingCookie)
	wSetup := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSetup, reqSetup)
	if wSetup.Code != http.StatusOK {
		t.Fatalf("GET /admin/mfa/setup falhou: %d", wSetup.Code)
	}

	// 2 tentativas com código errado -> redireciona para setup com erro
	for i := 1; i <= 2; i++ {
		formSetup := url.Values{"code": {fmt.Sprintf("99999%d", i)}}
		reqSubmit := httptest.NewRequest(http.MethodPost, "/admin/mfa/setup", strings.NewReader(formSetup.Encode()))
		reqSubmit.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqSubmit.Header.Set("Origin", origin)
		reqSubmit.AddCookie(pendingCookie)
		wSubmit := httptest.NewRecorder()
		srv.Handler.ServeHTTP(wSubmit, reqSubmit)

		if wSubmit.Code != http.StatusSeeOther || !strings.Contains(wSubmit.Header().Get("Location"), "/admin/mfa/setup") {
			t.Errorf("tentativa %d esperava redirect para /admin/mfa/setup, obtido code=%d loc=%s", i, wSubmit.Code, wSubmit.Header().Get("Location"))
		}
	}

	// 3a tentativa -> limite de 3 atingido, redireciona para login informando bloqueio
	formSetup3 := url.Values{"code": {"999993"}}
	reqSubmit3 := httptest.NewRequest(http.MethodPost, "/admin/mfa/setup", strings.NewReader(formSetup3.Encode()))
	reqSubmit3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSubmit3.Header.Set("Origin", origin)
	reqSubmit3.AddCookie(pendingCookie)
	wSubmit3 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wSubmit3, reqSubmit3)

	if wSubmit3.Code != http.StatusSeeOther || !strings.Contains(wSubmit3.Header().Get("Location"), "/admin/login") {
		t.Errorf("3a tentativa esperava redirect para /admin/login, obtido code=%d loc=%s", wSubmit3.Code, wSubmit3.Header().Get("Location"))
	}

	// Verifica no banco que a conta está bloqueada e segredo pendente foi limpo
	uCheck, err := store.GetUserByID(ctx, db, user.ID)
	if err != nil || !uCheck.User.IsLocked() {
		t.Errorf("usuário deveria estar bloqueado após 3 falhas na confirmação do setup")
	}
	if uCheck.MFAPendingSecretEncrypted != "" {
		t.Errorf("segredo pendente deveria ter sido limpo no bloqueio")
	}
}
