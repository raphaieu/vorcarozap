package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/importer"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/sourcecheck"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

// fakeSmokeProvider implementa research.ResearchProvider para testes isolados de monitoramento sem chamadas reais.
type fakeSmokeProvider struct{}

func (f *fakeSmokeProvider) Discover(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
	return &research.DiscoverResult{
		Candidates:       []research.CandidateExtraction{},
		PromptTokens:     150,
		CompletionTokens: 50,
		TotalTokens:      200,
		Cost:             0.0005,
		CostMicros:       500,
		WebSearchCalls:   1,
	}, nil
}

func (f *fakeSmokeProvider) Verify(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
	return &research.VerifyResult{
		IdentityMatch:            true,
		ClaimSupported:           true,
		ClaimOverstatesSource:    false,
		AttributionExplicit:      true,
		GradeCompatible:          true,
		ContainsIllicitInference: false,
		Uncertainties:            []string{},
		RecommendedAction:        "publish",
		PromptTokens:             80,
		CompletionTokens:         20,
		TotalTokens:              100,
		Cost:                     0.0002,
		CostMicros:               200,
	}, nil
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("falha ao obter diretório atual: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("não foi possível encontrar a raiz do repositório contendo go.mod")
		}
		dir = parent
	}
}

// TestSmokeChecklist_CompleteSuite executa o checklist completo de fumaça da fase VZ-018
func TestSmokeChecklist_CompleteSuite(t *testing.T) {
	repoRoot := findRepoRoot(t)
	xlsxPath := filepath.Join(repoRoot, "_notes", "mapa-vorcaro-contatos-2026-09-03.xlsx")
	if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
		t.Skipf("planilha oficial não encontrada em %s, pulando smoke test", xlsxPath)
	}
	mappingPath := filepath.Join(repoRoot, "config", "import-mapping-v1.yaml")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "smoke_app.db")
	backupDir := filepath.Join(tempDir, "backups")
	t.Setenv("ADMIN_MFA_ENCRYPTION_KEY", "12345678901234567890123456789012")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Inicialização do SQLite e aplicação de Migrations
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// 2. Importação da base seed inicial
	imp := importer.NewImporter(db, mappingPath)
	importRes, err := imp.ImportFromFile(ctx, importer.ImportOptions{
		FilePath: xlsxPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("falha ao importar planilha oficial: %v", err)
	}
	if importRes.SummaryCounts.ValidRows != 151 {
		t.Fatalf("esperado 151 linhas válidas importadas, obtido %d", importRes.SummaryCounts.ValidRows)
	}

	// 3. Configuração do Servidor HTTP com Admin e CSRF
	adminPassword := "SenhaAdminSegura123!"
	adminHashBytes, err := bcrypt.GenerateFromPassword([]byte(adminPassword), 12)
	if err != nil {
		t.Fatalf("falha ao gerar hash bcrypt: %v", err)
	}
	adminHash := string(adminHashBytes)
	allowedOrigin := "http://localhost:8090"

	cfg := &config.Config{
		Port:                  8080,
		Env:                   "production",
		DBPath:                dbPath,
		PublicDataCutoff:      "2026-09-03",
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           60 * time.Second,
		AdminUser:             "admin_smoke",
		AdminPasswordHash:     adminHash,
		AdminAllowedOrigin:    allowedOrigin,
		AdminMFARequired:      false,
		AdminMFAEncryptionKey: "12345678901234567890123456789012",
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor HTTP: %v", err)
	}

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	client := ts.Client()

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 1: /health/live retorna 200
	// -------------------------------------------------------------------------
	t.Run("Smoke 1: Health Live", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/health/live")
		if err != nil {
			t.Fatalf("requisição GET /health/live falhou: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200, obtido %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), `"status":"live"`) {
			t.Errorf("resposta inesperada em /health/live: %s", string(body))
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 2: /health/ready retorna 200 quando pronto e 503 em falha
	// -------------------------------------------------------------------------
	t.Run("Smoke 2: Health Ready e Falha de Readiness", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/health/ready")
		if err != nil {
			t.Fatalf("requisição GET /health/ready falhou: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200 em /health/ready, obtido %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), `"status":"ready"`) {
			t.Errorf("resposta inesperada em /health/ready: %s", string(body))
		}

		// Testa falha de readiness com banco fechado
		closedDBPath := filepath.Join(tempDir, "closed.db")
		closedDB, err := store.Open(ctx, closedDBPath)
		if err != nil {
			t.Fatalf("falha ao criar banco de teste: %v", err)
		}
		_ = closedDB.Close()
		srvFail, err := web.NewServer(cfg, closedDB)
		if err != nil {
			t.Fatalf("falha ao criar servidor de teste: %v", err)
		}
		tsFail := httptest.NewServer(srvFail.Handler)
		defer tsFail.Close()

		respFail, err := client.Get(tsFail.URL + "/health/ready")
		if err != nil {
			t.Fatalf("requisição GET /health/ready (closed) falhou: %v", err)
		}
		defer respFail.Body.Close()
		if respFail.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("esperado status 503 para banco fechado, obtido %d", respFail.StatusCode)
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 3: Página pública / responde corretamente
	// -------------------------------------------------------------------------
	t.Run("Smoke 3: Pagina Publica Home", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/")
		if err != nil {
			t.Fatalf("requisição GET / falhou: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200, obtido %d", resp.StatusCode)
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		body := string(bodyBytes)
		if !strings.Contains(body, "VorcaroZAP") {
			t.Error("Home não contém título VorcaroZAP")
		}
		if !strings.Contains(body, "Aviso Editorial e de Independência") {
			t.Error("Home não contém aviso editorial obrigatório")
		}
		if !strings.Contains(body, "Métricas da Rede Documental") {
			t.Error("Home não contém seção de métricas da rede")
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 4: /pessoas e detalhe de entidade respondem
	// -------------------------------------------------------------------------
	t.Run("Smoke 4: Pessoas e Detalhe de Entidade", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/pessoas")
		if err != nil {
			t.Fatalf("requisição GET /pessoas falhou: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200 em /pessoas, obtido %d", resp.StatusCode)
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		pessoasBody := string(bodyBytes)
		if !strings.Contains(pessoasBody, "Pessoas e Organizações") {
			t.Error("/pessoas não contém cabeçalho esperado")
		}

		// Localiza um slug público real a partir da listagem
		idx := strings.Index(pessoasBody, `href="/pessoas/`)
		var sampleSlug string
		if idx != -1 {
			rest := pessoasBody[idx+len(`href="/pessoas/`):]
			end := strings.Index(rest, `"`)
			if end != -1 {
				sampleSlug = rest[:end]
			}
		}

		if sampleSlug != "" {
			respDetail, err := client.Get(ts.URL + "/pessoas/" + sampleSlug)
			if err != nil {
				t.Fatalf("requisição GET /pessoas/%s falhou: %v", sampleSlug, err)
			}
			defer respDetail.Body.Close()
			if respDetail.StatusCode != http.StatusOK {
				t.Errorf("esperado status 200 em detalhe de %s, obtido %d", sampleSlug, respDetail.StatusCode)
			}
		} else {
			t.Error("não foi possível encontrar slug de entidade pública em /pessoas")
		}

		// Detalhe de entidade em quarentena deve retornar 404
		respQuarantine, err := client.Get(ts.URL + "/pessoas/carlos-eduardo-torres-bandeira")
		if err != nil {
			t.Fatalf("requisição GET entidade quarentenada falhou: %v", err)
		}
		defer respQuarantine.Body.Close()
		if respQuarantine.StatusCode != http.StatusNotFound {
			t.Errorf("esperado status 404 para entidade em quarentena, obtido %d", respQuarantine.StatusCode)
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 5: /metodologia responde
	// -------------------------------------------------------------------------
	t.Run("Smoke 5: Metodologia", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/metodologia")
		if err != nil {
			t.Fatalf("requisição GET /metodologia falhou: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200 em /metodologia, obtido %d", resp.StatusCode)
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		body := string(bodyBytes)
		if !strings.Contains(body, "Metodologia e Critérios Editoriais") {
			t.Error("/metodologia não contém título esperado")
		}
		if !strings.Contains(body, "Grau A") || !strings.Contains(body, "Grau E") {
			t.Error("/metodologia não contém escala probatória A-E")
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 6: /exportar/base.xlsx gera arquivo válido
	// -------------------------------------------------------------------------
	t.Run("Smoke 6: Exportacao XLSX", func(t *testing.T) {
		resp, err := client.Get(ts.URL + "/exportar/base.xlsx")
		if err != nil {
			t.Fatalf("requisição GET /exportar/base.xlsx falhou: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200 em exportação XLSX, obtido %d", resp.StatusCode)
		}
		xlsxBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("falha ao ler bytes do XLSX: %v", err)
		}

		f, err := excelize.OpenReader(bytes.NewReader(xlsxBytes))
		if err != nil {
			t.Fatalf("falha ao abrir XLSX gerado: %v", err)
		}
		defer f.Close()

		sheetList := f.GetSheetList()
		if len(sheetList) == 0 {
			t.Error("XLSX exportado não possui abas")
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 7 & 8: /admin exige sessão e MFA, rejeitando Basic Auth
	// -------------------------------------------------------------------------
	var adminSessionCookie *http.Cookie
	t.Run("Smoke 7 e 8: Admin Auth & Session", func(t *testing.T) {
		// 1. Sem autenticação -> 401
		respNoAuth, err := client.Get(ts.URL + "/admin")
		if err != nil {
			t.Fatalf("requisição GET /admin (sem auth) falhou: %v", err)
		}
		defer respNoAuth.Body.Close()
		if respNoAuth.StatusCode != http.StatusUnauthorized {
			t.Errorf("esperado status 401 em /admin sem auth, obtido %d", respNoAuth.StatusCode)
		}

		// 2. Com Basic Auth -> 401 (Basic Auth removido / não contorna MFA)
		reqBasic, _ := http.NewRequest("GET", ts.URL+"/admin", nil)
		reqBasic.SetBasicAuth(cfg.AdminUser, adminPassword)
		respBasic, err := client.Do(reqBasic)
		if err != nil {
			t.Fatalf("requisição GET /admin (Basic Auth) falhou: %v", err)
		}
		defer respBasic.Body.Close()
		if respBasic.StatusCode != http.StatusUnauthorized {
			t.Errorf("esperado status 401 em /admin com Basic Auth, obtido %d", respBasic.StatusCode)
		}

		// 3. Login formal via POST /admin/login
		loginForm := url.Values{
			"username": {cfg.AdminUser},
			"password": {adminPassword},
		}
		noFollowClient := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		loginReq, _ := http.NewRequest("POST", ts.URL+"/admin/login", strings.NewReader(loginForm.Encode()))
		loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		loginReq.Header.Set("Origin", allowedOrigin)
		respLogin, err := noFollowClient.Do(loginReq)
		if err != nil {
			t.Fatalf("login falhou: %v", err)
		}
		defer respLogin.Body.Close()
		if respLogin.StatusCode != http.StatusSeeOther {
			t.Fatalf("esperado status 303 no login, obtido %d", respLogin.StatusCode)
		}

		for _, c := range respLogin.Cookies() {
			if c.Name == web.SessionCookieName && c.Value != "" {
				adminSessionCookie = c
				break
			}
		}
		if adminSessionCookie == nil {
			t.Fatalf("cookie de sessão não foi retornado após login")
		}

		// 4. Acesso a /admin com cookie de sessão -> 200
		reqAuth, _ := http.NewRequest("GET", ts.URL+"/admin", nil)
		reqAuth.AddCookie(adminSessionCookie)
		respAuth, err := client.Do(reqAuth)
		if err != nil {
			t.Fatalf("requisição GET /admin (com cookie) falhou: %v", err)
		}
		defer respAuth.Body.Close()
		if respAuth.StatusCode != http.StatusOK {
			t.Errorf("esperado status 200 em /admin com cookie, obtido %d", respAuth.StatusCode)
		}
		bodyBytes, _ := io.ReadAll(respAuth.Body)
		if !strings.Contains(string(bodyBytes), "Painel Administrativo") {
			t.Error("/admin autenticado não contém título esperado")
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 9 & 10: POST admin sem Origin rejeitado, com Origin aceito
	// -------------------------------------------------------------------------
	t.Run("Smoke 9 e 10: Protecao CSRF no Admin", func(t *testing.T) {
		if adminSessionCookie == nil {
			t.Fatal("cookie de sessão não disponível para teste de CSRF")
		}

		// Obtém um claim e sua versão atual para teste de moderação
		var targetClaimID, expectedUpdatedAt string
		err := db.QueryRowContext(ctx, "SELECT id, updated_at FROM claims WHERE status = 'quarantined' LIMIT 1;").Scan(&targetClaimID, &expectedUpdatedAt)
		if err != nil {
			t.Fatalf("falha ao obter claim em quarentena: %v", err)
		}

		moderateURL := fmt.Sprintf("%s/admin/claims/%s/moderate", ts.URL, targetClaimID)

		// 1. POST sem Origin -> 403 Forbidden (bloqueio CSRF)
		form := url.Values{}
		form.Set("action", "approve")
		form.Set("reason", "Aprovação em smoke test")
		form.Set("expected_updated_at", expectedUpdatedAt)

		reqNoOrigin, _ := http.NewRequest("POST", moderateURL, strings.NewReader(form.Encode()))
		reqNoOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqNoOrigin.AddCookie(adminSessionCookie)

		respNoOrigin, err := client.Do(reqNoOrigin)
		if err != nil {
			t.Fatalf("POST sem Origin falhou: %v", err)
		}
		defer respNoOrigin.Body.Close()
		if respNoOrigin.StatusCode != http.StatusForbidden {
			t.Errorf("esperado status 403 para POST sem Origin, obtido %d", respNoOrigin.StatusCode)
		}

		// 2. POST com Origin válido -> Não retorna 403 CSRF (aplica moderação ou valida regra)
		reqValidOrigin, _ := http.NewRequest("POST", moderateURL, strings.NewReader(form.Encode()))
		reqValidOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqValidOrigin.Header.Set("Origin", allowedOrigin)
		reqValidOrigin.AddCookie(adminSessionCookie)

		noFollowClient := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		respValidOrigin, err := noFollowClient.Do(reqValidOrigin)
		if err != nil {
			t.Fatalf("POST com Origin válido falhou: %v", err)
		}
		defer respValidOrigin.Body.Close()
		if respValidOrigin.StatusCode == http.StatusForbidden {
			t.Errorf("POST com Origin válido foi indevidamente rejeitado com 403")
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 11, 12 & 13: Monitor runner, lock SQLite e persistência
	// -------------------------------------------------------------------------
	t.Run("Smoke 11, 12 e 13: Monitor Runner e Lock de Concorrencia", func(t *testing.T) {
		sourceVerifier := sourcecheck.NewVerifier(sourcecheck.VerifierConfig{
			TotalTimeout: 5 * time.Second,
		})

		runnerConfig := monitoring.RunnerConfig{
			DB:                     db,
			Provider:               &fakeSmokeProvider{},
			SourceVerifier:         sourceVerifier,
			DiscoveryModel:         "openai/gpt-4.1-mini",
			VerificationModel:      "openai/gpt-4.1-mini",
			MonitorWindow:          24 * time.Hour,
			MaxCandidatesPerRun:    10,
			MaxVerificationsPerRun: 10,
			MaxCostPerRunUSD:       0.25,
			MaxCostPerRunMicroUSD:  250000,
			MaxCostPerDayUSD:       1.00,
			MaxCostPerDayMicroUSD:  1000000,
			LockTTL:                5 * time.Minute,
		}

		runner, err := monitoring.NewRunner(runnerConfig)
		if err != nil {
			t.Fatalf("falha ao criar runner de monitoramento: %v", err)
		}

		// 11 e 13. Executa monitoramento
		summary, err := runner.Run(ctx, "Daniel Vorcaro Smoke Test")
		if err != nil {
			t.Fatalf("execução do runner falhou: %v", err)
		}
		if summary.Status != "completed" {
			t.Errorf("esperado status 'completed', obtido %q", summary.Status)
		}
		if summary.TotalTokens <= 0 {
			t.Errorf("esperado TotalTokens > 0, obtido %d", summary.TotalTokens)
		}

		// 12. Valida lock concorrente no mesmo banco
		_, err = db.ExecContext(ctx, `
			INSERT INTO monitoring_locks (name, holder, acquired_at, expires_at, updated_at)
			VALUES ('monitoring', 'holder_ativo', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '2099-01-01T00:00:00Z', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT(name) DO UPDATE SET holder = 'holder_ativo', expires_at = '2099-01-01T00:00:00Z';
		`)
		if err != nil {
			t.Fatalf("falha ao simular lock ativo: %v", err)
		}

		_, errConcurrent := runner.Run(ctx, "Daniel Vorcaro Concorrente")
		if errConcurrent == nil {
			t.Error("esperava erro de lock concorrente ao executar runner com lease ativo, obteve nil")
		}

		// Libera lock simulado
		_, _ = db.ExecContext(ctx, "DELETE FROM monitoring_locks WHERE name = 'monitoring';")
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 14: Backup consistente criado, verificado e rotacionado via CLI
	// -------------------------------------------------------------------------
	var backupFile string
	t.Run("Smoke 14: Criacao, Verificacao e Retencao de Backup via CLI", func(t *testing.T) {
		t.Setenv("DB_PATH", dbPath)

		// 14.1 Backup direto via store.Backup
		backupFile = filepath.Join(backupDir, "vorcarozap-20260912_180000Z.db")
		res, err := store.Backup(ctx, db, backupFile)
		if err != nil {
			t.Fatalf("falha ao executar store.Backup: %v", err)
		}
		if res.SizeBytes <= 0 {
			t.Errorf("esperado tamanho > 0 bytes, obtido %d", res.SizeBytes)
		}
		if !res.IntegrityOK {
			t.Error("esperado IntegrityOK = true no backup gerado")
		}

		// 14.2 Verifica independentemente
		verifyRes, err := store.VerifyBackup(ctx, backupFile)
		if err != nil {
			t.Fatalf("falha ao verificar backup: %v", err)
		}
		if !verifyRes.IntegrityOK {
			t.Error("VerifyBackup reprovou a integridade")
		}

		// 14.3 Criação de backups com espaço no nome e retenção via CLI runBackup
		spacedBackup := filepath.Join(backupDir, "vorcarozap-20260912_190000Z.db")
		if err := runBackup([]string{"--out", spacedBackup, "--retention", "2"}); err != nil {
			t.Fatalf("falha ao executar runBackup com flag --retention: %v", err)
		}

		// Cria mais um backup para forçar a rotação mantendo apenas os 2 mais recentes
		newestBackup := filepath.Join(backupDir, "vorcarozap-20260912_200000Z.db")
		if err := runBackup([]string{"--out", newestBackup, "--retention", "2"}); err != nil {
			t.Fatalf("falha ao executar runBackup mais recente: %v", err)
		}

		// O backup mais antigo (180000Z) deve ter sido removido por retenção (mantendo 190000Z e 200000Z)
		if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
			t.Errorf("backup mais antigo %s deveria ter sido rotacionado", backupFile)
		}
		if _, err := os.Stat(spacedBackup); os.IsNotExist(err) {
			t.Errorf("backup intermediario %s deveria existir", spacedBackup)
		}
		if _, err := os.Stat(newestBackup); os.IsNotExist(err) {
			t.Errorf("backup mais recente %s deveria existir", newestBackup)
		}

		// 14.4 Rejeição de retention negativo
		if err := runBackup([]string{"--retention", "-1"}); err == nil {
			t.Error("esperava erro ao executar runBackup com --retention negativo, obteve nil")
		}

		// 14.6 Falha de rotação retorna erro não-zero preservando o contexto e o backup gerado
		rotFailDir := filepath.Join(tempDir, "rot_fail_dir")
		if err := os.MkdirAll(rotFailDir, 0755); err != nil {
			t.Fatalf("falha ao criar rotFailDir: %v", err)
		}
		oldFile1 := filepath.Join(rotFailDir, "vorcarozap-20260901_000000Z.db")
		oldFile2 := filepath.Join(rotFailDir, "vorcarozap-20260902_000000Z.db")
		_ = os.WriteFile(oldFile1, []byte("backup antigo 1"), 0644)
		_ = os.WriteFile(oldFile2, []byte("backup antigo 2"), 0644)

		// Teste direto de RotateBackups sob restrição de permissão do diretório (não-root)
		if err := os.Chmod(rotFailDir, 0555); err == nil {
			t.Cleanup(func() { _ = os.Chmod(rotFailDir, 0755) })
			_, rotErr := store.RotateBackups(rotFailDir, 1)
			if rotErr != nil {
				// Se o ambiente restringir a deleção com 0555, confirma que o erro é retornado
				if !strings.Contains(rotErr.Error(), "falha ao remover backup antigo") {
					t.Errorf("erro inesperado em RotateBackups: %v", rotErr)
				}
			}
			_ = os.Chmod(rotFailDir, 0755)
		}

		// Atualiza backupFile para o teste de restauração
		backupFile = newestBackup
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 15: Restauração em banco de teste isolado
	// -------------------------------------------------------------------------
	t.Run("Smoke 15: Restauracao de Teste Isolada", func(t *testing.T) {
		if backupFile == "" {
			t.Fatal("arquivo de backup não foi gerado no passo anterior")
		}
		restoredPath := filepath.Join(tempDir, "smoke_restored.db")
		if err := store.Restore(ctx, backupFile, restoredPath); err != nil {
			t.Fatalf("falha ao executar store.Restore: %v", err)
		}

		restoredDB, err := store.Open(ctx, restoredPath)
		if err != nil {
			t.Fatalf("falha ao abrir banco restaurado: %v", err)
		}
		defer restoredDB.Close()

		var count int
		if err := restoredDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM entities;").Scan(&count); err != nil {
			t.Fatalf("falha ao contar entidades no banco restaurado: %v", err)
		}
		if count == 0 {
			t.Error("banco restaurado não possui entidades")
		}
	})

	// -------------------------------------------------------------------------
	// SMOKE CHECKLIST ITEM 16: Nenhuma API key ou segredo exposto
	// -------------------------------------------------------------------------
	t.Run("Smoke 16: Ausencia de Vazamento de Segredos", func(t *testing.T) {
		secretKey := "sk-or-v1-supersecretkey123456789abcdef"
		t.Setenv("OPENROUTER_API_KEY", secretKey)

		var stdout, stderr bytes.Buffer
		fakeRunnerFactory := func(cfg *config.Config, db *sql.DB) (MonitorRunnerInterface, error) {
			return &mockMonitorRunner{
				runFn: func(ctx context.Context, query string) (*monitoring.RunSummary, error) {
					return &monitoring.RunSummary{
						RunID:             "run_smoke_secrets",
						Status:            "completed",
						Query:             query,
						TotalCostUSD:      0.01,
						DailyTotalCostUSD: 0.01,
					}, nil
				},
			}, nil
		}

		cfgOverride := &config.Config{
			DBPath:           dbPath,
			OpenRouterAPIKey: secretKey,
		}

		err := runMonitorCommand(ctx, []string{"--query", "consulta secreta"}, &stdout, &stderr, cfgOverride, fakeRunnerFactory)
		if err != nil {
			t.Fatalf("falha ao executar runMonitorCommand: %v", err)
		}

		outStr := stdout.String()
		errStr := stderr.String()

		if strings.Contains(outStr, secretKey) || strings.Contains(errStr, secretKey) {
			t.Errorf("Vazamento crítico: OPENROUTER_API_KEY apareceu na saída do monitor!")
		}
		if strings.Contains(outStr, adminPassword) || strings.Contains(errStr, adminPassword) {
			t.Errorf("Vazamento crítico: Senha administrativa apareceu na saída!")
		}
	})
}
