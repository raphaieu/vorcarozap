package web

import (
	"mime"
	"net/http"
	"strings"
)

// AdminCSRFMiddleware valida a proteção CSRF para rotas mutantes da área administrativa.
// Para métodos mutantes (POST, PUT, PATCH, DELETE), exige correspondência exata do cabeçalho
// Origin com a origem configurada (ADMIN_ALLOWED_ORIGIN), sem fallback para Referer.
// Também assegura Content-Type application/x-www-form-urlencoded e limita o corpo a 64 KB.
func AdminCSRFMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			// Validação estrita do cabeçalho Origin
			origin := r.Header.Get("Origin")
			if origin == "" || origin != allowedOrigin {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Forbidden: invalid or missing Origin header\n"))
				return
			}

			// Validação do Content-Type
			contentType := r.Header.Get("Content-Type")
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err != nil || !strings.EqualFold(mediaType, "application/x-www-form-urlencoded") {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_, _ = w.Write([]byte("Unsupported Media Type: expected application/x-www-form-urlencoded\n"))
				return
			}

			// Limite estrito de 64 KB para o corpo da requisição
			r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

			next.ServeHTTP(w, r)
		})
	}
}
