package auth_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const testMFAEncryptionKey = "12345678901234567890123456789012"

func setupAuthTest(t *testing.T) (*auth.Service, *domain.AdminUser, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "auth_service_test.db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	svc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, true, testMFAEncryptionKey)
	if err != nil {
		t.Fatalf("falha ao inicializar auth.Service: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("SenhaForte123!"), 12)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste: %v", err)
	}

	// Cria usuário admin inicial
	adminUser, err := store.CreateAdminUser(ctx, db, store.CreateUserParams{
		Username:     "admin.principal",
		DisplayName:  "Admin Principal",
		PasswordHash: string(hash),
		Role:         domain.RoleAdmin,
		Status:       domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("falha ao criar admin de teste: %v", err)
	}

	return svc, adminUser, db
}

func TestAuthService_NewService_KeyValidation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "auth_keys_test.db")
	ctx := context.Background()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	t.Run("chave vazia falha fechada sem fallback", func(t *testing.T) {
		svc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, true, "")
		if err == nil || svc != nil {
			t.Fatalf("esperava erro para chave vazia, obteve svc=%v", svc)
		}
	})

	t.Run("chave com tamanho invalido falha", func(t *testing.T) {
		svc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, true, "chave-curta-16b!")
		if err == nil || svc != nil {
			t.Fatalf("esperava erro para chave curta, obteve svc=%v", svc)
		}
	})

	t.Run("chave hex de 64 caracteres com caracteres invalidos falha", func(t *testing.T) {
		svc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, true, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789zzzzzz")
		if err == nil || svc != nil {
			t.Fatalf("esperava erro para chave hex invalida, obteve svc=%v", svc)
		}
	})

	t.Run("chave valida de 32 bytes sucesso", func(t *testing.T) {
		svc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, true, "12345678901234567890123456789012")
		if err != nil || svc == nil {
			t.Fatalf("esperava sucesso para chave de 32 bytes, erro: %v", err)
		}
	})

	t.Run("chave valida de 64 caracteres hex sucesso", func(t *testing.T) {
		svc, err := auth.NewService(db, 8*time.Hour, 15*time.Minute, 3, true, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
		if err != nil || svc == nil {
			t.Fatalf("esperava sucesso para chave hex valida, erro: %v", err)
		}
	})
}

func TestAuthService_PasswordFlow_And_Lockout(t *testing.T) {
	svc, _, _ := setupAuthTest(t)
	ctx := context.Background()

	// 1. Usuário inexistente
	_, _, _, err := svc.AuthenticatePassword(ctx, "inexistente", "qualquersenha", "127.0.0.1", "TestAgent")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("esperava ErrInvalidCredentials para usuário inexistente, obtido: %v", err)
	}

	// 2. Senha errada (tentativas 1 e 2)
	_, _, _, err = svc.AuthenticatePassword(ctx, "admin.principal", "SenhaErrada1", "127.0.0.1", "TestAgent")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("tentativa 1: esperava ErrInvalidCredentials, obtido: %v", err)
	}

	_, _, _, err = svc.AuthenticatePassword(ctx, "admin.principal", "SenhaErrada2", "127.0.0.1", "TestAgent")
	if err != auth.ErrInvalidCredentials {
		t.Errorf("tentativa 2: esperava ErrInvalidCredentials, obtido: %v", err)
	}

	// 3. Terceira tentativa errada -> atinge maxAttempts (3) -> Lockout
	_, _, _, err = svc.AuthenticatePassword(ctx, "admin.principal", "SenhaErrada3", "127.0.0.1", "TestAgent")
	if err != auth.ErrAccountLocked {
		t.Errorf("tentativa 3: esperava ErrAccountLocked após 3 falhas, obtido: %v", err)
	}

	// Tentativa subsequente mesmo com senha certa deve ser bloqueada
	_, _, _, err = svc.AuthenticatePassword(ctx, "admin.principal", "SenhaForte123!", "127.0.0.1", "TestAgent")
	if err != auth.ErrAccountLocked {
		t.Errorf("após lockout: esperava ErrAccountLocked, obtido: %v", err)
	}
}

