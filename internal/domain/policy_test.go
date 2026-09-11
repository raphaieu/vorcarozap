package domain_test

import (
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

func validSemanticInput() domain.SemanticGateInput {
	return domain.SemanticGateInput{
		IdentityMatch:            true,
		ClaimSupported:           true,
		ClaimOverstatesSource:    false,
		AttributionExplicit:      true,
		GradeCompatible:          true,
		ContainsIllicitInference: false,
		Uncertainties:            []string{},
	}
}

func TestEvaluateSemanticGate_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(in *domain.SemanticGateInput)
		expectedPassed bool
		expectedReason string
	}{
		{
			name:           "Gate semântico totalmente aprovado",
			mutate:         func(in *domain.SemanticGateInput) {},
			expectedPassed: true,
		},
		{
			name: "Incompatibilidade de identidade",
			mutate: func(in *domain.SemanticGateInput) {
				in.IdentityMatch = false
			},
			expectedPassed: false,
			expectedReason: "semantic_identity_mismatch",
		},
		{
			name: "Alegação sem suporte",
			mutate: func(in *domain.SemanticGateInput) {
				in.ClaimSupported = false
			},
			expectedPassed: false,
			expectedReason: "semantic_claim_unsupported",
		},
		{
			name: "Extrapolação de fonte",
			mutate: func(in *domain.SemanticGateInput) {
				in.ClaimOverstatesSource = true
			},
			expectedPassed: false,
			expectedReason: "semantic_claim_overstates_source",
		},
		{
			name: "Atribuição não explícita",
			mutate: func(in *domain.SemanticGateInput) {
				in.AttributionExplicit = false
			},
			expectedPassed: false,
			expectedReason: "semantic_attribution_not_explicit",
		},
		{
			name: "Grau incompatível com evidência",
			mutate: func(in *domain.SemanticGateInput) {
				in.GradeCompatible = false
			},
			expectedPassed: false,
			expectedReason: "semantic_grade_incompatible",
		},
		{
			name: "Contém inferência ilícita ou de culpa",
			mutate: func(in *domain.SemanticGateInput) {
				in.ContainsIllicitInference = true
			},
			expectedPassed: false,
			expectedReason: "semantic_contains_illicit_inference",
		},
		{
			name: "Presença de incertezas declaradas",
			mutate: func(in *domain.SemanticGateInput) {
				in.Uncertainties = []string{"Possível homônimo entre pessoas públicas"}
			},
			expectedPassed: false,
			expectedReason: "semantic_uncertainty: Possível homônimo entre pessoas públicas",
		},
		{
			name: "Incerteza contendo apenas espaços deve reprovar",
			mutate: func(in *domain.SemanticGateInput) {
				in.Uncertainties = []string{"   "}
			},
			expectedPassed: false,
			expectedReason: "semantic_uncertainty: blank_uncertainty",
		},
		{
			name: "Incerteza contendo string vazia deve reprovar",
			mutate: func(in *domain.SemanticGateInput) {
				in.Uncertainties = []string{""}
			},
			expectedPassed: false,
			expectedReason: "semantic_uncertainty: blank_uncertainty",
		},
		{
			name: "Incerteza contendo quebra de linha/tab deve reprovar",
			mutate: func(in *domain.SemanticGateInput) {
				in.Uncertainties = []string{"\t\n  "}
			},
			expectedPassed: false,
			expectedReason: "semantic_uncertainty: blank_uncertainty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validSemanticInput()
			tt.mutate(&in)

			res := domain.EvaluateSemanticGate(in)
			if res.Passed != tt.expectedPassed {
				t.Errorf("esperava Passed=%v, obtido Passed=%v (reasons: %v)", tt.expectedPassed, res.Passed, res.Reasons)
			}
			if tt.expectedReason != "" {
				found := false
				for _, r := range res.Reasons {
					if r == tt.expectedReason {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("esperava motivo %q na lista de razões, obtido %v", tt.expectedReason, res.Reasons)
				}
			}
		})
	}
}

