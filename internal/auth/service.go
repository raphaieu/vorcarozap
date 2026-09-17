package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
)

var (
	ErrInvalidCredentials = errors.New("auth: credenciais inválidas")
	ErrAccountDisabled    = errors.New("auth: conta de usuário desativada")
	ErrAccountLocked      = errors.New("auth: conta temporariamente bloqueada por excesso de tentativas")
	ErrMFARequired        = errors.New("auth: segundo fator de autenticação (MFA) é obrigatório")
	ErrMFAInvalidCode     = errors.New("auth: código de segundo fator (MFA) inválido ou expirado")
	ErrSessionNotFound    = errors.New("auth: sessão inexistente ou expirada")
	ErrUnauthorized       = errors.New("auth: não autenticado")
	ErrForbidden          = errors.New("auth: acesso negado por permissão insuficiente")
	ErrConflict           = store.ErrConflict
)

const (
	DefaultSessionTTL      = 8 * time.Hour
	DefaultLockoutDuration = 15 * time.Minute
	DefaultMaxAttempts     = 5
	DefaultBcryptCost      = 12
)

// Service gerencia autenticação, sessões, MFA, RBAC e auditoria.
type Service struct {
	db              *sql.DB
	sessionTTL      time.Duration
	lockoutDuration time.Duration
	maxAttempts     int
	mfaRequired     bool
	encryptionKey   []byte
	dummyBcryptHash []byte
}

var (
	dummyBcryptOnce sync.Once
	dummyBcryptHash []byte
)

func getDummyBcryptHash() []byte {
	dummyBcryptOnce.Do(func() {
		h, err := bcrypt.GenerateFromPassword([]byte("vorcarozap-dummy-auth-constant-timing-seed"), DefaultBcryptCost)
		if err != nil {
			slog.Error("falha ao inicializar dummy bcrypt hash", "error", err)
		}
		dummyBcryptHash = h
	})
	return dummyBcryptHash
}

// NewService inicializa o serviço de autenticação com parâmetros de segurança e chave de cifra.
// Retorna erro se a chave de criptografia de MFA for inválida ou ausente (falha fechada).
func NewService(db *sql.DB, sessionTTL, lockoutDuration time.Duration, maxAttempts int, mfaRequired bool, mfaEncryptionKey string) (*Service, error) {
	if sessionTTL <= 0 {
		sessionTTL = DefaultSessionTTL
	}
	if lockoutDuration <= 0 {
		lockoutDuration = DefaultLockoutDuration
	}
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}

	key, err := DeriveKey(mfaEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("auth: chave de criptografia MFA inválida: %w", err)
	}

	return &Service{
		db:              db,
		sessionTTL:      sessionTTL,
		lockoutDuration: lockoutDuration,
		maxAttempts:     maxAttempts,
		mfaRequired:     mfaRequired,
		encryptionKey:   key,
		dummyBcryptHash: getDummyBcryptHash(),
	}, nil
}

