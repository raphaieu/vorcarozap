package config_test

import (
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/raphaieu/vorcarozap/internal/config"
)

func TestConfigProductionDefaultsAndSecurity(t *testing.T) {
	// Limpa variáveis de ambiente para avaliar defaults puros
	envKeys := []string{
		"APP_PORT", "APP_ENV", "DB_PATH", "PUBLIC_DATA_CUTOFF",
		"APP_READ_TIMEOUT", "APP_WRITE_TIMEOUT", "APP_IDLE_TIMEOUT",
		"SOURCE_NOT_CHECKED_POLICY_OPENROUTER", "SOURCE_NOT_CHECKED_POLICY_CURATED_SEED",
		"SOURCE_CHECK_TIMEOUT", "OPENROUTER_API_KEY", "OPENROUTER_BASE_URL",
		"OPENROUTER_DISCOVERY_MODEL", "OPENROUTER_VERIFICATION_MODEL", "OPENROUTER_TIMEOUT",
		"OPENROUTER_WEB_SEARCH_ENGINE", "OPENROUTER_WEB_SEARCH_MAX_RESULTS",
		"OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS", "OPENROUTER_WEB_SEARCH_MAX_USES",
		"OPENROUTER_MAX_TOOL_CALLS", "MONITOR_WINDOW", "MONITOR_MAX_CANDIDATES_PER_RUN",
		"MONITOR_MAX_VERIFICATIONS_PER_RUN", "MONITOR_MAX_COST_PER_RUN_USD",
		"MONITOR_MAX_COST_PER_DAY_USD", "MONITOR_LOCK_TTL",
		"ADMIN_USER", "ADMIN_PASSWORD_HASH", "ADMIN_ALLOWED_ORIGIN",
	}

	for _, k := range envKeys {
		t.Setenv(k, "")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("falha ao carregar config com defaults: %v", err)
	}

	// 1. Garantia de que segredos nunca possuem valores padrão no código
	if cfg.OpenRouterAPIKey != "" {
		t.Errorf("OPENROUTER_API_KEY não deve possuir valor default, obtido %q", cfg.OpenRouterAPIKey)
	}
	if cfg.AdminUser != "" {
		t.Errorf("ADMIN_USER não deve possuir valor default, obtido %q", cfg.AdminUser)
	}
	if cfg.AdminPasswordHash != "" {
		t.Errorf("ADMIN_PASSWORD_HASH não deve possuir valor default, obtido %q", cfg.AdminPasswordHash)
	}
	if cfg.AdminAllowedOrigin != "" {
		t.Errorf("ADMIN_ALLOWED_ORIGIN não deve possuir valor default, obtido %q", cfg.AdminAllowedOrigin)
	}

	// 2. Admin deve estar desabilitado por padrão (fail-closed)
	if cfg.IsAdminEnabled() {
		t.Error("Admin não deve estar habilitado por padrão sem credenciais configuradas")
	}

	// 3. Políticas seguras padrão
	if cfg.SourceNotCheckedPolicyOpenRouter != "quarantine" {
		t.Errorf("SOURCE_NOT_CHECKED_POLICY_OPENROUTER esperado 'quarantine', obtido %q", cfg.SourceNotCheckedPolicyOpenRouter)
	}
	if cfg.SourceNotCheckedPolicyCuratedSeed != "allow" {
		t.Errorf("SOURCE_NOT_CHECKED_POLICY_CURATED_SEED esperado 'allow', obtido %q", cfg.SourceNotCheckedPolicyCuratedSeed)
	}

	// 4. Limites operacionais seguros
	if cfg.MonitorMaxCostPerRunUSD != 0.25 {
		t.Errorf("MONITOR_MAX_COST_PER_RUN_USD esperado 0.25, obtido %f", cfg.MonitorMaxCostPerRunUSD)
	}
	if cfg.MonitorMaxCostPerDayUSD != 1.00 {
		t.Errorf("MONITOR_MAX_COST_PER_DAY_USD esperado 1.00, obtido %f", cfg.MonitorMaxCostPerDayUSD)
	}
	if cfg.MonitorLockTTL != 10*time.Minute {
		t.Errorf("MONITOR_LOCK_TTL esperado 10m, obtido %v", cfg.MonitorLockTTL)
	}
}

