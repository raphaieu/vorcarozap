package importer

import (
	"path/filepath"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

func TestLoadMappingFromFile_RealMapping(t *testing.T) {
	mappingPath := filepath.Join("..", "..", "config", "import-mapping-v1.yaml")
	cfg, err := LoadMappingFromFile(mappingPath)
	if err != nil {
		t.Fatalf("falha ao carregar mapping oficial: %v", err)
	}

	if cfg.Version != "import-mapping-v1" {
		t.Errorf("esperava versão import-mapping-v1, obteve %s", cfg.Version)
	}
	if cfg.Origin != string(domain.ClaimOriginCuratedSeed) {
		t.Errorf("esperava origin curated_seed, obteve %s", cfg.Origin)
	}
	if cfg.Source.Sheet != "Pessoas A-Z" {
		t.Errorf("esperava aba Pessoas A-Z, obteve %s", cfg.Source.Sheet)
	}
	if cfg.Source.HeaderRow != 6 {
		t.Errorf("esperava header_row 6, obteve %d", cfg.Source.HeaderRow)
	}
	if cfg.Source.LegacyGradeColumn != "Grau de confirmação" {
		t.Errorf("esperava coluna Grau de confirmação, obteve %s", cfg.Source.LegacyGradeColumn)
	}

	// Testa regras conhecidas
	testCases := []struct {
		rawLabel       string
		expectedGrade  domain.EvidenceGrade
		expectedState  domain.ClaimStatus
		expectedDisp   domain.ClaimDisposition
		expectedMetric bool
		expectedFound  bool
	}{
		{
			rawLabel:       "A — direto confirmado",
			expectedGrade:  domain.EvidenceGradeA,
			expectedState:  domain.ClaimStatusPublished,
			expectedDisp:   domain.DispositionSupportsLink,
			expectedMetric: true,
			expectedFound:  true,
		},
		{
			rawLabel:       "B — direto documentado/controvertido",
			expectedGrade:  domain.EvidenceGradeB,
			expectedState:  domain.ClaimStatusPublished,
			expectedDisp:   domain.DispositionSupportsLink,
			expectedMetric: true,
			expectedFound:  true,
		},
		{
			rawLabel:       "C — agenda apenas",
			expectedGrade:  domain.EvidenceGradeC,
			expectedState:  domain.ClaimStatusPublished,
			expectedDisp:   domain.DispositionSupportsLink,
			expectedMetric: true,
			expectedFound:  true,
		},
		{
			rawLabel:       "D — indireto/potencial",
			expectedGrade:  domain.EvidenceGradeD,
			expectedState:  domain.ClaimStatusPublished,
			expectedDisp:   domain.DispositionPossibleLink,
			expectedMetric: true,
			expectedFound:  true,
		},
		{
			rawLabel:       "E — fraco/ambíguo",
			expectedGrade:  domain.EvidenceGradeE,
			expectedState:  domain.ClaimStatusQuarantined,
			expectedDisp:   domain.DispositionPossibleLink,
			expectedMetric: false,
			expectedFound:  true,
		},
		{
			rawLabel:       "E — fraco/corrigido",
			expectedGrade:  domain.EvidenceGradeE,
			expectedState:  domain.ClaimStatusPublished,
			expectedDisp:   domain.DispositionContextOnly,
			expectedMetric: false,
			expectedFound:  true,
		},
		{
			rawLabel:       "Rótulo Desconhecido ou Inexistente",
			expectedGrade:  "",
			expectedState:  domain.ClaimStatusQuarantined,
			expectedDisp:   domain.DispositionPossibleLink,
			expectedMetric: false,
			expectedFound:  false,
		},
	}

	for _, tc := range testCases {
		rule, found := cfg.LookupGrade(tc.rawLabel)
		if found != tc.expectedFound {
			t.Errorf("LookupGrade(%q) found=%v; esperado %v", tc.rawLabel, found, tc.expectedFound)
		}
		if rule.Grade != string(tc.expectedGrade) {
			t.Errorf("LookupGrade(%q) grade=%s; esperado %s", tc.rawLabel, rule.Grade, tc.expectedGrade)
		}
		if rule.InitialState != string(tc.expectedState) {
			t.Errorf("LookupGrade(%q) initial_state=%s; esperado %s", tc.rawLabel, rule.InitialState, tc.expectedState)
		}
		if rule.Disposition != string(tc.expectedDisp) {
			t.Errorf("LookupGrade(%q) disposition=%s; esperado %s", tc.rawLabel, rule.Disposition, tc.expectedDisp)
		}
		if rule.MetricEligible != tc.expectedMetric {
			t.Errorf("LookupGrade(%q) metric_eligible=%v; esperado %v", tc.rawLabel, rule.MetricEligible, tc.expectedMetric)
		}
	}
}

func TestParseAndValidateMapping_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		errMsg string
	}{
		{
			name: "Versão ausente",
			yaml: `
origin: curated_seed
source: { sheet: "S", header_row: 6, legacy_grade_column: "G" }
legacy_grade_mapping:
  "A": { grade: "A", initial_state: "published", disposition: "supports_link", metric_eligible: true }
unmapped_grade_action: quarantined
`,
			errMsg: "mapping version é obrigatório",
		},
		{
			name: "Origem incompatível",
			yaml: `
version: v1
origin: openrouter
source: { sheet: "S", header_row: 6, legacy_grade_column: "G" }
legacy_grade_mapping:
  "A": { grade: "A", initial_state: "published", disposition: "supports_link", metric_eligible: true }
unmapped_grade_action: quarantined
`,
			errMsg: "origin esperado \"curated_seed\"",
		},
		{
			name: "Grau inválido",
			yaml: `
version: v1
origin: curated_seed
source: { sheet: "S", header_row: 6, legacy_grade_column: "G" }
legacy_grade_mapping:
  "Z": { grade: "Z", initial_state: "published", disposition: "supports_link", metric_eligible: true }
unmapped_grade_action: quarantined
`,
			errMsg: "grau \"Z\" inválido",
		},
		{
			name: "Status inicial inválido",
			yaml: `
version: v1
origin: curated_seed
source: { sheet: "S", header_row: 6, legacy_grade_column: "G" }
legacy_grade_mapping:
  "A": { grade: "A", initial_state: "approved_auto", disposition: "supports_link", metric_eligible: true }
unmapped_grade_action: quarantined
`,
			errMsg: "initial_state \"approved_auto\" inválido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAndValidateMapping([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("esperava erro contendo %q, mas obteve nil", tt.errMsg)
			}
		})
	}
}
