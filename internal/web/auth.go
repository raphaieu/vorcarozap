package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
	"github.com/raphaieu/vorcarozap/internal/domain"
)

const SessionCookieName = "admin_session"

type authUserContextKey struct{}
type authAdminUserContextKey struct{}
type authSessionContextKey struct{}

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

// WithCurrentUser anexa o objeto AdminUser e AdminSession ao contexto.
func WithCurrentUser(ctx context.Context, user *domain.AdminUser, session *domain.AdminSession) context.Context {
	ctx = context.WithValue(ctx, authUserContextKey{}, user.Username)
	ctx = context.WithValue(ctx, authAdminUserContextKey{}, user)
	if session != nil {
		ctx = context.WithValue(ctx, authSessionContextKey{}, session)
	}
	return ctx
}

// CurrentUserFromContext recupera o objeto AdminUser a partir do contexto HTTP.
func CurrentUserFromContext(ctx context.Context) (*domain.AdminUser, bool) {
	u, ok := ctx.Value(authAdminUserContextKey{}).(*domain.AdminUser)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

// CurrentSessionFromContext recupera o objeto AdminSession a partir do contexto HTTP.
func CurrentSessionFromContext(ctx context.Context) (*domain.AdminSession, bool) {
	s, ok := ctx.Value(authSessionContextKey{}).(*domain.AdminSession)
	if !ok || s == nil {
		return nil, false
	}
	return s, true
}

// SessionAuthMiddleware cria o middleware que valida sessões baseadas em cookie na árvore /admin.
func SessionAuthMiddleware(authSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Cabeçalhos restritivos para todas as respostas administrativas
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Vary", "Cookie")

			// 1. Tenta autenticação exclusivamente via Cookie de Sessão
			cookie, err := r.Cookie(SessionCookieName)
			if err == nil && strings.TrimSpace(cookie.Value) != "" {
				user, session, err := authSvc.ValidateSession(r.Context(), cookie.Value)
				if err == nil && user != nil && session != nil {
					ctx := WithCurrentUser(r.Context(), user, session)

					// Se a sessão ainda não passou pelo desafio de MFA, bloqueia rotas normais
					if !session.MFAVerified {
						// Permite apenas rotas do fluxo de MFA e logout
						path := strings.TrimSuffix(r.URL.Path, "/")
						if path != "/admin/mfa/challenge" && path != "/admin/mfa/setup" && path != "/admin/logout" && path != "/admin/login" {
							if user.MFAEnabled {
								http.Redirect(w, r, "/admin/mfa/challenge", http.StatusSeeOther)
							} else {
								http.Redirect(w, r, "/admin/mfa/setup", http.StatusSeeOther)
							}
							return
						}
					}

					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// Cookie inválido ou expirado -> limpa o cookie
				ClearSessionCookie(w, r)
			}

			// 2. Permite acesso à rota de login para usuários não autenticados
			path := strings.TrimSuffix(r.URL.Path, "/")
			if path == "/admin/login" {
				next.ServeHTTP(w, r)
				return
			}

			// 3. Não autenticado: Se for requisição HTML de navegador, redireciona para login
			if isBrowserRequest(r) {
				returnTo := r.URL.RequestURI()
				loginURL := "/admin/login"
				if returnTo != "/admin" && returnTo != "/admin/" && returnTo != "" {
					loginURL += "?return_to=" + url.QueryEscape(returnTo)
				}
				http.Redirect(w, r, loginURL, http.StatusSeeOther)
				return
			}

			// Para APIs e clientes que não esperam HTML, envia 401 Unauthorized
			sendUnauthorized(w)
		})
	}
}

// RequirePermission cria um middleware que verifica se o usuário autenticado possui a permissão requerida.
func RequirePermission(perm domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := CurrentUserFromContext(r.Context())
			if !ok || user == nil {
				sendUnauthorized(w)
				return
			}

			if !user.Role.HasPermission(perm) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Forbidden: permissão insuficiente para acessar este recurso\n"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireMFAVerified garante que apenas requisições com sessão válida e segundo fator verificado acessem rotas protegidas.
func RequireMFAVerified() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := CurrentUserFromContext(r.Context())
			if !ok || user == nil {
				sendUnauthorized(w)
				return
			}

			session, hasSession := CurrentSessionFromContext(r.Context())
			if !hasSession || session == nil || !session.MFAVerified {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("Forbidden: segundo fator (MFA) obrigatório para esta ação\n"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SetSessionCookie define o cookie HTTP-only seguro com o token de sessão.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  expiresAt,
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie limpa o cookie de sessão do navegador.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, cookie)
}

func isBrowserRequest(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	return strings.EqualFold(proto, "https")
}

func sendUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="VorcaroZAP admin", charset="UTF-8"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("Unauthorized\n"))
}
