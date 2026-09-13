package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupResult contém os metadados e o diagnóstico de um arquivo de backup SQLite.
type BackupResult struct {
	BackupPath    string
	SizeBytes     int64
	IntegrityOK   bool
	SchemaVersion int64
	CreatedAt     time.Time
}

// Backup gera um snapshot consistente do banco SQLite ativo utilizando o comando nativo VACUUM INTO.
//
// Distinção técnica de atomicidade:
//  1. No nível do SQLite: VACUUM INTO produz um snapshot transacionalmente consistente da base no ponto
//     da sua execução, capturando todas as transações comitadas no WAL (-wal e -shm) sem interromper leituras.
//  2. No nível do sistema de arquivos: Para garantir que o arquivo de destino final seja atômico, completo e
//     100% verificado (sem risco de leitores acessarem um arquivo em geração ou corrompido em caso de falha),
//     o backup é primeiramente gerado em um arquivo temporário exclusivo (.tmp.<timestamp>), verificado
//     forensicamente com PRAGMA integrity_check e CheckReady, e apenas após validação renomeado
//     atomicamente (os.Rename) para o destino definitivo.
//
// Proteções de segurança:
// - Nunca sobrescreve um arquivo de backup já existente no destino.
// - Remove arquivos temporários parciais em caso de cancelamento de contexto ou erro de integridade.
func Backup(ctx context.Context, db *sql.DB, destPath string) (*BackupResult, error) {
	if strings.TrimSpace(destPath) == "" {
		return nil, fmt.Errorf("store: caminho de destino do backup não informado")
	}

	cleanDest := filepath.Clean(destPath)
	dir := filepath.Dir(cleanDest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("store: falha ao criar diretório de backup %q: %w", dir, err)
	}

	// Proteção contra sobrescrita acidental de backups existentes
	if _, err := os.Stat(cleanDest); err == nil {
		return nil, fmt.Errorf("store: arquivo de backup já existe em %q; remova ou especifique outro destino", cleanDest)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("store: erro ao inspecionar destino de backup %q: %w", cleanDest, err)
	}

	// Criação de arquivo temporário exclusivo no mesmo diretório para garantir renomeação atômica (mesmo filesystem)
	tmpPath := fmt.Sprintf("%s.tmp.%d", cleanDest, time.Now().UnixNano())
	_ = os.Remove(tmpPath) // Garante que não existe resíduo

	// Executa VACUUM INTO nativo do SQLite gerando o snapshot consistente no arquivo temporário
	escapedTmpPath := strings.ReplaceAll(filepath.ToSlash(tmpPath), "'", "''")
	vacuumQuery := fmt.Sprintf("VACUUM INTO '%s';", escapedTmpPath)

	if _, err := db.ExecContext(ctx, vacuumQuery); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("store: falha ao executar VACUUM INTO: %w", err)
	}

	// Validação forense imediata do snapshot temporário antes de torná-lo público ou definitivo
	result, err := VerifyBackup(ctx, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("store: snapshot gerado em %q reprovou na verificação de integridade: %w", tmpPath, err)
	}

	// Verifica novamente se o destino final não foi criado por processo concorrente
	if _, err := os.Stat(cleanDest); err == nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("store: arquivo de destino %q foi criado concorrentemente", cleanDest)
	}

	// Renomeação atômica no filesystem: o backup definitivo só existe após ser validado
	if err := os.Rename(tmpPath, cleanDest); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("store: falha ao renomear snapshot validado para destino final: %w", err)
	}

	result.BackupPath = cleanDest
	return result, nil
}

