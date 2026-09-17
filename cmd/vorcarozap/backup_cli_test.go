package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/store"
)

func TestBackupCLI_Help(t *testing.T) {
	err := runBackup([]string{"--help"})
	if err != nil {
		t.Fatalf("esperado nil no help de backup, obtido: %v", err)
	}
}

func TestBackupCLI_InvalidRetention(t *testing.T) {
	err := runBackup([]string{"--retention", "-5"})
	if err == nil {
		t.Fatal("esperava erro para retenção negativa")
	}
}

func TestBackupCLI_LocalExecution(t *testing.T) {
	t.Setenv("ADMIN_MFA_ENCRYPTION_KEY", "12345678901234567890123456789012")
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "source.db")
	backupPath := filepath.Join(tempDir, "backup.db")

	t.Setenv("DB_PATH", dbPath)
	t.Setenv("BACKUP_REMOTE_ENABLED", "false")

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao criar banco de teste: %v", err)
	}
	if err := store.Migrate(ctx, db); err != nil {
		db.Close()
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}
	db.Close()

	err = runBackup([]string{"--out", backupPath, "--retention", "2"})
	if err != nil {
		t.Fatalf("falha ao executar runBackup: %v", err)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("arquivo de backup não foi gerado em %s: %v", backupPath, err)
	}

	// Testa verify-backup via CLI
	err = runVerifyBackup([]string{"--file", backupPath})
	if err != nil {
		t.Fatalf("falha ao executar runVerifyBackup: %v", err)
	}

	// Testa restore-sandbox via CLI com arquivo local
	err = runRestoreSandbox([]string{"--file", backupPath})
	if err != nil {
		t.Fatalf("falha ao executar runRestoreSandbox: %v", err)
	}

	// Testa restore via CLI com --confirm
	restoredPath := filepath.Join(tempDir, "restored.db")
	err = runRestore([]string{"--backup", backupPath, "--target", restoredPath, "--confirm"})
	if err != nil {
		t.Fatalf("falha ao executar runRestore: %v", err)
	}

	if _, err := os.Stat(restoredPath); err != nil {
		t.Fatalf("arquivo restaurado não foi gerado em %s: %v", restoredPath, err)
	}
}

func TestVerifyRemoteCLI_Disabled(t *testing.T) {
	t.Setenv("ADMIN_MFA_ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("BACKUP_REMOTE_ENABLED", "false")
	err := runVerifyRemote([]string{"--key", "vorcarozap-test.db"})
	if err == nil {
		t.Fatal("esperava erro quando backup remoto está desabilitado")
	}
}

func TestVerifyRemoteCLI_MissingKey(t *testing.T) {
	t.Setenv("ADMIN_MFA_ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("BACKUP_REMOTE_ENABLED", "true")
	t.Setenv("BACKUP_REMOTE_BUCKET", "my-bucket")
	t.Setenv("BACKUP_REMOTE_ACCESS_KEY", "AKIA123")
	t.Setenv("BACKUP_REMOTE_SECRET_KEY", "SECRET123")
	t.Setenv("BACKUP_REMOTE_ENDPOINT", "https://s3.us-east-1.amazonaws.com")
	err := runVerifyRemote([]string{})
	if err == nil {
		t.Fatal("esperava erro quando --key não é fornecida")
	}
}

func TestRestoreSandboxCLI_MissingArgs(t *testing.T) {
	t.Setenv("ADMIN_MFA_ENCRYPTION_KEY", "12345678901234567890123456789012")
	err := runRestoreSandbox([]string{})
	if err == nil {
		t.Fatal("esperava erro quando nem --file nem --remote-key são fornecidos")
	}
}
