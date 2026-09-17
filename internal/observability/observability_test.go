package observability_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/observability"
	"github.com/raphaieu/vorcarozap/internal/store"
)

func TestSanitizedHandler_SecretRedaction(t *testing.T) {
	var buf bytes.Buffer
	innerHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	sanitizedHandler := observability.NewSanitizedHandler(innerHandler)
	logger := slog.New(sanitizedHandler)

	logger.Info("mensagem de teste",
		"api_key", "sk-or-v1-supersecretkey12345",
		"password", "p@ssw0rdMinha123",
		"hash", "$2a$12$R9h/cIPz0gi.URNNXRkh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW",
		"token", "bearer_token_abc_xyz",
		"mfa_key", "A1B2C3D4E5F6G7H8",
		"auth_header", "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"contact_email", "contato.pessoal@dominio.com.br",
		"contact_phone", "+55 (11) 98765-4321",
		"normal_field", "valor_permitido",
	)

	out := buf.String()

	// 1. Campos sensíveis por chave devem ser [REDACTED]
	if !strings.Contains(out, "api_key=[REDACTED]") {
		t.Errorf("esperava api_key redigida, obtido: %s", out)
	}
	if strings.Contains(out, "sk-or-v1-supersecretkey12345") {
		t.Errorf("vazamento de API key detectado: %s", out)
	}
	if strings.Contains(out, "p@ssw0rdMinha123") {
		t.Errorf("vazamento de senha detectado: %s", out)
	}
	if strings.Contains(out, "$2a$12$R9h/cIPz0gi.URNNXRkh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW") {
		t.Errorf("vazamento de hash bcrypt detectado: %s", out)
	}
	if strings.Contains(out, "A1B2C3D4E5F6G7H8") {
		t.Errorf("vazamento de MFA key detectado: %s", out)
	}
	if strings.Contains(out, "contato.pessoal@dominio.com.br") {
		t.Errorf("vazamento de e-mail detectado: %s", out)
	}

	// 2. Campo normal deve ser preservado
	if !strings.Contains(out, "normal_field=valor_permitido") {
		t.Errorf("esperava normal_field=valor_permitido preservado, obtido: %s", out)
	}
}

func TestSanitizedHandler_StringContentSanitization(t *testing.T) {
	var buf bytes.Buffer
	innerHandler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	sanitizedHandler := observability.NewSanitizedHandler(innerHandler)
	logger := slog.New(sanitizedHandler)

	// String com segredo embutido em mensagem de erro ou campo genérico
	logger.Error("falha ao autenticar com chave sk-or-v1-chavevazada e usuario user@email.com")

	out := buf.String()
	if strings.Contains(out, "sk-or-v1-chavevazada") {
		t.Errorf("vazamento de padrão OpenRouter na mensagem: %s", out)
	}
	if strings.Contains(out, "user@email.com") {
		t.Errorf("vazamento de e-mail na mensagem: %s", out)
	}
	if !strings.Contains(out, "sk-or-v1-[REDACTED]") {
		t.Errorf("esperava substituição por sk-or-v1-[REDACTED]: %s", out)
	}
	if !strings.Contains(out, "[REDACTED_EMAIL]") {
		t.Errorf("esperava substituição por [REDACTED_EMAIL]: %s", out)
	}
}

func TestMetricsRegistry_ConcurrentAccess(t *testing.T) {
	registry := observability.NewMetricsRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			status := 200
			if idx%10 == 0 {
				status = 500
			} else if idx%5 == 0 {
				status = 404
			}
			registry.RecordHTTPRequest("GET", "/pessoas", status, time.Duration(idx*10)*time.Millisecond)
		}(i)
	}

	wg.Wait()

	snap, err := registry.CollectSnapshot(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("falha ao coletar snapshot: %v", err)
	}

	if snap.HTTP.TotalRequests != 50 {
		t.Errorf("esperado 50 requisições, obtido %d", snap.HTTP.TotalRequests)
	}
	if snap.HTTP.Status2xx == 0 {
		t.Errorf("esperado contagem positiva de status 2xx")
	}
	if snap.HTTP.Status500 == 0 {
		t.Errorf("esperado contagem positiva de status 500")
	}
	if snap.HTTP.AvgLatencyMs <= 0 {
		t.Errorf("esperado avg latency > 0, obtido %f", snap.HTTP.AvgLatencyMs)
	}
}

func TestMetricsRegistry_BackupTelemetry(t *testing.T) {
	registry := observability.NewMetricsRegistry()

	// 1. Registra backup local sucesso
	registry.RecordLocalBackup(1024*1024*5, 120*time.Millisecond, 11, nil)

	// 2. Registra backup remoto erro
	remoteErr := errors.New("timeout ao conectar no bucket")
	registry.RecordRemoteBackup(0, 5000*time.Millisecond, "s3", "my-bucket", "backups/test.db", "", remoteErr)

	snap, err := registry.CollectSnapshot(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("erro ao coletar snapshot: %v", err)
	}

	if snap.Backup.LocalStatus != "ok" {
		t.Errorf("esperado LocalStatus 'ok', obtido %q", snap.Backup.LocalStatus)
	}
	if snap.Backup.LocalSizeBytes != 1024*1024*5 {
		t.Errorf("esperado 5MB, obtido %d bytes", snap.Backup.LocalSizeBytes)
	}
	if snap.Backup.LocalSchemaVersion != 11 {
		t.Errorf("esperado schema version 11, obtido %d", snap.Backup.LocalSchemaVersion)
	}
	if snap.Backup.RemoteStatus != "error" {
		t.Errorf("esperado RemoteStatus 'error', obtido %q", snap.Backup.RemoteStatus)
	}
	if snap.Backup.RemoteLastError != "timeout ao conectar no bucket" {
		t.Errorf("esperado erro registrado, obtido %q", snap.Backup.RemoteLastError)
	}
}

func TestObservabilityMiddleware(t *testing.T) {
	registry := observability.NewMetricsRegistry()
	mw := observability.Middleware(registry)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("POST", "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("esperado status 201, obtido %d", rec.Code)
	}

	snap, err := registry.CollectSnapshot(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("falha ao coletar snapshot: %v", err)
	}
	if snap.HTTP.TotalRequests != 1 {
		t.Errorf("esperado 1 request, obtido %d", snap.HTTP.TotalRequests)
	}
	if snap.HTTP.Status2xx != 1 {
		t.Errorf("esperado 1 status 2xx, obtido %d", snap.HTTP.Status2xx)
	}
}

func TestCollectSnapshot_WithDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	cfg := &config.Config{
		Env:                     "test",
		DBPath:                  dbPath,
		MonitorMaxCostPerDayUSD: 1.00,
		BackupRemoteEnabled:     false,
	}

	registry := observability.NewMetricsRegistry()
	snap, err := registry.CollectSnapshot(ctx, db, cfg)
	if err != nil {
		t.Fatalf("falha ao coletar snapshot com DB: %v", err)
	}

	if snap.Environment != "test" {
		t.Errorf("esperado environment 'test', obtido %q", snap.Environment)
	}
	if snap.SQLite.PageCount <= 0 {
		t.Errorf("esperado page count > 0, obtido %d", snap.SQLite.PageCount)
	}
	if snap.SQLite.CurrentSchemaV <= 0 {
		t.Errorf("esperado schema version > 0, obtido %d", snap.SQLite.CurrentSchemaV)
	}
}
