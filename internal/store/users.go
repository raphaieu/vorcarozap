package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// ErrConflict indica que a atualização falhou por concorrência ou versão esperada desatualizada.
var ErrConflict = errors.New("store: conflito de concorrência ou versão desatualizada")

// AdminUserFilter define os filtros de consulta para listagem de contas de usuários.
type AdminUserFilter struct {
	Role     string
	Status   string
	Search   string
	Page     int
	PageSize int
}

// AdminUserListResult encapsula os resultados paginados da listagem de usuários.
type AdminUserListResult struct {
	Users      []domain.AdminUser
	Page       int
	PageSize   int
	TotalPages int
	TotalCount int64
}

// AdminAuditFilter define os filtros de consulta para logs de auditoria administrativa.
type AdminAuditFilter struct {
	Action      string
	Actor       string
	Period      string
	PeriodSince string
	Search      string
	Page        int
	PageSize    int
}

// AdminAuditListResult encapsula os resultados paginados de logs de auditoria.
type AdminAuditListResult struct {
	Logs       []domain.AdminAuditLog
	Page       int
	PageSize   int
	TotalPages int
	TotalCount int64
}

// AdminUserWithCredentials estende AdminUser contendo hash de senha e segredos MFA cifrados (apenas para autenticação interna).
type AdminUserWithCredentials struct {
	User                      domain.AdminUser
	PasswordHash              string
	MFASecretEncrypted        string
	MFAPendingSecretEncrypted string
	MFAPendingExpiresAt       *string
}

// SessionWithUser encapsula os dados de uma sessão ativa combinados com o usuário associado.
type SessionWithUser struct {
	Session domain.AdminSession
	User    domain.AdminUser
}

// SanitizeUserFilter sanitiza e aplica defaults seguros aos filtros de usuários.
func SanitizeUserFilter(f AdminUserFilter) AdminUserFilter {
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	role := strings.ToLower(strings.TrimSpace(f.Role))
	if role != "admin" && role != "editor" && role != "reviewer" && role != "auditor" {
		role = ""
	}

	status := strings.ToLower(strings.TrimSpace(f.Status))
	if status != "active" && status != "disabled" && status != "locked" {
		status = ""
	}

	return AdminUserFilter{
		Role:     role,
		Status:   status,
		Search:   strings.TrimSpace(f.Search),
		Page:     page,
		PageSize: pageSize,
	}
}

// SanitizeAuditFilter sanitiza e calcula períodos para filtros de auditoria.
func SanitizeAuditFilter(f AdminAuditFilter) AdminAuditFilter {
	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}

	now := time.Now().UTC()
	var periodSince string
	period := strings.TrimSpace(f.Period)
	switch period {
	case "24h":
		periodSince = now.Add(-24 * time.Hour).Format(time.RFC3339Nano)
	case "7d":
		periodSince = now.Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	case "30d":
		periodSince = now.Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)
	default:
		period = ""
		periodSince = ""
	}

	return AdminAuditFilter{
		Action:      strings.TrimSpace(f.Action),
		Actor:       strings.TrimSpace(f.Actor),
		Period:      period,
		PeriodSince: periodSince,
		Search:      strings.TrimSpace(f.Search),
		Page:        page,
		PageSize:    pageSize,
	}
}

// CountAdminUsers retorna a quantidade total de usuários administrativos cadastrados.
func CountAdminUsers(ctx context.Context, db *sql.DB) (int64, error) {
	q := sqlc.New(db)
	return q.CountAdminUsers(ctx)
}

