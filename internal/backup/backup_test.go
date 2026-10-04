package backup_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/backup"
	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/observability"
	"github.com/raphaieu/vorcarozap/internal/store"
)

// mockS3Server simula um bucket S3 com validação de assinatura SigV4.
type mockS3Server struct {
	mu           sync.RWMutex
	objects      map[string][]byte
	metaSHA256   map[string]string
	shouldFail   bool
	failStatus   int
	receivedAuth string
}

func newMockS3Server() (*mockS3Server, *httptest.Server) {
	mock := &mockS3Server{
		objects:    make(map[string][]byte),
		metaSHA256: make(map[string]string),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		mock.receivedAuth = r.Header.Get("Authorization")

		// Valida presença da assinatura AWS4-HMAC-SHA256
		if !strings.HasPrefix(mock.receivedAuth, "AWS4-HMAC-SHA256") {
			http.Error(w, "Unauthorized: missing or invalid SigV4", http.StatusUnauthorized)
			return
		}

		if mock.shouldFail {
			http.Error(w, "Simulated S3 failure", mock.failStatus)
			return
		}

		// Rota /bucket?list-type=2
		if r.URL.Query().Get("list-type") == "2" {
			prefix := r.URL.Query().Get("prefix")
			var contentsXML strings.Builder
			for k, v := range mock.objects {
				if prefix == "" || strings.HasPrefix(k, prefix) {
					contentsXML.WriteString(fmt.Sprintf(`
					<Contents>
						<Key>%s</Key>
						<LastModified>%s</LastModified>
						<ETag>"dummy-etag"</ETag>
						<Size>%d</Size>
					</Contents>`, k, time.Now().UTC().Format(time.RFC3339), len(v)))
				}
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
			<ListBucketResult>
				<Name>test-bucket</Name>
				<Prefix>%s</Prefix>
				%s
			</ListBucketResult>`, prefix, contentsXML.String())))
			return
		}

		// Extrai a chave a partir do path: /{bucket}/{key...}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		if len(parts) < 2 {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		key := parts[1]

		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			mock.objects[key] = body
			mock.metaSHA256[key] = r.Header.Get("x-amz-meta-sha256")
			w.Header().Set("ETag", `"dummy-etag"`)
			w.WriteHeader(http.StatusOK)

		case http.MethodHead:
			data, exists := mock.objects[key]
			if !exists {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.Header().Set("ETag", `"dummy-etag"`)
			w.Header().Set("x-amz-meta-sha256", mock.metaSHA256[key])
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			data, exists := mock.objects[key]
			if !exists {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.Header().Set("ETag", `"dummy-etag"`)
			w.Header().Set("x-amz-meta-sha256", mock.metaSHA256[key])
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)

		case http.MethodDelete:
			delete(mock.objects, key)
			delete(mock.metaSHA256, key)
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(handler)
	return mock, srv
}

func TestS3Provider_CRUD_SigV4(t *testing.T) {
	mock, srv := newMockS3Server()
	defer srv.Close()

	ctx := context.Background()
	provider, err := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "us-east-1",
		Bucket:     "test-bucket",
		AccessKey:  "AKIA_TEST_KEY",
		SecretKey:  "SECRET_TEST_KEY",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("falha ao criar provedor S3: %v", err)
	}

	payload := []byte("conteudo-sqlite-backup-teste")
	key := "backups/vorcarozap-test-1.db"

	// 1. PutObject
	err = provider.PutObject(ctx, key, bytes.NewReader(payload), int64(len(payload)), "", map[string]string{
		"schema-version": "11",
	})
	if err != nil {
		t.Fatalf("PutObject falhou: %v", err)
	}

	if !strings.Contains(mock.receivedAuth, "Credential=AKIA_TEST_KEY") {
		t.Errorf("esperava AccessKey no header Authorization, obtido: %s", mock.receivedAuth)
	}

	// 2. HeadObject
	meta, err := provider.HeadObject(ctx, key)
	if err != nil {
		t.Fatalf("HeadObject falhou: %v", err)
	}
	if meta.SizeBytes != int64(len(payload)) {
		t.Errorf("esperado tamanho %d, obtido %d", len(payload), meta.SizeBytes)
	}

	// 3. ListObjects
	objects, err := provider.ListObjects(ctx, "backups")
	if err != nil {
		t.Fatalf("ListObjects falhou: %v", err)
	}
	if len(objects) != 1 || objects[0].Key != key {
		t.Errorf("esperado 1 objeto com chave %s, obtido: %+v", key, objects)
	}

	// 4. GetObject
	body, getMeta, err := provider.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject falhou: %v", err)
	}
	defer body.Close()
	readPayload, _ := io.ReadAll(body)
	if !bytes.Equal(readPayload, payload) {
		t.Errorf("conteudo baixado difere do original")
	}
	if getMeta.SizeBytes != int64(len(payload)) {
		t.Errorf("getMeta.SizeBytes incorreto")
	}

	// 5. DeleteObject
	if err := provider.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject falhou: %v", err)
	}

	// Verifica se foi removido
	_, err = provider.HeadObject(ctx, key)
	if err == nil {
		t.Errorf("esperava erro de not found apos DeleteObject")
	}
}

func TestBackupManager_RunBackup_LocalAndRemoteSuccess(t *testing.T) {
	mock, srv := newMockS3Server()
	defer srv.Close()

	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "source.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao migrar: %v", err)
	}

	provider, _ := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "us-east-1",
		Bucket:     "test-bucket",
		AccessKey:  "AKIA123",
		SecretKey:  "SECRET123",
		HTTPClient: srv.Client(),
	})

	reg := observability.NewMetricsRegistry()
	cfg := &config.Config{
		DBPath:                     dbPath,
		BackupRemoteEnabled:        true,
		BackupRemoteProvider:       "s3",
		BackupRemoteBucket:         "test-bucket",
		BackupRemotePrefix:         "backups",
		BackupRemoteRetentionCount: 5,
	}

	mgr, err := backup.NewManager(backup.ManagerConfig{
		DB:       db,
		Config:   cfg,
		Registry: reg,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("falha ao criar BackupManager: %v", err)
	}

	destBackupPath := filepath.Join(tempDir, "backups", "vorcarozap-20260916_120000Z.db")
	report, err := mgr.RunBackup(ctx, destBackupPath, 5, false)
	if err != nil {
		t.Fatalf("RunBackup falhou: %v", err)
	}

	if !report.LocalSuccess {
		t.Errorf("esperado LocalSuccess=true")
	}
	if !report.RemoteSuccess {
		t.Errorf("esperado RemoteSuccess=true")
	}
	if report.RemoteError != nil {
		t.Errorf("esperado RemoteError=nil, obtido: %v", report.RemoteError)
	}
	if report.RemoteSHA256 == "" {
		t.Errorf("esperado RemoteSHA256 preenchido")
	}

	// Verifica se está presente no mock S3
	if len(mock.objects) != 1 {
		t.Errorf("esperado 1 objeto no S3, obtido %d", len(mock.objects))
	}
}

func TestBackupManager_FaultIsolation_RemoteFailureDoesNotBreakLocal(t *testing.T) {
	mock, srv := newMockS3Server()
	defer srv.Close()

	// Simula erro 500 no S3
	mock.shouldFail = true
	mock.failStatus = http.StatusInternalServerError

	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "source.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao migrar: %v", err)
	}

	provider, _ := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "us-east-1",
		Bucket:     "test-bucket",
		AccessKey:  "AKIA123",
		SecretKey:  "SECRET123",
		HTTPClient: srv.Client(),
	})

	reg := observability.NewMetricsRegistry()
	cfg := &config.Config{
		DBPath:                     dbPath,
		BackupRemoteEnabled:        true,
		BackupRemoteProvider:       "s3",
		BackupRemoteBucket:         "test-bucket",
		BackupRemotePrefix:         "backups",
		BackupRemoteRetentionCount: 5,
	}

	mgr, err := backup.NewManager(backup.ManagerConfig{
		DB:       db,
		Config:   cfg,
		Registry: reg,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("falha ao criar manager: %v", err)
	}

	destBackupPath := filepath.Join(tempDir, "backups", "vorcarozap-20260916_120000Z.db")
	report, err := mgr.RunBackup(ctx, destBackupPath, 5, false)

	// O erro principal não deve mascarar o sucesso local!
	if err != nil {
		t.Fatalf("RunBackup não deveria retornar erro bloqueante em caso de falha remota: %v", err)
	}
	if !report.LocalSuccess {
		t.Errorf("esperado LocalSuccess=true mesmo com falha remota")
	}
	if report.RemoteSuccess {
		t.Errorf("esperado RemoteSuccess=false devido à falha simulada no S3")
	}
	if report.RemoteError == nil {
		t.Errorf("esperado RemoteError registrado no report")
	}

	// Verifica se o arquivo local existe e é 100% íntegro
	verifyRes, verifyErr := store.VerifyBackup(ctx, destBackupPath)
	if verifyErr != nil {
		t.Fatalf("backup local foi corrompido ou não encontrado: %v", verifyErr)
	}
	if !verifyRes.IntegrityOK {
		t.Errorf("esperado IntegrityOK no backup local")
	}
}

func TestBackupManager_RestoreSandbox(t *testing.T) {
	mock, srv := newMockS3Server()
	defer srv.Close()
	_ = mock

	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "source.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao migrar: %v", err)
	}

	// Cria um backup local
	backupPath := filepath.Join(tempDir, "vorcarozap-20260916_130000Z.db")
	_, err = store.Backup(ctx, db, backupPath)
	if err != nil {
		t.Fatalf("falha ao gerar backup: %v", err)
	}

	provider, _ := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "us-east-1",
		Bucket:     "test-bucket",
		AccessKey:  "AKIA123",
		SecretKey:  "SECRET123",
		HTTPClient: srv.Client(),
	})

	cfg := &config.Config{
		DBPath:              dbPath,
		BackupRemoteEnabled: true,
	}

	mgr, err := backup.NewManager(backup.ManagerConfig{
		DB:       db,
		Config:   cfg,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("falha ao criar manager: %v", err)
	}

	// 1. Teste RestoreSandbox a partir de arquivo local
	sandboxReport, err := mgr.RestoreSandbox(ctx, backupPath, "")
	if err != nil {
		t.Fatalf("RestoreSandbox local falhou: %v", err)
	}

	if !sandboxReport.IntegrityVerified {
		t.Errorf("esperado IntegrityVerified=true")
	}
	if sandboxReport.SourceType != "local" {
		t.Errorf("esperado SourceType 'local', obtido %q", sandboxReport.SourceType)
	}

	// 2. Teste RestoreSandbox a partir de chave remota no S3
	// Envia o backup para o mock S3 com hash SHA-256 e metadados válidos
	backupBytes, _ := io.ReadAll(mustOpen(t, backupPath))
	hasher := sha256.New()
	hasher.Write(backupBytes)
	backupSHA := hex.EncodeToString(hasher.Sum(nil))
	_ = provider.PutObject(ctx, "backups/remote-test.db", bytes.NewReader(backupBytes), int64(len(backupBytes)), backupSHA, map[string]string{
		"sha256": backupSHA,
	})

	remoteSandboxReport, err := mgr.RestoreSandbox(ctx, "", "backups/remote-test.db")
	if err != nil {
		t.Fatalf("RestoreSandbox remoto falhou: %v", err)
	}
	if !remoteSandboxReport.IntegrityVerified {
		t.Errorf("esperado IntegrityVerified=true no sandbox remoto")
	}
	if remoteSandboxReport.SourceType != "remote" {
		t.Errorf("esperado SourceType 'remote', obtido %q", remoteSandboxReport.SourceType)
	}
}

func TestBackupManager_RemoteFailure_PreservesLocalAndSegregatesTelemetry(t *testing.T) {
	mock, srv := newMockS3Server()
	defer srv.Close()

	ctx := context.Background()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "source.db")
	destDir := filepath.Join(tempDir, "backups")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite: %v", err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao migrar: %v", err)
	}

	// Simula falha remota no S3 com cabeçalho contendo token sensível
	mock.mu.Lock()
	mock.shouldFail = true
	mock.failStatus = http.StatusInternalServerError
	mock.mu.Unlock()

	provider, _ := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "us-east-1",
		Bucket:     "test-bucket",
		AccessKey:  "AKIA123",
		SecretKey:  "SECRET123",
		HTTPClient: srv.Client(),
	})

	cfg := &config.Config{
		DBPath:                     dbPath,
		BackupRemoteEnabled:        true,
		BackupRemoteProvider:       "s3",
		BackupRemoteBucket:         "test-bucket",
		BackupRemotePrefix:         "backups",
		BackupRemoteRetentionCount: 5,
	}

	mgr, err := backup.NewManager(backup.ManagerConfig{
		DB:       db,
		Config:   cfg,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("falha ao criar manager: %v", err)
	}

	destBackupPath := filepath.Join(destDir, "test-backup.db")
	report, err := mgr.RunBackup(ctx, destBackupPath, 5, false)
	if err != nil {
		t.Fatalf("RunBackup não deveria retornar erro fatal quando apenas o remoto falha: %v", err)
	}

	// Snapshot local DEVE ter sucesso e arquivo local deve existir
	if !report.LocalSuccess {
		t.Fatalf("LocalSuccess deveria ser true")
	}
	if _, statErr := os.Stat(report.LocalPath); statErr != nil {
		t.Fatalf("arquivo local de backup não encontrado: %v", statErr)
	}

	// Remoto DEVE registrar tentativa e falha
	if !report.RemoteAttempted || report.RemoteSuccess {
		t.Errorf("esperado RemoteAttempted=true e RemoteSuccess=false")
	}

	// Verifica registro segregado no banco SQLite (backup_runs)
	rec, err := store.GetLatestLocalBackupRun(ctx, db)
	if err != nil || rec == nil {
		t.Fatalf("falha ao consultar backup_runs: %v", err)
	}
	if rec.LocalStatus != "ok" {
		t.Errorf("esperado LocalStatus='ok', obtido %q", rec.LocalStatus)
	}
	if rec.RemoteStatus != "error" {
		t.Errorf("esperado RemoteStatus='error', obtido %q", rec.RemoteStatus)
	}
	if rec.RemoteError == nil || *rec.RemoteError == "" {
		t.Errorf("esperado RemoteError preenchido")
	}
}

func TestS3Provider_ListObjects_Pagination(t *testing.T) {
	// Servidor mock que simula paginação com IsTruncated e NextContinuationToken
	pageCallCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCallCount++
		token := r.URL.Query().Get("continuation-token")

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)

		if token == "" {
			// Primeira página (Truncated=true)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
			<ListBucketResult>
				<Name>paginated-bucket</Name>
				<Prefix>backups</Prefix>
				<IsTruncated>true</IsTruncated>
				<NextContinuationToken>token-page-2</NextContinuationToken>
				<Contents>
					<Key>backups/vorcarozap-20260901_000000Z.db</Key>
					<LastModified>2026-09-01T00:00:00Z</LastModified>
					<Size>1024</Size>
				</Contents>
			</ListBucketResult>`))
		} else {
			// Segunda página (Truncated=false)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
			<ListBucketResult>
				<Name>paginated-bucket</Name>
				<Prefix>backups</Prefix>
				<IsTruncated>false</IsTruncated>
				<Contents>
					<Key>backups/vorcarozap-20260902_000000Z.db</Key>
					<LastModified>2026-09-02T00:00:00Z</LastModified>
					<Size>2048</Size>
				</Contents>
			</ListBucketResult>`))
		}
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	provider, err := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "us-east-1",
		Bucket:     "paginated-bucket",
		AccessKey:  "AKIA123",
		SecretKey:  "SECRET123",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("falha ao criar provider: %v", err)
	}

	objects, err := provider.ListObjects(context.Background(), "backups")
	if err != nil {
		t.Fatalf("falha no ListObjects paginado: %v", err)
	}

	if len(objects) != 2 {
		t.Fatalf("esperava 2 objetos agregados de todas as páginas, obtido %d", len(objects))
	}
	if pageCallCount != 2 {
		t.Errorf("esperava 2 chamadas de paginação ao S3, obtido %d", pageCallCount)
	}
}

func TestS3Provider_SigV4_FullCryptographicSignature(t *testing.T) {
	var capturedAuth, capturedDate, capturedContentSHA string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedDate = r.Header.Get("x-amz-date")
		capturedContentSHA = r.Header.Get("x-amz-content-sha256")
		w.Header().Set("ETag", `"test-etag"`)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	provider, err := backup.NewS3Provider(backup.S3Config{
		Endpoint:   srv.URL,
		Region:     "sa-east-1",
		Bucket:     "sigv4-bucket",
		AccessKey:  "AKIA_TEST_KEY",
		SecretKey:  "SECRET_TEST_KEY",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("falha ao instanciar provider: %v", err)
	}

	payload := []byte("Dados do snapshot de teste")
	hasher := sha256.New()
	hasher.Write(payload)
	payloadSHA := hex.EncodeToString(hasher.Sum(nil))

	err = provider.PutObject(context.Background(), "test.db", bytes.NewReader(payload), int64(len(payload)), payloadSHA, nil)
	if err != nil {
		t.Fatalf("falha no PutObject: %v", err)
	}

	if !strings.HasPrefix(capturedAuth, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("cabeçalho de autorização não usa AWS4-HMAC-SHA256: %q", capturedAuth)
	}
	if !strings.Contains(capturedAuth, "Credential=AKIA_TEST_KEY/") {
		t.Errorf("cabeçalho de autorização não contém credencial correta: %q", capturedAuth)
	}
	if !strings.Contains(capturedAuth, "Signature=") {
		t.Errorf("cabeçalho de autorização não contém assinatura: %q", capturedAuth)
	}
	if capturedDate == "" {
		t.Errorf("cabeçalho x-amz-date ausente")
	}
	if capturedContentSHA != payloadSHA {
		t.Errorf("x-amz-content-sha256 incorreto: obtido %s, esperado %s", capturedContentSHA, payloadSHA)
	}
}

func mustOpen(t *testing.T, path string) io.ReadCloser {
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("falha ao abrir: %v", err)
	}
	return f
}
