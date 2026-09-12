package web

import (
	"context"
	"crypto/subtle"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

type authUserContextKey struct{}

// WithAuthenticatedUser anexa o nome do usuário autenticado ao contexto.
func WithAuthenticatedUser(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, authUserContextKey{}, username)
}

// AuthenticatedUserFromContext recupera o nome do usuário autenticado a partir do contexto.
func AuthenticatedUserFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(authUserContextKey{}).(string)
	if !ok || val == "" {
		return "", false
	}
	return val, true
}

// BasicAuthMiddleware cria um middleware HTTP que protege rotas com autenticação Basic Auth.
// A comparação do usuário é feita em tempo constante e a verificação do hash bcrypt
// é executada incondicionalmente para equalizar o tempo de resposta contra ataques de temporização.
func BasicAuthMiddleware(expectedUser, expectedPasswordHash string) func(http.Handler) http.Handler {
	expectedUserBytes := []byte(expectedUser)
	expectedHashBytes := []byte(expectedPasswordHash)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok {
				sendUnauthorized(w)
				return
			}

			// Comparação do usuário em tempo constante
			userMatch := subtle.ConstantTimeCompare([]byte(username), expectedUserBytes) == 1

			// Execução incondicional do bcrypt para equalizar latência
			passErr := bcrypt.CompareHashAndPassword(expectedHashBytes, []byte(password))
			passMatch := (passErr == nil)

			if !userMatch || !passMatch {
				sendUnauthorized(w)
				return
			}

			// Cabeçalhos restritivos para respostas autenticadas
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Vary", "Authorization")

			ctx := WithAuthenticatedUser(r.Context(), username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sendUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="VorcaroZAP admin", charset="UTF-8"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("Unauthorized\n"))
}
