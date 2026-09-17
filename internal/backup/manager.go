package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/observability"
	"github.com/raphaieu/vorcarozap/internal/store"
)

// BackupExecutionReport resume a execução combinada do backup local e remoto.
type BackupExecutionReport struct {
	LocalSuccess       bool
	LocalPath          string
	LocalSizeBytes     int64
	LocalSchemaVersion int64
	LocalDuration      time.Duration
	LocalRotated       []string
	RemoteAttempted    bool
	RemoteSuccess      bool
	RemoteKey          string
	RemoteProvider     string
	RemoteBucket       string
	RemoteSHA256       string
	RemoteDuration     time.Duration
	RemoteRotated      []string
	RemoteError        error
}

// RestoreSandboxReport resume o diagnóstico da restauração não-destrutiva em sandbox.
type RestoreSandboxReport struct {
	SourceType        string // "local" ou "remote"
	SourceIdentifier  string
	SandboxPath       string
	SizeBytes         int64
	SchemaVersion     int64
	TotalClaims       int64
	TotalEntities     int64
	IntegrityVerified bool
	Duration          time.Duration
}

// ManagerConfig define as dependências e configurações para o BackupManager.
type ManagerConfig struct {
	DB       *sql.DB
	Config   *config.Config
	Registry *observability.MetricsRegistry
	Provider RemoteStorageProvider
}

// BackupManager orquestra snapshots locais e remotos com isolamento de falhas.
type BackupManager struct {
	db       *sql.DB
	cfg      *config.Config
	registry *observability.MetricsRegistry
	provider RemoteStorageProvider
}

