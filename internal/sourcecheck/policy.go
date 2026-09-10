package sourcecheck

import (
	"fmt"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
)

// GateDecision define a deliberação editorial do gate de acessibilidade.
type GateDecision string

const (
	GateDecisionAllow      GateDecision = "allow"
	GateDecisionQuarantine GateDecision = "quarantine"
)

// GateResult consolida a decisão do gate e o motivo editorial fundamentado.
type GateResult struct {
	Decision  GateDecision `json:"decision"`
	Rationale string       `json:"rationale"`
}

// EvaluateSourceAccessGate aplica a política editorial definida no ADR-006:
//
//  1. reachable: satisfaz o requisito de acessibilidade técnica (GateDecisionAllow).
//     Atenção: NÃO autoriza publicação por si só — os gates estrutural e semântico continuam exigidos.
//  2. unreachable: qualquer origem com falha definitiva vai imediatamente para quarentena (GateDecisionQuarantine).
//  3. cited_by_provider: mera citação pela LLM não substitui verificação GET própria -> quarentena (GateDecisionQuarantine).
//  4. not_checked: aplica tratamento diferenciado conforme a procedência e configuração:
//     - curated_seed: preserva o initial_state do mapeamento versionado caso a política seja 'allow' (padrão ADR-006).
//     - openrouter: quarentena conservadora por padrão caso a política seja 'quarantine' (padrão ADR-006).
func EvaluateSourceAccessGate(origin domain.ClaimOrigin, status domain.SourceAccessStatus, cfg *config.Config) GateResult {
	switch status {
	case domain.SourceAccessReachable:
		return GateResult{
			Decision:  GateDecisionAllow,
			Rationale: "fonte verificada com sucesso via GET seguro (reachable); requisito de acessibilidade atendido",
		}

	case domain.SourceAccessUnreachable:
		return GateResult{
			Decision:  GateDecisionQuarantine,
			Rationale: "fonte comprovadamente inacessível ou bloqueada por segurança (unreachable); quarentena obrigatória",
		}

	case domain.SourceAccessCitedByProvider:
		return GateResult{
			Decision:  GateDecisionQuarantine,
			Rationale: "fonte apenas citada pelo provedor (cited_by_provider) sem verificação GET própria; quarentena preventiva",
		}

	case domain.SourceAccessNotChecked:
		switch origin {
		case domain.ClaimOriginCuratedSeed:
			policy := "allow"
			if cfg != nil && cfg.SourceNotCheckedPolicyCuratedSeed != "" {
				policy = cfg.SourceNotCheckedPolicyCuratedSeed
			}
			if policy == "allow" {
				return GateResult{
					Decision:  GateDecisionAllow,
					Rationale: "curated_seed com status not_checked: preserva initial_state do mapeamento v1 conforme política do ADR-006",
				}
			}
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: "curated_seed com status not_checked: colocado em quarentena conforme configuração restritiva",
			}

		case domain.ClaimOriginOpenRouter:
			policy := "quarantine"
			if cfg != nil && cfg.SourceNotCheckedPolicyOpenRouter != "" {
				policy = cfg.SourceNotCheckedPolicyOpenRouter
			}
			if policy == "quarantine" {
				return GateResult{
					Decision:  GateDecisionQuarantine,
					Rationale: "openrouter com status not_checked: colocado em quarentena preventiva conforme política do ADR-006",
				}
			}
			return GateResult{
				Decision:  GateDecisionAllow,
				Rationale: "openrouter com status not_checked: permitido por política de configuração permissiva",
			}

		case domain.ClaimOriginAdmin:
			return GateResult{
				Decision:  GateDecisionAllow,
				Rationale: "inserção manual administrativa com status not_checked",
			}

		default:
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: fmt.Sprintf("origem desconhecida (%s) com status not_checked; quarentena", origin),
			}
		}

	default:
		return GateResult{
			Decision:  GateDecisionQuarantine,
			Rationale: fmt.Sprintf("status de acessibilidade desconhecido (%s); quarentena de segurança", status),
		}
	}
}

// ResolveStatusUpdate define a regra de transição de estado da fonte no banco de dados.
//
// Invariantes essenciais:
//   - Sucesso (reachable) sempre promove a fonte para reachable.
//   - Erro definitivo (unreachable: 404, 410, SSRF, DNS inexistente) sempre atualiza para unreachable.
//   - Resultado inconclusivo (not_checked: timeout, 429, 5xx):
//     Se a fonte já possuía comprovação anterior de 'reachable', ela NÃO é rebaixada para unreachable,
//     preservando o histórico legítimo anterior sem apagar evidência válida silenciosamente.
//     Se a fonte não era reachable, seu status persiste como not_checked.
func ResolveStatusUpdate(currentStatus domain.SourceAccessStatus, checkResult CheckResult) domain.SourceAccessStatus {
	switch checkResult.Status {
	case domain.SourceAccessReachable:
		return domain.SourceAccessReachable

	case domain.SourceAccessUnreachable:
		return domain.SourceAccessUnreachable

	case domain.SourceAccessNotChecked:
		if currentStatus == domain.SourceAccessReachable {
			// Não apaga evidência válida prévia diante de oscilação temporária de rede ou rate limit
			return domain.SourceAccessReachable
		}
		return domain.SourceAccessNotChecked

	default:
		return domain.SourceAccessNotChecked
	}
}
