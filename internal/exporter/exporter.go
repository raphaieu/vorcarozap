package exporter

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

const (
	SheetEntities    = "Entidades"
	SheetClaims      = "Alegações e Relações"
	SheetSources     = "Evidências e Fontes"
	SheetMethodology = "Metodologia e Critérios"
)

// Exporter encapsula a geração da planilha XLSX a partir do banco de dados SQLite.
type Exporter struct {
	db               *sql.DB
	publicDataCutoff string
}

// New cria uma nova instância do Exporter.
func New(db *sql.DB, cutoff string) *Exporter {
	return &Exporter{
		db:               db,
		publicDataCutoff: cutoff,
	}
}

// SanitizeCellText neutraliza caracteres disparadores de fórmulas de planilhas (Formula Injection).
// Se o texto iniciar com '=', '+', '-', '@', '\t' ou '\r', prefixa com apóstrofo "'".
func SanitizeCellText(s string) string {
	if len(s) == 0 {
		return ""
	}
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if len(trimmed) > 0 {
		first := trimmed[0]
		if first == '=' || first == '+' || first == '-' || first == '@' || first == '\t' || first == '\r' {
			return "'" + s
		}
	}
	return s
}

// Generate extrai os dados públicos atuais do banco e constrói o arquivo XLSX.
func (e *Exporter) Generate(ctx context.Context) (*excelize.File, error) {
	data, err := store.GetPublicExportData(ctx, e.db)
	if err != nil {
		return nil, fmt.Errorf("exporter: falha ao extrair dados públicos: %w", err)
	}

	f := excelize.NewFile()

	// Configuração das abas
	defaultSheet := f.GetSheetName(0)
	if defaultSheet != "" {
		_ = f.SetSheetName(defaultSheet, SheetEntities)
	} else {
		_, _ = f.NewSheet(SheetEntities)
	}
	_, _ = f.NewSheet(SheetClaims)
	_, _ = f.NewSheet(SheetSources)
	_, _ = f.NewSheet(SheetMethodology)

	// Estilos visuais
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:   true,
			Color:  "FFFFFF",
			Family: "Segoe UI",
			Size:   10,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"1B365D"}, // Azul escuro institucional
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
			WrapText:   true,
		},
		Border: []excelize.Border{
			{Type: "bottom", Color: "D0D7DE", Style: 1},
			{Type: "top", Color: "D0D7DE", Style: 1},
		},
	})
	if err != nil {
		headerStyle = 0
	}

	metaHeaderStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:   true,
			Color:  "FFFFFF",
			Family: "Segoe UI",
			Size:   10,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"244B7A"}, // Tom secundário
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
	})
	if err != nil {
		metaHeaderStyle = 0
	}

	// 1. Preenchimento da aba Entidades
	if err := e.populateEntitiesSheet(f, data.Entities, headerStyle); err != nil {
		return nil, fmt.Errorf("exporter: falha ao preencher aba %s: %w", SheetEntities, err)
	}

	// 2. Preenchimento da aba Alegações e Relações
	if err := e.populateClaimsSheet(f, data.Claims, headerStyle); err != nil {
		return nil, fmt.Errorf("exporter: falha ao preencher aba %s: %w", SheetClaims, err)
	}

	// 3. Preenchimento da aba Evidências e Fontes
	if err := e.populateSourcesSheet(f, data.Sources, headerStyle); err != nil {
		return nil, fmt.Errorf("exporter: falha ao preencher aba %s: %w", SheetSources, err)
	}

	// 4. Preenchimento da aba Metodologia e Critérios
	if err := e.populateMethodologySheet(f, metaHeaderStyle); err != nil {
		return nil, fmt.Errorf("exporter: falha ao preencher aba %s: %w", SheetMethodology, err)
	}

	// Define a aba de Entidades como ativa ao abrir a planilha
	entitiesIdx, _ := f.GetSheetIndex(SheetEntities)
	if entitiesIdx >= 0 {
		f.SetActiveSheet(entitiesIdx)
	}

	return f, nil
}

