package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store"
)

func TestBackupAndVerify(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "original.db")
	backupPath := filepath.Join(tempDir, "backups", "snapshot.db")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Inicializa banco original e aplica migrations
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco original: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// Insere registro de teste
	_, err = db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, summary, relevance, relevance_rationale, updated_at)
		VALUES ('ent_backup_test', 'person', 'Entidade Backup Teste', 'entidade backup teste', 'entidade-backup-teste', 'Resumo de teste', 3, 'Justificativa', '2026-09-12T18:00:00Z');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir entidade de teste: %v", err)
	}

	// 2. Executa Backup
	res, err := store.Backup(ctx, db, backupPath)
	if err != nil {
		t.Fatalf("falha ao executar store.Backup: %v", err)
	}

	if res == nil {
		t.Fatal("BackupResult é nil")
	}
	if res.BackupPath != backupPath {
		t.Errorf("esperado BackupPath %q, obtido %q", backupPath, res.BackupPath)
	}
	if res.SizeBytes <= 0 {
		t.Errorf("esperado SizeBytes > 0, obtido %d", res.SizeBytes)
	}
	if !res.IntegrityOK {
		t.Error("esperado IntegrityOK = true")
	}
	if res.SchemaVersion <= 0 {
		t.Errorf("esperado SchemaVersion > 0, obtido %d", res.SchemaVersion)
	}

	// 3. Tentar fazer backup no mesmo caminho existente deve falhar (proteção contra sobrescrita)
	if _, err := store.Backup(ctx, db, backupPath); err == nil {
		t.Fatal("esperava erro ao tentar sobrescrever backup existente, obteve nil")
	}

	// 4. Executa VerifyBackup independentemente
	verifyRes, err := store.VerifyBackup(ctx, backupPath)
	if err != nil {
		t.Fatalf("falha ao executar VerifyBackup: %v", err)
	}
	if !verifyRes.IntegrityOK {
		t.Error("esperado VerifyBackup com IntegrityOK = true")
	}
	if verifyRes.SchemaVersion != res.SchemaVersion {
		t.Errorf("esperado versão %d, obtido %d", res.SchemaVersion, verifyRes.SchemaVersion)
	}

	// 5. Restaura o backup em um banco de teste temporário
	restoredPath := filepath.Join(tempDir, "restored.db")
	if err := store.Restore(ctx, backupPath, restoredPath); err != nil {
		t.Fatalf("falha ao executar Restore: %v", err)
	}

	// Abre o banco restaurado e verifica a entidade
	restoredDB, err := store.Open(ctx, restoredPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco restaurado: %v", err)
	}
	defer restoredDB.Close()

	var name string
	err = restoredDB.QueryRowContext(ctx, "SELECT name FROM entities WHERE id = 'ent_backup_test';").Scan(&name)
	if err != nil {
		t.Fatalf("falha ao consultar entidade no banco restaurado: %v", err)
	}
	if name != "Entidade Backup Teste" {
		t.Errorf("esperado nome 'Entidade Backup Teste', obtido %q", name)
	}
}

func TestVerifyBackup_InvalidFiles(t *testing.T) {
	tempDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Arquivo inexistente
	if _, err := store.VerifyBackup(ctx, filepath.Join(tempDir, "inexistente.db")); err == nil {
		t.Error("esperava erro para arquivo inexistente, obteve nil")
	}

	// 2. Arquivo vazio (0 bytes)
	emptyPath := filepath.Join(tempDir, "empty.db")
	if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
		t.Fatalf("falha ao criar arquivo vazio: %v", err)
	}
	if _, err := store.VerifyBackup(ctx, emptyPath); err == nil {
		t.Error("esperava erro para arquivo de 0 bytes, obteve nil")
	}

	// 3. Arquivo corrompido / lixo binário
	corruptPath := filepath.Join(tempDir, "corrupt.db")
	if err := os.WriteFile(corruptPath, []byte("NÃO É UM BANCO SQLITE VÁLIDO 1234567890"), 0644); err != nil {
		t.Fatalf("falha ao criar arquivo corrompido: %v", err)
	}
	if _, err := store.VerifyBackup(ctx, corruptPath); err == nil {
		t.Error("esperava erro de integridade para arquivo corrompido, obteve nil")
	}
}

