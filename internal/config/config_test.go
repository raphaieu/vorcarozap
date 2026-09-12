package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

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
	_ = os.Unsetenv("ADMIN_USER")
	_ = os.Unsetenv("ADMIN_PASSWORD_HASH")
	_ = os.Unsetenv("ADMIN_ALLOWED_ORIGIN")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("esperava sucesso ao carregar defaults, erro: %v", err)
	}

	if cfg.AdminUser != "" {
		t.Errorf("AdminUser: esperado '', obtido %q", cfg.AdminUser)
	}
	if cfg.AdminPasswordHash != "" {
		t.Errorf("AdminPasswordHash: esperado '', obtido %q", cfg.AdminPasswordHash)
	}
	if cfg.IsAdminEnabled() {
		t.Errorf("IsAdminEnabled: esperado false quando desabilitado, obtido true")
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

func TestConfigAdminVariables(t *testing.T) {
	hashCost11Bytes, err := bcrypt.GenerateFromPassword([]byte("password"), 11)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste custo 11: %v", err)
	}
	hashCost12Bytes, err := bcrypt.GenerateFromPassword([]byte("password"), 12)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste custo 12: %v", err)
	}
	hashCost13Bytes, err := bcrypt.GenerateFromPassword([]byte("password"), 13)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste custo 13: %v", err)
	}
	hashCost14Bytes, err := bcrypt.GenerateFromPassword([]byte("password"), 14)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste custo 14: %v", err)
	}
	hashCost15Bytes, err := bcrypt.GenerateFromPassword([]byte("password"), 15)
	if err != nil {
		t.Fatalf("falha ao gerar hash de teste custo 15: %v", err)
	}

	hashCost11 := string(hashCost11Bytes)
	hashCost12 := string(hashCost12Bytes)
	hashCost13 := string(hashCost13Bytes)
	hashCost14 := string(hashCost14Bytes)
	hashCost15 := string(hashCost15Bytes)

	tests := []struct {
		name          string
		adminUser     string
		adminHash     string
		adminOrigin   string
		wantOrigin    string
		expectErr     bool
		errContains   string
		expectEnabled bool
	}{
		{
			name:          "ambos ausentes desabilitam admin sem erro",
			adminUser:     "",
			adminHash:     "",
			expectErr:     false,
			expectEnabled: false,
		},
		{
			name:        "somente ADMIN_USER configurado falha",
			adminUser:   "admin",
			adminHash:   "",
			expectErr:   true,
			errContains: "devem ser configurados em conjunto",
		},
		{
			name:        "somente ADMIN_PASSWORD_HASH configurado falha",
			adminUser:   "",
			adminHash:   hashCost12,
			expectErr:   true,
			errContains: "devem ser configurados em conjunto",
		},
		{
			name:        "ADMIN_USER vazio apos trim falha",
			adminUser:   "   ",
			adminHash:   hashCost12,
			expectErr:   true,
			errContains: "não pode ser vazio",
		},
		{
			name:        "ADMIN_USER com dois pontos falha",
			adminUser:   "admin:root",
			adminHash:   hashCost12,
			expectErr:   true,
			errContains: "não pode conter o caractere ':'",
		},
		{
			name:        "ADMIN_USER com quebra de linha falha",
			adminUser:   "admin\nuser",
			adminHash:   hashCost12,
			expectErr:   true,
			errContains: "não pode conter caracteres de controle",
		},
		{
			name:        "ADMIN_USER com caractere de controle tabulacao falha",
			adminUser:   "admin\tuser",
			adminHash:   hashCost12,
			expectErr:   true,
			errContains: "não pode conter caracteres de controle",
		},
		{
			name:        "ADMIN_USER excessivamente longo (>128) falha",
			adminUser:   strings.Repeat("a", 129),
			adminHash:   hashCost12,
			expectErr:   true,
			errContains: "excede limite de 128 caracteres",
		},
		{
			name:        "ADMIN_PASSWORD_HASH com texto puro falha",
			adminUser:   "admin",
			adminHash:   "senha_em_texto_puro_123",
			expectErr:   true,
			errContains: "deve conter um hash bcrypt válido",
		},
		{
			name:        "ADMIN_PASSWORD_HASH vazio apos trim falha",
			adminUser:   "admin",
			adminHash:   "   ",
			expectErr:   true,
			errContains: "não pode ser vazio",
		},
		{
			name:        "ADMIN_PASSWORD_HASH com prefixo invalido falha",
			adminUser:   "admin",
			adminHash:   "$1$abcdef1234567890",
			expectErr:   true,
			errContains: "deve conter um hash bcrypt válido",
		},
		{
			name:        "ADMIN_PASSWORD_HASH com custo 11 abaixo do minimo (12) falha",
			adminUser:   "admin",
			adminHash:   hashCost11,
			expectErr:   true,
			errContains: "custo bcrypt fora da faixa permitida",
		},
		{
			name:        "ADMIN_PASSWORD_HASH com custo 15 acima do maximo (14) falha",
			adminUser:   "admin",
			adminHash:   hashCost15,
			expectErr:   true,
			errContains: "custo bcrypt fora da faixa permitida",
		},
		{
			name:          "ADMIN_USER e ADMIN_PASSWORD_HASH validos com custo 12 habilitam admin",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "http://localhost:8090",
			wantOrigin:    "http://localhost:8090",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_USER e ADMIN_PASSWORD_HASH validos com custo 13 habilitam admin",
			adminUser:     "admin",
			adminHash:     hashCost13,
			adminOrigin:   "http://localhost:8090",
			wantOrigin:    "http://localhost:8090",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_USER e ADMIN_PASSWORD_HASH validos com custo 14 habilitam admin",
			adminUser:     "admin",
			adminHash:     hashCost14,
			adminOrigin:   "https://vorcaro.exemplo.org",
			wantOrigin:    "https://vorcaro.exemplo.org",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_USER com espacos nas bordas e normalizado com sucesso",
			adminUser:     "  admin_operator  ",
			adminHash:     hashCost12,
			adminOrigin:   "http://localhost:8090",
			wantOrigin:    "http://localhost:8090",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:        "ADMIN habilitado sem ADMIN_ALLOWED_ORIGIN falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "",
			expectErr:   true,
			errContains: "ADMIN_ALLOWED_ORIGIN é obrigatório",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com esquema invalido (ftp) falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "ftp://localhost:8090",
			expectErr:   true,
			errContains: "esquema deve ser http ou https",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN sem host falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://",
			expectErr:   true,
			errContains: "host não pode ser vazio",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com userinfo falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://user:pass@localhost:8090",
			expectErr:   true,
			errContains: "não deve conter credenciais",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com path subrota falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://localhost:8090/admin",
			expectErr:   true,
			errContains: "deve conter apenas esquema e host",
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN com esquema/host em maiúsculas e barra final é normalizado canonicamente",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "HTTPS://EXAMPLE.COM/",
			wantOrigin:    "https://example.com",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN com porta e maiúsculas e barra final é normalizado canonicamente",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "HTTP://LOCALHOST:8090/",
			wantOrigin:    "http://localhost:8090",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN http com porta padrao 80 elimina a porta",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "http://EXAMPLE.COM:80/",
			wantOrigin:    "http://example.com",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN https com porta padrao 443 elimina a porta",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "https://EXAMPLE.COM:443/",
			wantOrigin:    "https://example.com",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN http preserva porta 443 como nao padrao",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "http://example.com:443/",
			wantOrigin:    "http://example.com:443",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN https preserva porta 80 como nao padrao",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "https://example.com:80/",
			wantOrigin:    "https://example.com:80",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN IPv6 sem porta normalizado canonicamente",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "http://[::1]/",
			wantOrigin:    "http://[::1]",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN IPv6 longo sem porta normalizado canonicamente",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "https://[2001:DB8::1]/",
			wantOrigin:    "https://[2001:db8::1]",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN IPv6 com porta padrao 80 elimina porta",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "http://[::1]:80/",
			wantOrigin:    "http://[::1]",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN IPv6 com porta padrao 443 elimina porta",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "https://[2001:db8::1]:443/",
			wantOrigin:    "https://[2001:db8::1]",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN IPv6 com porta nao padrao preserva porta",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "http://[::1]:8080/",
			wantOrigin:    "http://[::1]:8080",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:          "ADMIN_ALLOWED_ORIGIN IPv6 longo com porta nao padrao preserva porta",
			adminUser:     "admin",
			adminHash:     hashCost12,
			adminOrigin:   "https://[2001:db8::1]:8443/",
			wantOrigin:    "https://[2001:db8::1]:8443",
			expectErr:     false,
			expectEnabled: true,
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com porta nao numerica falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://example.com:abc",
			expectErr:   true,
			errContains: "inválido",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com porta declarada vazia falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://example.com:/",
			expectErr:   true,
			errContains: "porta não pode ser vazia",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN IPv6 com porta declarada vazia falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://[::1]:",
			expectErr:   true,
			errContains: "porta não pode ser vazia",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com porta zero fora da faixa falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://example.com:0",
			expectErr:   true,
			errContains: "deve ser um número entre 1 e 65535",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com porta acima de 65535 falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://example.com:65536",
			expectErr:   true,
			errContains: "deve ser um número entre 1 e 65535",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com porta 99999 falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://example.com:99999",
			expectErr:   true,
			errContains: "deve ser um número entre 1 e 65535",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN IPv6 com conteudo invalido entre colchetes falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://[invalid_ipv6]/",
			expectErr:   true,
			errContains: "inválido",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN IPv6 sem colchetes falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://2001:db8::1/",
			expectErr:   true,
			errContains: "inválido",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com query string falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://localhost:8090?foo=bar",
			expectErr:   true,
			errContains: "deve conter apenas esquema e host",
		},
		{
			name:        "ADMIN_ALLOWED_ORIGIN com fragment falha",
			adminUser:   "admin",
			adminHash:   hashCost12,
			adminOrigin: "http://localhost:8090#section",
			expectErr:   true,
			errContains: "deve conter apenas esquema e host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ADMIN_USER", tt.adminUser)
			t.Setenv("ADMIN_PASSWORD_HASH", tt.adminHash)
			t.Setenv("ADMIN_ALLOWED_ORIGIN", tt.adminOrigin)

			cfg, err := config.Load()
			if (err != nil) != tt.expectErr {
				t.Fatalf("config.Load(): esperado erro=%v, obtido err=%v", tt.expectErr, err)
			}

			if tt.expectErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("erro %q não contém %q", err.Error(), tt.errContains)
				}
				// Garante que o hash ou senha nunca vazam na mensagem de erro
				if tt.adminHash != "" && strings.Contains(err.Error(), tt.adminHash) {
					t.Errorf("vazamento de segurança: mensagem de erro contém o hash da senha: %q", err.Error())
				}
				if tt.adminUser != "" && strings.Contains(tt.adminUser, ":") && strings.Contains(err.Error(), tt.adminUser) {
					t.Errorf("vazamento de segurança: mensagem de erro contém o usuário inválido com senha/separador: %q", err.Error())
				}
				return
			}

			if cfg.IsAdminEnabled() != tt.expectEnabled {
				t.Errorf("IsAdminEnabled(): esperado %v, obtido %v", tt.expectEnabled, cfg.IsAdminEnabled())
			}

			if tt.expectEnabled {
				trimmedExpectedUser := strings.TrimSpace(tt.adminUser)
				if cfg.AdminUser != trimmedExpectedUser {
					t.Errorf("AdminUser: esperado %q, obtido %q", trimmedExpectedUser, cfg.AdminUser)
				}
				trimmedExpectedHash := strings.TrimSpace(tt.adminHash)
				if cfg.AdminPasswordHash != trimmedExpectedHash {
					t.Errorf("AdminPasswordHash: esperado %q, obtido %q", trimmedExpectedHash, cfg.AdminPasswordHash)
				}
				if cfg.AdminAllowedOrigin == "" {
					t.Errorf("AdminAllowedOrigin não deve ser vazio quando admin habilitado")
				}
				if tt.wantOrigin != "" && cfg.AdminAllowedOrigin != tt.wantOrigin {
					t.Errorf("AdminAllowedOrigin: esperado %q, obtido %q", tt.wantOrigin, cfg.AdminAllowedOrigin)
				}
			}
		})
	}
}