// WriteTo gera a planilha e grava seu conteúdo binário diretamente no io.Writer.
func (e *Exporter) WriteTo(ctx context.Context, w io.Writer) error {
	f, err := e.Generate(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	return f.Write(w)
}

func (e *Exporter) populateEntitiesSheet(f *excelize.File, rows []sqlc.ListPublicEntitiesForExportRow, headerStyle int) error {
	headers := []string{
		"Slug / Identificador",
		"Nome",
		"Tipo",
		"Categoria",
		"Cargo / Contexto",
		"Alcance",
		"Resumo Editorial",
		"Relevância (1-5)",
		"Justificativa da Relevância",
		"Grau Mais Alto",
		"Alegações Públicas",
		"Última Atualização",
	}

	colWidths := map[string]float64{
		"A": 22, "B": 28, "C": 15, "D": 22, "E": 26, "F": 18,
		"G": 35, "H": 16, "I": 35, "J": 15, "K": 18, "L": 22,
	}
	for col, width := range colWidths {
		_ = f.SetColWidth(SheetEntities, col, col, width)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(SheetEntities, cell, h)
		if headerStyle != 0 {
			_ = f.SetCellStyle(SheetEntities, cell, cell, headerStyle)
		}
	}
	_ = f.SetRowHeight(SheetEntities, 1, 28)

	// Congela a linha de cabeçalho
	_ = f.SetPanes(SheetEntities, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	if len(rows) == 0 {
		_ = f.SetCellStr(SheetEntities, "A2", "Nenhuma entidade pública disponível no momento.")
		return nil
	}

	for idx, ent := range rows {
		rowNum := idx + 2
		tipoFormatado := "Pessoa física"
		if ent.Type == "organization" {
			tipoFormatado = "Organização"
		}

		vals := []string{
			SanitizeCellText(ent.Slug),
			SanitizeCellText(ent.Name),
			SanitizeCellText(tipoFormatado),
			SanitizeCellText(ent.Category),
			SanitizeCellText(ent.RoleOrContext),
			SanitizeCellText(ent.Reach),
			SanitizeCellText(ent.Summary),
			fmt.Sprintf("%d", ent.Relevance),
			SanitizeCellText(ent.RelevanceRationale),
			SanitizeCellText(ent.HighestGrade),
			fmt.Sprintf("%d", ent.PublicClaimsCount),
			SanitizeCellText(ent.LastPublicUpdatedAt),
		}

		for cIdx, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rowNum)
			_ = f.SetCellStr(SheetEntities, cell, val)
		}
	}

	return nil
}

func (e *Exporter) populateClaimsSheet(f *excelize.File, rows []sqlc.ListPublicClaimsForExportRow, headerStyle int) error {
	headers := []string{
		"ID da Alegação",
		"Entidade Sujeito",
		"Slug Sujeito",
		"Tipo de Relação",
		"Alvo da Relação",
		"Fato Documentado / Proposição",
		"Grau Editorial",
		"Disposição",
		"Elegível nas Métricas",
		"Situação / Contexto",
		"Atribuição",
		"Data de Criação",
		"Última Atualização",
	}

	colWidths := map[string]float64{
		"A": 36, "B": 28, "C": 22, "D": 22, "E": 28, "F": 45,
		"G": 15, "H": 18, "I": 20, "J": 25, "K": 25, "L": 22, "M": 22,
	}
	for col, width := range colWidths {
		_ = f.SetColWidth(SheetClaims, col, col, width)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(SheetClaims, cell, h)
		if headerStyle != 0 {
			_ = f.SetCellStyle(SheetClaims, cell, cell, headerStyle)
		}
	}
	_ = f.SetRowHeight(SheetClaims, 1, 28)

	_ = f.SetPanes(SheetClaims, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	if len(rows) == 0 {
		_ = f.SetCellStr(SheetClaims, "A2", "Nenhuma alegação pública disponível no momento.")
		return nil
	}

	for idx, c := range rows {
		rowNum := idx + 2
		alvoFormatado := c.TargetEntityName
		if alvoFormatado == "" && c.CaseName != "" {
			alvoFormatado = "Caso: " + c.CaseName
		}

		elegivelTexto := "Não"
		if c.MetricEligible == 1 {
			elegivelTexto = "Sim"
		}

		vals := []string{
			SanitizeCellText(c.ClaimID),
			SanitizeCellText(c.SubjectEntityName),
			SanitizeCellText(c.SubjectEntitySlug),
			SanitizeCellText(c.RelationshipType),
			SanitizeCellText(alvoFormatado),
			SanitizeCellText(c.Proposition),
			SanitizeCellText(c.Grade),
			SanitizeCellText(c.Disposition),
			SanitizeCellText(elegivelTexto),
			SanitizeCellText(c.ContextStatus),
			SanitizeCellText(c.Attribution),
			SanitizeCellText(c.CreatedAt),
			SanitizeCellText(c.UpdatedAt),
		}

		for cIdx, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rowNum)
			_ = f.SetCellStr(SheetClaims, cell, val)
		}
	}

	return nil
}