// GenerateSessionToken gera um token de sessão criptográfico seguro de 32 bytes (64 caracteres hexadecimais).
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: falha ao gerar token de sessão aleatório: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// AuthenticatePassword autentica usuário por login e senha com mitigação contra brute force e timing attacks.
// Retorna (user, session, needsMFA, error).
func (s *Service) AuthenticatePassword(ctx context.Context, username, password, ip, userAgent string) (*domain.AdminUser, *domain.AdminSession, bool, error) {
	cleanUser, err := domain.ValidateUsername(username)
	if err != nil {
		// Executa computação de bcrypt para equalizar latência mesmo em username inválido
		_ = bcrypt.CompareHashAndPassword(s.dummyBcryptHash, []byte(password))
		return nil, nil, false, ErrInvalidCredentials
	}

	uWithCreds, err := store.GetUserByUsername(ctx, s.db, cleanUser)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(s.dummyBcryptHash, []byte(password))
			s.recordAudit(ctx, nil, cleanUser, "login_failed_unknown_user", nil, "anonymous", "", "Tentativa de login com usuário inexistente", ip)
			return nil, nil, false, ErrInvalidCredentials
		}
		return nil, nil, false, fmt.Errorf("auth: erro ao consultar usuário: %w", err)
	}

	u := &uWithCreds.User

	// 1. Valida se a conta está bloqueada por lockout temporário
	if u.Status == domain.UserStatusLocked {
		if u.LockedUntil != nil && *u.LockedUntil != "" {
			lockedTime, parseErr := time.Parse(time.RFC3339Nano, *u.LockedUntil)
			if parseErr == nil && time.Now().UTC().Before(lockedTime) {
				_ = bcrypt.CompareHashAndPassword(s.dummyBcryptHash, []byte(password))
				s.recordAudit(ctx, &u.ID, u.Username, "login_rejected_locked", &u.ID, u.Username, u.ID, fmt.Sprintf("Conta bloqueada até %s", *u.LockedUntil), ip)
				return nil, nil, false, ErrAccountLocked
			}
		}
		// Se o tempo de bloqueio expirou, reativa a conta
		if unlockErr := store.UnlockAdminUser(ctx, s.db, u.ID); unlockErr != nil {
			slog.Error("falha ao reativar conta com bloqueio expirado", "user_id", u.ID, "error", unlockErr)
		}
		u.Status = domain.UserStatusActive
	}

	// 2. Valida se a conta está desativada
	if u.Status == domain.UserStatusDisabled {
		_ = bcrypt.CompareHashAndPassword(s.dummyBcryptHash, []byte(password))
		s.recordAudit(ctx, &u.ID, u.Username, "login_rejected_disabled", &u.ID, u.Username, u.ID, "Tentativa de login em conta desativada", ip)
		return nil, nil, false, ErrAccountDisabled
	}

	// 3. Validação da senha com bcrypt
	passErr := bcrypt.CompareHashAndPassword([]byte(uWithCreds.PasswordHash), []byte(password))
	if passErr != nil {
		lockUntil := time.Now().UTC().Add(s.lockoutDuration)
		failedRes, recordErr := store.RecordPasswordFailedAttemptAndLock(ctx, s.db, u.ID, s.maxAttempts, lockUntil)
		if recordErr != nil {
			return nil, nil, false, fmt.Errorf("auth: falha ao registrar tentativa incorreta no banco: %w", recordErr)
		}

		if failedRes.IsLocked {
			if delErr := store.DeleteUserAdminSessions(ctx, s.db, u.ID); delErr != nil {
				slog.Error("falha ao revogar sessões no lockout de senha", "user_id", u.ID, "error", delErr)
			}
			s.recordAudit(ctx, &u.ID, u.Username, "lockout_triggered", &u.ID, u.Username, u.ID, fmt.Sprintf("Bloqueio atômico por excesso de tentativas de senha (%d)", failedRes.FailedLoginAttempts), ip)
			return nil, nil, false, ErrAccountLocked
		}

		s.recordAudit(ctx, &u.ID, u.Username, "login_failed_password", &u.ID, u.Username, u.ID, fmt.Sprintf("Senha incorreta (tentativa %d de %d)", failedRes.FailedLoginAttempts, s.maxAttempts), ip)
		return nil, nil, false, ErrInvalidCredentials
	}

	// 4. Sucesso na validação da senha (reseta APENAS falhas de senha; preserva mfa_failed_attempts)
	if err := store.RecordUserPasswordLoginSuccess(ctx, s.db, u.ID); err != nil {
		return nil, nil, false, fmt.Errorf("auth: falha ao atualizar sucesso de login por senha: %w", err)
	}

	needsMFA := u.MFAEnabled || (s.mfaRequired && u.Role.RequiresMFA())

	// Cria sessão
	sessionToken, err := GenerateSessionToken()
	if err != nil {
		return nil, nil, false, err
	}

	now := time.Now().UTC()
	session := domain.AdminSession{
		ID:             sessionToken,
		UserID:         u.ID,
		MFAVerified:    !needsMFA,
		IPAddress:      ip,
		UserAgent:      userAgent,
		ExpiresAt:      now.Add(s.sessionTTL).Format(time.RFC3339Nano),
		LastActivityAt: now.Format(time.RFC3339Nano),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}

	if err := store.CreateAdminSession(ctx, s.db, session); err != nil {
		return nil, nil, false, fmt.Errorf("auth: falha ao criar sessão: %w", err)
	}

	s.recordAudit(ctx, &u.ID, u.Username, "login_password_success", &u.ID, u.Username, u.ID, fmt.Sprintf("Senha validada com sucesso (needs_mfa=%v)", needsMFA), ip)
	return u, &session, needsMFA, nil
}