func TestAuthService_MFAFlow_Setup_And_Verify(t *testing.T) {
	svc, adminUser, db := setupAuthTest(t)
	ctx := context.Background()

	// Cria usuário editor sem MFA inicial
	editor, err := svc.CreateUser(ctx, *adminUser, auth.CreateUserParams{
		Username:    "editor.maria",
		DisplayName: "Maria Editora",
		Password:    "SenhaSeguraEditor123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar editor: %v", err)
	}

	// 1. Login inicial por senha: como papel 'editor' exige MFA, needsMFA deve ser true
	u, sess, needsMFA, err := svc.AuthenticatePassword(ctx, "editor.maria", "SenhaSeguraEditor123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("falha no login inicial: %v", err)
	}
	if !needsMFA {
		t.Errorf("esperava needsMFA=true para role editor")
	}
	if sess.MFAVerified {
		t.Errorf("sessão antes do MFA não pode estar MFAVerified=true")
	}

	// 2. Setup de MFA
	secret, uri, err := svc.GenerateMFASetup(ctx, u.ID)
	if err != nil {
		t.Fatalf("falha ao gerar setup de MFA: %v", err)
	}
	if secret == "" || uri == "" {
		t.Fatalf("secret ou uri vazios")
	}

	// Verifica se o segredo pendente foi gravado criptografado no banco SQLite
	var pendingSecretEnc, activeSecretEnc string
	err = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted, mfa_secret_encrypted FROM admin_users WHERE id = ?", u.ID).Scan(&pendingSecretEnc, &activeSecretEnc)
	if err != nil {
		t.Fatalf("falha ao consultar banco bruto: %v", err)
	}
	if pendingSecretEnc == "" || pendingSecretEnc == secret {
		t.Fatalf("VULNERABILIDADE: mfa_pending_secret_encrypted vazio ou gravado em texto plano!")
	}
	keyBytes, err := auth.DeriveKey(testMFAEncryptionKey)
	if err != nil {
		t.Fatalf("falha ao derivar chave: %v", err)
	}
	decryptedPending, err := auth.DecryptString(pendingSecretEnc, keyBytes)
	if err != nil || decryptedPending != secret {
		t.Fatalf("falha ao decifrar segredo pendente: err=%v, obtido=%s, esperado=%s", err, decryptedPending, secret)
	}
	if activeSecretEnc != "" {
		t.Errorf("mfa_secret_encrypted ativo deve ser vazio antes da confirmação, obtido: %s", activeSecretEnc)
	}

	// 3. Confirmação com código inválido
	_, _, err = svc.ConfirmMFASetup(ctx, sess.ID, "000000", "127.0.0.1")
	if err != auth.ErrMFAInvalidCode {
		t.Errorf("esperava ErrMFAInvalidCode para código incorreto, obtido: %v", err)
	}

	// 4. Confirmação com código válido (código derivado do secret gerado no setup)
	code, err := auth.GenerateCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("falha ao derivar código TOTP: %v", err)
	}

	elevatedUser, elevatedSess, err := svc.ConfirmMFASetup(ctx, sess.ID, code, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao confirmar MFA: %v", err)
	}
	if elevatedUser == nil || !elevatedSess.MFAVerified {
		t.Errorf("sessão pós-confirmação deveria estar MFAVerified=true")
	}
	if elevatedSess.ID == sess.ID {
		t.Errorf("sessão deveria ter sido rotacionada (IDs devem ser diferentes)")
	}

	// Verifica se o segredo ativo agora está gravado criptografado e o pendente foi limpo
	err = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted, mfa_secret_encrypted FROM admin_users WHERE id = ?", u.ID).Scan(&pendingSecretEnc, &activeSecretEnc)
	if err != nil {
		t.Fatalf("falha ao consultar banco bruto pós-confirmação: %v", err)
	}
	if pendingSecretEnc != "" {
		t.Errorf("mfa_pending_secret_encrypted deveria ter sido limpo, obtido: %s", pendingSecretEnc)
	}
	if activeSecretEnc == "" || activeSecretEnc == secret {
		t.Fatalf("VULNERABILIDADE: mfa_secret_encrypted vazio ou gravado em texto plano!")
	}
	decryptedActive, err := auth.DecryptString(activeSecretEnc, keyBytes)
	if err != nil || decryptedActive != secret {
		t.Fatalf("falha ao decifrar segredo ativo: err=%v, obtido=%s, esperado=%s", err, decryptedActive, secret)
	}

	// Tentar gerar setup novamente quando MFA já está ativo deve ser recusado
	_, _, err = svc.GenerateMFASetup(ctx, u.ID)
	if err == nil {
		t.Errorf("esperava erro ao gerar setup quando MFA já está ativo")
	}

	// 5. Novo login com MFA ativado
	_, sess2, needsMFA2, err := svc.AuthenticatePassword(ctx, "editor.maria", "SenhaSeguraEditor123!", "127.0.0.1", "TestAgent")
	if err != nil || !needsMFA2 {
		t.Fatalf("segundo login com MFA ativo falhou: err=%v, needsMFA=%v", err, needsMFA2)
	}

	code2, _ := auth.GenerateCode(secret, time.Now().UTC())
	verifiedUser, verifiedSess, err := svc.VerifyMFALogin(ctx, sess2.ID, code2, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha na verificação de MFA: %v", err)
	}
	if !verifiedSess.MFAVerified || verifiedUser.Username != "editor.maria" {
		t.Errorf("usuário ou sessão verificada incorretos: user=%s, mfaVerified=%v", verifiedUser.Username, verifiedSess.MFAVerified)
	}

	// 6. Desativação de MFA por Admin
	err = svc.DisableMFA(ctx, *adminUser, editor.ID, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao desativar MFA por admin: %v", err)
	}

	// Verifica se segredos foram apagados no banco
	err = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted, mfa_secret_encrypted FROM admin_users WHERE id = ?", u.ID).Scan(&pendingSecretEnc, &activeSecretEnc)
	if err != nil {
		t.Fatalf("falha ao consultar banco bruto pós-disable: %v", err)
	}
	if activeSecretEnc != "" || pendingSecretEnc != "" {
		t.Errorf("segredos de MFA deveriam ser limpos no banco, obtido active=%q pending=%q", activeSecretEnc, pendingSecretEnc)
	}

	// A sessão anterior deve ter sido revogada
	_, _, errSessRevoked := svc.ValidateSession(ctx, verifiedSess.ID)
	if errSessRevoked != auth.ErrSessionNotFound {
		t.Errorf("sessão antiga após disable MFA deveria estar revogada, obtido: %v", errSessRevoked)
	}
}