func (e *Exporter) populateSourcesSheet(f *excelize.File, rows []sqlc.ListPublicEvidenceSourcesForExportRow, headerStyle int) error {
	headers := []string{
		"ID da Evidência",
		"ID da Alegação",
		"Entidade Sujeito",
		"Slug Sujeito",
		"Resumo da Evidência",
		"Papel da Fonte",
		"Localizador Documental",
		"Trecho Literal Citado",
		"Título da Fonte",
		"Veículo / Autor",
		"URL Canônica",
		"Status de Acesso",
	}

	colWidths := map[string]float64{
		"A": 36, "B": 36, "C": 28, "D": 22, "E": 40, "F": 18,
		"G": 22, "H": 45, "I": 35, "J": 26, "K": 40, "L": 18,
	}
	for col, width := range colWidths {
		_ = f.SetColWidth(SheetSources, col, col, width)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellStr(SheetSources, cell, h)
		if headerStyle != 0 {
			_ = f.SetCellStyle(SheetSources, cell, cell, headerStyle)
		}
	}
	_ = f.SetRowHeight(SheetSources, 1, 28)

	_ = f.SetPanes(SheetSources, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	if len(rows) == 0 {
		_ = f.SetCellStr(SheetSources, "A2", "Nenhuma fonte pública disponível no momento.")
		return nil
	}

	for idx, s := range rows {
		rowNum := idx + 2
		papelTexto := s.Role
		switch s.Role {
		case "supports":
			papelTexto = "Sustentação"
		case "contradicts":
			papelTexto = "Contraponto / Defesa"
		case "contextualizes":
			papelTexto = "Contextualização"
		}

		statusAcessoTexto := s.SourceAccessStatus
		switch s.SourceAccessStatus {
		case "reachable":
			statusAcessoTexto = "Acessível (verificado)"
		case "cited_by_provider":
			statusAcessoTexto = "Citado por provedor"
		case "not_checked":
			statusAcessoTexto = "Não verificado previamente"
		case "unreachable":
			statusAcessoTexto = "Inacessível"
		}

		vals := []string{
			SanitizeCellText(s.EvidenceID),
			SanitizeCellText(s.ClaimID),
			SanitizeCellText(s.SubjectEntityName),
			SanitizeCellText(s.SubjectEntitySlug),
			SanitizeCellText(s.EvidenceSummary),
			SanitizeCellText(papelTexto),
			SanitizeCellText(s.Locator),
			SanitizeCellText(s.Excerpt),
			SanitizeCellText(s.SourceTitle),
			SanitizeCellText(s.SourcePublisher),
			SanitizeCellText(s.CanonicalUrl),
			SanitizeCellText(statusAcessoTexto),
		}

		for cIdx, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rowNum)
			_ = f.SetCellStr(SheetSources, cell, val)
		}
	}

	return nil
}

