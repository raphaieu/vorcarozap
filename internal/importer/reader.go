package importer

import (
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

// RequiredColumns lista os nomes canônicos esperados no cabeçalho funcional da planilha.
var RequiredColumns = []string{
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

// ReadSpreadsheet abre um arquivo Excel a partir de um io.Reader ou caminho e extrai as linhas funcionais.
func ReadSpreadsheet(r io.Reader, sheetName string, headerRow int) ([]RawRow, []string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("importer: falha ao abrir planilha excel: %w", err)
	}
	defer f.Close()

	// 1. Validação de aba
	sheetIndex, err := f.GetSheetIndex(sheetName)
	if err != nil || sheetIndex < 0 {
		return nil, nil, fmt.Errorf("importer: aba obrigatória %q não encontrada na planilha", sheetName)
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, nil, fmt.Errorf("importer: falha ao ler linhas da aba %q: %w", sheetName, err)
	}

	if len(rows) < headerRow {
		return nil, nil, fmt.Errorf("importer: planilha possui %d linhas, insuficiente para conter cabeçalho na linha %d", len(rows), headerRow)
	}

	// 2. Leitura e validação do cabeçalho
	headerCells := rows[headerRow-1] // 0-indexed
	colMap := make(map[string]int)
	var warnings []string

	for colIdx, cell := range headerCells {
		normCol := NormalizeUnicode(NormalizeString(cell))
		if normCol == "" {
			continue
		}

		if _, exists := colMap[normCol]; exists {
			return nil, nil, fmt.Errorf("importer: coluna duplicada no cabeçalho: %q", cell)
		}
		colMap[normCol] = colIdx
	}

	// Valida presença de todas as colunas obrigatórias
	for _, reqCol := range RequiredColumns {
		normReq := NormalizeUnicode(NormalizeString(reqCol))
		if _, exists := colMap[normReq]; !exists {
			return nil, nil, fmt.Errorf("importer: coluna obrigatória ausente no cabeçalho: %q", reqCol)
		}
	}

	// Detecta colunas extras
	reqSet := make(map[string]bool, len(RequiredColumns))
	for _, col := range RequiredColumns {
		reqSet[NormalizeUnicode(NormalizeString(col))] = true
	}
	for normCol := range colMap {
		if !reqSet[normCol] {
			warnings = append(warnings, fmt.Sprintf("coluna extra ignorada: %q", normCol))
		}
	}

	// 3. Extração das linhas de dados
	getVal := func(row []string, colName string) string {
		idx, ok := colMap[NormalizeUnicode(NormalizeString(colName))]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	var rawRows []RawRow
	for i := headerRow; i < len(rows); i++ {
		rowNum := i + 1
		rowCells := rows[i]

		raw := RawRow{
			RowNumber:           rowNum,
			Nome:                getVal(rowCells, "Nome"),
			Area:                getVal(rowCells, "Área"),
			CargoPapel:          getVal(rowCells, "Cargo / papel"),
			GrauConfirmacao:     getVal(rowCells, "Grau de confirmação"),
			TipoDeVinculo:       getVal(rowCells, "Tipo de vínculo"),
			OQueEstaDocumentado: getVal(rowCells, "O que está documentado"),
			ContrapontoLimite:   getVal(rowCells, "Contraponto / limite"),
			Situacao:            getVal(rowCells, "Situação"),
			Relevancia:          getVal(rowCells, "Relevância (1–5)"),
			Alcance:             getVal(rowCells, "Alcance"),
			PorQueImporta:       getVal(rowCells, "Por que importa"),
			FontePrincipal:      getVal(rowCells, "Fonte principal"),
			FonteAdicional:      getVal(rowCells, "Fonte adicional"),
		}

		if raw.IsEmpty() {
			continue
		}

		rawRows = append(rawRows, raw)
	}

	return rawRows, warnings, nil
}