// VerifyBackup abre o arquivo de backup SQLite isoladamente, executa PRAGMA integrity_check
// e valida se todas as migrations esperadas pelo binário estão aplicadas.
func VerifyBackup(ctx context.Context, backupPath string) (*BackupResult, error) {
	if strings.TrimSpace(backupPath) == "" {
		return nil, fmt.Errorf("store: caminho do arquivo de backup não informado")
	}

	cleanPath := filepath.Clean(backupPath)
	fi, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("store: arquivo de backup não encontrado em %q: %w", cleanPath, err)
	}

	if fi.IsDir() {
		return nil, fmt.Errorf("store: caminho %q é um diretório, esperado arquivo de banco", cleanPath)
	}

	if fi.Size() == 0 {
		return nil, fmt.Errorf("store: arquivo de backup em %q está vazio (0 bytes)", cleanPath)
	}

	// Abre conexão com o banco de backup isolado
	backupDB, err := Open(ctx, cleanPath)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao abrir arquivo de backup para verificação: %w", err)
	}
	defer backupDB.Close()

	// 1. Executa PRAGMA integrity_check
	var integrityResult string
	if err := backupDB.QueryRowContext(ctx, "PRAGMA integrity_check;").Scan(&integrityResult); err != nil {
		return nil, fmt.Errorf("store: falha ao executar PRAGMA integrity_check: %w", err)
	}

	if !strings.EqualFold(integrityResult, "ok") {
		return nil, fmt.Errorf("store: integridade corrompida no backup: %s", integrityResult)
	}

	// 2. Valida compatibilidade de migrations com o binário
	if err := CheckReady(ctx, backupDB); err != nil {
		return nil, fmt.Errorf("store: validação de migrations falhou no backup: %w", err)
	}

	// 3. Obtém versão aplicada do Goose
	p, err := newGooseProvider(backupDB)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao inspecionar versão do banco: %w", err)
	}

	currentVersion, _, err := p.GetVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao consultar versão de migrations: %w", err)
	}

	return &BackupResult{
		BackupPath:    cleanPath,
		SizeBytes:     fi.Size(),
		IntegrityOK:   true,
		SchemaVersion: currentVersion,
		CreatedAt:     fi.ModTime().UTC(),
	}, nil
}

// RotateBackups aplica a política de retenção em um diretório de backups, mantendo os keepCount arquivos
// mais recentes que atendam estritamente ao padrão "vorcarozap-*.db".
//
// Invariantes de segurança:
// 1. keepCount deve ser >= 0. Se keepCount == 0, a rotação é considerada desabilitada (nenhuma remoção).
// 2. Somente arquivos regulares correspondendo ao padrão "vorcarozap-*.db" são considerados.
// 3. Symlinks não são seguidos nem removidos (inspecionados via os.Lstat).
// 4. Nenhum arquivo fora do diretório configurado é afetado.
// 5. Retorna a lista dos caminhos de arquivos removidos com sucesso.
func RotateBackups(dir string, keepCount int) ([]string, error) {
	if keepCount < 0 {
		return nil, fmt.Errorf("store: keepCount deve ser um número inteiro não negativo (recebido %d)", keepCount)
	}

	if keepCount == 0 {
		return nil, nil // Rotação desabilitada
	}

	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("store: diretório de backup não informado")
	}

	cleanDir := filepath.Clean(dir)
	dirInfo, err := os.Stat(cleanDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: erro ao inspecionar diretório de backups %q: %w", cleanDir, err)
	}

	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("store: caminho %q não é um diretório", cleanDir)
	}

	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("store: falha ao listar arquivos de backup em %q: %w", cleanDir, err)
	}

	type backupFileInfo struct {
		path    string
		name    string
		modTime time.Time
	}

	var matchingFiles []backupFileInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		matched, err := filepath.Match("vorcarozap-*.db", name)
		if err != nil || !matched {
			continue
		}

		fullPath := filepath.Join(cleanDir, name)

		// Inspeciona via os.Lstat para rejeitar symlinks
		lfi, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		if lfi.Mode()&os.ModeSymlink != 0 {
			// Não seguir nem remover links simbólicos
			continue
		}

		matchingFiles = append(matchingFiles, backupFileInfo{
			path:    fullPath,
			name:    name,
			modTime: lfi.ModTime().UTC(),
		})
	}

	// Ordena decrescente por ModTime (mais recentes primeiro), desempate por nome decrescente
	sort.Slice(matchingFiles, func(i, j int) bool {
		if matchingFiles[i].modTime.Equal(matchingFiles[j].modTime) {
			return matchingFiles[i].name > matchingFiles[j].name
		}
		return matchingFiles[i].modTime.After(matchingFiles[j].modTime)
	})

	if len(matchingFiles) <= keepCount {
		return nil, nil
	}

	var removed []string
	for _, file := range matchingFiles[keepCount:] {
		// Invariante de segurança: certificar que o arquivo está no diretório correto
		if filepath.Dir(file.path) != cleanDir {
			continue
		}

		if err := os.Remove(file.path); err != nil {
			return removed, fmt.Errorf("store: falha ao remover backup antigo %q: %w", file.path, err)
		}
		removed = append(removed, file.path)
	}

	return removed, nil
}

