package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
)

func TestConfigLoadDefaults(t *testing.T) {
	// Limpa variáveis de ambiente relevantes
	_ = os.Unsetenv("APP_PORT")
	_ = os.Unsetenv("APP_ENV")
	_ = os.Unsetenv("DB_PATH")
	_ = os.Unsetenv("APP_READ_TIMEOUT")
	_ = os.Unsetenv("APP_WRITE_TIMEOUT")
	_ = os.Unsetenv("APP_IDLE_TIMEOUT")
	_ = os.Unsetenv("SOURCE_NOT_CHECKED_POLICY_OPENROUTER")
	_ = os.Unsetenv("SOURCE_NOT_CHECKED_POLICY_CURATED_SEED")
	_ = os.Unsetenv("SOURCE_CHECK_TIMEOUT")
	_ = os.Unsetenv("OPENROUTER_API_KEY")
	_ = os.Unsetenv("OPENROUTER_BASE_URL")
	_ = os.Unsetenv("OPENROUTER_DISCOVERY_MODEL")
	_ = os.Unsetenv("OPENROUTER_VERIFICATION_MODEL")
	_ = os.Unsetenv("OPENROUTER_TIMEOUT")
	_ = os.Unsetenv("OPENROUTER_WEB_SEARCH_ENGINE")
	_ = os.Unsetenv("OPENROUTER_WEB_SEARCH_MAX_RESULTS")
	_ = os.Unsetenv("OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS")
	_ = os.Unsetenv("OPENROUTER_WEB_SEARCH_MAX_USES")
	_ = os.Unsetenv("OPENROUTER_MAX_TOOL_CALLS")
	_ = os.Unsetenv("MONITOR_WINDOW")
	_ = os.Unsetenv("MONITOR_MAX_CANDIDATES_PER_RUN")
	_ = os.Unsetenv("MONITOR_MAX_VERIFICATIONS_PER_RUN")
	_ = os.Unsetenv("MONITOR_MAX_COST_PER_RUN_USD")
	_ = os.Unsetenv("MONITOR_MAX_COST_PER_DAY_USD")
	_ = os.Unsetenv("MONITOR_LOCK_TTL")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("esperava sucesso ao carregar defaults, erro: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port: esperado 8080, obtido %d", cfg.Port)
	}
	if cfg.Env != "development" {
		t.Errorf("Env: esperado 'development', obtido %q", cfg.Env)
	}
	if cfg.DBPath != "./data/vorcarozap.db" {
		t.Errorf("DBPath: esperado './data/vorcarozap.db', obtido %q", cfg.DBPath)
	}
	if cfg.PublicDataCutoff != "2026-09-03" {
		t.Errorf("PublicDataCutoff: esperado '2026-09-03', obtido %q", cfg.PublicDataCutoff)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout: esperado 5s, obtido %v", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout: esperado 10s, obtido %v", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout: esperado 60s, obtido %v", cfg.IdleTimeout)
	}
	if cfg.SourceNotCheckedPolicyOpenRouter != "quarantine" {
		t.Errorf("SourceNotCheckedPolicyOpenRouter: esperado 'quarantine', obtido %q", cfg.SourceNotCheckedPolicyOpenRouter)
	}
	if cfg.SourceNotCheckedPolicyCuratedSeed != "allow" {
		t.Errorf("SourceNotCheckedPolicyCuratedSeed: esperado 'allow', obtido %q", cfg.SourceNotCheckedPolicyCuratedSeed)
	}
	if cfg.SourceCheckTimeout != 5*time.Second {
		t.Errorf("SourceCheckTimeout: esperado 5s, obtido %v", cfg.SourceCheckTimeout)
	}
	if cfg.OpenRouterAPIKey != "" {
		t.Errorf("OpenRouterAPIKey: esperado '', obtido %q", cfg.OpenRouterAPIKey)
	}
	if cfg.OpenRouterBaseURL != "https://openrouter.ai/api/v1/chat/completions" {
		t.Errorf("OpenRouterBaseURL: esperado 'https://openrouter.ai/api/v1/chat/completions', obtido %q", cfg.OpenRouterBaseURL)
	}
	if cfg.OpenRouterDiscoveryModel != "openai/gpt-4.1-mini" {
		t.Errorf("OpenRouterDiscoveryModel: esperado 'openai/gpt-4.1-mini', obtido %q", cfg.OpenRouterDiscoveryModel)
	}
	if cfg.OpenRouterVerificationModel != "openai/gpt-4.1-mini" {
		t.Errorf("OpenRouterVerificationModel: esperado 'openai/gpt-4.1-mini', obtido %q", cfg.OpenRouterVerificationModel)
	}
	if cfg.OpenRouterTimeout != 30*time.Second {
		t.Errorf("OpenRouterTimeout: esperado 30s, obtido %v", cfg.OpenRouterTimeout)
	}
	if cfg.OpenRouterWebSearchEngine != "auto" {
		t.Errorf("OpenRouterWebSearchEngine: esperado 'auto', obtido %q", cfg.OpenRouterWebSearchEngine)
	}
	if cfg.OpenRouterWebSearchMaxResults != 5 {
		t.Errorf("OpenRouterWebSearchMaxResults: esperado 5, obtido %d", cfg.OpenRouterWebSearchMaxResults)
	}
	if cfg.OpenRouterWebSearchMaxTotalResults != 15 {
		t.Errorf("OpenRouterWebSearchMaxTotalResults: esperado 15, obtido %d", cfg.OpenRouterWebSearchMaxTotalResults)
	}
	if cfg.OpenRouterWebSearchMaxUses != 3 {
		t.Errorf("OpenRouterWebSearchMaxUses: esperado 3, obtido %d", cfg.OpenRouterWebSearchMaxUses)
	}
	if cfg.OpenRouterMaxToolCalls != 5 {
		t.Errorf("OpenRouterMaxToolCalls: esperado 5, obtido %d", cfg.OpenRouterMaxToolCalls)
	}
	if cfg.MonitorWindow != 24*time.Hour {
		t.Errorf("MonitorWindow: esperado 24h, obtido %v", cfg.MonitorWindow)
	}
	if cfg.MonitorMaxCandidatesPerRun != 10 {
		t.Errorf("MonitorMaxCandidatesPerRun: esperado 10, obtido %d", cfg.MonitorMaxCandidatesPerRun)
	}
	if cfg.MonitorMaxVerificationsPerRun != 10 {
		t.Errorf("MonitorMaxVerificationsPerRun: esperado 10, obtido %d", cfg.MonitorMaxVerificationsPerRun)
	}
	if cfg.MonitorMaxCostPerRunUSD != 0.25 {
		t.Errorf("MonitorMaxCostPerRunUSD: esperado 0.25, obtido %v", cfg.MonitorMaxCostPerRunUSD)
	}
	if cfg.MonitorMaxCostPerDayUSD != 1.00 {
		t.Errorf("MonitorMaxCostPerDayUSD: esperado 1.00, obtido %v", cfg.MonitorMaxCostPerDayUSD)
	}
	if cfg.MonitorLockTTL != 10*time.Minute {
		t.Errorf("MonitorLockTTL: esperado 10m, obtido %v", cfg.MonitorLockTTL)
	}
}