func TestRestore_AtomicAndSafe(t *testing.T) {
	tempDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Cria um backup válido de origem
	srcDBPath := filepath.Join(tempDir, "source_original.db")
	srcDB, err := store.Open(ctx, srcDBPath)
	if err != nil {
		t.Fatalf("falha ao criar banco de origem: %v", err)
	}
	if err := store.Migrate(ctx, srcDB); err != nil {
		t.Fatalf("falha ao migrar banco de origem: %v", err)
	}
	_, err = srcDB.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, summary, relevance, relevance_rationale, updated_at)
		VALUES ('ent_restore_valid', 'person', 'Entidade Válida Restaurada', 'entidade valida restaurada', 'entidade-valida-restaurada', 'Resumo', 3, 'Justificativa', '2026-09-13T10:00:00Z');
	`)
	if err != nil {
		t.Fatalf("falha ao inserir na origem: %v", err)
	}
	validBackupPath := filepath.Join(tempDir, "valid_backup.db")
	if _, err := store.Backup(ctx, srcDB, validBackupPath); err != nil {
		t.Fatalf("falha ao gerar backup de origem: %v", err)
	}
	srcDB.Close()

	t.Run("Destino existente permanece 100% intacto quando a validação/cópia falha", func(t *testing.T) {
		destDir := filepath.Join(tempDir, "dest_fail_test")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("falha ao criar destDir: %v", err)
		}
		destPath := filepath.Join(destDir, "active.db")

		// Inicializa banco de destino com dados originais
		destDB, err := store.Open(ctx, destPath)
		if err != nil {
			t.Fatalf("falha ao abrir destDB: %v", err)
		}
		if err := store.Migrate(ctx, destDB); err != nil {
			t.Fatalf("falha ao migrar destDB: %v", err)
		}
		_, err = destDB.ExecContext(ctx, `
			INSERT INTO entities (id, type, name, normalized_name, slug, summary, relevance, relevance_rationale, updated_at)
			VALUES ('ent_original', 'person', 'Original Intacto', 'original intacto', 'original-intacto', 'Resumo', 3, 'Justificativa', '2026-09-13T10:00:00Z');
		`)
		if err != nil {
			t.Fatalf("falha ao inserir no destDB: %v", err)
		}
		destDB.Close()

		// Cria arquivo de backup corrompido
		corruptBackup := filepath.Join(tempDir, "corrupt_snap.db")
		_ = os.WriteFile(corruptBackup, []byte("LIXO BINARIO QUE NAO E BANCO"), 0644)

		// Tenta restaurar a partir do backup corrompido
		err = store.Restore(ctx, corruptBackup, destPath)
		if err == nil {
			t.Fatal("esperava erro ao tentar restaurar backup corrompido, obteve nil")
		}

		// Garante que o banco de destino original continua intacto e acessível
		destCheckDB, err := store.Open(ctx, destPath)
		if err != nil {
			t.Fatalf("falha ao reabrir destPath após falha de restore: %v", err)
		}
		defer destCheckDB.Close()

		var name string
		err = destCheckDB.QueryRowContext(ctx, "SELECT name FROM entities WHERE id = 'ent_original';").Scan(&name)
		if err != nil {
			t.Fatalf("falha ao consultar entidade original após falha de restore: %v", err)
		}
		if name != "Original Intacto" {
			t.Errorf("dado original foi corrompido! Obtido %q", name)
		}

		// Garante que nenhum arquivo temporário residual permaneceu no diretório
		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatalf("falha ao listar destDir: %v", err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "restore-") || strings.HasSuffix(e.Name(), ".tmp") {
				t.Errorf("arquivo temporário residual encontrado no destino: %s", e.Name())
			}
		}
	})

	t.Run("Restore bem-sucedido substitui o destino atomicamente e limpa temporários", func(t *testing.T) {
		destDir := filepath.Join(tempDir, "dest_success_test")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatalf("falha ao criar destDir: %v", err)
		}
		destPath := filepath.Join(destDir, "active_success.db")

		// Restaura backup válido
		if err := store.Restore(ctx, validBackupPath, destPath); err != nil {
			t.Fatalf("falha ao executar store.Restore: %v", err)
		}

		// Verifica que o banco restaurado funciona perfeitamente
		destDB, err := store.Open(ctx, destPath)
		if err != nil {
			t.Fatalf("falha ao abrir banco restaurado com sucesso: %v", err)
		}
		defer destDB.Close()

		var name string
		err = destDB.QueryRowContext(ctx, "SELECT name FROM entities WHERE id = 'ent_restore_valid';").Scan(&name)
		if err != nil {
			t.Fatalf("falha ao consultar entidade restaurada: %v", err)
		}
		if name != "Entidade Válida Restaurada" {
			t.Errorf("esperado 'Entidade Válida Restaurada', obtido %q", name)
		}

		// Verifica ausência de arquivos temporários residuais
		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatalf("falha ao listar destDir: %v", err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "restore-") || strings.HasSuffix(e.Name(), ".tmp") {
				t.Errorf("arquivo temporário residual encontrado após sucesso: %s", e.Name())
			}
		}
	})

	t.Run("Arquivos WAL e SHM residuais são removidos sem contaminar o banco restaurado", func(t *testing.T) {
		destDir := filepath.Join(tempDir, "dest_wal_test")
		_ = os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, "wal_target.db")

		// Cria arquivos residuais de teste
		walPath := destPath + "-wal"
		shmPath := destPath + "-shm"
		_ = os.WriteFile(walPath, []byte("WAL_RESIDUAL_CORROMPIDO_12345"), 0644)
		_ = os.WriteFile(shmPath, []byte("SHM_RESIDUAL_CORROMPIDO_12345"), 0644)

		if err := store.Restore(ctx, validBackupPath, destPath); err != nil {
			t.Fatalf("falha ao restaurar banco sobre arquivos WAL/SHM: %v", err)
		}

		// Abre o banco restaurado e confirma que lê normalmente sem ser contaminado
		walDB, err := store.Open(ctx, destPath)
		if err != nil {
			t.Fatalf("falha ao abrir banco restaurado: %v", err)
		}
		defer walDB.Close()

		var name string
		err = walDB.QueryRowContext(ctx, "SELECT name FROM entities WHERE id = 'ent_restore_valid';").Scan(&name)
		if err != nil {
			t.Fatalf("falha ao consultar banco restaurado: %v", err)
		}
		if name != "Entidade Válida Restaurada" {
			t.Errorf("esperado 'Entidade Válida Restaurada', obtido %q", name)
		}
	})

	t.Run("Rejeição de origem e destino idênticos e symlinks", func(t *testing.T) {
		// 1. Mesmo caminho
		if err := store.Restore(ctx, validBackupPath, validBackupPath); err == nil {
			t.Error("esperava erro ao restaurar com origem e destino idênticos, obteve nil")
		}

		// 2. Destino como symlink
		symlinkTarget := filepath.Join(tempDir, "symlink_target.db")
		realTarget := filepath.Join(tempDir, "real_target.db")
		_ = os.WriteFile(realTarget, []byte("real"), 0644)
		if err := os.Symlink(realTarget, symlinkTarget); err == nil {
			if err := store.Restore(ctx, validBackupPath, symlinkTarget); err == nil {
				t.Error("esperava erro de segurança ao tentar restaurar em symlink, obteve nil")
			}
		}
	})
}

func TestRestore_ValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Parâmetros vazios
	if err := store.Restore(ctx, "", filepath.Join(tempDir, "dest.db")); err == nil {
		t.Error("esperava erro para backupPath vazio, obteve nil")
	}
	if err := store.Restore(ctx, filepath.Join(tempDir, "src.db"), ""); err == nil {
		t.Error("esperava erro para targetPath vazio, obteve nil")
	}

	// 2. Origem e destino iguais
	samePath := filepath.Join(tempDir, "same.db")
	if err := store.Restore(ctx, samePath, samePath); err == nil {
		t.Error("esperava erro para caminhos iguais, obteve nil")
	}
}

func TestRotateBackups_TableDriven(t *testing.T) {
	t.Run("Rejeição de contagem negativa", func(t *testing.T) {
		tempDir := t.TempDir()
		_, err := store.RotateBackups(tempDir, -1)
		if err == nil {
			t.Error("esperava erro ao passar keepCount negativo (-1), obteve nil")
		}
	})

	t.Run("Contagem zero desabilita rotação", func(t *testing.T) {
		tempDir := t.TempDir()
		file1 := filepath.Join(tempDir, "vorcarozap-20260901_100000Z.db")
		if err := os.WriteFile(file1, []byte("banco1"), 0644); err != nil {
			t.Fatalf("falha ao criar arquivo: %v", err)
		}

		removed, err := store.RotateBackups(tempDir, 0)
		if err != nil {
			t.Fatalf("erro inesperado com keepCount=0: %v", err)
		}
		if len(removed) != 0 {
			t.Errorf("esperava 0 arquivos removidos com keepCount=0, obteve %d", len(removed))
		}
		if _, err := os.Stat(file1); os.IsNotExist(err) {
			t.Error("arquivo não deveria ter sido removido com keepCount=0")
		}
	})

	t.Run("Preservação dos N backups mais recentes e remoção dos excedentes", func(t *testing.T) {
		tempDir := t.TempDir()
		now := time.Now().UTC()

		// Cria 5 backups com timestamps decrescentes
		files := []string{
			filepath.Join(tempDir, "vorcarozap-20260912_120000Z.db"), // Mais recente (index 0)
			filepath.Join(tempDir, "vorcarozap-20260911_120000Z.db"), // (index 1)
			filepath.Join(tempDir, "vorcarozap-20260910_120000Z.db"), // (index 2)
			filepath.Join(tempDir, "vorcarozap-20260909_120000Z.db"), // Excedente (index 3)
			filepath.Join(tempDir, "vorcarozap-20260908_120000Z.db"), // Excedente (index 4)
		}

		for i, f := range files {
			if err := os.WriteFile(f, []byte("conteudo"), 0644); err != nil {
				t.Fatalf("falha ao criar arquivo %s: %v", f, err)
			}
			mtime := now.Add(-time.Duration(i) * time.Hour)
			if err := os.Chtimes(f, mtime, mtime); err != nil {
				t.Fatalf("falha ao alterar mtime: %v", err)
			}
		}

		// Mantém 3 mais recentes
		removed, err := store.RotateBackups(tempDir, 3)
		if err != nil {
			t.Fatalf("falha ao executar RotateBackups: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("esperado 2 arquivos removidos, obtido %d", len(removed))
		}

		// Verifica que os 3 mais recentes ainda existem
		for _, kept := range files[:3] {
			if _, err := os.Stat(kept); os.IsNotExist(err) {
				t.Errorf("arquivo recente %s foi removido indevidamente", kept)
			}
		}

		// Verifica que os 2 mais antigos foram removidos
		for _, rem := range files[3:] {
			if _, err := os.Stat(rem); !os.IsNotExist(err) {
				t.Errorf("arquivo excedente %s não foi removido", rem)
			}
		}
	})

	t.Run("Arquivos fora do padrão vorcarozap-*.db não são removidos", func(t *testing.T) {
		tempDir := t.TempDir()
		now := time.Now().UTC()

		// Cria backups padrão
		b1 := filepath.Join(tempDir, "vorcarozap-20260912_100000Z.db")
		b2 := filepath.Join(tempDir, "vorcarozap-20260911_100000Z.db")
		_ = os.WriteFile(b1, []byte("b1"), 0644)
		_ = os.Chtimes(b1, now, now)
		_ = os.WriteFile(b2, []byte("b2"), 0644)
		_ = os.Chtimes(b2, now.Add(-24*time.Hour), now.Add(-24*time.Hour))

		// Cria arquivos especiais fora do padrão
		otherFiles := []string{
			filepath.Join(tempDir, "vorcarozap.db"),
			filepath.Join(tempDir, "vorcarozap.db-wal"),
			filepath.Join(tempDir, "backup_manual.db"),
			filepath.Join(tempDir, "vorcarozap-notas.txt"),
			filepath.Join(tempDir, "config.yaml"),
		}

		for _, of := range otherFiles {
			if err := os.WriteFile(of, []byte("outros dados"), 0644); err != nil {
				t.Fatalf("falha ao criar arquivo extra: %v", err)
			}
			oldTime := now.Add(-48 * time.Hour)
			_ = os.Chtimes(of, oldTime, oldTime)
		}

		// Retém apenas 1 backup padrão
		removed, err := store.RotateBackups(tempDir, 1)
		if err != nil {
			t.Fatalf("falha ao rotacionar: %v", err)
		}

		if len(removed) != 1 || removed[0] != b2 {
			t.Fatalf("esperava remoção de apenas %s, obtido %v", b2, removed)
		}

		// Todos os outros arquivos devem permanecer intactos
		for _, of := range otherFiles {
			if _, err := os.Stat(of); os.IsNotExist(err) {
				t.Errorf("arquivo fora do padrão %s foi removido indevidamente!", of)
			}
		}
	})

	t.Run("Links simbólicos (symlinks) não são seguidos nem removidos", func(t *testing.T) {
		tempDir := t.TempDir()

		targetFile := filepath.Join(tempDir, "real_file.db")
		if err := os.WriteFile(targetFile, []byte("conteudo real"), 0644); err != nil {
			t.Fatalf("falha ao criar arquivo alvo: %v", err)
		}

		symlinkPath := filepath.Join(tempDir, "vorcarozap-20260901_000000Z.db")
		if err := os.Symlink(targetFile, symlinkPath); err != nil {
			t.Skipf("symlink não suportado no ambiente: %v", err)
		}

		// Executa rotação com keepCount 1
		removed, err := store.RotateBackups(tempDir, 1)
		if err != nil {
			t.Fatalf("falha ao rotacionar com symlink: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("symlink não deveria ter sido considerado na rotação, removidos: %v", removed)
		}

		if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
			t.Error("symlink foi indevidamente removido")
		}
		if _, err := os.Stat(targetFile); os.IsNotExist(err) {
			t.Error("arquivo alvo do symlink foi indevidamente removido")
		}
	})
}
