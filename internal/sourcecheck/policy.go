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

// GateInput define as informações de acessibilidade submetidas à deliberação do gate editorial.
type GateInput struct {
	Origin       domain.ClaimOrigin
	StoredStatus domain.SourceAccessStatus
	CurrentCheck *CheckResult // resultado da tentativa nesta execução (nil se nenhuma tentativa foi feita agora)
}

// EvaluateSourceAccessGate aplica a política editorial definida no ADR-006:
//
// Regra fundamental contra aprovação indevida por histórico:
// Quando uma tentativa atual foi realizada (input.CurrentCheck != nil), a deliberação do gate
// baseia-se OBRIGATORIAMENTE no resultado da verificação atual. Uma comprovação histórica passada
// (reachable anterior) NUNCA pode autorizar ou aprovar uma tentativa atual que resultou inconclusiva ou em falha.
//
// 1. CurrentCheck != nil:
//   - CurrentCheck.Status == reachable: atende o requisito técnico de acessibilidade (GateDecisionAllow).
//     Atenção: NÃO autoriza publicação por si só — os gates estrutural e semântico continuam exigidos.
//   - CurrentCheck.Status == unreachable: erro definitivo ou violação de segurança -> quarentena (GateDecisionQuarantine).
//   - CurrentCheck.Status == not_checked: falha transitória ou inconclusiva (timeout, 429, 5xx, anti-bot):
//   - curated_seed: preserva initial_state do mapeamento caso a política configurada seja 'allow'.
//   - openrouter: QUARENTENA OBRIGATÓRIA preventiva por padrão. Status histórico prévio não autoriza publicação.
//
// 2. CurrentCheck == nil (avaliação de registro já existente sem nova tentativa de rede):
//   - StoredStatus == reachable: GateDecisionAllow
//   - StoredStatus == unreachable: GateDecisionQuarantine
//   - StoredStatus == cited_by_provider: GateDecisionQuarantine
//   - StoredStatus == not_checked:
//   - curated_seed: preserva se 'allow'
//   - openrouter: quarentena
func EvaluateSourceAccessGate(input GateInput, cfg *config.Config) GateResult {
	if input.CurrentCheck != nil {
		switch input.CurrentCheck.Status {
		case domain.SourceAccessReachable:
			return GateResult{
				Decision:  GateDecisionAllow,
				Rationale: "fonte verificada com sucesso via GET seguro (reachable) na tentativa atual; requisito de acessibilidade atendido",
			}

		case domain.SourceAccessUnreachable:
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: fmt.Sprintf("tentativa atual comprovadamente inacessível ou bloqueada por segurança (%s: %s); quarentena obrigatória", input.CurrentCheck.TechnicalReason, input.CurrentCheck.NormalizedErrorCode),
			}

		case domain.SourceAccessNotChecked:
			if input.Origin == domain.ClaimOriginCuratedSeed {
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
			}

			// Para openrouter (e novas descobertas automatizadas):
			// Fail-closed para quarentena. Histórico reachable no banco não substitui verificação atual!
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: fmt.Sprintf("tentativa atual de verificação via openrouter inconclusiva (%s: %s); quarentena obrigatória preventiva (histórico prévio não autoriza aprovação)", input.CurrentCheck.TechnicalReason, input.CurrentCheck.NormalizedErrorCode),
			}

		default:
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: fmt.Sprintf("status da tentativa atual desconhecido (%s); quarentena", input.CurrentCheck.Status),
			}
		}
	}

	// Avaliação com base exclusivamente no status armazenado (sem verificação executada nesta rodada)
	switch input.StoredStatus {
	case domain.SourceAccessReachable:
		return GateResult{
			Decision:  GateDecisionAllow,
			Rationale: "fonte possui comprovação histórica reachable gravada; requisito de acessibilidade atendido",
		}

	case domain.SourceAccessUnreachable:
		return GateResult{
			Decision:  GateDecisionQuarantine,
			Rationale: "fonte registrada como unreachable; quarentena obrigatória",
		}

	case domain.SourceAccessCitedByProvider:
		return GateResult{
			Decision:  GateDecisionQuarantine,
			Rationale: "fonte registrada como cited_by_provider sem verificação GET própria; quarentena preventiva",
		}

	case domain.SourceAccessNotChecked:
		switch input.Origin {
		case domain.ClaimOriginCuratedSeed:
			policy := "allow"
			if cfg != nil && cfg.SourceNotCheckedPolicyCuratedSeed != "" {
				policy = cfg.SourceNotCheckedPolicyCuratedSeed
			}
			if policy == "allow" {
				return GateResult{
					Decision:  GateDecisionAllow,
					Rationale: "curated_seed com status not_checked gravado: preserva initial_state do mapeamento v1",
				}
			}
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: "curated_seed com status not_checked gravado: colocado em quarentena conforme configuração restritiva",
			}

		case domain.ClaimOriginOpenRouter:
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: "openrouter com status not_checked gravado: colocado em quarentena preventiva",
			}

		case domain.ClaimOriginAdmin:
			return GateResult{
				Decision:  GateDecisionAllow,
				Rationale: "inserção manual administrativa com status not_checked",
			}

		default:
			return GateResult{
				Decision:  GateDecisionQuarantine,
				Rationale: fmt.Sprintf("origem desconhecida (%s) com status not_checked; quarentena", input.Origin),
			}
		}

	default:
		return GateResult{
			Decision:  GateDecisionQuarantine,
			Rationale: fmt.Sprintf("status de acessibilidade desconhecido (%s); quarentena de segurança", input.StoredStatus),
		}
	}
}

// EvaluateSourceStatusGate é um atalho para avaliação do gate quando não há tentativa atual de verificação.
func EvaluateSourceStatusGate(origin domain.ClaimOrigin, status domain.SourceAccessStatus, cfg *config.Config) GateResult {
	return EvaluateSourceAccessGate(GateInput{
		Origin:       origin,
		StoredStatus: status,
		CurrentCheck: nil,
	}, cfg)
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