func TestConfigOpenRouter(t *testing.T) {
	t.Run("Configurações válidas customizadas", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test-key")
		t.Setenv("OPENROUTER_BASE_URL", "https://custom.openrouter.test/v1/chat/completions")
		t.Setenv("OPENROUTER_DISCOVERY_MODEL", "google/gemini-2.5-flash")
		t.Setenv("OPENROUTER_VERIFICATION_MODEL", "anthropic/claude-3.5-sonnet")
		t.Setenv("OPENROUTER_TIMEOUT", "45s")
		t.Setenv("OPENROUTER_WEB_SEARCH_ENGINE", "exa")
		t.Setenv("OPENROUTER_WEB_SEARCH_MAX_RESULTS", "10")
		t.Setenv("OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS", "30")
		t.Setenv("OPENROUTER_WEB_SEARCH_MAX_USES", "5")
		t.Setenv("OPENROUTER_MAX_TOOL_CALLS", "10")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("esperava sucesso, erro: %v", err)
		}
		if cfg.OpenRouterAPIKey != "sk-or-v1-test-key" {
			t.Errorf("OpenRouterAPIKey: esperado 'sk-or-v1-test-key', obtido %q", cfg.OpenRouterAPIKey)
		}
		if cfg.OpenRouterBaseURL != "https://custom.openrouter.test/v1/chat/completions" {
			t.Errorf("OpenRouterBaseURL: esperado 'https://custom.openrouter.test/v1/chat/completions', obtido %q", cfg.OpenRouterBaseURL)
		}
		if cfg.OpenRouterDiscoveryModel != "google/gemini-2.5-flash" {
			t.Errorf("OpenRouterDiscoveryModel: esperado 'google/gemini-2.5-flash', obtido %q", cfg.OpenRouterDiscoveryModel)
		}
		if cfg.OpenRouterTimeout != 45*time.Second {
			t.Errorf("OpenRouterTimeout: esperado 45s, obtido %v", cfg.OpenRouterTimeout)
		}
		if cfg.OpenRouterWebSearchEngine != "exa" {
			t.Errorf("OpenRouterWebSearchEngine: esperado 'exa', obtido %q", cfg.OpenRouterWebSearchEngine)
		}
		if cfg.OpenRouterWebSearchMaxResults != 10 {
			t.Errorf("OpenRouterWebSearchMaxResults: esperado 10, obtido %d", cfg.OpenRouterWebSearchMaxResults)
		}
		if cfg.OpenRouterWebSearchMaxTotalResults != 30 {
			t.Errorf("OpenRouterWebSearchMaxTotalResults: esperado 30, obtido %d", cfg.OpenRouterWebSearchMaxTotalResults)
		}
		if cfg.OpenRouterWebSearchMaxUses != 5 {
			t.Errorf("OpenRouterWebSearchMaxUses: esperado 5, obtido %d", cfg.OpenRouterWebSearchMaxUses)
		}
		if cfg.OpenRouterMaxToolCalls != 10 {
			t.Errorf("OpenRouterMaxToolCalls: esperado 10, obtido %d", cfg.OpenRouterMaxToolCalls)
		}
	})

	t.Run("Engines válidos", func(t *testing.T) {
		validEngines := []string{"auto", "native", "exa", "firecrawl", "parallel", "perplexity"}
		for _, eng := range validEngines {
			t.Setenv("OPENROUTER_WEB_SEARCH_ENGINE", eng)
			cfg, err := config.Load()
			if err != nil {
				t.Errorf("esperava sucesso para engine %q, obtido err: %v", eng, err)
			} else if cfg.OpenRouterWebSearchEngine != eng {
				t.Errorf("esperado %q, obtido %q", eng, cfg.OpenRouterWebSearchEngine)
			}
		}
	})

	t.Run("Engine inválido", func(t *testing.T) {
		t.Setenv("OPENROUTER_WEB_SEARCH_ENGINE", "bing")
		_, err := config.Load()
		if err == nil {
			t.Fatal("esperava erro para engine inválido 'bing'")
		}
	})

	tests := []struct {
		name      string
		envKey    string
		envVal    string
		expectErr bool
	}{
		{"timeout inválido", "OPENROUTER_TIMEOUT", "invalid", true},
		{"timeout zero", "OPENROUTER_TIMEOUT", "0s", true},
		{"timeout negativo", "OPENROUTER_TIMEOUT", "-5s", true},
		{"max results zero", "OPENROUTER_WEB_SEARCH_MAX_RESULTS", "0", true},
		{"max results acima do máximo", "OPENROUTER_WEB_SEARCH_MAX_RESULTS", "26", true},
		{"max results inválido", "OPENROUTER_WEB_SEARCH_MAX_RESULTS", "abc", true},
		{"max total results zero", "OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS", "0", true},
		{"max total results negativo", "OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS", "-1", true},
		{"max uses zero", "OPENROUTER_WEB_SEARCH_MAX_USES", "0", true},
		{"max uses negativo", "OPENROUTER_WEB_SEARCH_MAX_USES", "-3", true},
		{"max tool calls zero", "OPENROUTER_MAX_TOOL_CALLS", "0", true},
		{"max tool calls acima de 30", "OPENROUTER_MAX_TOOL_CALLS", "31", true},
		{"max tool calls texto", "OPENROUTER_MAX_TOOL_CALLS", "cinco", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envVal)
			_, err := config.Load()
			if (err != nil) != tt.expectErr {
				t.Errorf("config.Load() com %s=%q: esperado erro=%v, obtido err=%v", tt.envKey, tt.envVal, tt.expectErr, err)
			}
		})
	}
}