// NewManager instancia um novo BackupManager.
func NewManager(cfg ManagerConfig) (*BackupManager, error) {
	if cfg.Config == nil {
		return nil, fmt.Errorf("backup: configuração é obrigatória")
	}

	provider := cfg.Provider
	if provider == nil && cfg.Config.BackupRemoteEnabled {
		p, err := NewS3Provider(S3Config{
			Endpoint:  cfg.Config.BackupRemoteEndpoint,
			Region:    cfg.Config.BackupRemoteRegion,
			Bucket:    cfg.Config.BackupRemoteBucket,
			AccessKey: cfg.Config.BackupRemoteAccessKey,
			SecretKey: cfg.Config.BackupRemoteSecretKey,
			Timeout:   cfg.Config.BackupRemoteTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("backup: falha ao inicializar provedor S3: %w", err)
		}
		provider = p
	}

	reg := cfg.Registry
	if reg == nil {
		reg = observability.GetRegistry()
	}

	return &BackupManager{
		db:       cfg.DB,
		cfg:      cfg.Config,
		registry: reg,
		provider: provider,
	}, nil
}

// RunBackup executa o ciclo completo de backup local consistente e envio remoto opcional.
func (m *BackupManager) RunBackup(ctx context.Context, destPath string, localRetention int, forceRemote bool) (*BackupExecutionReport, error) {
	if destPath == "" {
		timestamp := time.Now().UTC().Format("20060102_150405Z")
		if _, err := os.Stat("/backups"); err == nil {
			destPath = fmt.Sprintf("/backups/vorcarozap-%s.db", timestamp)
		} else {
			destPath = fmt.Sprintf("backups/vorcarozap-%s.db", timestamp)
		}
	}

	report := &BackupExecutionReport{
		LocalPath: destPath,
	}

	// 1. Snapshot Local Consistente via VACUUM INTO
	localStart := time.Now()
	res, err := store.Backup(ctx, m.db, destPath)
	localDuration := time.Since(localStart)
	report.LocalDuration = localDuration

	if err != nil {
		m.registry.RecordLocalBackup(0, localDuration, 0, err)
		return nil, fmt.Errorf("backup: falha na geração local: %w", err)
	}

	report.LocalSuccess = true
	report.LocalPath = res.BackupPath
	report.LocalSizeBytes = res.SizeBytes
	report.LocalSchemaVersion = res.SchemaVersion

	// 2. Rotação Local
	if localRetention > 0 {
		backupDir := filepath.Dir(res.BackupPath)
		rotated, rotErr := store.RotateBackups(backupDir, localRetention)
		if rotErr != nil {
			return nil, fmt.Errorf("backup gerado com sucesso em %q, mas a política de retenção falhou: %w", res.BackupPath, rotErr)
		}
		report.LocalRotated = rotated
	}

	m.registry.RecordLocalBackup(res.SizeBytes, localDuration, res.SchemaVersion, nil)

	// 3. Envio Remoto Seguro (Se habilitado ou forçado)
	shouldUploadRemote := m.cfg.BackupRemoteEnabled || forceRemote
	if !shouldUploadRemote {
		return report, nil
	}

	report.RemoteAttempted = true
	report.RemoteProvider = m.cfg.BackupRemoteProvider
	report.RemoteBucket = m.cfg.BackupRemoteBucket

	if m.provider == nil {
		p, err := NewS3Provider(S3Config{
			Endpoint:  m.cfg.BackupRemoteEndpoint,
			Region:    m.cfg.BackupRemoteRegion,
			Bucket:    m.cfg.BackupRemoteBucket,
			AccessKey: m.cfg.BackupRemoteAccessKey,
			SecretKey: m.cfg.BackupRemoteSecretKey,
			Timeout:   m.cfg.BackupRemoteTimeout,
		})
		if err != nil {
			report.RemoteError = fmt.Errorf("inicialização do provedor remoto falhou: %w", err)
			slog.Error("falha ao inicializar provedor remoto de backup", "error", report.RemoteError)
			m.registry.RecordRemoteBackup(0, 0, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, "", "", report.RemoteError)
			// Isolamento de falha: não propaga erro para manter sucesso local
			return report, nil
		}
		m.provider = p
	}

	remoteStart := time.Now()
	remoteKey := filepath.Base(res.BackupPath)
	if m.cfg.BackupRemotePrefix != "" {
		remoteKey = strings.Trim(m.cfg.BackupRemotePrefix, "/") + "/" + remoteKey
	}
	report.RemoteKey = remoteKey

	// 3.1 Abre arquivo local e calcula SHA-256
	localFile, err := os.Open(res.BackupPath)
	if err != nil {
		report.RemoteError = fmt.Errorf("abertura de backup local para upload remoto falhou: %w", err)
		slog.Error("falha ao abrir backup local para envio remoto", "error", report.RemoteError)
		m.registry.RecordRemoteBackup(0, 0, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, "", report.RemoteError)
		return report, nil
	}
	defer localFile.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, localFile); err != nil {
		report.RemoteError = fmt.Errorf("cálculo de checksum SHA256 falhou: %w", err)
		slog.Error("falha ao calcular SHA256 do backup", "error", report.RemoteError)
		m.registry.RecordRemoteBackup(0, 0, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, "", report.RemoteError)
		return report, nil
	}
	sha256Hex := hex.EncodeToString(hasher.Sum(nil))
	report.RemoteSHA256 = sha256Hex

	if _, err := localFile.Seek(0, io.SeekStart); err != nil {
		report.RemoteError = fmt.Errorf("reset de cursor no arquivo falhou: %w", err)
		slog.Error("falha no seek do arquivo", "error", report.RemoteError)
		m.registry.RecordRemoteBackup(0, 0, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, sha256Hex, report.RemoteError)
		return report, nil
	}

	// 3.2 Upload Remoto com Metadados
	meta := map[string]string{
		"created-at":     res.CreatedAt.Format(time.RFC3339),
		"schema-version": fmt.Sprintf("%d", res.SchemaVersion),
	}

	uploadErr := m.provider.PutObject(ctx, remoteKey, localFile, res.SizeBytes, sha256Hex, meta)
	remoteDuration := time.Since(remoteStart)
	report.RemoteDuration = remoteDuration

	if uploadErr != nil {
		report.RemoteError = fmt.Errorf("upload remoto falhou: %w", uploadErr)
		slog.Error("erro no envio de backup para Object Storage", "remote_key", remoteKey, "error", report.RemoteError)
		m.registry.RecordRemoteBackup(0, remoteDuration, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, sha256Hex, report.RemoteError)
		// Isolamento de falha: backup local é preservado
		return report, nil
	}

	// 3.3 Verificação Remota via HeadObject (Tamanho e Checksum SHA-256)
	headMeta, headErr := m.provider.HeadObject(ctx, remoteKey)
	if headErr != nil {
		report.RemoteError = fmt.Errorf("verificação remota pós-upload falhou: %w", headErr)
		slog.Warn("aviso: verificação pós-upload remoto falhou", "remote_key", remoteKey, "error", report.RemoteError)
		m.registry.RecordRemoteBackup(res.SizeBytes, remoteDuration, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, sha256Hex, report.RemoteError)
		return report, nil
	}

	if headMeta.SizeBytes != res.SizeBytes {
		report.RemoteError = fmt.Errorf("inconsistência de tamanho: local=%d bytes, remoto=%d bytes", res.SizeBytes, headMeta.SizeBytes)
		slog.Error("inconsistência de tamanho no backup remoto", "error", report.RemoteError)
		m.registry.RecordRemoteBackup(headMeta.SizeBytes, remoteDuration, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, sha256Hex, report.RemoteError)
		return report, nil
	}

	if headMeta.SHA256Hex != "" && headMeta.SHA256Hex != sha256Hex {
		report.RemoteError = fmt.Errorf("inconsistência de checksum SHA-256: local=%s, remoto=%s", sha256Hex, headMeta.SHA256Hex)
		slog.Error("inconsistência de checksum no backup remoto", "error", report.RemoteError)
		m.registry.RecordRemoteBackup(headMeta.SizeBytes, remoteDuration, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, sha256Hex, report.RemoteError)
		return report, nil
	}

	report.RemoteSuccess = true
	m.registry.RecordRemoteBackup(res.SizeBytes, remoteDuration, m.cfg.BackupRemoteProvider, m.cfg.BackupRemoteBucket, remoteKey, sha256Hex, nil)

	// 3.4 Rotação Remota
	if m.cfg.BackupRemoteRetentionCount > 0 {
		rotatedRemote, rotErr := m.RotateRemoteBackups(ctx, m.cfg.BackupRemoteRetentionCount)
		if rotErr != nil {
			slog.Warn("aviso: rotação remota de backups falhou", "error", rotErr)
		} else {
			report.RemoteRotated = rotatedRemote
		}
	}

	return report, nil
}

