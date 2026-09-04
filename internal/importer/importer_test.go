package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func setupTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_importer.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations no banco de teste: %v", err)
	}

	return db, ctx
}

func getOfficialMappingPath() string {
	return filepath.Join("..", "..", "config", "import-mapping-v1.yaml")
}

// createTestExcelFile cria um arquivo XLSX temporário com o formato esperado da aba "Pessoas A-Z".
func createTestExcelFile(t *testing.T, rows [][]string) string {
	t.Helper()
	f := excelize.NewFile()
	sheet := "Pessoas A-Z"
	index, err := f.NewSheet(sheet)
	if err != nil {
		t.Fatalf("falha ao criar aba: %v", err)
	}
	f.SetActiveSheet(index)

	// Linha 6: cabeçalho oficial
	header := []any{
		"Nome",
		"Área",
		"Cargo / papel",
		"Grau de confirmação",
		"Tipo de vínculo",
		"O que está documentado",
		"Contraponto / limite",
		"Situação",
		"Relevância (1–5)",
		"Alcance",
		"Por que importa",
		"Fonte principal",
		"Fonte adicional",
	}
	for colIdx, val := range header {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 6)
		_ = f.SetCellValue(sheet, cell, val)
	}

	// Linhas 7 em diante: dados
	for rIdx, row := range rows {
		rowNum := 7 + rIdx
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rowNum)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_import.xlsx")
	if err := f.SaveAs(filePath); err != nil {
		t.Fatalf("falha ao salvar arquivo excel temporário: %v", err)
	}
	return filePath
}