func TestConfigOpenRouterKeyPresence(t *testing.T) {
	t.Run("Chave ausente", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("esperava sucesso ao carregar com chave ausente, err: %v", err)
		}
		isConfigured := cfg.OpenRouterAPIKey != ""
		if isConfigured {
			t.Errorf("esperava isConfigured=false, obtido %v", isConfigured)
		}
	})

	t.Run("Chave presente", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "dummy-secret-presence-check")
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("esperava sucesso ao carregar com chave presente, err: %v", err)
		}
		isConfigured := cfg.OpenRouterAPIKey != ""
		if !isConfigured {
			t.Errorf("esperava isConfigured=true, obtido %v", isConfigured)
		}
	})
}

func TestConfigSourcePolicies(t *testing.T) {
	t.Run("Políticas válidas customizadas", func(t *testing.T) {
		t.Setenv("SOURCE_NOT_CHECKED_POLICY_OPENROUTER", "allow")
		t.Setenv("SOURCE_NOT_CHECKED_POLICY_CURATED_SEED", "quarantine")
		t.Setenv("SOURCE_CHECK_TIMEOUT", "8s")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("esperava sucesso, erro: %v", err)
		}
		if cfg.SourceNotCheckedPolicyOpenRouter != "allow" {
			t.Errorf("esperado 'allow', obtido %q", cfg.SourceNotCheckedPolicyOpenRouter)
		}
		if cfg.SourceNotCheckedPolicyCuratedSeed != "quarantine" {
			t.Errorf("esperado 'quarantine', obtido %q", cfg.SourceNotCheckedPolicyCuratedSeed)
		}
		if cfg.SourceCheckTimeout != 8*time.Second {
			t.Errorf("esperado 8s, obtido %v", cfg.SourceCheckTimeout)
		}
	})

	t.Run("OpenRouter policy inválida", func(t *testing.T) {
		t.Setenv("SOURCE_NOT_CHECKED_POLICY_OPENROUTER", "invalid_policy")
		_, err := config.Load()
		if err == nil {
			t.Fatal("esperava erro para SOURCE_NOT_CHECKED_POLICY_OPENROUTER inválida")
		}
	})

	t.Run("CuratedSeed policy inválida", func(t *testing.T) {
		t.Setenv("SOURCE_NOT_CHECKED_POLICY_CURATED_SEED", "invalid_policy")
		_, err := config.Load()
		if err == nil {
			t.Fatal("esperava erro para SOURCE_NOT_CHECKED_POLICY_CURATED_SEED inválida")
		}
	})
}