func TestEvaluatePublishingPolicy_TableDriven(t *testing.T) {
	tests := []struct {
		name             string
		input            domain.PublishingPolicyInput
		expectedAction   domain.PolicyAction
		expectedDisp     domain.ClaimDisposition
		expectedEligible bool
		expectedReason   string
	}{
		{
			name: "Grau A com gates aprovados -> Publicação supports_link",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeA,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Aquisição de controle societário aprovada",
				ContextLimits:    "Limites contratuais",
			},
			expectedAction:   domain.PolicyActionPublish,
			expectedDisp:     domain.DispositionSupportsLink,
			expectedEligible: true,
			expectedReason:   "grade_ab_approved",
		},
		{
			name: "Grau B com gates aprovados -> Publicação supports_link",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeB,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Apuração jornalística detalhada de vínculo societário",
				ContextLimits:    "Conforme documentos públicos",
			},
			expectedAction:   domain.PolicyActionPublish,
			expectedDisp:     domain.DispositionSupportsLink,
			expectedEligible: true,
			expectedReason:   "grade_ab_approved",
		},
		{
			name: "Grau C com limites e linguagem conservadora -> Publicação possible_link",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeC,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Pessoa citada em apuração sobre contratos societários",
				ContextLimits:    "Investigação em andamento sem conclusão judicial definitiva",
			},
			expectedAction:   domain.PolicyActionPublish,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: true,
			expectedReason:   "grade_c_approved_with_limits",
		},
		{
			name: "Grau C sem context_limits -> Quarentena",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeC,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Pessoa citada em apuração sobre contratos societários",
				ContextLimits:    "   ",
			},
			expectedAction:   domain.PolicyActionQuarantine,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: false,
			expectedReason:   "grade_c_missing_context_limits",
		},
		{
			name: "Grau C sem linguagem de associação -> Quarentena",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeC,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Aquisição concluída em 2026",
				ContextLimits:    "Limite contextual presente",
			},
			expectedAction:   domain.PolicyActionQuarantine,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: false,
			expectedReason:   "grade_c_lacks_conservative_language",
		},
		{
			name: "Grau D com gates aprovados -> Quarentena por padrão",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeD,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Alegação de terceiros citada",
				ContextLimits:    "Sem corroboração",
			},
			expectedAction:   domain.PolicyActionQuarantine,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: false,
			expectedReason:   "grade_de_quarantine_default",
		},
		{
			name: "Grau E com gates aprovados -> Quarentena por padrão",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeE,
				StructuralPassed: true,
				SemanticPassed:   true,
				Proposition:      "Menção indireta periférica",
				ContextLimits:    "Pista apenas",
			},
			expectedAction:   domain.PolicyActionQuarantine,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: false,
			expectedReason:   "grade_de_quarantine_default",
		},
		{
			name: "Gate estrutural reprovado -> Quarentena independente do grau",
			input: domain.PublishingPolicyInput{
				Grade:             domain.EvidenceGradeA,
				StructuralPassed:  false,
				SemanticPassed:    true,
				Proposition:       "Aquisição societária",
				StructuralReasons: []string{"source_unreachable"},
			},
			expectedAction:   domain.PolicyActionQuarantine,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: false,
			expectedReason:   "structural_gate_failed",
		},
		{
			name: "Gate semântico reprovado -> Quarentena independente do grau",
			input: domain.PublishingPolicyInput{
				Grade:            domain.EvidenceGradeA,
				StructuralPassed: true,
				SemanticPassed:   false,
				Proposition:      "Aquisição societária",
				SemanticReasons:  []string{"semantic_claim_unsupported"},
			},
			expectedAction:   domain.PolicyActionQuarantine,
			expectedDisp:     domain.DispositionPossibleLink,
			expectedEligible: false,
			expectedReason:   "semantic_gate_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := domain.EvaluatePublishingPolicy(tt.input)

			if res.Action != tt.expectedAction {
				t.Errorf("Action: esperado %s, obtido %s (reasons: %v)", tt.expectedAction, res.Action, res.Reasons)
			}
			if res.Disposition != tt.expectedDisp {
				t.Errorf("Disposition: esperado %s, obtido %s", tt.expectedDisp, res.Disposition)
			}
			if res.MetricEligible != tt.expectedEligible {
				t.Errorf("MetricEligible: esperado %v, obtido %v", tt.expectedEligible, res.MetricEligible)
			}

			if tt.expectedReason != "" {
				found := false
				for _, r := range res.Reasons {
					if r == tt.expectedReason {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("esperava motivo %q na lista de razões, obtido %v", tt.expectedReason, res.Reasons)
				}
			}
		})
	}
}
