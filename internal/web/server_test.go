package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

func setupTestServer(t *testing.T) *http.Server {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "web_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha nas migrations: %v", err)
	}

	cfg := &config.Config{
		Port:         8080,
		Env:          "test",
		DBPath:       dbPath,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	return srv
}

func TestHealthLiveEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"status":"live"`) {
		t.Errorf("body inesperado: %s", body)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type inesperado: %s", ct)
	}
}

func TestHealthReadyEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"status":"ready"`) {
		t.Errorf("body inesperado: %s", body)
	}
}

func TestHomeEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "VorcaroZAP") {
		t.Errorf("página não contém nome do projeto 'VorcaroZAP'")
	}
	if !strings.Contains(body, "Aviso Editorial e de Independência") {
		t.Errorf("página não contém aviso editorial obrigatório")
	}
	if !strings.Contains(body, "Fase 1 — Fundação") {
		t.Errorf("página não contém menção à Fase 1")
	}

	// Verifica headers de segurança
	if h := w.Header().Get("X-Content-Type-Options"); h != "nosniff" {
		t.Errorf("X-Content-Type-Options: esperado 'nosniff', obtido %q", h)
	}
	if h := w.Header().Get("X-Frame-Options"); h != "DENY" {
		t.Errorf("X-Frame-Options: esperado 'DENY', obtido %q", h)
	}
}

func TestStaticCSSEndpoint(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: esperado 200, obtido %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Errorf("Content-Type inesperado: %s", ct)
	}
}