func TestConfigPublicDataCutoff(t *testing.T) {
	t.Run("Cutoff customizado válido", func(t *testing.T) {
		t.Setenv("PUBLIC_DATA_CUTOFF", "2026-12-31")
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("esperava sucesso, erro: %v", err)
		}
		if cfg.PublicDataCutoff != "2026-12-31" {
			t.Errorf("esperado '2026-12-31', obtido %q", cfg.PublicDataCutoff)
		}
	})

	t.Run("Cutoff inválido formato brasileiro", func(t *testing.T) {
		t.Setenv("PUBLIC_DATA_CUTOFF", "31/12/2026")
		_, err := config.Load()
		if err == nil {
			t.Fatal("esperava erro para formato brasileiro DD/MM/YYYY")
		}
	})

	t.Run("Cutoff inválido texto", func(t *testing.T) {
		t.Setenv("PUBLIC_DATA_CUTOFF", "nao-eh-data")
		_, err := config.Load()
		if err == nil {
			t.Fatal("esperava erro para texto não data")
		}
	})
}

func TestConfigCustomEnv(t *testing.T) {
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_PATH", "/tmp/custom.db")
	t.Setenv("APP_READ_TIMEOUT", "2s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("esperava sucesso, erro: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port: esperado 9090, obtido %d", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Errorf("Env: esperado 'production', obtido %q", cfg.Env)
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("DBPath: esperado '/tmp/custom.db', obtido %q", cfg.DBPath)
	}
	if cfg.ReadTimeout != 2*time.Second {
		t.Errorf("ReadTimeout: esperado 2s, obtido %v", cfg.ReadTimeout)
	}
}

