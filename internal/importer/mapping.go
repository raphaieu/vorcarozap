package importer

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

// MappingConfig representa o schema completo do arquivo declarativo de mapping versionado.
type MappingConfig struct {
	Version             string               `yaml:"version"`
	Origin              string               `yaml:"origin"`
	Source              SourceConfig         `yaml:"source"`
	LegacyGradeMapping  map[string]GradeRule `yaml:"legacy_grade_mapping"`
	UnmappedGradeAction string               `yaml:"unmapped_grade_action"`
	InitialState        InitialStateConfig   `yaml:"initial_state"`

	// normalizedRules mapeia o rótulo normalizado (Unicode NFC + espaços compactados) para a regra
	normalizedRules map[string]GradeRule
}

// SourceConfig define os parâmetros de localização funcional dentro da planilha.
type SourceConfig struct {
	Sheet             string `yaml:"sheet"`
	HeaderRow         int    `yaml:"header_row"`
	LegacyGradeColumn string `yaml:"legacy_grade_column"`
}

// GradeRule define a transformação editorial de um rótulo legado para o domínio.
type GradeRule struct {
	Grade          string `yaml:"grade"`
	InitialState   string `yaml:"initial_state"`
	Disposition    string `yaml:"disposition"`
	MetricEligible bool   `yaml:"metric_eligible"`
}

// InitialStateConfig enumera os requisitos de publicação e critérios de quarentena.
type InitialStateConfig struct {
	PublishRequires []string `yaml:"publish_requires"`
	QuarantineOn    []string `yaml:"quarantine_on"`
}

// LoadMappingFromFile carrega e valida o arquivo de configuração de mapeamento YAML.
func LoadMappingFromFile(filePath string) (*MappingConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("importer: falha ao ler arquivo de mapping %q: %w", filePath, err)
	}

	return ParseAndValidateMapping(data)
}

// ParseAndValidateMapping analisa os bytes YAML e valida rigorosamente as regras contra o domínio.
func ParseAndValidateMapping(data []byte) (*MappingConfig, error) {
	var cfg MappingConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("importer: erro ao decodificar yaml de mapping: %w", err)
	}

	if NormalizeString(cfg.Version) == "" {
		return nil, fmt.Errorf("importer: mapping version é obrigatório")
	}

	if cfg.Origin != string(domain.ClaimOriginCuratedSeed) {
		return nil, fmt.Errorf("importer: origin esperado %q, obtido %q", domain.ClaimOriginCuratedSeed, cfg.Origin)
	}

	if NormalizeString(cfg.Source.Sheet) == "" {
		return nil, fmt.Errorf("importer: source.sheet é obrigatório")
	}

	if cfg.Source.HeaderRow < 1 {
		return nil, fmt.Errorf("importer: source.header_row deve ser >= 1, obtido %d", cfg.Source.HeaderRow)
	}

	if NormalizeString(cfg.Source.LegacyGradeColumn) == "" {
		return nil, fmt.Errorf("importer: source.legacy_grade_column é obrigatório")
	}

	if len(cfg.LegacyGradeMapping) == 0 {
		return nil, fmt.Errorf("importer: legacy_grade_mapping não pode ser vazio")
	}

	if cfg.UnmappedGradeAction != string(domain.ClaimStatusQuarantined) {
		return nil, fmt.Errorf("importer: unmapped_grade_action deve ser %q, obtido %q", domain.ClaimStatusQuarantined, cfg.UnmappedGradeAction)
	}

	cfg.normalizedRules = make(map[string]GradeRule, len(cfg.LegacyGradeMapping))

	for rawLabel, rule := range cfg.LegacyGradeMapping {
		normLabel := NormalizeUnicode(NormalizeString(rawLabel))
		if normLabel == "" {
			return nil, fmt.Errorf("importer: rótulo de grau legado vazio encontrado no mapping")
		}

		if _, exists := cfg.normalizedRules[normLabel]; exists {
			return nil, fmt.Errorf("importer: rótulo legado duplicado após normalização: %q", rawLabel)
		}

		// Valida enums contra internal/domain
		grade := domain.EvidenceGrade(rule.Grade)
		if !grade.IsValid() {
			return nil, fmt.Errorf("importer: grau %q inválido para o rótulo %q (permitidos: A..E)", rule.Grade, rawLabel)
		}

		status := domain.ClaimStatus(rule.InitialState)
		if !status.IsValid() {
			return nil, fmt.Errorf("importer: initial_state %q inválido para o rótulo %q", rule.InitialState, rawLabel)
		}

		disp := domain.ClaimDisposition(rule.Disposition)
		if !disp.IsValid() {
			return nil, fmt.Errorf("importer: disposition %q inválida para o rótulo %q", rule.Disposition, rawLabel)
		}

		cfg.normalizedRules[normLabel] = rule
	}

	return &cfg, nil
}

// LookupGrade busca a regra correspondente a um rótulo legado via comparação exata pós-normalização.
// Retorna a regra e um booleano indicando se o rótulo foi reconhecido.
func (m *MappingConfig) LookupGrade(rawLabel string) (GradeRule, bool) {
	normLabel := NormalizeUnicode(NormalizeString(rawLabel))
	rule, found := m.normalizedRules[normLabel]
	if !found {
		return GradeRule{
			Grade:          string(domain.EvidenceGradeE),
			InitialState:   string(domain.ClaimStatusQuarantined),
			Disposition:    string(domain.DispositionPossibleLink),
			MetricEligible: false,
		}, false
	}
	return rule, true
}