// RotateRemoteBackups aplica a retenção no Object Storage mantendo os N backups mais recentes.
func (m *BackupManager) RotateRemoteBackups(ctx context.Context, keepCount int) ([]string, error) {
	if keepCount <= 0 || m.provider == nil {
		return nil, nil
	}

	objects, err := m.provider.ListObjects(ctx, m.cfg.BackupRemotePrefix)
	if err != nil {
		return nil, fmt.Errorf("backup: falha ao listar objetos remotos: %w", err)
	}

	type backupObj struct {
		key          string
		lastModified time.Time
	}

	var matching []backupObj
	for _, obj := range objects {
		base := filepath.Base(obj.Key)
		matched, _ := filepath.Match("vorcarozap-*.db", base)
		if matched {
			matching = append(matching, backupObj{
				key:          obj.Key,
				lastModified: obj.LastModified,
			})
		}
	}

	sort.Slice(matching, func(i, j int) bool {
		return matching[i].lastModified.After(matching[j].lastModified)
	})

	if len(matching) <= keepCount {
		return nil, nil
	}

	var deleted []string
	for _, oldObj := range matching[keepCount:] {
		if err := m.provider.DeleteObject(ctx, oldObj.key); err != nil {
			return deleted, fmt.Errorf("backup: falha ao deletar objeto remoto obsoleto %q: %w", oldObj.key, err)
		}
		deleted = append(deleted, oldObj.key)
	}

	return deleted, nil
}