func TestImporter_DryRunAndAppliedImport(t *testing.T) {
	db, ctx := setupTestDB(t)
	mappingPath := getOfficialMappingPath()
	imp := NewImporter(db, mappingPath)

	rows := [][]string{
		{
			"Pessoa A Confirmada", "Empresas", "Empresário", "A — direto confirmado",
			"Sócio", "Documento comprova sociedade", "Nenhum", "Investigado",
			"4", "Nacional", "Empresário de destaque", "https://noticias.example.com/artigo-a", "",
		},
		{
			"Pessoa B Noticiada", "Finanças", "Banqueiro", "B — direto documentado/controvertido",
			"Operador", "Reportagem detalhada", "Contesta", "Citado",
			"3", "Regional", "Relevante no setor", "https://noticias.example.com/artigo-b", "https://noticias.example.com/artigo-b-extra",
		},
		{
			"Pessoa C Agenda", "Política", "Parlamentar", "C — agenda apenas",
			"Encontro", "Consta em registro de agenda", "Sem tratativa ilícita", "Testemunha",
			"3", "Nacional", "Cargo público", "https://noticias.example.com/artigo-c", "",
		},
		{
			"Pessoa D Potencial", "Construção", "Diretor", "D — indireto/potencial",
			"Contato", "Citação indireta em depoimento", "Não confirmado", "Mencionado",
			"2", "Local", "Atuação secundária", "https://noticias.example.com/artigo-d", "",
		},
		{
			"Pessoa E Ambigua", "Jurídico", "Advogado", "E — fraco/ambíguo",
			"Pista", "Menção vaga sem documento", "Sem confirmação", "Desconhecido",
			"2", "Local", "Pista inicial", "https://noticias.example.com/artigo-e", "",
		},
		{
			"Pessoa E Corrigida", "Comunicação", "Jornalista", "E — fraco/corrigido",
			"Correção", "Menção incorreta retificada", "Retificação pública", "Esclarecido",
			"1", "Local", "Contexto editorial", "https://noticias.example.com/artigo-e-corr", "",
		},
	}

	xlsxPath := createTestExcelFile(t, rows)

	// 1. Executa DRY-RUN
	dryRes, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: xlsxPath,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("dry-run falhou: %v", err)
	}

	if !dryRes.DryRun {
		t.Fatal("esperava DryRun = true no resultado")
	}
	if dryRes.SummaryCounts.ValidRows != 6 {
		t.Fatalf("esperava 6 linhas válidas, obteve %d", dryRes.SummaryCounts.ValidRows)
	}
	if dryRes.SummaryCounts.PublishedRows != 5 { // A, B, C, D e E-corrigido
		t.Fatalf("esperava 5 publicados, obteve %d", dryRes.SummaryCounts.PublishedRows)
	}
	if dryRes.SummaryCounts.QuarantinedRows != 1 { // E-ambíguo
		t.Fatalf("esperava 1 em quarentena, obteve %d", dryRes.SummaryCounts.QuarantinedRows)
	}

	// Valida que o banco continua COMPLETAMENTE vazio após dry-run
	var count int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM entities").Scan(&count)
	if count != 0 {
		t.Fatalf("dry-run gravou entidades no banco: count = %d", count)
	}
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM import_runs").Scan(&count)
	if count != 0 {
		t.Fatalf("dry-run gravou import_runs no banco: count = %d", count)
	}

	// 2. Executa IMPORTAÇÃO APLICADA
	appliedRes, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: xlsxPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("importação aplicada falhou: %v", err)
	}

	if appliedRes.DryRun {
		t.Fatal("esperava DryRun = false")
	}
	if appliedRes.AlreadyImported {
		t.Fatal("não deveria constar como already_imported na primeira execução")
	}
	if appliedRes.SummaryCounts.EntitiesCreated != 6 {
		t.Fatalf("esperava 6 entidades criadas, obteve %d", appliedRes.SummaryCounts.EntitiesCreated)
	}
	if appliedRes.SummaryCounts.ClaimsCreated != 6 {
		t.Fatalf("esperava 6 claims criados, obteve %d", appliedRes.SummaryCounts.ClaimsCreated)
	}
	if appliedRes.SummaryCounts.PublishedRows != 5 {
		t.Fatalf("esperava 5 publicados, obteve %d", appliedRes.SummaryCounts.PublishedRows)
	}
	if appliedRes.SummaryCounts.QuarantinedRows != 1 {
		t.Fatalf("esperava 1 em quarentena, obteve %d", appliedRes.SummaryCounts.QuarantinedRows)
	}

	// 3. Validações diretas no banco de dados pós-importação
	queries := sqlc.New(db)

	// Valida que o caso raiz existe
	rootCase, err := queries.GetCaseBySlug(ctx, RootCaseSlug)
	if err != nil {
		t.Fatalf("caso raiz não encontrado: %v", err)
	}
	if rootCase.Name != RootCaseName {
		t.Errorf("nome do caso raiz: esperado %q, obteve %q", RootCaseName, rootCase.Name)
	}

	// Valida que as sources criadas NÃO possuem título nem autor inventados
	var emptyTitleCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE title = '' AND publisher_or_author = ''").Scan(&emptyTitleCount)
	if err != nil || emptyTitleCount == 0 {
		t.Fatalf("fontes devem conter título e autor vazios para curated_seed: count=%d, err=%v", emptyTitleCount, err)
	}

	// Valida que os evidence_sources NÃO possuem excerpt preenchido (resumo editorial não é citação literal)
	var emptyExcerptCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM evidence_sources WHERE excerpt = ''").Scan(&emptyExcerptCount)
	if err != nil || emptyExcerptCount == 0 {
		t.Fatalf("evidence_sources devem conter excerpt vazio: count=%d, err=%v", emptyExcerptCount, err)
	}

	// Valida que o status de acesso das fontes é 'not_checked'
	var notCheckedCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE source_access_status = 'not_checked'").Scan(&notCheckedCount)
	if err != nil || notCheckedCount == 0 {
		t.Fatalf("todas as fontes do curated_seed devem iniciar como not_checked: count=%d", notCheckedCount)
	}

	// Valida estados dos claims conforme mapping
	// A: published, supports_link, metric_eligible=1
	var claimAStatus string
	var claimAMetric int64
	err = db.QueryRowContext(ctx, "SELECT c.status, c.metric_eligible FROM claims c JOIN entities e ON e.id = (SELECT subject_entity_id FROM relationships WHERE id = c.relationship_id) WHERE e.name = 'Pessoa A Confirmada'").Scan(&claimAStatus, &claimAMetric)
	if err != nil || claimAStatus != "published" || claimAMetric != 1 {
		t.Errorf("Pessoa A: esperado status=published, metric=1; obteve %s, %d", claimAStatus, claimAMetric)
	}

	// D: published, possible_link, metric_eligible=1
	var claimDStatus, claimDDisp string
	var claimDMetric int64
	err = db.QueryRowContext(ctx, "SELECT c.status, c.disposition, c.metric_eligible FROM claims c JOIN entities e ON e.id = (SELECT subject_entity_id FROM relationships WHERE id = c.relationship_id) WHERE e.name = 'Pessoa D Potencial'").Scan(&claimDStatus, &claimDDisp, &claimDMetric)
	if err != nil || claimDStatus != "published" || claimDDisp != "possible_link" || claimDMetric != 1 {
		t.Errorf("Pessoa D: esperado published/possible_link/1; obteve %s/%s/%d", claimDStatus, claimDDisp, claimDMetric)
	}

	// E ambíguo: quarantined, possible_link, metric_eligible=0
	var claimEAmbStatus string
	var claimEAmbMetric int64
	err = db.QueryRowContext(ctx, "SELECT c.status, c.metric_eligible FROM claims c JOIN entities e ON e.id = (SELECT subject_entity_id FROM relationships WHERE id = c.relationship_id) WHERE e.name = 'Pessoa E Ambigua'").Scan(&claimEAmbStatus, &claimEAmbMetric)
	if err != nil || claimEAmbStatus != "quarantined" || claimEAmbMetric != 0 {
		t.Errorf("Pessoa E ambígua: esperado quarantined/0; obteve %s/%d", claimEAmbStatus, claimEAmbMetric)
	}

	// E corrigido: published, context_only, metric_eligible=0
	var claimECorrStatus, claimECorrDisp string
	var claimECorrMetric int64
	err = db.QueryRowContext(ctx, "SELECT c.status, c.disposition, c.metric_eligible FROM claims c JOIN entities e ON e.id = (SELECT subject_entity_id FROM relationships WHERE id = c.relationship_id) WHERE e.name = 'Pessoa E Corrigida'").Scan(&claimECorrStatus, &claimECorrDisp, &claimECorrMetric)
	if err != nil || claimECorrStatus != "published" || claimECorrDisp != "context_only" || claimECorrMetric != 0 {
		t.Errorf("Pessoa E corrigida: esperado published/context_only/0; obteve %s/%s/%d", claimECorrStatus, claimECorrDisp, claimECorrMetric)
	}

	// 4. SEGUNDA EXECUÇÃO: IDEMPOTÊNCIA COMPROVADA
	secondRes, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: xlsxPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("segunda importação falhou com erro: %v", err)
	}
	if !secondRes.AlreadyImported {
		t.Fatal("segunda execução deveria retornar AlreadyImported = true")
	}

	// Valida que nenhum registro adicional foi criado
	var finalEntityCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM entities").Scan(&finalEntityCount)
	if finalEntityCount != 6 {
		t.Fatalf("idempotência violada: quantidade de entidades mudou de 6 para %d", finalEntityCount)
	}
	var finalRunCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM import_runs").Scan(&finalRunCount)
	if finalRunCount != 1 {
		t.Fatalf("idempotência violada: import_runs mudou de 1 para %d", finalRunCount)
	}
}