func TestConfigInvalidPort(t *testing.T) {
	t.Setenv("APP_PORT", "invalida")
	_, err := config.Load()
	if err == nil {
		t.Fatal("esperava erro para porta inválida, obteve nil")
	}
}

func TestConfigTimeoutsTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		envKey    string
		envVal    string
		expectErr bool
	}{
		// APP_READ_TIMEOUT
		{"read timeout invalid string", "APP_READ_TIMEOUT", "invalid-duration", true},
		{"read timeout zero seconds", "APP_READ_TIMEOUT", "0s", true},
		{"read timeout zero without unit", "APP_READ_TIMEOUT", "0", true},
		{"read timeout negative duration", "APP_READ_TIMEOUT", "-1s", true},
		{"read timeout valid duration", "APP_READ_TIMEOUT", "2s", false},

		// APP_WRITE_TIMEOUT
		{"write timeout invalid string", "APP_WRITE_TIMEOUT", "xyz", true},
		{"write timeout zero duration", "APP_WRITE_TIMEOUT", "0s", true},
		{"write timeout negative duration", "APP_WRITE_TIMEOUT", "-500ms", true},
		{"write timeout valid duration", "APP_WRITE_TIMEOUT", "15s", false},

		// APP_IDLE_TIMEOUT
		{"idle timeout invalid string", "APP_IDLE_TIMEOUT", "not-time", true},
		{"idle timeout zero duration", "APP_IDLE_TIMEOUT", "0m", true},
		{"idle timeout negative duration", "APP_IDLE_TIMEOUT", "-10s", true},
		{"idle timeout valid duration", "APP_IDLE_TIMEOUT", "120s", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all timeouts first
			t.Setenv("APP_READ_TIMEOUT", "")
			t.Setenv("APP_WRITE_TIMEOUT", "")
			t.Setenv("APP_IDLE_TIMEOUT", "")

			t.Setenv(tt.envKey, tt.envVal)
			_, err := config.Load()
			if (err != nil) != tt.expectErr {
				t.Errorf("config.Load() com %s=%q: esperado erro=%v, obtido err=%v", tt.envKey, tt.envVal, tt.expectErr, err)
			}
		})
	}
}

