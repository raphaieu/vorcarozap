package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store"
)

func TestOpenAndValidatePragmas(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	// Verifica se a conexão está ativa
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("falha no ping do banco: %v", err)
	}
}

func TestMigrateAndCheckReady(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_migrate.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	// Antes de migrar, CheckReady deve reportar pendências
	if err := store.CheckReady(ctx, db); err == nil {
		t.Fatal("esperava erro de migrations pendentes antes de migrar, obteve nil")
	}

	// Executa migrations
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao executar migrations: %v", err)
	}

	// Após migrar, CheckReady deve passar
	if err := store.CheckReady(ctx, db); err != nil {
		t.Fatalf("esperava sucesso no CheckReady após migrations, obteve erro: %v", err)
	}

	// Teste de idempotência (segunda execução não deve falhar)
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("segunda execução de migrations falhou: %v", err)
	}

	// Banco à frente do binário deve falhar
	if _, err := db.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, 1);"); err != nil {
		t.Fatalf("falha ao simular versão futura do banco: %v", err)
	}
	if err := store.CheckReady(ctx, db); err == nil {
		t.Fatal("esperava erro quando banco está à frente do binário, obteve nil")
	}

	// Limpa a versão simulada
	if _, err := db.ExecContext(ctx, "DELETE FROM goose_db_version WHERE version_id = 99999;"); err != nil {
		t.Fatalf("falha ao limpar versão simulada: %v", err)
	}

	// Contexto cancelado/expirado deve falhar
	canceledCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if err := store.CheckReady(canceledCtx, db); err == nil {
		t.Fatal("esperava falha com contexto cancelado, obteve nil")
	}
}

func TestCheckReadyClosedDB(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_closed.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	// Fecha o banco imediatamente
	db.Close()

	if err := store.CheckReady(ctx, db); err == nil {
		t.Fatal("esperava falha no CheckReady com banco fechado, obteve nil")
	}
}
