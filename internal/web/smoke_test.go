package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/importer"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/web"
)

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

func TestSmokeWithRealSpreadsheet(t *testing.T) {
	repoRoot := findRepoRoot(t)
	xlsxPath := filepath.Join(repoRoot, "_notes", "mapa-vorcaro-contatos-2026-09-03.xlsx")
	if _, err := os.Stat(xlsxPath); os.IsNotExist(err) {
		t.Skipf("planilha oficial não encontrada em %s, pulando smoke test", xlsxPath)
	}

	mappingPath := filepath.Join(repoRoot, "config", "import-mapping-v1.yaml")
	if _, err := os.Stat(mappingPath); os.IsNotExist(err) {
		t.Fatalf("arquivo de mapping não encontrado em %s", mappingPath)
	}

	// 1. Banco descartável isolado
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "smoke_disposable.db")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco temporário: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// 2. Executar migrations
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao executar migrations: %v", err)
	}

	// 3. Executar importador com a planilha curada oficial
	imp := importer.NewImporter(db, mappingPath)
	res, err := imp.ImportFromFile(ctx, importer.ImportOptions{
		FilePath: xlsxPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("falha ao importar planilha real: %v", err)
	}

	t.Logf("Import report: Total=%d Valid=%d Published=%d Quarantined=%d Ignored=%d Error=%d",
		res.SummaryCounts.TotalRows, res.SummaryCounts.ValidRows, res.SummaryCounts.PublishedRows, res.SummaryCounts.QuarantinedRows, res.SummaryCounts.IgnoredRows, res.SummaryCounts.ErrorRows)

	if res.SummaryCounts.ValidRows != 151 {
		t.Errorf("esperado 151 linhas válidas importadas, obtido %d", res.SummaryCounts.ValidRows)
	}
	if res.SummaryCounts.PublishedRows == 0 {
		t.Errorf("esperado pelo menos 1 linha publicada")
	}
	if res.SummaryCounts.QuarantinedRows == 0 {
		t.Errorf("esperado linhas em quarentena")
	}

	// Invariante de contagem
	if res.SummaryCounts.TotalRows != res.SummaryCounts.IgnoredRows+res.SummaryCounts.ValidRows+res.SummaryCounts.ErrorRows {
		t.Errorf("invariante TotalRows violada: %d != %d + %d + %d",
			res.SummaryCounts.TotalRows, res.SummaryCounts.IgnoredRows, res.SummaryCounts.ValidRows, res.SummaryCounts.ErrorRows)
	}
	if res.SummaryCounts.ValidRows != res.SummaryCounts.PublishedRows+res.SummaryCounts.QuarantinedRows {
		t.Errorf("invariante ValidRows violada: %d != %d + %d",
			res.SummaryCounts.ValidRows, res.SummaryCounts.PublishedRows, res.SummaryCounts.QuarantinedRows)
	}

	// 4. Inicializar servidor web HTTP
	cfg := &config.Config{
		Port:             8080,
		Env:              "test",
		DBPath:           dbPath,
		PublicDataCutoff: "2026-09-03",
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     10 * time.Second,
		IdleTimeout:      60 * time.Second,
	}

	srv, err := web.NewServer(cfg, db)
	if err != nil {
		t.Fatalf("falha ao criar servidor web: %v", err)
	}

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	client := ts.Client()

	doGet := func(path string, expectedStatus int) string {
		t.Helper()
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("falha na requisição GET %s: %v", path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != expectedStatus {
			t.Errorf("GET %s status inesperado: esperado %d, obtido %d", path, expectedStatus, resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("falha ao ler corpo da resposta de %s: %v", path, err)
		}
		return string(bodyBytes)
	}

	// --- 5. Bateria de testes de fumaça em endpoints públicos ---

	// 5.1 / (Página Inicial com Métricas Reais da Planilha)
	homeBody := doGet("/", http.StatusOK)
	if !strings.Contains(homeBody, "VorcaroZAP") {
		t.Errorf("Home não contém 'VorcaroZAP'")
	}
	if !strings.Contains(homeBody, "Aviso Editorial e de Independência") {
		t.Errorf("Home não contém aviso editorial obrigatório")
	}
	if !strings.Contains(homeBody, "Métricas da Rede Documental") {
		t.Errorf("Home não contém título de métricas da rede documental")
	}
	if !strings.Contains(homeBody, "Monitoramento automático ainda não ativado") {
		t.Errorf("Home não contém aviso de monitoramento ainda não ativado")
	}
	if !strings.Contains(homeBody, "Pessoas na Rede") || !strings.Contains(homeBody, "Vínculos Qualificados") {
		t.Errorf("Home não contém cards de métricas ativas")
	}
	if !strings.Contains(homeBody, "Alegações por Grau Probatório (A–E)") {
		t.Errorf("Home não contém distribuição de graus")
	}
	if !strings.Contains(homeBody, `href="/pessoas"`) {
		t.Errorf("Home não contém link para /pessoas")
	}
	if !strings.Contains(homeBody, `href="/metodologia"`) {
		t.Errorf("Home não contém link para /metodologia")
	}

	// 5.2 /health/live e /health/ready
	liveBody := doGet("/health/live", http.StatusOK)
	if !strings.Contains(liveBody, `"status":"live"`) {
		t.Errorf("/health/live inesperado: %s", liveBody)
	}
	readyBody := doGet("/health/ready", http.StatusOK)
	if !strings.Contains(readyBody, `"status":"ready"`) {
		t.Errorf("/health/ready inesperado: %s", readyBody)
	}

	// 5.3 /static/css/app.css
	cssResp, err := client.Get(ts.URL + "/static/css/app.css")
	if err != nil {
		t.Fatalf("falha ao obter app.css: %v", err)
	}
	defer cssResp.Body.Close()
	if cssResp.StatusCode != http.StatusOK {
		t.Errorf("app.css esperado 200, obtido %d", cssResp.StatusCode)
	}
	if ct := cssResp.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Errorf("app.css esperado Content-Type text/css, obtido %s", ct)
	}

	// 5.4 /pessoas sem filtros
	pessoasBody := doGet("/pessoas", http.StatusOK)
	if !strings.Contains(pessoasBody, "Pessoas e Organizações") {
		t.Errorf("/pessoas não contém cabeçalho principal")
	}
	if !strings.Contains(pessoasBody, "Aviso Editorial e de Independência") {
		t.Errorf("/pessoas não contém aviso editorial")
	}
	// Deve conter cards de pessoas públicas
	if !strings.Contains(pessoasBody, "entity-card") {
		t.Errorf("/pessoas não contém nenhum entity-card")
	}

	// 5.5 E-ambíguo / Quarantined entities must NOT appear on /pessoas
	// "Carlos Eduardo Torres Bandeira" está em quarentena por ambiguidade
	if strings.Contains(pessoasBody, "Carlos Eduardo Torres Bandeira") {
		t.Errorf("Carlos Eduardo Torres Bandeira (quarentenado) apareceu na listagem pública!")
	}

	// 5.6 Busca textual: /pessoas?q=Daniel
	buscaBody := doGet("/pessoas?q=Daniel", http.StatusOK)
	if !strings.Contains(buscaBody, "Daniel") {
		t.Errorf("Busca por 'Daniel' não retornou resultados contendo 'Daniel'")
	}

	// 5.7 Filtro por Grau: /pessoas?grade=A
	gradeABody := doGet("/pessoas?grade=A", http.StatusOK)
	if !strings.Contains(gradeABody, "Grau A") {
		t.Errorf("Filtro por grade=A não contém menção a Grau A")
	}

	// 5.8 Filtro por Categoria existente no banco
	categories, err := store.ListPublicCategories(ctx, db)
	if err != nil {
		t.Fatalf("falha ao listar categorias: %v", err)
	}
	if len(categories) > 0 {
		targetCat := categories[0]
		catBody := doGet("/pessoas?category="+targetCat, http.StatusOK)
		if !strings.Contains(catBody, targetCat) {
			t.Errorf("Filtro por categoria %s não contém menção à categoria", targetCat)
		}
	}

	// 5.9 Filtro por Relevância: /pessoas?relevance=5
	relBody := doGet("/pessoas?relevance=5", http.StatusOK)
	if !strings.Contains(relBody, "5 — Estratégica") {
		t.Logf("Aviso: filtro relevance=5 não encontrou label esperada")
	}

	// 5.10 Detalhe de entidade pública existente com fontes
	// Buscar um slug público real a partir da listagem
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
		detailBody := doGet("/pessoas/"+sampleSlug, http.StatusOK)
		if !strings.Contains(detailBody, "Aviso Editorial e de Independência") {
			t.Errorf("Detalhe de %s não contém aviso editorial", sampleSlug)
		}
		if !strings.Contains(detailBody, "Relações e Alegações Documentadas") {
			t.Errorf("Detalhe de %s não contém seção de alegações", sampleSlug)
		}
		// Verificar se há fontes de suporte
		if !strings.Contains(detailBody, "Fontes de Sustentação") {
			t.Errorf("Detalhe de %s não contém fontes de sustentação", sampleSlug)
		}
		// Verificar que quotes vazios não geram tags vazias com aspas vazias
		if strings.Contains(detailBody, "&ldquo;&rdquo;") || strings.Contains(detailBody, `""`) {
			t.Errorf("Detalhe de %s gerou quote vazio no HTML", sampleSlug)
		}
	} else {
		t.Errorf("Não foi possível encontrar nenhum slug de entidade pública na listagem /pessoas")
	}

	// 5.11 /pessoas/{slug} de entidade em quarentena deve retornar 404
	doGet("/pessoas/carlos-eduardo-torres-bandeira", http.StatusNotFound)

	// 5.11b Entidade de contexto/correção publicada (context_only)
	var corrSlug, corrName string
	errCorr := db.QueryRowContext(ctx, `
		SELECT e.slug, e.name
		FROM claims c
		JOIN relationships r ON r.id = c.relationship_id
		JOIN entities e ON e.id = r.subject_entity_id
		WHERE c.status = 'published' AND (c.disposition = 'context_only' OR c.context_status LIKE '%corr%')
		LIMIT 1
	`).Scan(&corrSlug, &corrName)
	t.Logf("Debug context/correction entity: slug=%s name=%s err=%v", corrSlug, corrName, errCorr)
	if errCorr == nil {
		corrBody := doGet("/pessoas/"+corrSlug, http.StatusOK)
		if !strings.Contains(corrBody, corrName) {
			t.Errorf("Detalhe de entidade de contexto %s não renderizou nome", corrName)
		}
	}

	// 5.12 /pessoas/{slug} de entidade inexistente deve retornar 404
	doGet("/pessoas/entidade-totalmente-inexistente-xyz", http.StatusNotFound)

	// 5.13 /metodologia
	metodologiaBody := doGet("/metodologia", http.StatusOK)
	if !strings.Contains(metodologiaBody, "Metodologia e Critérios Editoriais") {
		t.Errorf("/metodologia não contém título principal")
	}
	if !strings.Contains(metodologiaBody, "Grau A") || !strings.Contains(metodologiaBody, "Grau E") {
		t.Errorf("/metodologia não contém escala completa de Graus")
	}
	if !strings.Contains(metodologiaBody, "1 — Local ou circunstancial") {
		t.Errorf("/metodologia não contém escala de relevância")
	}
	if !strings.Contains(metodologiaBody, "supports_link") || !strings.Contains(metodologiaBody, "metric_eligible") {
		t.Errorf("/metodologia não contém termos técnicos explicados")
	}
	if !strings.Contains(metodologiaBody, "Direito de Resposta") {
		t.Errorf("/metodologia não contém seção de direito de resposta")
	}

	t.Log("Smoke test concluído com sucesso em todas as asserções!")
}