// BootstrapAdminUser insere a conta administradora inicial caso a tabela admin_users esteja vazia.
func BootstrapAdminUser(ctx context.Context, db *sql.DB, username, passwordHash, displayName string) (bool, error) {
	q := sqlc.New(db)
	count, err := q.CountAdminUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("store: falha ao verificar contagem de usuários para bootstrap: %w", err)
	}

	if count > 0 {
		return false, nil
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	cleanUser, err := domain.ValidateUsername(username)
	if err != nil {
		return false, fmt.Errorf("store: username de bootstrap inválido: %w", err)
	}

	cleanDisplay := strings.TrimSpace(displayName)
	if cleanDisplay == "" {
		cleanDisplay = "Administrador do Sistema"
	}

	err = q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
		ID:                        id,
		Username:                  cleanUser,
		DisplayName:               cleanDisplay,
		PasswordHash:              passwordHash,
		Role:                      string(domain.RoleAdmin),
		Status:                    string(domain.UserStatusActive),
		FailedLoginAttempts:       0,
		MfaFailedAttempts:         0,
		LockedUntil:               sql.NullString{},
		MfaEnabled:                0,
		MfaSecretEncrypted:        "",
		MfaPendingSecretEncrypted: "",
		MfaPendingExpiresAt:       sql.NullString{},
		MfaEnrolledAt:             sql.NullString{},
		LastLoginAt:               sql.NullString{},
		CreatedAt:                 now,
		UpdatedAt:                 now,
	})
	if err != nil {
		return false, fmt.Errorf("store: falha ao inserir usuário de bootstrap: %w", err)
	}

	return true, nil
}

// GetUserByUsername recupera os dados de autenticação de um usuário a partir do username normalizado.
func GetUserByUsername(ctx context.Context, db *sql.DB, username string) (*AdminUserWithCredentials, error) {
	q := sqlc.New(db)
	cleanUser := strings.ToLower(strings.TrimSpace(username))

	u, err := q.GetAdminUserByUsername(ctx, cleanUser)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao consultar usuário por username %q: %w", username, err)
	}

	var lockedUntil *string
	if u.LockedUntil.Valid && u.LockedUntil.String != "" {
		lockedUntil = &u.LockedUntil.String
	}

	var mfaEnrolledAt *string
	if u.MfaEnrolledAt.Valid && u.MfaEnrolledAt.String != "" {
		mfaEnrolledAt = &u.MfaEnrolledAt.String
	}

	var lastLoginAt *string
	if u.LastLoginAt.Valid && u.LastLoginAt.String != "" {
		lastLoginAt = &u.LastLoginAt.String
	}

	var pendingExpiresAt *string
	if u.MfaPendingExpiresAt.Valid && u.MfaPendingExpiresAt.String != "" {
		pendingExpiresAt = &u.MfaPendingExpiresAt.String
	}

	return &AdminUserWithCredentials{
		User: domain.AdminUser{
			ID:                  u.ID,
			Username:            u.Username,
			DisplayName:         u.DisplayName,
			Role:                domain.UserRole(u.Role),
			Status:              domain.UserStatus(u.Status),
			FailedLoginAttempts: int(u.FailedLoginAttempts),
			MFAFailedAttempts:   int(u.MfaFailedAttempts),
			LockedUntil:         lockedUntil,
			MFAEnabled:          u.MfaEnabled == 1,
			MFAEnrolledAt:       mfaEnrolledAt,
			LastLoginAt:         lastLoginAt,
			CreatedAt:           u.CreatedAt,
			UpdatedAt:           u.UpdatedAt,
		},
		PasswordHash:              u.PasswordHash,
		MFASecretEncrypted:        u.MfaSecretEncrypted,
		MFAPendingSecretEncrypted: u.MfaPendingSecretEncrypted,
		MFAPendingExpiresAt:       pendingExpiresAt,
	}, nil
}

