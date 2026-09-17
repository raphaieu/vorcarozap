package observability

import (
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
