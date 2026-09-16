package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
)

func TestStore_UsersAndSessions_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "users_test.db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// 1. Bootstrap Admin User
	bootstrapped, err := store.BootstrapAdminUser(ctx, db, "admin.master", "$2a$12$dummyhashforadminuser1234567890", "Admin Master")
	if err != nil || !bootstrapped {
		t.Fatalf("falha no bootstrap inicial do usuário admin: %v", err)
	}

	// Segundo bootstrap não deve fazer nada
	b2, err := store.BootstrapAdminUser(ctx, db, "admin.master2", "hash2", "Admin 2")
	if err != nil || b2 {
		t.Fatalf("segundo bootstrap deveria retornar false, obtido: %v, err: %v", b2, err)
	}

	// 2. Consulta de usuário por Username
	u1, err := store.GetUserByUsername(ctx, db, "admin.master")
	if err != nil {
		t.Fatalf("falha ao consultar usuário admin.master: %v", err)
	}
	if u1.User.Role != domain.RoleAdmin || u1.User.Status != domain.UserStatusActive {
		t.Errorf("dados de bootstrap incorretos: role=%s, status=%s", u1.User.Role, u1.User.Status)
	}

	// 3. Criação de novo usuário editor
	editorUser, err := store.CreateAdminUser(ctx, db, store.CreateUserParams{
		Username:     "editor.joao",
		DisplayName:  "João Editor",
		PasswordHash: "$2a$12$editorhash1234567890abcdef",
		Role:         domain.RoleEditor,
		Status:       domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("falha ao criar usuário editor: %v", err)
	}

	// 4. Listagem de usuários
	listRes, err := store.ListAdminUsers(ctx, db, store.AdminUserFilter{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("falha ao listar usuários: %v", err)
	}
	if listRes.TotalCount != 2 {
		t.Errorf("esperava 2 usuários, obtido: %d", listRes.TotalCount)
	}

	// 5. Atualização de papel com OCC
	err = store.UpdateAdminUserRole(ctx, db, editorUser.ID, editorUser.UpdatedAt, domain.RoleReviewer)
	if err != nil {
		t.Fatalf("falha ao atualizar papel com OCC: %v", err)
	}

	// Tentativa com expectedUpdatedAt antigo deve retornar ErrConflict
	errConflict := store.UpdateAdminUserRole(ctx, db, editorUser.ID, editorUser.UpdatedAt, domain.RoleAuditor)
	if errConflict != store.ErrConflict {
		t.Errorf("esperava ErrConflict ao passar updated_at antigo, obtido: %v", errConflict)
	}

	// 6. Atualização de status com OCC
	uEditorUpdated, err := store.GetUserByID(ctx, db, editorUser.ID)
	if err != nil {
		t.Fatalf("falha ao consultar editor atualizado: %v", err)
	}
	if uEditorUpdated.User.Role != domain.RoleReviewer {
		t.Errorf("papel atualizado esperado 'reviewer', obtido: %q", uEditorUpdated.User.Role)
	}

	err = store.UpdateAdminUserStatus(ctx, db, editorUser.ID, uEditorUpdated.User.UpdatedAt, domain.UserStatusDisabled)
	if err != nil {
		t.Fatalf("falha ao desativar usuário com OCC: %v", err)
	}

	// 7. Teste de ciclo de sessão
	sessionID := "test-session-token-123456"
	expiresAt := time.Now().UTC().Add(8 * time.Hour).Format(time.RFC3339Nano)
	err = store.CreateAdminSession(ctx, db, domain.AdminSession{
		ID:             sessionID,
		UserID:         u1.User.ID,
		MFAVerified:    false,
		IPAddress:      "192.168.1.100",
		UserAgent:      "Mozilla/5.0 Test",
		ExpiresAt:      expiresAt,
		LastActivityAt: time.Now().UTC().Format(time.RFC3339Nano),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("falha ao criar sessão: %v", err)
	}

	// Recupera sessão
	swu, err := store.GetAdminSessionWithUser(ctx, db, sessionID)
	if err != nil {
		t.Fatalf("falha ao recuperar sessão com usuário: %v", err)
	}
	if swu.Session.MFAVerified {
		t.Errorf("sessão nova não deveria estar verificada por MFA")
	}
	if swu.User.Username != "admin.master" {
		t.Errorf("usuário da sessão incorreto: %s", swu.User.Username)
	}

	// Eleva MFA
	err = store.ElevateAdminSessionMFA(ctx, db, sessionID)
	if err != nil {
		t.Fatalf("falha ao elevar MFA da sessão: %v", err)
	}
	swuElevated, _ := store.GetAdminSessionWithUser(ctx, db, sessionID)
	if !swuElevated.Session.MFAVerified {
		t.Errorf("sessão deveria estar com MFAVerified=true após elevação")
	}

	// Exclusão de sessão
	err = store.DeleteAdminSession(ctx, db, sessionID)
	if err != nil {
		t.Fatalf("falha ao deletar sessão: %v", err)
	}
	_, errNotFound := store.GetAdminSessionWithUser(ctx, db, sessionID)
	if errNotFound != store.ErrNotFound {
		t.Errorf("sessão deletada deveria retornar ErrNotFound, obtido: %v", errNotFound)
	}

	// 8. Teste de trilha de auditoria
	err = store.InsertAdminAuditLog(ctx, db, domain.AdminAuditLog{
		UserID:        &u1.User.ID,
		Username:      u1.User.Username,
		Action:        "login_success",
		ActorID:       &u1.User.ID,
		ActorUsername: u1.User.Username,
		TargetID:      u1.User.ID,
		Details:       "Login bem-sucedido via senha e MFA",
		IPAddress:     "192.168.1.100",
	})
	if err != nil {
		t.Fatalf("falha ao inserir log de auditoria: %v", err)
	}

	auditLogs, err := store.ListAdminAuditLogs(ctx, db, store.AdminAuditFilter{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("falha ao listar logs de auditoria: %v", err)
	}
	if auditLogs.TotalCount != 1 {
		t.Errorf("esperava 1 log de auditoria, obtido: %d", auditLogs.TotalCount)
	}
	if auditLogs.Logs[0].Action != "login_success" {
		t.Errorf("ação do log incorreta: %s", auditLogs.Logs[0].Action)
	}

	// 9. Teste do ciclo de MFA no Store
	err = store.SetAdminUserPendingMFA(ctx, db, u1.User.ID, "pending_secret_token", time.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatalf("falha ao salvar MFA pendente: %v", err)
	}

	uWithCreds, err := store.GetUserByID(ctx, db, u1.User.ID)
	if err != nil {
		t.Fatalf("falha ao buscar credenciais: %v", err)
	}
	if uWithCreds.MFAPendingSecretEncrypted != "pending_secret_token" {
		t.Errorf("pending secret encrypted incorreto: %s", uWithCreds.MFAPendingSecretEncrypted)
	}

	err = store.ConfirmAdminUserMFA(ctx, db, u1.User.ID)
	if err != nil {
		t.Fatalf("falha ao confirmar MFA no store: %v", err)
	}

	uWithCredsConfirmed, err := store.GetUserByID(ctx, db, u1.User.ID)
	if err != nil {
		t.Fatalf("falha ao buscar credenciais confirmadas: %v", err)
	}
	if uWithCredsConfirmed.MFASecretEncrypted != "pending_secret_token" {
		t.Errorf("mfa secret encrypted ativo incorreto: %s", uWithCredsConfirmed.MFASecretEncrypted)
	}
	if uWithCredsConfirmed.MFAPendingSecretEncrypted != "" {
		t.Errorf("mfa pending secret deveria estar vazio após confirmação")
	}
	if !uWithCredsConfirmed.User.MFAEnabled {
		t.Errorf("MFAEnabled deveria ser true após confirmação")
	}

	err = store.DisableAdminUserMFA(ctx, db, u1.User.ID)
	if err != nil {
		t.Fatalf("falha ao desativar MFA no store: %v", err)
	}

	uWithCredsDisabled, err := store.GetUserByID(ctx, db, u1.User.ID)
	if err != nil {
		t.Fatalf("falha ao buscar credenciais desativadas: %v", err)
	}
	if uWithCredsDisabled.MFASecretEncrypted != "" || uWithCredsDisabled.User.MFAEnabled {
		t.Errorf("MFA deveria estar desativado e sem segredo")
	}
}

func TestStore_RecordUserFailedAttemptAndLock(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "lockout_store_test.db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha na migração: %v", err)
	}

	u, err := store.CreateAdminUser(ctx, db, store.CreateUserParams{
		Username:     "test.atomic.lock",
		DisplayName:  "Test Atomic Lock",
		PasswordHash: "dummyhash",
		Role:         domain.RoleEditor,
		Status:       domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	lockUntil := time.Now().UTC().Add(15 * time.Minute)

	// Tentativa 1 de 3: não bloqueia
	res1, err := store.RecordPasswordFailedAttemptAndLock(ctx, db, u.ID, 3, lockUntil)
	if err != nil {
		t.Fatalf("falha tentativa 1: %v", err)
	}
	if res1.FailedLoginAttempts != 1 || res1.IsLocked || res1.Status != domain.UserStatusActive {
		t.Errorf("res1 incorreto: attempts=%d isLocked=%v status=%s", res1.FailedLoginAttempts, res1.IsLocked, res1.Status)
	}

	// Tentativa 2 de 3: não bloqueia
	res2, err := store.RecordPasswordFailedAttemptAndLock(ctx, db, u.ID, 3, lockUntil)
	if err != nil {
		t.Fatalf("falha tentativa 2: %v", err)
	}
	if res2.FailedLoginAttempts != 2 || res2.IsLocked || res2.Status != domain.UserStatusActive {
		t.Errorf("res2 incorreto: attempts=%d isLocked=%v status=%s", res2.FailedLoginAttempts, res2.IsLocked, res2.Status)
	}

	// Tentativa 3 de 3: bloqueia atomicamente!
	res3, err := store.RecordPasswordFailedAttemptAndLock(ctx, db, u.ID, 3, lockUntil)
	if err != nil {
		t.Fatalf("falha tentativa 3: %v", err)
	}
	if res3.FailedLoginAttempts != 3 || !res3.IsLocked || res3.Status != domain.UserStatusLocked {
		t.Errorf("res3 incorreto: attempts=%d isLocked=%v status=%s", res3.FailedLoginAttempts, res3.IsLocked, res3.Status)
	}
	if res3.LockedUntil == nil || *res3.LockedUntil == "" {
		t.Errorf("res3 locked_until deveria estar preenchido")
	}

	// Consulta direta no banco para confirmar persistência
	uCheck, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao consultar usuário pós-lock: %v", err)
	}
	if uCheck.User.Status != domain.UserStatusLocked || uCheck.User.FailedLoginAttempts != 3 || !uCheck.User.IsLocked() {
		t.Errorf("persistência no banco inválida: status=%s attempts=%d isLocked=%v", uCheck.User.Status, uCheck.User.FailedLoginAttempts, uCheck.User.IsLocked())
	}
}

func TestStore_RecordMFAFailedAttemptAndLock_And_Resets(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "mfa_lockout_store_test.db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha na migração: %v", err)
	}

	u, err := store.CreateAdminUser(ctx, db, store.CreateUserParams{
		Username:     "test.mfa.atomic.lock",
		DisplayName:  "Test MFA Atomic Lock",
		PasswordHash: "dummyhash",
		Role:         domain.RoleEditor,
		Status:       domain.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("falha ao criar usuário: %v", err)
	}

	lockUntil := time.Now().UTC().Add(15 * time.Minute)

	// 1. Falha de MFA incrementa mfa_failed_attempts mantendo failed_login_attempts em 0
	res1, err := store.RecordMFAFailedAttemptAndLock(ctx, db, u.ID, 3, lockUntil)
	if err != nil {
		t.Fatalf("falha tentativa 1 MFA: %v", err)
	}
	if res1.MFAFailedAttempts != 1 || res1.IsLocked || res1.Status != domain.UserStatusActive {
		t.Errorf("res1 MFA incorreto: mfaAttempts=%d isLocked=%v status=%s", res1.MFAFailedAttempts, res1.IsLocked, res1.Status)
	}

	uCheck1, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao buscar uCheck1: %v", err)
	}
	if uCheck1.User.FailedLoginAttempts != 0 || uCheck1.User.MFAFailedAttempts != 1 {
		t.Errorf("esperado FailedLoginAttempts=0 e MFAFailedAttempts=1, obtido %d e %d", uCheck1.User.FailedLoginAttempts, uCheck1.User.MFAFailedAttempts)
	}

	// 2. Simula login de senha com sucesso: NÃO deve zerar mfa_failed_attempts
	if err := store.RecordUserPasswordLoginSuccess(ctx, db, u.ID); err != nil {
		t.Fatalf("falha ao registrar sucesso de senha: %v", err)
	}
	uCheck2, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao buscar uCheck2: %v", err)
	}
	if uCheck2.User.MFAFailedAttempts != 1 {
		t.Errorf("RecordUserPasswordLoginSuccess NÃO deveria zerar MFAFailedAttempts, obtido %d", uCheck2.User.MFAFailedAttempts)
	}

	// 3. Segunda e terceira falhas de MFA -> Bloqueio atômico
	res2, err := store.RecordMFAFailedAttemptAndLock(ctx, db, u.ID, 3, lockUntil)
	if err != nil || res2.MFAFailedAttempts != 2 || res2.IsLocked {
		t.Fatalf("res2 MFA incorreto: %v", res2)
	}

	res3, err := store.RecordMFAFailedAttemptAndLock(ctx, db, u.ID, 3, lockUntil)
	if err != nil {
		t.Fatalf("res3 MFA falhou: %v", err)
	}
	if res3.MFAFailedAttempts != 3 || !res3.IsLocked || res3.Status != domain.UserStatusLocked {
		t.Fatalf("res3 MFA deveria ter bloqueado: attempts=%d isLocked=%v status=%s", res3.MFAFailedAttempts, res3.IsLocked, res3.Status)
	}

	// 4. Teste de RecordUserMFASuccess: destrava e zera ambos os contadores
	if err := store.UnlockAdminUser(ctx, db, u.ID); err != nil {
		t.Fatalf("falha ao desbloquear: %v", err)
	}
	if err := store.RecordUserMFASuccess(ctx, db, u.ID); err != nil {
		t.Fatalf("falha ao registrar sucesso MFA: %v", err)
	}

	uCheckFinal, err := store.GetUserByID(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("falha ao buscar uCheckFinal: %v", err)
	}
	if uCheckFinal.User.MFAFailedAttempts != 0 || uCheckFinal.User.FailedLoginAttempts != 0 {
		t.Errorf("após RecordUserMFASuccess ambos os contadores devem ser 0, obtido mfa=%d pass=%d", uCheckFinal.User.MFAFailedAttempts, uCheckFinal.User.FailedLoginAttempts)
	}
}