// GetUserByID recupera os dados de autenticação de um usuário a partir do ID.
func GetUserByID(ctx context.Context, db *sql.DB, id string) (*AdminUserWithCredentials, error) {
	q := sqlc.New(db)
	u, err := q.GetAdminUserByID(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao consultar usuário por id %q: %w", id, err)
	}

	var lockedUntil *string
	if u.LockedUntil.Valid && u.LockedUntil.String != "" {
		lockedUntil = &u.LockedUntil.String
	}

	var mfaEnrolledAt *string
	if u.MfaEnrolledAt.Valid && u.MfaEnrolledAt.String != "" {
		mfaEnrolledAt = &u.MfaEnrolledAt.String
	}

	var lastLoginAt *string
	if u.LastLoginAt.Valid && u.LastLoginAt.String != "" {
		lastLoginAt = &u.LastLoginAt.String
	}

	var pendingExpiresAt *string
	if u.MfaPendingExpiresAt.Valid && u.MfaPendingExpiresAt.String != "" {
		pendingExpiresAt = &u.MfaPendingExpiresAt.String
	}

	return &AdminUserWithCredentials{
		User: domain.AdminUser{
			ID:                  u.ID,
			Username:            u.Username,
			DisplayName:         u.DisplayName,
			Role:                domain.UserRole(u.Role),
			Status:              domain.UserStatus(u.Status),
			FailedLoginAttempts: int(u.FailedLoginAttempts),
			MFAFailedAttempts:   int(u.MfaFailedAttempts),
			LockedUntil:         lockedUntil,
			MFAEnabled:          u.MfaEnabled == 1,
			MFAEnrolledAt:       mfaEnrolledAt,
			LastLoginAt:         lastLoginAt,
			CreatedAt:           u.CreatedAt,
			UpdatedAt:           u.UpdatedAt,
		},
		PasswordHash:              u.PasswordHash,
		MFASecretEncrypted:        u.MfaSecretEncrypted,
		MFAPendingSecretEncrypted: u.MfaPendingSecretEncrypted,
		MFAPendingExpiresAt:       pendingExpiresAt,
	}, nil
}