// VerifyMFALogin valida o código TOTP no fluxo de login e rotaciona a sessão para uma sessão verificada com proteção contra brute force.
func (s *Service) VerifyMFALogin(ctx context.Context, sessionID, code, ip string) (*domain.AdminUser, *domain.AdminSession, error) {
	swu, err := store.GetAdminSessionWithUser(ctx, s.db, sessionID)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}

	uWithCreds, err := store.GetUserByID(ctx, s.db, swu.User.ID)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}

	u := uWithCreds.User

	// Verifica se usuário está inativo ou bloqueado
	if u.Status == domain.UserStatusDisabled {
		if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
			slog.Error("falha ao revogar sessão de usuário desativado", "session_id", sessionID, "error", delErr)
		}
		return nil, nil, ErrAccountDisabled
	}

	if u.IsLocked() {
		if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
			return nil, nil, fmt.Errorf("auth: falha ao revogar sessão de conta bloqueada: %w", delErr)
		}
		return nil, nil, ErrAccountLocked
	}

	if !u.MFAEnabled || uWithCreds.MFASecretEncrypted == "" {
		return nil, nil, errors.New("auth: MFA não está configurado para esta conta")
	}

	rawSecret, err := DecryptString(uWithCreds.MFASecretEncrypted, s.encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: falha na decriptação do segredo MFA: %w", err)
	}

	valid, _, err := ValidateCode(rawSecret, code, time.Now().UTC(), TOTPDefaultWindow)
	if err != nil || !valid {
		lockUntil := time.Now().UTC().Add(s.lockoutDuration)
		failedRes, recordErr := store.RecordMFAFailedAttemptAndLock(ctx, s.db, u.ID, s.maxAttempts, lockUntil)
		if recordErr != nil {
			return nil, nil, fmt.Errorf("auth: falha ao registrar tentativa incorreta no banco: %w", recordErr)
		}

		if failedRes.IsLocked {
			if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
				return nil, nil, fmt.Errorf("auth: falha ao revogar sessão durante lockout: %w", delErr)
			}
			s.recordAudit(ctx, &u.ID, u.Username, "mfa_lockout_triggered", &u.ID, u.Username, u.ID, fmt.Sprintf("Bloqueio atômico de conta por excesso de falhas no desafio MFA (%d tentativas)", failedRes.MFAFailedAttempts), ip)
			return nil, nil, ErrAccountLocked
		}

		s.recordAudit(ctx, &u.ID, u.Username, "mfa_verification_failed", &u.ID, u.Username, u.ID, fmt.Sprintf("Código TOTP incorreto (tentativa %d de %d)", failedRes.MFAFailedAttempts, s.maxAttempts), ip)
		return nil, nil, ErrMFAInvalidCode
	}

	// Sucesso total no MFA: reseta contadores de senha E de MFA
	if err := store.RecordUserMFASuccess(ctx, s.db, u.ID); err != nil {
		return nil, nil, fmt.Errorf("auth: falha ao registrar sucesso de autenticação MFA: %w", err)
	}

	// Rotaciona a sessão para prevenir Session Fixation
	newSessionToken, err := GenerateSessionToken()
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	newSession := domain.AdminSession{
		ID:             newSessionToken,
		UserID:         swu.User.ID,
		MFAVerified:    true,
		IPAddress:      ip,
		UserAgent:      swu.Session.UserAgent,
		ExpiresAt:      now.Add(s.sessionTTL).Format(time.RFC3339Nano),
		LastActivityAt: now.Format(time.RFC3339Nano),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}

	if err := store.CreateAdminSession(ctx, s.db, newSession); err != nil {
		return nil, nil, fmt.Errorf("auth: falha ao criar sessão rotacionada pós-MFA: %w", err)
	}

	// Remove sessão anterior não verificada
	if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
		return nil, nil, fmt.Errorf("auth: falha ao revogar sessão antiga pós-rotação: %w", delErr)
	}

	s.recordAudit(ctx, &u.ID, u.Username, "mfa_verification_success", &u.ID, u.Username, u.ID, "Segundo fator TOTP validado com sucesso", ip)
	return &u, &newSession, nil
}