func TestAuthService_PendingMFASetup_ExpirationAndCleanup(t *testing.T) {
	svc, adminUser, db := setupAuthTest(t)
	ctx := context.Background()

	// Cria revisor
	reviewer, err := svc.CreateUser(ctx, *adminUser, auth.CreateUserParams{
		Username:    "revisor.pedro",
		DisplayName: "Pedro Revisor",
		Password:    "SenhaSeguraRevisor123!",
		Role:        domain.RoleReviewer,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar revisor: %v", err)
	}

	_, sess, _, err := svc.AuthenticatePassword(ctx, "revisor.pedro", "SenhaSeguraRevisor123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login inicial falhou: %v", err)
	}

	// Inicia setup
	secret, _, err := svc.GenerateMFASetup(ctx, reviewer.ID)
	if err != nil {
		t.Fatalf("falha ao gerar setup: %v", err)
	}

	// Força expiração do setup pendente para 1 minuto no passado
	pastExpiresAt := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, "UPDATE admin_users SET mfa_pending_expires_at = ? WHERE id = ?", pastExpiresAt, reviewer.ID)
	if err != nil {
		t.Fatalf("falha ao forçar expiração no banco: %v", err)
	}

	// Tenta confirmar com código válido derivado do segredo
	code, _ := auth.GenerateCode(secret, time.Now().UTC())
	_, _, err = svc.ConfirmMFASetup(ctx, sess.ID, code, "127.0.0.1")
	if err == nil {
		t.Fatalf("esperava erro por expiração do setup pendente")
	}

	// Verifica se o estado pendente foi limpo após expiração
	var pendingSecretEnc string
	_ = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted FROM admin_users WHERE id = ?", reviewer.ID).Scan(&pendingSecretEnc)
	if pendingSecretEnc != "" {
		t.Errorf("mfa_pending_secret_encrypted deveria ter sido limpo após expiração, obtido: %s", pendingSecretEnc)
	}
}