// ListAdminUsers consulta e pagina os usuários administrativos conforme filtros.
func ListAdminUsers(ctx context.Context, db *sql.DB, rawFilter AdminUserFilter) (*AdminUserListResult, error) {
	f := SanitizeUserFilter(rawFilter)
	q := sqlc.New(db)

	var searchQuery string
	if f.Search != "" {
		searchQuery = "%" + EscapeLike(f.Search) + "%"
	}

	totalCount, err := q.CountFilteredAdminUsers(ctx, sqlc.CountFilteredAdminUsersParams{
		FilterRole:   f.Role,
		FilterStatus: f.Status,
		SearchQuery:  searchQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar usuários com filtros: %w", err)
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(f.PageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	offset := (f.Page - 1) * f.PageSize

	rows, err := q.ListAdminUsers(ctx, sqlc.ListAdminUsersParams{
		FilterRole:   f.Role,
		FilterStatus: f.Status,
		SearchQuery:  searchQuery,
		PageLimit:    int64(f.PageSize),
		PageOffset:   int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar usuários com filtros: %w", err)
	}

	var users []domain.AdminUser
	for _, u := range rows {
		var lockedUntil *string
		if u.LockedUntil.Valid && u.LockedUntil.String != "" {
			lockedUntil = &u.LockedUntil.String
		}

		var mfaEnrolledAt *string
		if u.MfaEnrolledAt.Valid && u.MfaEnrolledAt.String != "" {
			mfaEnrolledAt = &u.MfaEnrolledAt.String
		}

		var lastLoginAt *string
		if u.LastLoginAt.Valid && u.LastLoginAt.String != "" {
			lastLoginAt = &u.LastLoginAt.String
		}

		users = append(users, domain.AdminUser{
			ID:                  u.ID,
			Username:            u.Username,
			DisplayName:         u.DisplayName,
			Role:                domain.UserRole(u.Role),
			Status:              domain.UserStatus(u.Status),
			FailedLoginAttempts: int(u.FailedLoginAttempts),
			MFAFailedAttempts:   int(u.MfaFailedAttempts),
			LockedUntil:         lockedUntil,
			MFAEnabled:          u.MfaEnabled == 1,
			MFAEnrolledAt:       mfaEnrolledAt,
			LastLoginAt:         lastLoginAt,
			CreatedAt:           u.CreatedAt,
			UpdatedAt:           u.UpdatedAt,
		})
	}

	return &AdminUserListResult{
		Users:      users,
		Page:       f.Page,
		PageSize:   f.PageSize,
		TotalPages: totalPages,
		TotalCount: totalCount,
	}, nil
}

// CreateUserParams parâmetros para criação de usuário.
type CreateUserParams struct {
	Username     string
	DisplayName  string
	PasswordHash string
	Role         domain.UserRole
	Status       domain.UserStatus
}

// CreateAdminUser cria um novo usuário no banco de dados.
func CreateAdminUser(ctx context.Context, db *sql.DB, p CreateUserParams) (*domain.AdminUser, error) {
	cleanUser, err := domain.ValidateUsername(p.Username)
	if err != nil {
		return nil, err
	}
	cleanDisplay, err := domain.ValidateDisplayName(p.DisplayName)
	if err != nil {
		return nil, err
	}
	if !p.Role.IsValid() {
		return nil, fmt.Errorf("domain: papel de usuário inválido %q", p.Role)
	}
	if p.Status == "" {
		p.Status = domain.UserStatusActive
	}
	if !p.Status.IsValid() {
		return nil, fmt.Errorf("domain: status de usuário inválido %q", p.Status)
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(db)
	err = q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
		ID:                        id,
		Username:                  cleanUser,
		DisplayName:               cleanDisplay,
		PasswordHash:              p.PasswordHash,
		Role:                      string(p.Role),
		Status:                    string(p.Status),
		FailedLoginAttempts:       0,
		MfaFailedAttempts:         0,
		LockedUntil:               sql.NullString{},
		MfaEnabled:                0,
		MfaSecretEncrypted:        "",
		MfaPendingSecretEncrypted: "",
		MfaPendingExpiresAt:       sql.NullString{},
		MfaEnrolledAt:             sql.NullString{},
		LastLoginAt:               sql.NullString{},
		CreatedAt:                 now,
		UpdatedAt:                 now,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao inserir usuário %q: %w", cleanUser, err)
	}

	return &domain.AdminUser{
		ID:                  id,
		Username:            cleanUser,
		DisplayName:         cleanDisplay,
		Role:                p.Role,
		Status:              p.Status,
		FailedLoginAttempts: 0,
		MFAFailedAttempts:   0,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

// UpdateAdminUserRole atualiza o papel do usuário com OCC.
func UpdateAdminUserRole(ctx context.Context, db *sql.DB, id, expectedUpdatedAt string, newRole domain.UserRole) error {
	if !newRole.IsValid() {
		return fmt.Errorf("domain: papel de usuário inválido %q", newRole)
	}
	if strings.TrimSpace(expectedUpdatedAt) == "" {
		return fmt.Errorf("store: expected_updated_at é obrigatório para atualização com controle otimista")
	}

	q := sqlc.New(db)
	rows, err := q.UpdateAdminUserRole(ctx, sqlc.UpdateAdminUserRoleParams{
		ID:                id,
		ExpectedUpdatedAt: expectedUpdatedAt,
		NewRole:           string(newRole),
	})
	if err != nil {
		return fmt.Errorf("store: falha ao atualizar papel do usuário %s: %w", id, err)
	}
	if rows == 0 {
		return ErrConflict
	}
	return nil
}

// UpdateAdminUserStatus atualiza o status do usuário com OCC.
func UpdateAdminUserStatus(ctx context.Context, db *sql.DB, id, expectedUpdatedAt string, newStatus domain.UserStatus) error {
	if !newStatus.IsValid() {
		return fmt.Errorf("domain: status de usuário inválido %q", newStatus)
	}
	if strings.TrimSpace(expectedUpdatedAt) == "" {
		return fmt.Errorf("store: expected_updated_at é obrigatório para atualização com controle otimista")
	}

	q := sqlc.New(db)
	rows, err := q.UpdateAdminUserStatus(ctx, sqlc.UpdateAdminUserStatusParams{
		ID:                id,
		ExpectedUpdatedAt: expectedUpdatedAt,
		NewStatus:         string(newStatus),
	})
	if err != nil {
		return fmt.Errorf("store: falha ao atualizar status do usuário %s: %w", id, err)
	}
	if rows == 0 {
		return ErrConflict
	}
	return nil
}

// UpdateAdminUserPassword atualiza a senha (hash) do usuário.
func UpdateAdminUserPassword(ctx context.Context, db *sql.DB, id, passwordHash string) error {
	q := sqlc.New(db)
	return q.UpdateAdminUserPassword(ctx, sqlc.UpdateAdminUserPasswordParams{
		ID:           id,
		PasswordHash: passwordHash,
	})
}

// SetAdminUserPendingMFA armazena o segredo pendente cifrado e o tempo de expiração do setup server-side.
func SetAdminUserPendingMFA(ctx context.Context, db *sql.DB, id, pendingSecretEncrypted string, expiresAt time.Time) error {
	q := sqlc.New(db)
	return q.SetAdminUserPendingMFA(ctx, sqlc.SetAdminUserPendingMFAParams{
		ID:                        id,
		MfaPendingSecretEncrypted: pendingSecretEncrypted,
		MfaPendingExpiresAt:       sql.NullString{String: expiresAt.UTC().Format(time.RFC3339Nano), Valid: true},
	})
}

// ConfirmAdminUserMFA ativa o MFA promovendo o segredo pendente para ativo e limpando o estado pendente.
func ConfirmAdminUserMFA(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.ConfirmAdminUserMFA(ctx, id)
}

// DisableAdminUserMFA desativa o MFA de um usuário e limpa todos os segredos cifrados.
func DisableAdminUserMFA(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.DisableAdminUserMFA(ctx, id)
}

// ClearAdminUserPendingMFA limpa qualquer segredo pendente de setup e sua expiração.
func ClearAdminUserPendingMFA(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.ClearAdminUserPendingMFA(ctx, id)
}

// RecordUserPasswordLoginSuccess registra o sucesso na senha sem resetar falhas pendentes de MFA.
func RecordUserPasswordLoginSuccess(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.UpdateAdminUserPasswordSuccess(ctx, id)
}

// RecordUserMFASuccess registra o sucesso completo de MFA e reseta todos os contadores de falhas.
func RecordUserMFASuccess(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.UpdateAdminUserMFASuccess(ctx, id)
}

// RecordUserLoginSuccess registra o sucesso de login total e reseta contadores de falhas.
func RecordUserLoginSuccess(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.UpdateAdminUserLoginSuccess(ctx, id)
}

// FailedAttemptResult encapsula o resultado atômico do incremento de falhas de login.
type FailedAttemptResult struct {
	UserID              string
	Username            string
	FailedLoginAttempts int
	MFAFailedAttempts   int
	Status              domain.UserStatus
	LockedUntil         *string
	IsLocked            bool
}

// RecordPasswordFailedAttemptAndLock incrementa atomicamente a contagem de falhas de senha e bloqueia a conta se o limite for atingido.
func RecordPasswordFailedAttemptAndLock(ctx context.Context, db *sql.DB, id string, maxAttempts int, lockedUntil time.Time) (*FailedAttemptResult, error) {
	q := sqlc.New(db)
	row, err := q.RecordPasswordFailedAttemptAndLock(ctx, sqlc.RecordPasswordFailedAttemptAndLockParams{
		ID:          id,
		MaxAttempts: int64(maxAttempts),
		LockedUntil: sql.NullString{String: lockedUntil.UTC().Format(time.RFC3339Nano), Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao registrar tentativa de senha incorreta no banco: %w", err)
	}

	var lockedUntilStr *string
	if row.LockedUntil.Valid && row.LockedUntil.String != "" {
		lockedUntilStr = &row.LockedUntil.String
	}

	status := domain.UserStatus(row.Status)
	isLocked := status == domain.UserStatusLocked || int(row.FailedLoginAttempts) >= maxAttempts

	return &FailedAttemptResult{
		UserID:              row.ID,
		Username:            row.Username,
		FailedLoginAttempts: int(row.FailedLoginAttempts),
		Status:              status,
		LockedUntil:         lockedUntilStr,
		IsLocked:            isLocked,
	}, nil
}

// RecordMFAFailedAttemptAndLock incrementa atomicamente a contagem de falhas de MFA (independente de senha) e bloqueia a conta se o limite for atingido.
func RecordMFAFailedAttemptAndLock(ctx context.Context, db *sql.DB, id string, maxAttempts int, lockedUntil time.Time) (*FailedAttemptResult, error) {
	q := sqlc.New(db)
	row, err := q.RecordMFAFailedAttemptAndLock(ctx, sqlc.RecordMFAFailedAttemptAndLockParams{
		ID:          id,
		MaxAttempts: int64(maxAttempts),
		LockedUntil: sql.NullString{String: lockedUntil.UTC().Format(time.RFC3339Nano), Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao registrar tentativa de MFA incorreta no banco: %w", err)
	}

	var lockedUntilStr *string
	if row.LockedUntil.Valid && row.LockedUntil.String != "" {
		lockedUntilStr = &row.LockedUntil.String
	}

	status := domain.UserStatus(row.Status)
	isLocked := status == domain.UserStatusLocked || int(row.MfaFailedAttempts) >= maxAttempts

	return &FailedAttemptResult{
		UserID:            row.ID,
		Username:          row.Username,
		MFAFailedAttempts: int(row.MfaFailedAttempts),
		Status:            status,
		LockedUntil:       lockedUntilStr,
		IsLocked:          isLocked,
	}, nil
}

// RecordUserFailedAttemptAndLock é mantido para compatibilidade com o fluxo de senha.
func RecordUserFailedAttemptAndLock(ctx context.Context, db *sql.DB, id string, maxAttempts int, lockedUntil time.Time) (*FailedAttemptResult, error) {
	return RecordPasswordFailedAttemptAndLock(ctx, db, id, maxAttempts, lockedUntil)
}

// RecordUserFailedAttempt incrementa a contagem de falhas do usuário.
func RecordUserFailedAttempt(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.IncrementAdminUserFailedAttempts(ctx, id)
}

// LockAdminUser bloqueia a conta do usuário até determinado instante.
func LockAdminUser(ctx context.Context, db *sql.DB, id string, until time.Time) error {
	q := sqlc.New(db)
	return q.LockAdminUser(ctx, sqlc.LockAdminUserParams{
		ID:          id,
		LockedUntil: sql.NullString{String: until.UTC().Format(time.RFC3339Nano), Valid: true},
	})
}

// UnlockAdminUser reativa a conta do usuário após bloqueio ou por ação de admin.
func UnlockAdminUser(ctx context.Context, db *sql.DB, id string) error {
	q := sqlc.New(db)
	return q.UnlockAdminUser(ctx, id)
}

// CreateAdminSession persiste uma nova sessão ativa.
func CreateAdminSession(ctx context.Context, db *sql.DB, session domain.AdminSession) error {
	q := sqlc.New(db)
	mfaVal := int64(0)
	if session.MFAVerified {
		mfaVal = 1
	}

	return q.CreateAdminSession(ctx, sqlc.CreateAdminSessionParams{
		ID:             session.ID,
		UserID:         session.UserID,
		MfaVerified:    mfaVal,
		IpAddress:      session.IPAddress,
		UserAgent:      session.UserAgent,
		ExpiresAt:      session.ExpiresAt,
		LastActivityAt: session.LastActivityAt,
		CreatedAt:      session.CreatedAt,
	})
}

// GetAdminSessionWithUser recupera a sessão e os dados do usuário associado.
func GetAdminSessionWithUser(ctx context.Context, db *sql.DB, sessionID string) (*SessionWithUser, error) {
	q := sqlc.New(db)
	row, err := q.GetAdminSessionWithUser(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: falha ao consultar sessão %s: %w", sessionID, err)
	}

	return &SessionWithUser{
		Session: domain.AdminSession{
			ID:             row.SessionID,
			UserID:         row.UserID,
			MFAVerified:    row.MfaVerified == 1,
			IPAddress:      row.IpAddress,
			UserAgent:      row.UserAgent,
			ExpiresAt:      row.ExpiresAt,
			LastActivityAt: row.LastActivityAt,
			CreatedAt:      row.SessionCreatedAt,
		},
		User: domain.AdminUser{
			ID:          row.UserID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Role:        domain.UserRole(row.Role),
			Status:      domain.UserStatus(row.Status),
			MFAEnabled:  row.MfaEnabled == 1,
		},
	}, nil
}

// UpdateAdminSessionActivity atualiza a última atividade da sessão.
func UpdateAdminSessionActivity(ctx context.Context, db *sql.DB, sessionID string) error {
	q := sqlc.New(db)
	return q.UpdateAdminSessionActivity(ctx, sessionID)
}

// ElevateAdminSessionMFA marca a sessão como verificada por MFA.
func ElevateAdminSessionMFA(ctx context.Context, db *sql.DB, sessionID string) error {
	q := sqlc.New(db)
	return q.ElevateAdminSessionMFA(ctx, sessionID)
}

// DeleteAdminSession remove uma sessão específica.
func DeleteAdminSession(ctx context.Context, db *sql.DB, sessionID string) error {
	q := sqlc.New(db)
	return q.DeleteAdminSession(ctx, sessionID)
}

// DeleteUserAdminSessions remove todas as sessões de um usuário.
func DeleteUserAdminSessions(ctx context.Context, db *sql.DB, userID string) error {
	q := sqlc.New(db)
	return q.DeleteUserAdminSessions(ctx, userID)
}

// DeleteExpiredAdminSessions expurga sessões expiradas.
func DeleteExpiredAdminSessions(ctx context.Context, db *sql.DB) error {
	q := sqlc.New(db)
	return q.DeleteExpiredAdminSessions(ctx)
}

// InsertAdminAuditLog grava um evento na trilha de auditoria administrativa.
func InsertAdminAuditLog(ctx context.Context, db *sql.DB, log domain.AdminAuditLog) error {
	q := sqlc.New(db)
	id := log.ID
	if id == "" {
		id = uuid.New().String()
	}
	createdAt := log.CreatedAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	var userID sql.NullString
	if log.UserID != nil && *log.UserID != "" {
		userID = sql.NullString{String: *log.UserID, Valid: true}
	}

	var actorID sql.NullString
	if log.ActorID != nil && *log.ActorID != "" {
		actorID = sql.NullString{String: *log.ActorID, Valid: true}
	}

	return q.InsertAdminAuditLog(ctx, sqlc.InsertAdminAuditLogParams{
		ID:            id,
		UserID:        userID,
		Username:      log.Username,
		Action:        log.Action,
		ActorID:       actorID,
		ActorUsername: log.ActorUsername,
		TargetID:      log.TargetID,
		Details:       log.Details,
		IpAddress:     log.IPAddress,
		CreatedAt:     createdAt,
	})
}

// ListAdminAuditLogs consulta e pagina logs de auditoria administrativa.
func ListAdminAuditLogs(ctx context.Context, db *sql.DB, rawFilter AdminAuditFilter) (*AdminAuditListResult, error) {
	f := SanitizeAuditFilter(rawFilter)
	q := sqlc.New(db)

	var searchQuery string
	if f.Search != "" {
		searchQuery = "%" + EscapeLike(f.Search) + "%"
	}

	totalCount, err := q.CountFilteredAdminAuditLogs(ctx, sqlc.CountFilteredAdminAuditLogsParams{
		FilterAction:      f.Action,
		FilterActor:       f.Actor,
		FilterPeriodSince: f.PeriodSince,
		SearchQuery:       searchQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao contar logs de auditoria: %w", err)
	}

	totalPages := int(math.Ceil(float64(totalCount) / float64(f.PageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	offset := (f.Page - 1) * f.PageSize

	rows, err := q.ListAdminAuditLogs(ctx, sqlc.ListAdminAuditLogsParams{
		FilterAction:      f.Action,
		FilterActor:       f.Actor,
		FilterPeriodSince: f.PeriodSince,
		SearchQuery:       searchQuery,
		PageLimit:         int64(f.PageSize),
		PageOffset:        int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar logs de auditoria: %w", err)
	}

	var logs []domain.AdminAuditLog
	for _, r := range rows {
		var uID *string
		if r.UserID.Valid && r.UserID.String != "" {
			uID = &r.UserID.String
		}
		var actID *string
		if r.ActorID.Valid && r.ActorID.String != "" {
			actID = &r.ActorID.String
		}

		logs = append(logs, domain.AdminAuditLog{
			ID:            r.ID,
			UserID:        uID,
			Username:      r.Username,
			Action:        r.Action,
			ActorID:       actID,
			ActorUsername: r.ActorUsername,
			TargetID:      r.TargetID,
			Details:       r.Details,
			IPAddress:     r.IpAddress,
			CreatedAt:     r.CreatedAt,
		})
	}

	return &AdminAuditListResult{
		Logs:       logs,
		Page:       f.Page,
		PageSize:   f.PageSize,
		TotalPages: totalPages,
		TotalCount: totalCount,
	}, nil
}
