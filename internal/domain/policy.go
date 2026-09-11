package domain

import (
	"strings"
)

// PolicyAction define a deliberação final da política de publicação em Go.
type PolicyAction string

const (
	PolicyActionPublish    PolicyAction = "publish"
	PolicyActionQuarantine PolicyAction = "quarantine"
)

func (a PolicyAction) IsValid() bool {
	switch a {
	case PolicyActionPublish, PolicyActionQuarantine:
		return true
	default:
		return false
	}
}

// SemanticGateInput encapsula o resultado estruturado retornado pelo gate semântico.
type SemanticGateInput struct {
	IdentityMatch            bool
	ClaimSupported           bool
	ClaimOverstatesSource    bool
	AttributionExplicit      bool
	GradeCompatible          bool
	ContainsIllicitInference bool
	Uncertainties            []string
}

// SemanticGateResult representa a deliberação do gate semântico puro em Go.
type SemanticGateResult struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons"`
}

// EvaluateSemanticGate valida se todos os 6 critérios semânticos foram atendidos e se uncertainties é vazio.
func EvaluateSemanticGate(input SemanticGateInput) SemanticGateResult {
	var reasons []string

	if !input.IdentityMatch {
		reasons = append(reasons, "semantic_identity_mismatch")
	}
	if !input.ClaimSupported {
		reasons = append(reasons, "semantic_claim_unsupported")
	}
	if input.ClaimOverstatesSource {
		reasons = append(reasons, "semantic_claim_overstates_source")
	}
	if !input.AttributionExplicit {
		reasons = append(reasons, "semantic_attribution_not_explicit")
	}
	if !input.GradeCompatible {
		reasons = append(reasons, "semantic_grade_incompatible")
	}
	if input.ContainsIllicitInference {
		reasons = append(reasons, "semantic_contains_illicit_inference")
	}

	if len(input.Uncertainties) > 0 {
		for _, u := range input.Uncertainties {
			clean := strings.TrimSpace(u)
			if clean != "" {
				reasons = append(reasons, "semantic_uncertainty: "+clean)
			} else {
				reasons = append(reasons, "semantic_uncertainty: blank_uncertainty")
			}
		}
	}

	return SemanticGateResult{
		Passed:  len(reasons) == 0,
		Reasons: reasons,
	}
}

// PublishingPolicyInput consolida as entradas necessárias para a decisão da política Go.
type PublishingPolicyInput struct {
	Grade             EvidenceGrade
	StructuralPassed  bool
	SemanticPassed    bool
	Proposition       string
	ContextLimits     string
	StructuralReasons []string
	SemanticReasons   []string
}

// PublishingPolicyResult encapsula a decisão editorial, disposição e elegibilidade a métricas.
type PublishingPolicyResult struct {
	Action         PolicyAction     `json:"action"`
	Disposition    ClaimDisposition `json:"disposition"`
	MetricEligible bool             `json:"metric_eligible"`
	Reasons        []string         `json:"reasons"`
}

// EvaluatePublishingPolicy aplica a matriz editorial A–E determinística em Go.
func EvaluatePublishingPolicy(input PublishingPolicyInput) PublishingPolicyResult {
	// 1. Falha no gate estrutural leva à quarentena imediata
	if !input.StructuralPassed {
		var reasons []string
		reasons = append(reasons, "structural_gate_failed")
		reasons = append(reasons, input.StructuralReasons...)
		return PublishingPolicyResult{
			Action:         PolicyActionQuarantine,
			Disposition:    DispositionPossibleLink,
			MetricEligible: false,
			Reasons:        reasons,
		}
	}

	// 2. Falha no gate semântico leva à quarentena auditável
	if !input.SemanticPassed {
		var reasons []string
		reasons = append(reasons, "semantic_gate_failed")
		reasons = append(reasons, input.SemanticReasons...)
		return PublishingPolicyResult{
			Action:         PolicyActionQuarantine,
			Disposition:    DispositionPossibleLink,
			MetricEligible: false,
			Reasons:        reasons,
		}
	}

	// 3. Matriz de deliberação por Grau com ambos os gates aprovados
	switch input.Grade {
	case EvidenceGradeA, EvidenceGradeB:
		return PublishingPolicyResult{
			Action:         PolicyActionPublish,
			Disposition:    DispositionSupportsLink,
			MetricEligible: true,
			Reasons:        []string{"grade_ab_approved"},
		}

	case EvidenceGradeC:
		hasLimits := strings.TrimSpace(input.ContextLimits) != ""
		hasConservativeLang := HasConservativeAssociationLanguage(input.Proposition)

		if hasLimits && hasConservativeLang {
			return PublishingPolicyResult{
				Action:         PolicyActionPublish,
				Disposition:    DispositionPossibleLink,
				MetricEligible: true,
				Reasons:        []string{"grade_c_approved_with_limits"},
			}
		}

		var reasons []string
		if !hasLimits {
			reasons = append(reasons, "grade_c_missing_context_limits")
		}
		if !hasConservativeLang {
			reasons = append(reasons, "grade_c_lacks_conservative_language")
		}
		return PublishingPolicyResult{
			Action:         PolicyActionQuarantine,
			Disposition:    DispositionPossibleLink,
			MetricEligible: false,
			Reasons:        reasons,
		}

	case EvidenceGradeD, EvidenceGradeE:
		return PublishingPolicyResult{
			Action:         PolicyActionQuarantine,
			Disposition:    DispositionPossibleLink,
			MetricEligible: false,
			Reasons:        []string{"grade_de_quarantine_default"},
		}

	default:
		return PublishingPolicyResult{
			Action:         PolicyActionQuarantine,
			Disposition:    DispositionPossibleLink,
			MetricEligible: false,
			Reasons:        []string{"invalid_grade"},
		}
	}
}

// HasConservativeAssociationLanguage verifica se a proposição de grau C utiliza linguagem
// descritiva e conservadora de associação/vínculo factual, sem asserções categóricas desprovidas de ressalva.
func HasConservativeAssociationLanguage(prop string) bool {
	p := strings.ToLower(strings.TrimSpace(prop))
	if p == "" {
		return false
	}

	// Termos que expressam enquadramento relacional ou contextual apropriado para grau C
	associationMarkers := []string{
		"citad",
		"mencionad",
		"apontad",
		"investigad",
		"relatad",
		"associad",
		"vinculad",
		"consta",
		"reunião",
		"reuniao",
		"societár",
		"societar",
		"contrat",
		"particip",
		"contato",
		"referênc",
		"referenc",
		"declaraç",
		"declarac",
		"depoiment",
		"apuração",
		"apuracao",
		"alega",
		"acusad",
		"suspeit",
		"process",
		"operação",
		"operacao",
		"documentad",
		"sócio",
		"socio",
		"parceria",
		"negociação",
		"negociacao",
		"interlocut",
	}

	for _, marker := range associationMarkers {
		if strings.Contains(p, marker) {
			return true
		}
	}

	return false
}