func (e *Exporter) populateMethodologySheet(f *excelize.File, metaHeaderStyle int) error {
	_ = f.SetColWidth(SheetMethodology, "A", "A", 28)
	_ = f.SetColWidth(SheetMethodology, "B", "B", 75)

	sectionStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:   true,
			Color:  "1B365D",
			Family: "Segoe UI",
			Size:   12,
		},
		Border: []excelize.Border{
			{Type: "bottom", Color: "1B365D", Style: 2},
		},
	})

	boldStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:   true,
			Family: "Segoe UI",
			Size:   10,
		},
		Alignment: &excelize.Alignment{
			Vertical: "top",
		},
	})

	textStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Family: "Segoe UI",
			Size:   10,
		},
		Alignment: &excelize.Alignment{
			Vertical: "top",
			WrapText: true,
		},
	})

	row := 1

	// Título Principal
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "VorcaroZAP — Metodologia, Critérios Editoriais e Notas Legais")
	if sectionStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), sectionStyle)
	}
	_ = f.SetRowHeight(SheetMethodology, row, 26)
	row += 2

	// Informações Gerais
	geralData := [][]string{
		{"Data da Exportação", time.Now().UTC().Format(time.RFC3339)},
		{"Data de Corte da Base", e.publicDataCutoff},
		{"Origem dos Dados", "Base SQLite canônica pública do VorcaroZAP (public_claims_view)."},
		{"Escopo do Arquivo", "Esta planilha contém exclusivamente registros com estado 'published' sustentados por fontes ativas. Registros em quarentena, rejeitados ou administrativos não são incluídos."},
	}

	for _, g := range geralData {
		_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), SanitizeCellText(g[0]))
		_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("B%d", row), SanitizeCellText(g[1]))
		if boldStyle != 0 {
			_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), boldStyle)
		}
		if textStyle != 0 {
			_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("B%d", row), fmt.Sprintf("B%d", row), textStyle)
		}
		row++
	}
	row++

	// Aviso Editorial e Independência
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "Aviso Editorial e de Independência")
	if sectionStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), sectionStyle)
	}
	row++

	avisoTexto := "Projeto independente de pesquisa e documentação jornalística. A presença de uma pessoa ou organização nesta base documental decorre de citação em documentos, inquéritos ou reportagens públicas e NÃO implica culpa, envolvimento em conluio, julgamento moral ou imputação de ilícito. A avaliação documental distingue a origem primária da informação do veículo publicador: múltiplos veículos reproduzindo a mesma peça não configuram corroboração independente."
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "Nota de Responsabilidade")
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("B%d", row), SanitizeCellText(avisoTexto))
	if boldStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), boldStyle)
	}
	if textStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("B%d", row), fmt.Sprintf("B%d", row), textStyle)
	}
	_ = f.SetRowHeight(SheetMethodology, row, 45)
	row += 2

	// Escala de Graus A a E
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "Escala Editorial de Graus (A a E)")
	if sectionStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), sectionStyle)
	}
	row++

	graus := [][]string{
		{"Grau A", "Direto confirmado — Citação ou registro em laudo pericial, documento oficial ou registro probatório direto que atesta o vínculo ou contato."},
		{"Grau B", "Direto documentado / controvertido — Documentado em fontes públicas ou reportagens investigativas, com contraponto, contestação ou pendência de confirmação pericial plena."},
		{"Grau C", "Agenda / Menção em registro — Registro em lista de contatos, agenda ou menção incidental em comunicação, sem demonstração de tratativa ilícita ou relacionamento operacional continuado."},
		{"Grau D", "Indireto / Potencial — Vínculo atribuído por terceiros ou relação societária/familiar indireta, dependente de apuração documental posterior. Elegível nas métricas de rede como vínculo potencial."},
		{"Grau E (corrigido)", "Fraco / Corrigido — Menção fraca, esclarecida, retificada ou de mero contexto histórico. Mantida no acervo público como contexto/correção, mas NÃO elegível nas métricas de rede."},
		{"Grau E (ambíguo)", "Fraco / Ambíguo — Homônimos não confirmados, fontes inconclusivas ou alegações sem suporte. Ficam mantidos em quarentena técnica e NÃO constam nesta exportação pública."},
	}

	for _, g := range graus {
		_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), SanitizeCellText(g[0]))
		_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("B%d", row), SanitizeCellText(g[1]))
		if boldStyle != 0 {
			_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), boldStyle)
		}
		if textStyle != 0 {
			_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("B%d", row), fmt.Sprintf("B%d", row), textStyle)
		}
		_ = f.SetRowHeight(SheetMethodology, row, 28)
		row++
	}
	row++

	// Escala de Relevância Institucional (1 a 5)
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "Escala de Relevância Pública (1 a 5)")
	if sectionStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), sectionStyle)
	}
	row++

	relevancias := [][]string{
		{"Nível 5", "Protagonista central / Principal investigado ou controlador institucional diretamente envolvido."},
		{"Nível 4", "Alta relevância / Decisor financeiro ou institucional de primeiro escalão com papel executivo documentado."},
		{"Nível 3", "Relevância média / Operador, intermediário direto ou beneficiário com participação comprovada."},
		{"Nível 2", "Relevância moderada / Citado contextual com cargo institucional ou representação funcional."},
		{"Nível 1", "Relevância periférica / Contato pontual ou menção incidental sem papel deliberativo demonstrado."},
	}

	for _, r := range relevancias {
		_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), SanitizeCellText(r[0]))
		_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("B%d", row), SanitizeCellText(r[1]))
		if boldStyle != 0 {
			_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), boldStyle)
		}
		if textStyle != 0 {
			_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("B%d", row), fmt.Sprintf("B%d", row), textStyle)
		}
		row++
	}
	row++

	// Elegibilidade Métrica de Rede
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "Critérios de Elegibilidade de Métricas")
	if sectionStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), sectionStyle)
	}
	row++

	metricasTexto := "As métricas de rede apresentadas no VorcaroZAP consideram exclusivamente alegações publicadas com 'metric_eligible = true' (Graus A, B, C e D no modelo inicial). Alegações de contexto, correções e retificações (como o Grau E corrigido) permanecem públicas para integridade documental e transparência, mas possuem 'metric_eligible = false', evitando inflar artificialmente os indicadores de vínculos e conexões da rede."
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("A%d", row), "Rede vs. Acervo")
	_ = f.SetCellStr(SheetMethodology, fmt.Sprintf("B%d", row), SanitizeCellText(metricasTexto))
	if boldStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), boldStyle)
	}
	if textStyle != 0 {
		_ = f.SetCellStyle(SheetMethodology, fmt.Sprintf("B%d", row), fmt.Sprintf("B%d", row), textStyle)
	}
	_ = f.SetRowHeight(SheetMethodology, row, 45)

	return nil
}
