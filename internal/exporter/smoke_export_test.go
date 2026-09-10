package exporter_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/raphaieu/vorcarozap/internal/importer"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/xuri/excelize/v2"
)

func TestSmokeExportFromCuratedSeed(t *testing.T) {
	// Verifica se a planilha seed existe no caminho esperado
	seedPath := filepath.Join("..", "..", "_notes", "mapa-vorcaro-contatos-2026-09-03.xlsx")
	if _, err := os.Stat(seedPath); os.IsNotExist(err) {
		t.Skip("planilha seed não encontrada para smoke test:", seedPath)
	}
	mappingPath := filepath.Join("..", "..", "config", "import-mapping-v1.yaml")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "smoke_export.db")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// Executa importação real do seed
	imp := importer.NewImporter(db, mappingPath)
	res, err := imp.ImportFromFile(ctx, importer.ImportOptions{
		FilePath: seedPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("falha na importação do seed: %v", err)
	}

	if res.SummaryCounts.PublishedRows != 130 {
		t.Fatalf("esperava 130 linhas publicadas, obteve %d", res.SummaryCounts.PublishedRows)
	}
	if res.SummaryCounts.QuarantinedRows != 21 {
		t.Fatalf("esperava 21 linhas em quarentena, obteve %d", res.SummaryCounts.QuarantinedRows)
	}

	// Gera a exportação XLSX
	exp := exporter.New(db, "2026-09-03")
	var buf bytes.Buffer
	if err := exp.WriteTo(ctx, &buf); err != nil {
		t.Fatalf("falha ao gerar XLSX: %v", err)
	}

	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX exportado: %v", err)
	}
	defer f.Close()

	// Valida as 4 abas
	sheets := f.GetSheetList()
	if len(sheets) != 4 {
		t.Fatalf("esperava 4 abas, obteve %d: %v", len(sheets), sheets)
	}

	// 1. Aba Entidades: deve conter 130 entidades públicas + 1 cabeçalho = 131 linhas
	entRows, err := f.GetRows(exporter.SheetEntities)
	if err != nil {
		t.Fatalf("falha ao ler linhas de Entidades: %v", err)
	}
	if len(entRows) != 131 {
		t.Errorf("esperava 131 linhas em Entidades (1 cabeçalho + 130 públicas), obteve %d", len(entRows))
	}

	// 2. Aba Alegações: deve conter 130 claims publicados + 1 cabeçalho = 131 linhas
	claimRows, err := f.GetRows(exporter.SheetClaims)
	if err != nil {
		t.Fatalf("falha ao ler linhas de Alegações: %v", err)
	}
	if len(claimRows) != 131 {
		t.Errorf("esperava 131 linhas em Alegações (1 cabeçalho + 130 públicas), obteve %d", len(claimRows))
	}

	// Valida que nenhum claim de quarentena (21 do seed) vazou
	for _, cr := range claimRows[1:] {
		statusDesc := cr[9] // Situação / Contexto
		disp := cr[7]
		if statusDesc == "quarantined" || disp == "quarantined" {
			t.Errorf("claim em quarentena vazou na exportação: %v", cr)
		}
	}

	// 3. Aba Fontes: deve conter cabeçalho + fontes ativas
	sourceRows, err := f.GetRows(exporter.SheetSources)
	if err != nil {
		t.Fatalf("falha ao ler linhas de Fontes: %v", err)
	}
	if len(sourceRows) < 100 {
		t.Errorf("esperava mais de 100 vínculos de fontes ativas, obteve %d", len(sourceRows))
	}

	// 4. Aba Metodologia: deve conter notas metodológicas
	metaRows, err := f.GetRows(exporter.SheetMethodology)
	if err != nil {
		t.Fatalf("falha ao ler linhas de Metodologia: %v", err)
	}
	if len(metaRows) < 15 {
		t.Errorf("esperava conteúdo extenso em Metodologia, obteve %d linhas", len(metaRows))
	}
}