func TestAuthService_PendingMFASetup_InvalidationOnLogout(t *testing.T) {
	svc, adminUser, db := setupAuthTest(t)
	ctx := context.Background()

	// Cria auditor
	auditor, err := svc.CreateUser(ctx, *adminUser, auth.CreateUserParams{
		Username:    "auditor.lucas",
		DisplayName: "Lucas Auditor",
		Password:    "SenhaSeguraAuditor123!",
		Role:        domain.RoleAuditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar auditor: %v", err)
	}

	_, sess, _, err := svc.AuthenticatePassword(ctx, "auditor.lucas", "SenhaSeguraAuditor123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login inicial falhou: %v", err)
	}

	// Inicia setup
	_, _, err = svc.GenerateMFASetup(ctx, auditor.ID)
	if err != nil {
		t.Fatalf("falha ao gerar setup: %v", err)
	}

	// Verifica se há segredo pendente
	var pendingSecretEnc string
	_ = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted FROM admin_users WHERE id = ?", auditor.ID).Scan(&pendingSecretEnc)
	if pendingSecretEnc == "" {
		t.Fatalf("esperava segredo pendente gravado antes do logout")
	}

	// Logout
	err = svc.RevokeSession(ctx, sess.ID, "127.0.0.1")
	if err != nil {
		t.Fatalf("logout falhou: %v", err)
	}

	// Verifica se o segredo pendente foi invalidado/limpo
	_ = db.QueryRowContext(ctx, "SELECT mfa_pending_secret_encrypted FROM admin_users WHERE id = ?", auditor.ID).Scan(&pendingSecretEnc)
	if pendingSecretEnc != "" {
		t.Errorf("mfa_pending_secret_encrypted deveria ter sido limpo no logout, obtido: %s", pendingSecretEnc)
	}
}