func TestImporter_QuarantineAndPartialResult(t *testing.T) {
	db, ctx := setupTestDB(t)
	mappingPath := getOfficialMappingPath()
	imp := NewImporter(db, mappingPath)

	rows := [][]string{
		// Linha 1: Válida
		{
			"Pessoa Válida", "Área", "Cargo", "A — direto confirmado",
			"Vínculo", "Documentado", "Limites", "Situação",
			"3", "Alcance", "Justificativa", "https://noticias.example.com/ok", "",
		},
		// Linha 2: Sem nome (erro estrutural -> partial run, linha ignorada)
		{
			"", "Área", "Cargo", "A — direto confirmado",
			"Vínculo", "Documentado", "Limites", "Situação",
			"3", "Alcance", "Justificativa", "https://noticias.example.com/broken1", "",
		},
		// Linha 3: Sem URL válida na fonte principal (erro estrutural -> partial run, linha ignorada)
		{
			"Pessoa Sem URL", "Área", "Cargo", "A — direto confirmado",
			"Vínculo", "Documentado", "Limites", "Situação",
			"3", "Alcance", "Justificativa", "url-invalida-sem-esquema", "",
		},
		// Linha 4: Grau legado desconhecido (vai para quarentena como grau E)
		{
			"Pessoa Grau Desconhecido", "Área", "Cargo", "Grau Inventado Z",
			"Vínculo", "Documentado", "Limites", "Situação",
			"3", "Alcance", "Justificativa", "https://noticias.example.com/grau-desconhecido", "",
		},
		// Linha 5: Cargo/papel ausente (vai para quarentena)
		{
			"Pessoa Sem Cargo", "Área", "", "B — direto documentado/controvertido",
			"Vínculo", "Documentado", "Limites", "Situação",
			"3", "Alcance", "Justificativa", "https://noticias.example.com/sem-cargo", "",
		},
		// Linha 6: Falta justificativa de relevância (erro estrutural -> não pode criar entidade sem justificativa)
		{
			"Pessoa Sem Justificativa", "Área", "Cargo", "B — direto documentado/controvertido",
			"Vínculo", "Documentado", "Limites", "Situação",
			"3", "Alcance", "", "https://noticias.example.com/sem-justificativa", "",
		},
	}

	xlsxPath := createTestExcelFile(t, rows)

	res, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: xlsxPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("importação parcial falhou: %v", err)
	}

	if len(res.RowErrors) != 3 { // linhas 2, 3 e 6
		t.Fatalf("esperava 3 erros estruturais de linha, obteve %d", len(res.RowErrors))
	}
	if res.SummaryCounts.ValidRows != 3 { // linhas 1, 4 e 5
		t.Fatalf("esperava 3 linhas válidas persistidas, obteve %d", res.SummaryCounts.ValidRows)
	}
	if res.SummaryCounts.PublishedRows != 1 { // linha 1
		t.Fatalf("esperava 1 publicado, obteve %d", res.SummaryCounts.PublishedRows)
	}
	if res.SummaryCounts.QuarantinedRows != 2 { // linhas 4 e 5
		t.Fatalf("esperava 2 em quarentena, obteve %d", res.SummaryCounts.QuarantinedRows)
	}

	// Status do import_run no banco deve ser 'partial'
	var runStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM import_runs LIMIT 1").Scan(&runStatus)
	if err != nil || runStatus != "partial" {
		t.Fatalf("status do import_run: esperado 'partial', obteve %q (err=%v)", runStatus, err)
	}

	// Nenhuma entidade deve ter sido criada para as linhas 2, 3 e 6 (sem órfãos)
	var brokenCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM entities WHERE name = 'Pessoa Sem URL'").Scan(&brokenCount)
	if brokenCount != 0 {
		t.Fatalf("linha com erro estrutural não deveria persistir entidade")
	}
}

