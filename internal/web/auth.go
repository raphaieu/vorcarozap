package web

import (
	"crypto/subtle"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

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

			next.ServeHTTP(w, r)
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