func TestAuthService_SessionValidation_And_Revocation(t *testing.T) {
	svc, adminUser, _ := setupAuthTest(t)
	ctx := context.Background()

	_, sess, _, err := svc.AuthenticatePassword(ctx, adminUser.Username, "SenhaForte123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	// Validação de sessão ativa
	u, s, err := svc.ValidateSession(ctx, sess.ID)
	if err != nil || u.ID != adminUser.ID || s.ID != sess.ID {
		t.Fatalf("sessão ativa falhou na validação: err=%v", err)
	}

	// Logout / Revogação de sessão
	err = svc.RevokeSession(ctx, sess.ID, "127.0.0.1")
	if err != nil {
		t.Fatalf("logout falhou: %v", err)
	}

	// Validação após revogação deve falhar
	_, _, err = svc.ValidateSession(ctx, sess.ID)
	if err != auth.ErrSessionNotFound {
		t.Errorf("esperava ErrSessionNotFound após logout, obtido: %v", err)
	}
}

func TestAuthService_RBAC_UserManagement_Restrictions(t *testing.T) {
	svc, adminUser, _ := setupAuthTest(t)
	ctx := context.Background()

	// Cria editor
	editor, err := svc.CreateUser(ctx, *adminUser, auth.CreateUserParams{
		Username:    "editor.carlos",
		DisplayName: "Carlos Editor",
		Password:    "SenhaValida123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar editor: %v", err)
	}

	// Editor tentando criar outro usuário -> Deve falhar com ErrForbidden
	_, err = svc.CreateUser(ctx, *editor, auth.CreateUserParams{
		Username:    "novo.auditor",
		DisplayName: "Novo Auditor",
		Password:    "SenhaValida123!",
		Role:        domain.RoleAuditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != auth.ErrForbidden {
		t.Errorf("editor criando usuário esperava ErrForbidden, obtido: %v", err)
	}

	// Editor tentando alterar papel -> Deve falhar com ErrForbidden
	err = svc.UpdateUserRole(ctx, *editor, adminUser.ID, adminUser.UpdatedAt, domain.RoleAuditor, "127.0.0.1")
	if err != auth.ErrForbidden {
		t.Errorf("editor alterando papel esperava ErrForbidden, obtido: %v", err)
	}
}

func TestAuthService_VerifyMFALogin_RateLimitAndLockout(t *testing.T) {
	svc, _, db := setupAuthTest(t)
	ctx := context.Background()

	// 1. Cria usuário com MFA ativo
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("falha ao gerar segredo: %v", err)
	}
	keyBytes, _ := auth.DeriveKey("12345678901234567890123456789012")
	encSecret, err := auth.EncryptString(secret, keyBytes)
	if err != nil {
		t.Fatalf("falha ao encriptar segredo: %v", err)
	}

	actor := domain.AdminUser{
		ID:       "admin-actor-id",
		Username: "admin-actor",
		Role:     domain.RoleAdmin,
	}
	u, err := svc.CreateUser(ctx, actor, auth.CreateUserParams{
		Username:    "user.mfa.lockout",
		DisplayName: "User MFA Lockout",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// Ativa MFA diretamente no banco
	_, err = db.ExecContext(ctx, "UPDATE admin_users SET mfa_enabled = 1, mfa_secret_encrypted = ? WHERE id = ?", encSecret, u.ID)
	if err != nil {
		t.Fatalf("falha ao ativar MFA: %v", err)
	}

	// 2. Realiza login por senha -> obtém sessão não verificada
	_, sess, needsMFA, err := svc.AuthenticatePassword(ctx, "user.mfa.lockout", "SenhaForte123!", "127.0.0.1", "TestAgent")
	if err != nil || !needsMFA || sess == nil {
		t.Fatalf("login falhou ou needsMFA=false: err=%v", err)
	}

	// 3. Tenta códigos inválidos sucessivamente (table-driven com maxAttempts=3)
	attempts := []struct {
		code          string
		expectedErr   error
		expectedCount int
		shouldLock    bool
	}{
		{code: "000001", expectedErr: auth.ErrMFAInvalidCode, expectedCount: 1, shouldLock: false},
		{code: "000002", expectedErr: auth.ErrMFAInvalidCode, expectedCount: 2, shouldLock: false},
		{code: "000003", expectedErr: auth.ErrAccountLocked, expectedCount: 3, shouldLock: true},
	}

	for i, att := range attempts {
		_, _, err := svc.VerifyMFALogin(ctx, sess.ID, att.code, "127.0.0.1")
		if !errors.Is(err, att.expectedErr) {
			t.Errorf("tentativa %d (code=%s): esperado erro %v, obtido %v", i+1, att.code, att.expectedErr, err)
		}

		// Consulta estado do usuário no banco
		uCheck, err := store.GetUserByID(ctx, db, u.ID)
		if err != nil {
			t.Fatalf("falha ao consultar usuário após tentativa %d: %v", i+1, err)
		}

		if uCheck.User.MFAFailedAttempts != att.expectedCount {
			t.Errorf("tentativa %d: esperado MFAFailedAttempts=%d, obtido %d", i+1, att.expectedCount, uCheck.User.MFAFailedAttempts)
		}
		if uCheck.User.FailedLoginAttempts != 0 {
			t.Errorf("tentativa %d: esperado FailedLoginAttempts=0 (senha válida), obtido %d", i+1, uCheck.User.FailedLoginAttempts)
		}

		if att.shouldLock {
			if !uCheck.User.IsLocked() {
				t.Errorf("usuário deveria estar bloqueado após %d falhas no MFA", att.expectedCount)
			}
			// Sessão deve ter sido deletada
			_, _, err := svc.ValidateSession(ctx, sess.ID)
			if err != auth.ErrSessionNotFound {
				t.Errorf("sessão pendente deveria ter sido invalidada após lockout, obtido err=%v", err)
			}
		}
	}

	// Tentativa adicional após bloqueio
	_, _, err = svc.VerifyMFALogin(ctx, sess.ID, "000006", "127.0.0.1")
	if err != auth.ErrSessionNotFound {
		t.Errorf("tentativa após lockout deveria retornar ErrSessionNotFound (sessão deletada), obtido %v", err)
	}
}

func TestAuthService_ConfirmMFASetup_RateLimitAndLockout(t *testing.T) {
	svc, _, db := setupAuthTest(t)
	ctx := context.Background()

	actor := domain.AdminUser{
		ID:       "admin-actor-id-2",
		Username: "admin-actor-2",
		Role:     domain.RoleAdmin,
	}
	u, err := svc.CreateUser(ctx, actor, auth.CreateUserParams{
		Username:    "user.setup.lockout",
		DisplayName: "User Setup Lockout",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	// Login por senha
	_, sess, _, err := svc.AuthenticatePassword(ctx, "user.setup.lockout", "SenhaForte123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	// Gera setup de MFA
	_, _, err = svc.GenerateMFASetup(ctx, u.ID)
	if err != nil {
		t.Fatalf("falha ao gerar setup: %v", err)
	}

	// 3 tentativas erradas na confirmação do setup (maxAttempts=3)
	for i := 1; i <= 3; i++ {
		_, _, err := svc.ConfirmMFASetup(ctx, sess.ID, fmt.Sprintf("99999%d", i), "127.0.0.1")
		if i < 3 {
			if !errors.Is(err, auth.ErrMFAInvalidCode) {
				t.Errorf("tentativa %d esperava ErrMFAInvalidCode, obtido %v", i, err)
			}
		} else {
			if !errors.Is(err, auth.ErrAccountLocked) {
				t.Errorf("3a tentativa esperava ErrAccountLocked, obtido %v", err)
			}
		}
	}

	// Verifica se usuário está bloqueado e segredo pendente foi limpo
	uCheck, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao consultar usuário pós-lockout: %v", err)
	}
	if !uCheck.User.IsLocked() {
		t.Errorf("usuário deveria estar bloqueado após 3 falhas no setup")
	}
	if uCheck.User.MFAFailedAttempts != 3 {
		t.Errorf("esperado MFAFailedAttempts=3, obtido %d", uCheck.User.MFAFailedAttempts)
	}
	if uCheck.MFAPendingSecretEncrypted != "" {
		t.Errorf("segredo pendente deveria ter sido limpo no lockout")
	}
}

func TestAuthService_VerifyMFALogin_ConcurrentBruteForce(t *testing.T) {
	svc, _, db := setupAuthTest(t)
	ctx := context.Background()

	secret, _ := auth.GenerateSecret()
	keyBytes, _ := auth.DeriveKey("12345678901234567890123456789012")
	encSecret, _ := auth.EncryptString(secret, keyBytes)

	actor := domain.AdminUser{
		ID:       "admin-actor-id-3",
		Username: "admin-actor-3",
		Role:     domain.RoleAdmin,
	}
	u, err := svc.CreateUser(ctx, actor, auth.CreateUserParams{
		Username:    "user.mfa.concurrent",
		DisplayName: "User Concurrent MFA",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	_, _ = db.ExecContext(ctx, "UPDATE admin_users SET mfa_enabled = 1, mfa_secret_encrypted = ? WHERE id = ?", encSecret, u.ID)

	_, sess, _, err := svc.AuthenticatePassword(ctx, "user.mfa.concurrent", "SenhaForte123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	// Executa 20 tentativas concorrentes simultâneas com códigos inválidos
	const concurrency = 20
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			_, _, err := svc.VerifyMFALogin(ctx, sess.ID, fmt.Sprintf("%06d", idx+100), "127.0.0.1")
			done <- err
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	// 1. Verifica se o usuário no banco está categoricamente bloqueado
	uCheck, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao buscar usuário: %v", err)
	}
	if uCheck.User.Status != domain.UserStatusLocked {
		t.Errorf("status esperado %q, obtido %q", domain.UserStatusLocked, uCheck.User.Status)
	}
	if !uCheck.User.IsLocked() {
		t.Errorf("IsLocked() deveria retornar true após ataques concorrentes")
	}
	if uCheck.User.MFAFailedAttempts < 3 {
		t.Errorf("MFAFailedAttempts deveria ser >= 3, obtido %d", uCheck.User.MFAFailedAttempts)
	}

	// 2. Verifica se a sessão pendente foi completamente removida do banco
	_, err = store.GetAdminSessionWithUser(ctx, db, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("sessão pendente deveria ter sido removida do banco após lockout, obtido err=%v", err)
	}

	_, _, err = svc.ValidateSession(ctx, sess.ID)
	if err != auth.ErrSessionNotFound {
		t.Errorf("ValidateSession esperava ErrSessionNotFound, obtido %v", err)
	}
}

func TestAuthService_ConfirmMFASetup_ConcurrentBruteForce(t *testing.T) {
	svc, _, db := setupAuthTest(t)
	ctx := context.Background()

	actor := domain.AdminUser{
		ID:       "admin-actor-id-4",
		Username: "admin-actor-4",
		Role:     domain.RoleAdmin,
	}
	u, err := svc.CreateUser(ctx, actor, auth.CreateUserParams{
		Username:    "user.setup.concurrent",
		DisplayName: "User Setup Concurrent",
		Password:    "SenhaForte123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	_, sess, _, err := svc.AuthenticatePassword(ctx, "user.setup.concurrent", "SenhaForte123!", "127.0.0.1", "TestAgent")
	if err != nil {
		t.Fatalf("login falhou: %v", err)
	}

	// Inicia setup de MFA
	_, _, err = svc.GenerateMFASetup(ctx, u.ID)
	if err != nil {
		t.Fatalf("falha ao gerar setup: %v", err)
	}

	// Executa 20 tentativas concorrentes com códigos inválidos
	const concurrency = 20
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			_, _, err := svc.ConfirmMFASetup(ctx, sess.ID, fmt.Sprintf("%06d", idx+500), "127.0.0.1")
			done <- err
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	// 1. Verifica se usuário está bloqueado
	uCheck, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao buscar usuário pós-ataque concorrente: %v", err)
	}
	if uCheck.User.Status != domain.UserStatusLocked {
		t.Errorf("status esperado %q, obtido %q", domain.UserStatusLocked, uCheck.User.Status)
	}
	if !uCheck.User.IsLocked() {
		t.Errorf("IsLocked() deveria ser true após ataques concorrentes")
	}
	if uCheck.User.MFAFailedAttempts < 3 {
		t.Errorf("MFAFailedAttempts deveria ser >= 3, obtido %d", uCheck.User.MFAFailedAttempts)
	}

	// 2. Verifica se a sessão pendente foi deletada
	_, err = store.GetAdminSessionWithUser(ctx, db, sess.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("sessão deveria estar deletada pós-lockout, obtido %v", err)
	}

	// 3. Verifica se o segredo pendente efêmero foi purgado
	if uCheck.MFAPendingSecretEncrypted != "" {
		t.Errorf("MFAPendingSecretEncrypted deveria ter sido limpo no lockout, obtido %q", uCheck.MFAPendingSecretEncrypted)
	}
	if uCheck.MFAPendingExpiresAt != nil {
		t.Errorf("MFAPendingExpiresAt deveria ser nil no lockout")
	}
}

func TestAuthService_MFALockout_PersistsAcrossMultiplePasswordLogins(t *testing.T) {
	svc, _, db := setupAuthTest(t)
	ctx := context.Background()

	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("falha ao gerar segredo MFA: %v", err)
	}
	keyBytes, _ := auth.DeriveKey("12345678901234567890123456789012")
	encSecret, err := auth.EncryptString(secret, keyBytes)
	if err != nil {
		t.Fatalf("falha ao encriptar segredo: %v", err)
	}

	actor := domain.AdminUser{
		ID:       "admin-actor-multi",
		Username: "admin-actor-multi",
		Role:     domain.RoleAdmin,
	}
	u, err := svc.CreateUser(ctx, actor, auth.CreateUserParams{
		Username:    "user.multisession.mfa",
		DisplayName: "User Multi Session MFA",
		Password:    "SenhaCorreta123!",
		Role:        domain.RoleEditor,
		Status:      domain.UserStatusActive,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	_, err = db.ExecContext(ctx, "UPDATE admin_users SET mfa_enabled = 1, mfa_secret_encrypted = ? WHERE id = ?", encSecret, u.ID)
	if err != nil {
		t.Fatalf("falha ao ativar MFA: %v", err)
	}

	// Sessão 1: Login com senha válida -> Erra TOTP -> mfa_failed_attempts deve ser 1
	_, sess1, needsMFA1, err := svc.AuthenticatePassword(ctx, "user.multisession.mfa", "SenhaCorreta123!", "127.0.0.1", "Browser1")
	if err != nil || !needsMFA1 {
		t.Fatalf("login 1 com senha correta falhou: err=%v needsMFA=%v", err, needsMFA1)
	}

	_, _, err = svc.VerifyMFALogin(ctx, sess1.ID, "111111", "127.0.0.1")
	if !errors.Is(err, auth.ErrMFAInvalidCode) {
		t.Fatalf("esperava ErrMFAInvalidCode no login 1, obtido: %v", err)
	}

	uCheck1, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao consultar usuário pós-sessão 1: %v", err)
	}
	if uCheck1.User.MFAFailedAttempts != 1 {
		t.Errorf("após falha de MFA 1, esperava MFAFailedAttempts=1, obtido %d", uCheck1.User.MFAFailedAttempts)
	}
	if uCheck1.User.FailedLoginAttempts != 0 {
		t.Errorf("FailedLoginAttempts deveria permanecer 0, obtido %d", uCheck1.User.FailedLoginAttempts)
	}

	// Sessão 2: NOVO login com senha válida (Tentativa de burlar o contador de MFA)
	// O login por senha NÃO DEVE zerar mfa_failed_attempts!
	_, sess2, needsMFA2, err := svc.AuthenticatePassword(ctx, "user.multisession.mfa", "SenhaCorreta123!", "127.0.0.1", "Browser2")
	if err != nil || !needsMFA2 {
		t.Fatalf("login 2 com senha correta falhou: err=%v needsMFA=%v", err, needsMFA2)
	}

	uCheck2Before, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao consultar usuário antes do TOTP da sessão 2: %v", err)
	}
	if uCheck2Before.User.MFAFailedAttempts != 1 {
		t.Fatalf("VULNERABILIDADE: AuthenticatePassword zerou MFAFailedAttempts! Obtido %d, esperado 1", uCheck2Before.User.MFAFailedAttempts)
	}

	_, _, err = svc.VerifyMFALogin(ctx, sess2.ID, "222222", "127.0.0.1")
	if !errors.Is(err, auth.ErrMFAInvalidCode) {
		t.Fatalf("esperava ErrMFAInvalidCode no login 2, obtido: %v", err)
	}

	uCheck2After, _ := store.GetUserByID(ctx, db, u.ID)
	if uCheck2After.User.MFAFailedAttempts != 2 {
		t.Errorf("após falha de MFA 2, esperava MFAFailedAttempts=2, obtido %d", uCheck2After.User.MFAFailedAttempts)
	}

	// Sessão 3: NOVO login com senha válida -> Erra TOTP pela 3a vez -> Bloqueio atômico de conta!
	_, sess3, needsMFA3, err := svc.AuthenticatePassword(ctx, "user.multisession.mfa", "SenhaCorreta123!", "127.0.0.1", "Browser3")
	if err != nil || !needsMFA3 {
		t.Fatalf("login 3 com senha correta falhou: err=%v needsMFA=%v", err, needsMFA3)
	}

	_, _, err = svc.VerifyMFALogin(ctx, sess3.ID, "333333", "127.0.0.1")
	if !errors.Is(err, auth.ErrAccountLocked) {
		t.Fatalf("3a tentativa consecutiva de MFA entre sessões DEVE bloquear a conta (ErrAccountLocked), obtido: %v", err)
	}

	// Verifica se a conta está travada no banco
	uCheckLocked, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao consultar usuário bloqueado: %v", err)
	}
	if !uCheckLocked.User.IsLocked() || uCheckLocked.User.Status != domain.UserStatusLocked {
		t.Errorf("usuário deveria estar bloqueado após 3 falhas de MFA através de sessões distintas")
	}

	// Sessão 4: Tentativa de login por senha quando a conta está bloqueada deve ser rejeitada imediatamente
	_, _, _, err = svc.AuthenticatePassword(ctx, "user.multisession.mfa", "SenhaCorreta123!", "127.0.0.1", "Browser4")
	if !errors.Is(err, auth.ErrAccountLocked) {
		t.Errorf("tentativa de login com senha em conta bloqueada deveria retornar ErrAccountLocked, obtido %v", err)
	}
}