func TestImporter_RealSpreadsheetSmoke(t *testing.T) {
	db, ctx := setupTestDB(t)
	mappingPath := getOfficialMappingPath()
	realSpreadsheet := filepath.Join("..", "..", "_notes", "mapa-vorcaro-contatos-2026-09-03.xlsx")

	imp := NewImporter(db, mappingPath)

	// 1. Dry run da planilha real
	dryRes, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: realSpreadsheet,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("dry-run na planilha real falhou: %v", err)
	}

	// Verifica se processou as 151 pessoas da planilha atual
	if dryRes.SummaryCounts.ValidRows != 151 {
		t.Errorf("esperava 151 pessoas processadas na planilha real, obteve %d", dryRes.SummaryCounts.ValidRows)
	}
	if dryRes.SummaryCounts.ErrorRows != 0 {
		t.Errorf("planilha real não deveria ter erros estruturais de linha, obteve %d", dryRes.SummaryCounts.ErrorRows)
	}

	// 2. Importação aplicada da planilha real
	appliedRes, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: realSpreadsheet,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("importação aplicada da planilha real falhou: %v", err)
	}

	if appliedRes.SummaryCounts.ValidRows != 151 {
		t.Errorf("esperava 151 pessoas aplicadas, obteve %d", appliedRes.SummaryCounts.ValidRows)
	}
	if appliedRes.SummaryCounts.ClaimsCreated != 151 {
		t.Errorf("esperava 151 claims criados, obteve %d", appliedRes.SummaryCounts.ClaimsCreated)
	}

	// 3. Idempotência da planilha real
	secondRes, err := imp.ImportFromFile(ctx, ImportOptions{
		FilePath: realSpreadsheet,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("segunda importação da planilha real falhou: %v", err)
	}
	if !secondRes.AlreadyImported {
		t.Error("segunda importação da planilha real deveria retornar AlreadyImported = true")
	}
}

func TestImporter_SpreadsheetRejections(t *testing.T) {
	db, ctx := setupTestDB(t)
	mappingPath := getOfficialMappingPath()
	imp := NewImporter(db, mappingPath)

	t.Run("Aba ausente", func(t *testing.T) {
		f := excelize.NewFile()
		_ = f.SetSheetName("Sheet1", "OutraAba")
		tmpPath := filepath.Join(t.TempDir(), "sem_aba.xlsx")
		_ = f.SaveAs(tmpPath)

		_, err := imp.ImportFromFile(ctx, ImportOptions{FilePath: tmpPath})
		if err == nil {
			t.Fatal("esperava erro para aba ausente")
		}
	})

	t.Run("Extensão incompatível", func(t *testing.T) {
		tmpPath := filepath.Join(t.TempDir(), "arquivo.csv")
		_, err := imp.ImportFromFile(ctx, ImportOptions{FilePath: tmpPath})
		if err == nil {
			t.Fatal("esperava erro para extensão incompatível")
		}
	})

	t.Run("Arquivo inexistente", func(t *testing.T) {
		_, err := imp.ImportFromFile(ctx, ImportOptions{FilePath: "/caminho/inexistente.xlsx"})
		if err == nil {
			t.Fatal("esperava erro para arquivo inexistente")
		}
	})
}