// GenerateMFASetup gera segredo efêmero, armazena estado pendente cifrado server-side e retorna segredo em texto plano apenas para renderização única de setup.
// Se o usuário já possuir MFA ativado ou estiver bloqueado, recusa a operação.
func (s *Service) GenerateMFASetup(ctx context.Context, userID string) (secret string, uri string, err error) {
	uWithCreds, err := store.GetUserByID(ctx, s.db, userID)
	if err != nil {
		return "", "", ErrSessionNotFound
	}

	if uWithCreds.User.Status == domain.UserStatusDisabled {
		return "", "", ErrAccountDisabled
	}

	if uWithCreds.User.IsLocked() {
		return "", "", ErrAccountLocked
	}

	if uWithCreds.User.MFAEnabled {
		return "", "", errors.New("auth: MFA já está ativado para este usuário")
	}

	secret, err = GenerateSecret()
	if err != nil {
		return "", "", err
	}

	encryptedSecret, err := EncryptString(secret, s.encryptionKey)
	if err != nil {
		return "", "", fmt.Errorf("auth: falha ao criptografar segredo de setup: %w", err)
	}

	// Armazena segredo pendente cifrado com validade de 15 minutos
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	if err := store.SetAdminUserPendingMFA(ctx, s.db, userID, encryptedSecret, expiresAt); err != nil {
		return "", "", fmt.Errorf("auth: falha ao salvar segredo pendente de setup: %w", err)
	}

	uri = GenerateOTPAuthURI("VorcaroZAP", uWithCreds.User.Username, secret)
	return secret, uri, nil
}