// Restore restaura de forma atômica e segura o conteúdo de um backup verificado para o caminho de destino.
//
// Protocolo operacional de segurança:
//  1. Em produção, a aplicação deve estar parada antes do restore e um backup prévio deve ser realizado.
//  2. Origem e destino não podem ser o mesmo arquivo nem apontar para o mesmo arquivo canônico no disco.
//  3. O backup de origem é preliminarmente verificado com VerifyBackup.
//  4. A cópia é feita primeiramente para um arquivo temporário exclusivo criado via os.CreateTemp no mesmo
//     diretório do destino (garantindo permissões restritivas e que os.Rename seja uma operação atômica no mesmo filesystem).
//  5. O arquivo temporário é sincronizado com o disco (Sync), fechado e validado com VerifyBackup.
//  6. Somente após a validação bem-sucedida do temporário:
//     - Os arquivos residuais -wal e -shm do destino são removidos.
//     - O arquivo temporário é renomeado atomicamente (os.Rename) para o destino final definitivo.
//  7. Se qualquer erro ocorrer antes da renomeação, o arquivo temporário é removido e o destino original
//     permanece 100% intacto e inalterado.
func Restore(ctx context.Context, backupPath, targetDBPath string) error {
	if strings.TrimSpace(backupPath) == "" {
		return fmt.Errorf("store: caminho do backup de origem não informado")
	}
	if strings.TrimSpace(targetDBPath) == "" {
		return fmt.Errorf("store: caminho do banco de destino não informado")
	}

	cleanBackup := filepath.Clean(backupPath)
	cleanTarget := filepath.Clean(targetDBPath)

	if cleanBackup == cleanTarget {
		return fmt.Errorf("store: caminho de origem e destino são idênticos (%s)", cleanBackup)
	}

	// 1. Inspeciona arquivo de origem
	srcInfo, err := os.Stat(cleanBackup)
	if err != nil {
		return fmt.Errorf("store: backup de origem não encontrado em %q: %w", cleanBackup, err)
	}

	// 2. Proteção contra symlinks que apontem para o mesmo arquivo físico
	if targetInfo, err := os.Stat(cleanTarget); err == nil {
		if os.SameFile(srcInfo, targetInfo) {
			return fmt.Errorf("store: caminho de origem e destino apontam para o mesmo arquivo físico (%s)", cleanBackup)
		}
	}

	// 3. Rejeita se o destino for um link simbólico
	if targetLfi, err := os.Lstat(cleanTarget); err == nil && targetLfi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("store: caminho de destino %q é um link simbólico; restauração abortada por segurança", cleanTarget)
	}

	// 4. Verifica integridade do backup de origem antes de qualquer operação
	if _, err := VerifyBackup(ctx, cleanBackup); err != nil {
		return fmt.Errorf("store: backup de origem inválido: %w", err)
	}

	// 5. Garante existência do diretório de destino
	targetDir := filepath.Dir(cleanTarget)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("store: falha ao criar diretório de destino %q: %w", targetDir, err)
	}

	// 6. Cria arquivo temporário exclusivo no mesmo diretório do destino com permissões restritivas
	tmpFile, err := os.CreateTemp(targetDir, "restore-*.tmp")
	if err != nil {
		return fmt.Errorf("store: falha ao criar arquivo temporário de restauração em %q: %w", targetDir, err)
	}
	tmpPath := tmpFile.Name()

	var success bool
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
			_ = os.Remove(tmpPath + "-wal")
			_ = os.Remove(tmpPath + "-shm")
		}
	}()

	// 7. Abre o backup de origem para leitura
	srcFile, err := os.Open(cleanBackup)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("store: falha ao abrir backup para leitura: %w", err)
	}
	defer srcFile.Close()

	// 8. Copia dados para o arquivo temporário isolado
	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("store: falha ao copiar dados do backup: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("store: falha ao sincronizar arquivo temporário com disco: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("store: falha ao fechar arquivo temporário: %w", err)
	}

	// 9. Validação forense completa do arquivo temporário restaurado antes de tocar no destino
	if _, err := VerifyBackup(ctx, tmpPath); err != nil {
		return fmt.Errorf("store: arquivo temporário restaurado em %q reprovou na verificação de integridade: %w", tmpPath, err)
	}

	// 10. Remove eventuais arquivos WAL/SHM residuais do destino para evitar contaminação
	_ = os.Remove(cleanTarget + "-wal")
	_ = os.Remove(cleanTarget + "-shm")

	// 11. Renomeação atômica no filesystem: o destino só é substituído após validação completa
	if err := os.Rename(tmpPath, cleanTarget); err != nil {
		return fmt.Errorf("store: falha ao renomear arquivo temporário para o destino definitivo %q: %w", cleanTarget, err)
	}

	_ = os.Chmod(cleanTarget, 0644)
	success = true
	return nil
}