// VerifyRemote valida a integridade e metadados de um snapshot no Object Storage.
func (m *BackupManager) VerifyRemote(ctx context.Context, key string) (*ObjectMetadata, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("backup: provedor remoto não inicializado")
	}
	return m.provider.HeadObject(ctx, key)
}

// RestoreSandbox executa restauração não-destrutiva em sandbox isolado sem alterar o banco de produção.
func (m *BackupManager) RestoreSandbox(ctx context.Context, sourceLocalPath, sourceRemoteKey string) (*RestoreSandboxReport, error) {
	start := time.Now()

	sourceFile := sourceLocalPath
	sourceType := "local"
	sourceID := sourceLocalPath

	if sourceRemoteKey != "" {
		sourceType = "remote"
		sourceID = sourceRemoteKey

		if m.provider == nil {
			return nil, fmt.Errorf("backup: provedor remoto não inicializado")
		}

		body, _, err := m.provider.GetObject(ctx, sourceRemoteKey)
		if err != nil {
			return nil, fmt.Errorf("backup: falha ao baixar objeto remoto %q: %w", sourceRemoteKey, err)
		}
		defer body.Close()

		tmpDownload, err := os.CreateTemp("", "vorcarozap_remote_download_*.db")
		if err != nil {
			return nil, fmt.Errorf("backup: falha ao criar arquivo temporário de download: %w", err)
		}
		tmpPath := tmpDownload.Name()
		defer os.Remove(tmpPath)

		if _, err := io.Copy(tmpDownload, body); err != nil {
			_ = tmpDownload.Close()
			return nil, fmt.Errorf("backup: falha ao salvar dados baixados: %w", err)
		}
		_ = tmpDownload.Close()
		sourceFile = tmpPath
	}

	if sourceFile == "" {
		return nil, fmt.Errorf("backup: origem de restauração não informada (informe caminho local ou chave remota)")
	}

	// Cria caminho de banco temporário sandbox
	sandboxFile, err := os.CreateTemp("", "vorcarozap_sandbox_*.db")
	if err != nil {
		return nil, fmt.Errorf("backup: falha ao criar arquivo temporário de sandbox: %w", err)
	}
	sandboxPath := sandboxFile.Name()
	_ = sandboxFile.Close()
	_ = os.Remove(sandboxPath) // Remove para que store.Restore crie o arquivo com segurança

	defer func() {
		_ = os.Remove(sandboxPath)
		_ = os.Remove(sandboxPath + "-wal")
		_ = os.Remove(sandboxPath + "-shm")
	}()

	// Executa restauração segura no sandbox
	if err := store.Restore(ctx, sourceFile, sandboxPath); err != nil {
		return nil, fmt.Errorf("backup: restauração no sandbox falhou: %w", err)
	}

	// Valida integridade e migrations no sandbox
	verifyRes, err := store.VerifyBackup(ctx, sandboxPath)
	if err != nil {
		return nil, fmt.Errorf("backup: verificação de integridade no sandbox falhou: %w", err)
	}

	// Abre sandbox para consulta operacional de sanidade
	sandboxDB, err := store.Open(ctx, sandboxPath)
	if err != nil {
		return nil, fmt.Errorf("backup: abertura do sandbox para consulta de teste falhou: %w", err)
	}
	defer sandboxDB.Close()

	var totalClaims, totalEntities int64
	_ = sandboxDB.QueryRowContext(ctx, "SELECT count(*) FROM claims;").Scan(&totalClaims)
	_ = sandboxDB.QueryRowContext(ctx, "SELECT count(*) FROM entities;").Scan(&totalEntities)

	return &RestoreSandboxReport{
		SourceType:        sourceType,
		SourceIdentifier:  sourceID,
		SandboxPath:       sandboxPath,
		SizeBytes:         verifyRes.SizeBytes,
		SchemaVersion:     verifyRes.SchemaVersion,
		TotalClaims:       totalClaims,
		TotalEntities:     totalEntities,
		IntegrityVerified: true,
		Duration:          time.Since(start),
	}, nil
}