// ConfirmMFASetup confirma a ativação de MFA validando o primeiro código TOTP contra o segredo pendente server-side com proteção contra brute force.
func (s *Service) ConfirmMFASetup(ctx context.Context, sessionID, code, ip string) (*domain.AdminUser, *domain.AdminSession, error) {
	swu, err := store.GetAdminSessionWithUser(ctx, s.db, sessionID)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}

	uWithCreds, err := store.GetUserByID(ctx, s.db, swu.User.ID)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}

	u := uWithCreds.User

	// Verifica se usuário está inativo ou bloqueado
	if u.Status == domain.UserStatusDisabled {
		if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
			slog.Error("falha ao revogar sessão de usuário desativado", "session_id", sessionID, "error", delErr)
		}
		if clearErr := store.ClearAdminUserPendingMFA(ctx, s.db, u.ID); clearErr != nil {
			slog.Error("falha ao limpar setup MFA de usuário desativado", "user_id", u.ID, "error", clearErr)
		}
		return nil, nil, ErrAccountDisabled
	}

	if u.IsLocked() {
		if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
			return nil, nil, fmt.Errorf("auth: falha ao revogar sessão de conta bloqueada: %w", delErr)
		}
		if clearErr := store.ClearAdminUserPendingMFA(ctx, s.db, u.ID); clearErr != nil {
			return nil, nil, fmt.Errorf("auth: falha ao limpar setup pendente de conta bloqueada: %w", clearErr)
		}
		return nil, nil, ErrAccountLocked
	}

	if u.MFAEnabled {
		return nil, nil, errors.New("auth: MFA já está ativado para este usuário")
	}

	if uWithCreds.MFAPendingSecretEncrypted == "" {
		return nil, nil, errors.New("auth: nenhum setup de MFA pendente encontrado para este usuário")
	}

	// Valida expiração do setup pendente
	if uWithCreds.MFAPendingExpiresAt != nil && *uWithCreds.MFAPendingExpiresAt != "" {
		expTime, err := time.Parse(time.RFC3339Nano, *uWithCreds.MFAPendingExpiresAt)
		if err == nil && time.Now().UTC().After(expTime) {
			if clearErr := store.ClearAdminUserPendingMFA(ctx, s.db, u.ID); clearErr != nil {
				slog.Error("falha ao limpar setup MFA expirado", "user_id", u.ID, "error", clearErr)
			}
			return nil, nil, errors.New("auth: o tempo limite de configuração do MFA expirou; reinicie a configuração")
		}
	}

	rawSecret, err := DecryptString(uWithCreds.MFAPendingSecretEncrypted, s.encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: falha na decriptação do segredo pendente: %w", err)
	}

	valid, _, err := ValidateCode(rawSecret, code, time.Now().UTC(), TOTPDefaultWindow)
	if err != nil || !valid {
		lockUntil := time.Now().UTC().Add(s.lockoutDuration)
		failedRes, recordErr := store.RecordMFAFailedAttemptAndLock(ctx, s.db, u.ID, s.maxAttempts, lockUntil)
		if recordErr != nil {
			return nil, nil, fmt.Errorf("auth: falha ao registrar tentativa de setup incorreta no banco: %w", recordErr)
		}

		if failedRes.IsLocked {
			if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
				return nil, nil, fmt.Errorf("auth: falha ao revogar sessão durante lockout de setup: %w", delErr)
			}
			if clearErr := store.ClearAdminUserPendingMFA(ctx, s.db, u.ID); clearErr != nil {
				return nil, nil, fmt.Errorf("auth: falha ao limpar segredo pendente durante lockout de setup: %w", clearErr)
			}
			s.recordAudit(ctx, &u.ID, u.Username, "mfa_setup_lockout_triggered", &u.ID, u.Username, u.ID, fmt.Sprintf("Bloqueio atômico de conta por excesso de falhas no setup de MFA (%d tentativas)", failedRes.MFAFailedAttempts), ip)
			return nil, nil, ErrAccountLocked
		}

		s.recordAudit(ctx, &u.ID, u.Username, "mfa_setup_failed", &u.ID, u.Username, u.ID, fmt.Sprintf("Código de confirmação de setup MFA incorreto (tentativa %d de %d)", failedRes.MFAFailedAttempts, s.maxAttempts), ip)
		return nil, nil, ErrMFAInvalidCode
	}

	// Sucesso total no setup MFA: reseta contadores de senha E de MFA
	if err := store.RecordUserMFASuccess(ctx, s.db, u.ID); err != nil {
		return nil, nil, fmt.Errorf("auth: falha ao registrar sucesso de setup MFA: %w", err)
	}

	// Ativa MFA promovendo o segredo pendente para ativo
	if err := store.ConfirmAdminUserMFA(ctx, s.db, u.ID); err != nil {
		return nil, nil, fmt.Errorf("auth: falha ao ativar MFA no banco de dados: %w", err)
	}

	// Rotaciona a sessão
	newSessionToken, err := GenerateSessionToken()
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	newSession := domain.AdminSession{
		ID:             newSessionToken,
		UserID:         u.ID,
		MFAVerified:    true,
		IPAddress:      ip,
		UserAgent:      swu.Session.UserAgent,
		ExpiresAt:      now.Add(s.sessionTTL).Format(time.RFC3339Nano),
		LastActivityAt: now.Format(time.RFC3339Nano),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}

	if err := store.CreateAdminSession(ctx, s.db, newSession); err != nil {
		return nil, nil, fmt.Errorf("auth: falha ao persistir sessão ativada: %w", err)
	}

	// Remove sessão anterior
	if delErr := store.DeleteAdminSession(ctx, s.db, sessionID); delErr != nil {
		slog.Error("falha ao excluir sessão antiga durante confirmação MFA", "session_id", sessionID, "error", delErr)
	}

	s.recordAudit(ctx, &u.ID, u.Username, "mfa_setup_confirmed", &u.ID, u.Username, u.ID, "MFA ativado com sucesso após validação de primeiro código", ip)
	return &u, &newSession, nil
}

// DisableMFA desativa o MFA de um usuário (apenas por admin ou próprio usuário se permitido).
func (s *Service) DisableMFA(ctx context.Context, actor domain.AdminUser, targetUserID, ip string) error {
	if !actor.Role.HasPermission(domain.PermManageUsers) && actor.ID != targetUserID {
		return ErrForbidden
	}

	target, err := store.GetUserByID(ctx, s.db, targetUserID)
	if err != nil {
		return err
	}

	if err := store.DisableAdminUserMFA(ctx, s.db, targetUserID); err != nil {
		return fmt.Errorf("auth: falha ao desativar MFA: %w", err)
	}

	// Revoga sessões do usuário alvo para forçar reautenticação
	if delErr := store.DeleteUserAdminSessions(ctx, s.db, targetUserID); delErr != nil {
		slog.Error("falha ao revogar sessões no disable MFA", "user_id", targetUserID, "error", delErr)
	}

	s.recordAudit(ctx, &target.User.ID, target.User.Username, "mfa_disabled", &actor.ID, actor.Username, target.User.ID, "MFA desativado por administrador", ip)
	return nil
}

