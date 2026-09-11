package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contém os parâmetros operacionais da aplicação.
type Config struct {
	Port                               int
	Env                                string
	DBPath                             string
	PublicDataCutoff                   string
	ReadTimeout                        time.Duration
	WriteTimeout                       time.Duration
	IdleTimeout                        time.Duration
	SourceNotCheckedPolicyOpenRouter   string
	SourceNotCheckedPolicyCuratedSeed  string
	SourceCheckTimeout                 time.Duration
	OpenRouterAPIKey                   string
	OpenRouterBaseURL                  string
	OpenRouterDiscoveryModel           string
	OpenRouterTimeout                  time.Duration
	OpenRouterWebSearchEngine          string
	OpenRouterWebSearchMaxResults      int
	OpenRouterWebSearchMaxTotalResults int
	OpenRouterWebSearchMaxUses         int
	OpenRouterMaxToolCalls             int
}

// Load carrega a configuração a partir de variáveis de ambiente com defaults seguros.
func Load() (*Config, error) {
	port := 8080
	if portStr := os.Getenv("APP_PORT"); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			return nil, fmt.Errorf("config: APP_PORT inválida %q: deve ser um número entre 1 e 65535", portStr)
		}
		port = p
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/vorcarozap.db"
	}

	cutoff := os.Getenv("PUBLIC_DATA_CUTOFF")
	if cutoff == "" {
		cutoff = "2026-09-03"
	} else {
		if _, err := time.Parse("2006-01-02", cutoff); err != nil {
			return nil, fmt.Errorf("config: PUBLIC_DATA_CUTOFF inválida %q: deve estar no formato YYYY-MM-DD", cutoff)
		}
	}

	readTimeout, err := parseTimeout("APP_READ_TIMEOUT", os.Getenv("APP_READ_TIMEOUT"), 5*time.Second)
	if err != nil {
		return nil, err
	}

	writeTimeout, err := parseTimeout("APP_WRITE_TIMEOUT", os.Getenv("APP_WRITE_TIMEOUT"), 10*time.Second)
	if err != nil {
		return nil, err
	}

	idleTimeout, err := parseTimeout("APP_IDLE_TIMEOUT", os.Getenv("APP_IDLE_TIMEOUT"), 60*time.Second)
	if err != nil {
		return nil, err
	}

	sourceCheckTimeout, err := parseTimeout("SOURCE_CHECK_TIMEOUT", os.Getenv("SOURCE_CHECK_TIMEOUT"), 5*time.Second)
	if err != nil {
		return nil, err
	}

	openRouterPolicy := os.Getenv("SOURCE_NOT_CHECKED_POLICY_OPENROUTER")
	if openRouterPolicy == "" {
		openRouterPolicy = "quarantine"
	} else if openRouterPolicy != "quarantine" && openRouterPolicy != "allow" {
		return nil, fmt.Errorf("config: SOURCE_NOT_CHECKED_POLICY_OPENROUTER inválida %q: deve ser 'quarantine' ou 'allow'", openRouterPolicy)
	}

	curatedSeedPolicy := os.Getenv("SOURCE_NOT_CHECKED_POLICY_CURATED_SEED")
	if curatedSeedPolicy == "" {
		curatedSeedPolicy = "allow"
	} else if curatedSeedPolicy != "quarantine" && curatedSeedPolicy != "allow" {
		return nil, fmt.Errorf("config: SOURCE_NOT_CHECKED_POLICY_CURATED_SEED inválida %q: deve ser 'quarantine' ou 'allow'", curatedSeedPolicy)
	}

	openRouterAPIKey := os.Getenv("OPENROUTER_API_KEY")

	openRouterBaseURL := os.Getenv("OPENROUTER_BASE_URL")
	if openRouterBaseURL == "" {
		openRouterBaseURL = "https://openrouter.ai/api/v1/chat/completions"
	}

	openRouterDiscoveryModel := os.Getenv("OPENROUTER_DISCOVERY_MODEL")
	if openRouterDiscoveryModel == "" {
		openRouterDiscoveryModel = "openai/gpt-4.1-mini"
	}

	openRouterTimeout, err := parseTimeout("OPENROUTER_TIMEOUT", os.Getenv("OPENROUTER_TIMEOUT"), 30*time.Second)
	if err != nil {
		return nil, err
	}

	openRouterEngine := os.Getenv("OPENROUTER_WEB_SEARCH_ENGINE")
	if openRouterEngine == "" {
		openRouterEngine = "auto"
	} else {
		switch openRouterEngine {
		case "auto", "native", "exa", "firecrawl", "parallel", "perplexity":
			// engine válido
		default:
			return nil, fmt.Errorf("config: OPENROUTER_WEB_SEARCH_ENGINE inválido %q: deve ser 'auto', 'native', 'exa', 'firecrawl', 'parallel' ou 'perplexity'", openRouterEngine)
		}
	}

	openRouterMaxResults, err := parseInt("OPENROUTER_WEB_SEARCH_MAX_RESULTS", os.Getenv("OPENROUTER_WEB_SEARCH_MAX_RESULTS"), 5, 1, 25)
	if err != nil {
		return nil, err
	}

	openRouterMaxTotalResults, err := parseInt("OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS", os.Getenv("OPENROUTER_WEB_SEARCH_MAX_TOTAL_RESULTS"), 15, 1, 1000)
	if err != nil {
		return nil, err
	}

	openRouterMaxUses, err := parseInt("OPENROUTER_WEB_SEARCH_MAX_USES", os.Getenv("OPENROUTER_WEB_SEARCH_MAX_USES"), 3, 1, 100)
	if err != nil {
		return nil, err
	}

	openRouterMaxToolCalls, err := parseInt("OPENROUTER_MAX_TOOL_CALLS", os.Getenv("OPENROUTER_MAX_TOOL_CALLS"), 5, 1, 30)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:                               port,
		Env:                                env,
		DBPath:                             dbPath,
		PublicDataCutoff:                   cutoff,
		ReadTimeout:                        readTimeout,
		WriteTimeout:                       writeTimeout,
		IdleTimeout:                        idleTimeout,
		SourceNotCheckedPolicyOpenRouter:   openRouterPolicy,
		SourceNotCheckedPolicyCuratedSeed:  curatedSeedPolicy,
		SourceCheckTimeout:                 sourceCheckTimeout,
		OpenRouterAPIKey:                   openRouterAPIKey,
		OpenRouterBaseURL:                  openRouterBaseURL,
		OpenRouterDiscoveryModel:           openRouterDiscoveryModel,
		OpenRouterTimeout:                  openRouterTimeout,
		OpenRouterWebSearchEngine:          openRouterEngine,
		OpenRouterWebSearchMaxResults:      openRouterMaxResults,
		OpenRouterWebSearchMaxTotalResults: openRouterMaxTotalResults,
		OpenRouterWebSearchMaxUses:         openRouterMaxUses,
		OpenRouterMaxToolCalls:             openRouterMaxToolCalls,
	}, nil
}

func parseTimeout(envKey, val string, defaultVal time.Duration) (time.Duration, error) {
	if val == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválido %q: %w", envKey, val, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("config: %s deve ser maior que zero, obtido %q", envKey, val)
	}
	return d, nil
}

func parseInt(envKey, val string, defaultVal, minVal, maxVal int) (int, error) {
	if val == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválido %q: deve ser um número inteiro", envKey, val)
	}
	if n < minVal || n > maxVal {
		return 0, fmt.Errorf("config: %s inválido %d: deve estar entre %d e %d", envKey, n, minVal, maxVal)
	}
	return n, nil
}
