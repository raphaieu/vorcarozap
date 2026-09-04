package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

// ImportOptions define os parâmetros para uma execução de importação.
type ImportOptions struct {
	FilePath string
	DryRun   bool
}

// RowError registra um erro estrutural ou de validação associado a uma linha específica.
type RowError struct {
	RowNumber int    `json:"row_number"`
	Field     string `json:"field"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

func (e RowError) String() string {
	return fmt.Sprintf("Linha %d [%s]: %s (%s)", e.RowNumber, e.Field, e.Message, e.Code)
}

// SummaryCounts consolida contagens quantitativas da importação.
type SummaryCounts struct {
	TotalRows       int `json:"total_rows"`
	IgnoredRows     int `json:"ignored_rows"`
	ValidRows       int `json:"valid_rows"`
	PublishedRows   int `json:"published_rows"`
	QuarantinedRows int `json:"quarantined_rows"`
	ErrorRows       int `json:"error_rows"`
	EntitiesCreated int `json:"entities_created"`
	EntitiesReused  int `json:"entities_reused"`
	SourcesCreated  int `json:"sources_created"`
	SourcesReused   int `json:"sources_reused"`
	ClaimsCreated   int `json:"claims_created"`
}

// JSON converte os contadores para representação JSON persistível em import_runs.
func (s SummaryCounts) JSON() string {
	b, err := json.Marshal(s)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ImportResult representa o resultado auditável completo de uma importação (dry-run ou aplicada).
type ImportResult struct {
	FileHash        string        `json:"file_hash"`
	MappingVersion  string        `json:"mapping_version"`
	DryRun          bool          `json:"dry_run"`
	AlreadyImported bool          `json:"already_imported"`
	SummaryCounts   SummaryCounts `json:"summary_counts"`
	Warnings        []string      `json:"warnings,omitempty"`
	RowErrors       []RowError    `json:"row_errors,omitempty"`
}

// SummaryReport gera o relatório textual humano do resultado.
func (r *ImportResult) SummaryReport() string {
	var sb strings.Builder

	mode := "APLICADA"
	if r.DryRun {
		mode = "DRY-RUN (SIMULAÇÃO SEM GRAVAÇÃO)"
	}

	sb.WriteString(fmt.Sprintf("=== RESUMO DA IMPORTAÇÃO (%s) ===\n", mode))
	sb.WriteString(fmt.Sprintf("Arquivo (SHA-256): %s\n", r.FileHash))
	sb.WriteString(fmt.Sprintf("Versão do Mapping: %s\n", r.MappingVersion))

	if r.AlreadyImported {
		sb.WriteString("Status: ARQUIVO JÁ IMPORTADO ANTERIORMENTE (idempotência confirmada)\n")
		sb.WriteString("Nenhuma alteração foi efetuada no banco de dados.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Total de linhas na planilha: %d\n", r.SummaryCounts.TotalRows))
	sb.WriteString(fmt.Sprintf("Linhas vazias/ignoradas:     %d\n", r.SummaryCounts.IgnoredRows))
	sb.WriteString(fmt.Sprintf("Linhas válidas processadas:  %d\n", r.SummaryCounts.ValidRows))
	sb.WriteString(fmt.Sprintf("  - Publicadas (published):  %d\n", r.SummaryCounts.PublishedRows))
	sb.WriteString(fmt.Sprintf("  - Em quarentena:           %d\n", r.SummaryCounts.QuarantinedRows))
	sb.WriteString(fmt.Sprintf("Linhas com erro:             %d\n", r.SummaryCounts.ErrorRows))
	sb.WriteString(fmt.Sprintf("Entidades criadas / reuso:   %d / %d\n", r.SummaryCounts.EntitiesCreated, r.SummaryCounts.EntitiesReused))
	sb.WriteString(fmt.Sprintf("Fontes criadas / reuso:      %d / %d\n", r.SummaryCounts.SourcesCreated, r.SummaryCounts.SourcesReused))
	sb.WriteString(fmt.Sprintf("Claims criados:              %d\n", r.SummaryCounts.ClaimsCreated))

	if len(r.Warnings) > 0 {
		sb.WriteString("\nAvisos:\n")
		for _, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", w))
		}
	}

	if len(r.RowErrors) > 0 {
		sb.WriteString(fmt.Sprintf("\nErros por linha (%d):\n", len(r.RowErrors)))
		for _, e := range r.RowErrors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e.String()))
		}
	}

	return sb.String()
}

// RawRow armazena os valores brutos lidos de uma linha funcional da planilha.
type RawRow struct {
	RowNumber           int
	Nome                string
	Area                string
	CargoPapel          string
	GrauConfirmacao     string
	TipoDeVinculo       string
	OQueEstaDocumentado string
	ContrapontoLimite   string
	Situacao            string
	Relevancia          string
	Alcance             string
	PorQueImporta       string
	FontePrincipal      string
	FonteAdicional      string
}

// IsEmpty informa se a linha não contém dados em nenhuma coluna.
func (r *RawRow) IsEmpty() bool {
	return strings.TrimSpace(r.Nome) == "" &&
		strings.TrimSpace(r.Area) == "" &&
		strings.TrimSpace(r.CargoPapel) == "" &&
		strings.TrimSpace(r.GrauConfirmacao) == "" &&
		strings.TrimSpace(r.TipoDeVinculo) == "" &&
		strings.TrimSpace(r.OQueEstaDocumentado) == "" &&
		strings.TrimSpace(r.ContrapontoLimite) == "" &&
		strings.TrimSpace(r.Situacao) == "" &&
		strings.TrimSpace(r.Relevancia) == "" &&
		strings.TrimSpace(r.Alcance) == "" &&
		strings.TrimSpace(r.PorQueImporta) == "" &&
		strings.TrimSpace(r.FontePrincipal) == "" &&
		strings.TrimSpace(r.FonteAdicional) == ""
}

// ParsedRow contém os dados validados e normalizados prontos para persistência/dry-run.
type ParsedRow struct {
	Raw             RawRow
	NormalizedName  string
	Slug            string
	Relevance       domain.Relevance
	Grade           domain.EvidenceGrade
	InitialState    domain.ClaimStatus
	Disposition     domain.ClaimDisposition
	MetricEligible  bool
	PrimaryURL      string
	PrimaryCanonURL string
	AddURL          string
	AddCanonURL     string
	QuarantineNotes []string
}

// ShouldQuarantine indica se a linha possui inconsistências que forçam status quarantined.
func (p *ParsedRow) ShouldQuarantine() bool {
	return p.InitialState == domain.ClaimStatusQuarantined || len(p.QuarantineNotes) > 0
}