func TestConfigMonitorVariables(t *testing.T) {
	tests := []struct {
		name      string
		envKey    string
		envVal    string
		expectErr bool
	}{
		// MONITOR_WINDOW
		{"monitor window invalid string", "MONITOR_WINDOW", "invalid", true},
		{"monitor window zero duration", "MONITOR_WINDOW", "0s", true},
		{"monitor window negative", "MONITOR_WINDOW", "-1h", true},
		{"monitor window too small (<1m)", "MONITOR_WINDOW", "30s", true},
		{"monitor window too large (>720h)", "MONITOR_WINDOW", "750h", true},
		{"monitor window valid 12h", "MONITOR_WINDOW", "12h", false},
		{"monitor window valid 48h", "MONITOR_WINDOW", "48h", false},

		// MONITOR_MAX_CANDIDATES_PER_RUN
		{"max candidates non-integer", "MONITOR_MAX_CANDIDATES_PER_RUN", "abc", true},
		{"max candidates zero", "MONITOR_MAX_CANDIDATES_PER_RUN", "0", true},
		{"max candidates negative", "MONITOR_MAX_CANDIDATES_PER_RUN", "-5", true},
		{"max candidates too high (>100)", "MONITOR_MAX_CANDIDATES_PER_RUN", "101", true},
		{"max candidates valid 25", "MONITOR_MAX_CANDIDATES_PER_RUN", "25", false},

		// MONITOR_MAX_VERIFICATIONS_PER_RUN
		{"max verifications non-integer", "MONITOR_MAX_VERIFICATIONS_PER_RUN", "xyz", true},
		{"max verifications zero", "MONITOR_MAX_VERIFICATIONS_PER_RUN", "0", true},
		{"max verifications negative", "MONITOR_MAX_VERIFICATIONS_PER_RUN", "-1", true},
		{"max verifications too high (>100)", "MONITOR_MAX_VERIFICATIONS_PER_RUN", "150", true},
		{"max verifications valid 5", "MONITOR_MAX_VERIFICATIONS_PER_RUN", "5", false},

		// MONITOR_MAX_COST_PER_RUN_USD
		{"max cost per run invalid string", "MONITOR_MAX_COST_PER_RUN_USD", "not-a-number", true},
		{"max cost per run zero", "MONITOR_MAX_COST_PER_RUN_USD", "0.0", true},
		{"max cost per run negative", "MONITOR_MAX_COST_PER_RUN_USD", "-0.10", true},
		{"max cost per run too high (>1000)", "MONITOR_MAX_COST_PER_RUN_USD", "1500.0", true},
		{"max cost per run valid 0.50", "MONITOR_MAX_COST_PER_RUN_USD", "0.50", false},

		// MONITOR_MAX_COST_PER_DAY_USD
		{"max cost per day invalid string", "MONITOR_MAX_COST_PER_DAY_USD", "invalid", true},
		{"max cost per day zero", "MONITOR_MAX_COST_PER_DAY_USD", "0.0", true},
		{"max cost per day negative", "MONITOR_MAX_COST_PER_DAY_USD", "-1.00", true},
		{"max cost per day too high (>10000)", "MONITOR_MAX_COST_PER_DAY_USD", "15000.0", true},
		{"max cost per day valid 2.50", "MONITOR_MAX_COST_PER_DAY_USD", "2.50", false},

		// MONITOR_LOCK_TTL
		{"lock ttl invalid string", "MONITOR_LOCK_TTL", "invalid", true},
		{"lock ttl zero duration", "MONITOR_LOCK_TTL", "0s", true},
		{"lock ttl negative", "MONITOR_LOCK_TTL", "-10s", true},
		{"lock ttl too small (<5s)", "MONITOR_LOCK_TTL", "2s", true},
		{"lock ttl too large (>24h)", "MONITOR_LOCK_TTL", "30h", true},
		{"lock ttl valid 5m", "MONITOR_LOCK_TTL", "5m", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MONITOR_WINDOW", "")
			t.Setenv("MONITOR_MAX_CANDIDATES_PER_RUN", "")
			t.Setenv("MONITOR_MAX_VERIFICATIONS_PER_RUN", "")
			t.Setenv("MONITOR_MAX_COST_PER_RUN_USD", "")
			t.Setenv("MONITOR_MAX_COST_PER_DAY_USD", "")
			t.Setenv("MONITOR_LOCK_TTL", "")

			t.Setenv(tt.envKey, tt.envVal)
			_, err := config.Load()
			if (err != nil) != tt.expectErr {
				t.Errorf("config.Load() com %s=%q: esperado erro=%v, obtido err=%v", tt.envKey, tt.envVal, tt.expectErr, err)
			}
		})
	}
}