// ValidateSession valida se uma sessão existe, não está expirada e pertence a um usuário ativo.
func (s *Service) ValidateSession(ctx context.Context, sessionID string) (*domain.AdminUser, *domain.AdminSession, error) {
	cleanID := strings.TrimSpace(sessionID)
	if cleanID == "" {
		return nil, nil, ErrSessionNotFound
	}

	swu, err := store.GetAdminSessionWithUser(ctx, s.db, cleanID)
	if err != nil {
		return nil, nil, ErrSessionNotFound
	}

	// 1. Verifica expiração da sessão
	expiresAt, err := time.Parse(time.RFC3339Nano, swu.Session.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		if delErr := store.DeleteAdminSession(ctx, s.db, cleanID); delErr != nil {
			slog.Error("falha ao excluir sessão expirada", "session_id", cleanID, "error", delErr)
		}
		return nil, nil, ErrSessionNotFound
	}

	// 2. Verifica estado do usuário
	if !swu.User.Status.CanAuthenticate() {
		if delErr := store.DeleteAdminSession(ctx, s.db, cleanID); delErr != nil {
			slog.Error("falha ao excluir sessão de usuário inativo", "session_id", cleanID, "error", delErr)
		}
		return nil, nil, ErrAccountDisabled
	}

	// 3. Atualiza heartbeat de atividade da sessão
	_ = store.UpdateAdminSessionActivity(ctx, s.db, cleanID)

	return &swu.User, &swu.Session, nil
}

// RevokeSession encerra a sessão ativa (Logout) e limpa eventuais configurações de MFA pendentes.
func (s *Service) RevokeSession(ctx context.Context, sessionID, ip string) error {
	swu, err := store.GetAdminSessionWithUser(ctx, s.db, sessionID)
	if err == nil {
		if clearErr := store.ClearAdminUserPendingMFA(ctx, s.db, swu.User.ID); clearErr != nil {
			slog.Error("falha ao limpar setup MFA pendente no logout", "user_id", swu.User.ID, "error", clearErr)
		}
		s.recordAudit(ctx, &swu.User.ID, swu.User.Username, "logout", &swu.User.ID, swu.User.Username, swu.User.ID, "Sessão encerrada voluntariamente", ip)
	}
	return store.DeleteAdminSession(ctx, s.db, sessionID)
}

// RevokeAllUserSessions encerra todas as sessões de um usuário.
func (s *Service) RevokeAllUserSessions(ctx context.Context, userID string) error {
	return store.DeleteUserAdminSessions(ctx, s.db, userID)
}

// CreateUserParams parâmetros para criação de usuário.
type CreateUserParams struct {
	Username    string
	DisplayName string
	Password    string
	Role        domain.UserRole
	Status      domain.UserStatus
}

// CreateUser cria uma nova conta administrativa (restrito a admin).
func (s *Service) CreateUser(ctx context.Context, actor domain.AdminUser, p CreateUserParams, ip string) (*domain.AdminUser, error) {
	if !actor.Role.HasPermission(domain.PermManageUsers) {
		return nil, ErrForbidden
	}

	if err := domain.ValidatePasswordStrength(p.Password); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(p.Password), DefaultBcryptCost)
	if err != nil {
		return nil, fmt.Errorf("auth: falha ao gerar hash da senha: %w", err)
	}

	user, err := store.CreateAdminUser(ctx, s.db, store.CreateUserParams{
		Username:     p.Username,
		DisplayName:  p.DisplayName,
		PasswordHash: string(hash),
		Role:         p.Role,
		Status:       p.Status,
	})
	if err != nil {
		return nil, err
	}

	s.recordAudit(ctx, &user.ID, user.Username, "user_created", &actor.ID, actor.Username, user.ID, fmt.Sprintf("Conta criada com papel '%s'", user.Role), ip)
	return user, nil
}

// UpdateUserRole atualiza o papel de um usuário com OCC.
func (s *Service) UpdateUserRole(ctx context.Context, actor domain.AdminUser, targetUserID, expectedUpdatedAt string, newRole domain.UserRole, ip string) error {
	if !actor.Role.HasPermission(domain.PermManageUsers) {
		return ErrForbidden
	}

	err := store.UpdateAdminUserRole(ctx, s.db, targetUserID, expectedUpdatedAt, newRole)
	if err != nil {
		return err
	}

	// Ao alterar papel, revoga sessões antigas para forçar reautenticação e atualização de permissões
	if delErr := store.DeleteUserAdminSessions(ctx, s.db, targetUserID); delErr != nil {
		slog.Error("falha ao revogar sessões após alteração de papel", "user_id", targetUserID, "error", delErr)
	}

	s.recordAudit(ctx, nil, targetUserID, "user_role_updated", &actor.ID, actor.Username, targetUserID, fmt.Sprintf("Papel alterado para '%s'", newRole), ip)
	return nil
}