func TestConfigValidation_TableDriven(t *testing.T) {
	validHashBytes, err := bcrypt.GenerateFromPassword([]byte("SenhaSegura123!"), 12)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste: %v", err)
	}
	validHash := string(validHashBytes)

	lowCostHashBytes, err := bcrypt.GenerateFromPassword([]byte("SenhaFraca123!"), 4)
	if err != nil {
		t.Fatalf("falha ao gerar hash fraco: %v", err)
	}
	lowCostHash := string(lowCostHashBytes)

	tests := []struct {
		name        string
		envMods     map[string]string
		expectError bool
		errorSubstr string
	}{
		{
			name: "Configuração válida com admin e origin",
			envMods: map[string]string{
				"ADMIN_USER":               "admin_operador",
				"ADMIN_PASSWORD_HASH":      validHash,
				"ADMIN_ALLOWED_ORIGIN":     "https://vorcarozap.exemplo.org",
				"ADMIN_MFA_ENCRYPTION_KEY": "12345678901234567890123456789012",
			},
			expectError: false,
		},
		{
			name: "Admin incompleto: usuário sem senha",
			envMods: map[string]string{
				"ADMIN_USER":           "admin_operador",
				"ADMIN_PASSWORD_HASH":  "",
				"ADMIN_ALLOWED_ORIGIN": "https://vorcarozap.exemplo.org",
			},
			expectError: true,
			errorSubstr: "devem ser configurados em conjunto",
		},
		{
			name: "Admin incompleto: senha sem usuário",
			envMods: map[string]string{
				"ADMIN_USER":           "",
				"ADMIN_PASSWORD_HASH":  validHash,
				"ADMIN_ALLOWED_ORIGIN": "https://vorcarozap.exemplo.org",
			},
			expectError: true,
			errorSubstr: "devem ser configurados em conjunto",
		},
		{
			name: "Admin sem ADMIN_ALLOWED_ORIGIN",
			envMods: map[string]string{
				"ADMIN_USER":           "admin_operador",
				"ADMIN_PASSWORD_HASH":  validHash,
				"ADMIN_ALLOWED_ORIGIN": "",
			},
			expectError: true,
			errorSubstr: "ADMIN_ALLOWED_ORIGIN é obrigatório",
		},
		{
			name: "Bcrypt com custo abaixo de 12 rejeitado",
			envMods: map[string]string{
				"ADMIN_USER":           "admin_operador",
				"ADMIN_PASSWORD_HASH":  lowCostHash,
				"ADMIN_ALLOWED_ORIGIN": "https://vorcarozap.exemplo.org",
			},
			expectError: true,
			errorSubstr: "custo bcrypt fora da faixa permitida",
		},
		{
			name: "Senha em texto puro rejeitada no lugar do hash",
			envMods: map[string]string{
				"ADMIN_USER":           "admin_operador",
				"ADMIN_PASSWORD_HASH":  "senha_em_texto_puro_insegura",
				"ADMIN_ALLOWED_ORIGIN": "https://vorcarozap.exemplo.org",
			},
			expectError: true,
			errorSubstr: "hash bcrypt válido",
		},
		{
			name: "Porta inválida (fora da faixa)",
			envMods: map[string]string{
				"APP_PORT": "99999",
			},
			expectError: true,
			errorSubstr: "APP_PORT inválida",
		},
		{
			name: "Timeout inválido",
			envMods: map[string]string{
				"APP_READ_TIMEOUT": "invalid_duration",
			},
			expectError: true,
			errorSubstr: "APP_READ_TIMEOUT inválido",
		},
		{
			name: "Política OpenRouter desconhecida",
			envMods: map[string]string{
				"SOURCE_NOT_CHECKED_POLICY_OPENROUTER": "invalida",
			},
			expectError: true,
			errorSubstr: "SOURCE_NOT_CHECKED_POLICY_OPENROUTER inválida",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reseta ambiente
			for k := range tt.envMods {
				t.Setenv(k, "")
			}
			for k, v := range tt.envMods {
				t.Setenv(k, v)
			}

			cfg, err := config.Load()
			if tt.expectError {
				if err == nil {
					t.Fatalf("esperava erro contendo %q, mas obteve sucesso", tt.errorSubstr)
				}
				if tt.errorSubstr != "" && !stringContains(err.Error(), tt.errorSubstr) {
					t.Fatalf("erro obtido %q não contém substring esperada %q", err.Error(), tt.errorSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("esperava sucesso, mas obteve erro: %v", err)
				}
				if cfg == nil {
					t.Fatal("config retornado é nil")
				}
			}
		})
	}
}

func stringContains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && (s == substr || len(s) > 0 && os.Getenv("FORCE_FAIL") == "" && len(s) >= len(substr) && containsSubstr(s, substr)))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
