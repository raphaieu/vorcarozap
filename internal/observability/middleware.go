package observability

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Middleware intercepta requisições HTTP e registra telemetria de código de status e latência no registry.
func Middleware(registry *MetricsRegistry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				duration := time.Since(start)
				statusCode := ww.Status()
				if statusCode == 0 {
					statusCode = http.StatusOK
				}
				registry.RecordHTTPRequest(r.Method, r.URL.Path, statusCode, duration)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// HTTPLoggingMiddleware realiza logging estruturado e sanitizado de requisições HTTP via slog.
func HTTPLoggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				duration := time.Since(start)
				statusCode := ww.Status()
				if statusCode == 0 {
					statusCode = http.StatusOK
				}
				rawQuery := r.URL.RawQuery
				var sanitizedQuery string
				if rawQuery != "" {
					sanitizedQuery = SanitizeStringValue(rawQuery)
				}

				reqID := middleware.GetReqID(r.Context())
				slog.Info("http_request",
					"method", r.Method,
					"path", r.URL.Path,
					"query", sanitizedQuery,
					"status", statusCode,
					"duration_ms", duration.Milliseconds(),
					"bytes_written", ww.BytesWritten(),
					"request_id", reqID,
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