// UpdateUserStatus atualiza o status de um usuário (ativação/desativação/bloqueio) com OCC.
func (s *Service) UpdateUserStatus(ctx context.Context, actor domain.AdminUser, targetUserID, expectedUpdatedAt string, newStatus domain.UserStatus, ip string) error {
	if !actor.Role.HasPermission(domain.PermManageUsers) {
		return ErrForbidden
	}

	// Impede que o próprio admin se desative
	if actor.ID == targetUserID && newStatus != domain.UserStatusActive {
		return fmt.Errorf("auth: não é permitido desativar a própria conta conectada")
	}

	err := store.UpdateAdminUserStatus(ctx, s.db, targetUserID, expectedUpdatedAt, newStatus)
	if err != nil {
		return err
	}

	if newStatus != domain.UserStatusActive {
		if delErr := store.DeleteUserAdminSessions(ctx, s.db, targetUserID); delErr != nil {
			slog.Error("falha ao revogar sessões após alteração de status", "user_id", targetUserID, "error", delErr)
		}
	}

	s.recordAudit(ctx, nil, targetUserID, "user_status_updated", &actor.ID, actor.Username, targetUserID, fmt.Sprintf("Status alterado para '%s'", newStatus), ip)
	return nil
}

// ResetUserPassword redefine a senha de um usuário por ação administrativa.
func (s *Service) ResetUserPassword(ctx context.Context, actor domain.AdminUser, targetUserID, newPassword, ip string) error {
	if !actor.Role.HasPermission(domain.PermManageUsers) {
		return ErrForbidden
	}

	if err := domain.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), DefaultBcryptCost)
	if err != nil {
		return fmt.Errorf("auth: falha ao gerar hash da nova senha: %w", err)
	}

	if err := store.UpdateAdminUserPassword(ctx, s.db, targetUserID, string(hash)); err != nil {
		return err
	}

	if delErr := store.DeleteUserAdminSessions(ctx, s.db, targetUserID); delErr != nil {
		slog.Error("falha ao revogar sessões após reset de senha", "user_id", targetUserID, "error", delErr)
	}

	s.recordAudit(ctx, nil, targetUserID, "password_reset_by_admin", &actor.ID, actor.Username, targetUserID, "Senha redefinida por administrador", ip)
	return nil
}

// ChangeOwnPassword permite ao usuário conectado alterar a própria senha com confirmação da senha anterior.
func (s *Service) ChangeOwnPassword(ctx context.Context, userID, currentPassword, newPassword, ip string) error {
	uWithCreds, err := store.GetUserByID(ctx, s.db, userID)
	if err != nil {
		return ErrSessionNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(uWithCreds.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	if err := domain.ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), DefaultBcryptCost)
	if err != nil {
		return fmt.Errorf("auth: falha ao gerar hash da nova senha: %w", err)
	}

	if err := store.UpdateAdminUserPassword(ctx, s.db, userID, string(hash)); err != nil {
		return err
	}

	s.recordAudit(ctx, &uWithCreds.User.ID, uWithCreds.User.Username, "password_changed_by_user", &uWithCreds.User.ID, uWithCreds.User.Username, uWithCreds.User.ID, "Senha alterada pelo próprio operador", ip)
	return nil
}

func (s *Service) recordAudit(ctx context.Context, userID *string, username, action string, actorID *string, actorUsername, targetID, details, ip string) {
	log := domain.AdminAuditLog{
		ID:            uuid.New().String(),
		UserID:        userID,
		Username:      username,
		Action:        action,
		ActorID:       actorID,
		ActorUsername: actorUsername,
		TargetID:      targetID,
		Details:       details,
		IPAddress:     ip,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := store.InsertAdminAuditLog(ctx, s.db, log); err != nil {
		slog.Error("falha ao gravar log de auditoria administrativa", "action", action, "error", err)
	}
}
