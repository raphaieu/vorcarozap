package web_test

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

// createSessionCookieForUser cria/atualiza um usuário e cria uma sessão válida e verificada no banco,
// retornando o cookie correspondente para uso em requisições de teste.
func createSessionCookieForUser(t *testing.T, db *sql.DB, username string, role domain.UserRole) *http.Cookie {
	t.Helper()
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID := "user-" + username
	_, err := db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, display_name, password_hash, role, status, mfa_enabled, mfa_secret_encrypted, created_at, updated_at)
		VALUES (?, ?, ?, '$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', ?, 'active', 1, 'enc_dummy', ?, ?)
		ON CONFLICT(username) DO UPDATE SET role = excluded.role, status = 'active'
	`, userID, username, username, string(role), now, now)
	if err != nil {
		t.Fatalf("falha ao criar/atualizar usuário de teste %s: %v", username, err)
	}

	var actualID string
	err = db.QueryRowContext(ctx, "SELECT id FROM admin_users WHERE username = ?", username).Scan(&actualID)
	if err != nil {
		t.Fatalf("falha ao consultar id do usuário %s: %v", username, err)
	}

	rawToken, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("falha ao gerar token de sessão: %v", err)
	}

	session := domain.AdminSession{
		ID:             rawToken,
		UserID:         actualID,
		MFAVerified:    true,
		IPAddress:      "127.0.0.1",
		UserAgent:      "TestAgent/1.0",
		ExpiresAt:      time.Now().UTC().Add(8 * time.Hour).Format(time.RFC3339Nano),
		LastActivityAt: now,
		CreatedAt:      now,
	}
	if err := store.CreateAdminSession(ctx, db, session); err != nil {
		t.Fatalf("falha ao criar sessão de teste: %v", err)
	}

	return &http.Cookie{
		Name:     web.SessionCookieName,
		Value:    rawToken,
		Path:     "/admin",
		HttpOnly: true,
	}
}
