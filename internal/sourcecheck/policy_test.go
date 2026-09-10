package sourcecheck_test

import (
	"testing"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/sourcecheck"
)

func TestEvaluateSourceAccessGate_TableDriven(t *testing.T) {
	defaultCfg := &config.Config{
		SourceNotCheckedPolicyOpenRouter:  "quarantine",
		SourceNotCheckedPolicyCuratedSeed: "allow",
	}

	strictCfg := &config.Config{
		SourceNotCheckedPolicyOpenRouter:  "quarantine",
		SourceNotCheckedPolicyCuratedSeed: "quarantine",
	}

	permissiveCfg := &config.Config{
		SourceNotCheckedPolicyOpenRouter:  "allow",
		SourceNotCheckedPolicyCuratedSeed: "allow",
	}

	tests := []struct {
		name             string
		origin           domain.ClaimOrigin
		status           domain.SourceAccessStatus
		cfg              *config.Config
		expectedDecision sourcecheck.GateDecision
	}{
		// 1. reachable: sempre permite o gate técnico de acessibilidade
		{
			name:             "reachable openrouter permite gate",
			origin:           domain.ClaimOriginOpenRouter,
			status:           domain.SourceAccessReachable,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
		{
			name:             "reachable curated_seed permite gate",
			origin:           domain.ClaimOriginCuratedSeed,
			status:           domain.SourceAccessReachable,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
		{
			name:             "reachable admin permite gate",
			origin:           domain.ClaimOriginAdmin,
			status:           domain.SourceAccessReachable,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},

		// 2. unreachable: qualquer origem leva a quarentena obrigatória
		{
			name:             "unreachable openrouter quarentena",
			origin:           domain.ClaimOriginOpenRouter,
			status:           domain.SourceAccessUnreachable,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name:             "unreachable curated_seed quarentena",
			origin:           domain.ClaimOriginCuratedSeed,
			status:           domain.SourceAccessUnreachable,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name:             "unreachable admin quarentena",
			origin:           domain.ClaimOriginAdmin,
			status:           domain.SourceAccessUnreachable,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},

		// 3. cited_by_provider: mera citação pela LLM não substitui GET próprio -> quarentena
		{
			name:             "cited_by_provider openrouter quarentena",
			origin:           domain.ClaimOriginOpenRouter,
			status:           domain.SourceAccessCitedByProvider,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name:             "cited_by_provider curated_seed quarentena",
			origin:           domain.ClaimOriginCuratedSeed,
			status:           domain.SourceAccessCitedByProvider,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},

		// 4. not_checked com curated_seed (exceção ADR-006):
		{
			name:             "curated_seed not_checked com policy allow preserva estado inicial",
			origin:           domain.ClaimOriginCuratedSeed,
			status:           domain.SourceAccessNotChecked,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
		{
			name:             "curated_seed not_checked com policy quarantine quarentena",
			origin:           domain.ClaimOriginCuratedSeed,
			status:           domain.SourceAccessNotChecked,
			cfg:              strictCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},

		// 5. not_checked com openrouter:
		{
			name:             "openrouter not_checked com policy quarantine quarentena preventiva",
			origin:           domain.ClaimOriginOpenRouter,
			status:           domain.SourceAccessNotChecked,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name:             "openrouter not_checked sempre resulta em quarentena preventiva mesmo com config permissiva",
			origin:           domain.ClaimOriginOpenRouter,
			status:           domain.SourceAccessNotChecked,
			cfg:              permissiveCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},

		// 6. not_checked com admin ou origem desconhecida
		{
			name:             "admin not_checked permite gate",
			origin:           domain.ClaimOriginAdmin,
			status:           domain.SourceAccessNotChecked,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
		{
			name:             "origem desconhecida com not_checked quarentena",
			origin:           domain.ClaimOrigin("desconhecida"),
			status:           domain.SourceAccessNotChecked,
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := sourcecheck.EvaluateSourceStatusGate(tt.origin, tt.status, tt.cfg)
			if res.Decision != tt.expectedDecision {
				t.Errorf("EvaluateSourceStatusGate(%s, %s) = %s; esperado %s (motivo: %s)",
					tt.origin, tt.status, res.Decision, tt.expectedDecision, res.Rationale)
			}
		})
	}
}

func TestEvaluateSourceAccessGate_WithCurrentCheck(t *testing.T) {
	defaultCfg := &config.Config{
		SourceNotCheckedPolicyOpenRouter:  "quarantine",
		SourceNotCheckedPolicyCuratedSeed: "allow",
	}

	tests := []struct {
		name             string
		input            sourcecheck.GateInput
		cfg              *config.Config
		expectedDecision sourcecheck.GateDecision
	}{
		{
			name: "HISTORICO REACHABLE COM TENTATIVA ATUAL 429 NOT_CHECKED: DEVE SER QUARENTENA",
			input: sourcecheck.GateInput{
				Origin:       domain.ClaimOriginOpenRouter,
				StoredStatus: domain.SourceAccessReachable,
				CurrentCheck: &sourcecheck.CheckResult{
					Status:              domain.SourceAccessNotChecked,
					TechnicalReason:     sourcecheck.ReasonHTTP429,
					HTTPStatus:          429,
					NormalizedErrorCode: "http_429",
					IsInconclusive:      true,
				},
			},
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name: "HISTORICO REACHABLE COM TENTATIVA ATUAL TIMEOUT: DEVE SER QUARENTENA",
			input: sourcecheck.GateInput{
				Origin:       domain.ClaimOriginOpenRouter,
				StoredStatus: domain.SourceAccessReachable,
				CurrentCheck: &sourcecheck.CheckResult{
					Status:              domain.SourceAccessNotChecked,
					TechnicalReason:     sourcecheck.ReasonTimeout,
					NormalizedErrorCode: "timeout",
					IsInconclusive:      true,
				},
			},
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name: "HISTORICO REACHABLE COM TENTATIVA ATUAL 404 UNREACHABLE: DEVE SER QUARENTENA",
			input: sourcecheck.GateInput{
				Origin:       domain.ClaimOriginOpenRouter,
				StoredStatus: domain.SourceAccessReachable,
				CurrentCheck: &sourcecheck.CheckResult{
					Status:              domain.SourceAccessUnreachable,
					TechnicalReason:     sourcecheck.ReasonHTTP404,
					HTTPStatus:          404,
					NormalizedErrorCode: "http_404",
				},
			},
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionQuarantine,
		},
		{
			name: "HISTORICO CITED_BY_PROVIDER COM TENTATIVA ATUAL 200 REACHABLE: DEVE SER ALLOW",
			input: sourcecheck.GateInput{
				Origin:       domain.ClaimOriginOpenRouter,
				StoredStatus: domain.SourceAccessCitedByProvider,
				CurrentCheck: &sourcecheck.CheckResult{
					Status:          domain.SourceAccessReachable,
					TechnicalReason: sourcecheck.ReasonOK,
					HTTPStatus:      200,
				},
			},
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
		{
			name: "CURATED_SEED COM TENTATIVA ATUAL NOT_CHECKED SOB POLICY ALLOW: PRESERVA ESTADO",
			input: sourcecheck.GateInput{
				Origin:       domain.ClaimOriginCuratedSeed,
				StoredStatus: domain.SourceAccessNotChecked,
				CurrentCheck: &sourcecheck.CheckResult{
					Status:              domain.SourceAccessNotChecked,
					TechnicalReason:     sourcecheck.ReasonHTTP5xx,
					HTTPStatus:          503,
					NormalizedErrorCode: "http_503",
					IsInconclusive:      true,
				},
			},
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
		{
			name: "AVALIAÇÃO SEM TENTATIVA ATUAL COM HISTORICO REACHABLE: ALLOW",
			input: sourcecheck.GateInput{
				Origin:       domain.ClaimOriginOpenRouter,
				StoredStatus: domain.SourceAccessReachable,
				CurrentCheck: nil,
			},
			cfg:              defaultCfg,
			expectedDecision: sourcecheck.GateDecisionAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := sourcecheck.EvaluateSourceAccessGate(tt.input, tt.cfg)
			if res.Decision != tt.expectedDecision {
				t.Errorf("EvaluateSourceAccessGate(%+v) = %s; esperado %s (motivo: %s)",
					tt.input, res.Decision, tt.expectedDecision, res.Rationale)
			}
		})
	}
}

func TestResolveStatusUpdate_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		current        domain.SourceAccessStatus
		checkResult    sourcecheck.CheckResult
		expectedStatus domain.SourceAccessStatus
	}{
		{
			name:    "sucesso reachable promove qualquer estado para reachable",
			current: domain.SourceAccessNotChecked,
			checkResult: sourcecheck.CheckResult{
				Status:          domain.SourceAccessReachable,
				TechnicalReason: sourcecheck.ReasonOK,
				HTTPStatus:      200,
			},
			expectedStatus: domain.SourceAccessReachable,
		},
		{
			name:    "sucesso reachable sobre cited_by_provider promove para reachable",
			current: domain.SourceAccessCitedByProvider,
			checkResult: sourcecheck.CheckResult{
				Status:          domain.SourceAccessReachable,
				TechnicalReason: sourcecheck.ReasonOK,
				HTTPStatus:      200,
			},
			expectedStatus: domain.SourceAccessReachable,
		},
		{
			name:    "erro definitivo unreachable atualiza de reachable para unreachable",
			current: domain.SourceAccessReachable,
			checkResult: sourcecheck.CheckResult{
				Status:              domain.SourceAccessUnreachable,
				TechnicalReason:     sourcecheck.ReasonHTTP404,
				HTTPStatus:          404,
				NormalizedErrorCode: "http_404",
			},
			expectedStatus: domain.SourceAccessUnreachable,
		},
		{
			name:    "erro definitivo SSRF atualiza para unreachable",
			current: domain.SourceAccessNotChecked,
			checkResult: sourcecheck.CheckResult{
				Status:              domain.SourceAccessUnreachable,
				TechnicalReason:     sourcecheck.ReasonSSRFBlocked,
				NormalizedErrorCode: "ssrf_blocked",
				IsSSRFBlocked:       true,
			},
			expectedStatus: domain.SourceAccessUnreachable,
		},
		{
			name:    "falha inconclusiva não apaga histórico reachable prévio",
			current: domain.SourceAccessReachable,
			checkResult: sourcecheck.CheckResult{
				Status:              domain.SourceAccessNotChecked,
				TechnicalReason:     sourcecheck.ReasonHTTP429,
				HTTPStatus:          429,
				NormalizedErrorCode: "http_429",
				IsInconclusive:      true,
			},
			expectedStatus: domain.SourceAccessReachable,
		},
		{
			name:    "timeout não apaga histórico reachable prévio",
			current: domain.SourceAccessReachable,
			checkResult: sourcecheck.CheckResult{
				Status:              domain.SourceAccessNotChecked,
				TechnicalReason:     sourcecheck.ReasonTimeout,
				NormalizedErrorCode: "timeout",
				IsInconclusive:      true,
			},
			expectedStatus: domain.SourceAccessReachable,
		},
		{
			name:    "resultado inconclusivo sobre not_checked permanece not_checked",
			current: domain.SourceAccessNotChecked,
			checkResult: sourcecheck.CheckResult{
				Status:              domain.SourceAccessNotChecked,
				TechnicalReason:     sourcecheck.ReasonHTTP5xx,
				HTTPStatus:          503,
				NormalizedErrorCode: "http_503",
				IsInconclusive:      true,
			},
			expectedStatus: domain.SourceAccessNotChecked,
		},
		{
			name:    "resultado inconclusivo sobre cited_by_provider vira not_checked",
			current: domain.SourceAccessCitedByProvider,
			checkResult: sourcecheck.CheckResult{
				Status:              domain.SourceAccessNotChecked,
				TechnicalReason:     sourcecheck.ReasonHTTP403,
				HTTPStatus:          403,
				NormalizedErrorCode: "http_403",
				IsInconclusive:      true,
			},
			expectedStatus: domain.SourceAccessNotChecked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourcecheck.ResolveStatusUpdate(tt.current, tt.checkResult)
			if got != tt.expectedStatus {
				t.Errorf("ResolveStatusUpdate(%s, %+v) = %s; esperado %s",
					tt.current, tt.checkResult.Status, got, tt.expectedStatus)
			}
		})
	}
}
