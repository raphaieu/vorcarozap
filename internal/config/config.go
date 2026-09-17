package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/raphaieu/vorcarozap/internal/research"
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
	OpenRouterVerificationModel        string
	OpenRouterTimeout                  time.Duration
	OpenRouterWebSearchEngine          string
	OpenRouterWebSearchMaxResults      int
	OpenRouterWebSearchMaxTotalResults int
	OpenRouterWebSearchMaxUses         int
	OpenRouterMaxToolCalls             int
	MonitorWindow                      time.Duration
	MonitorMaxCandidatesPerRun         int
	MonitorMaxVerificationsPerRun      int
	MonitorMaxCostPerRunUSD            float64
	MonitorMaxCostPerRunMicroUSD       int64
	MonitorMaxCostPerDayUSD            float64
	MonitorMaxCostPerDayMicroUSD       int64
	MonitorLockTTL                     time.Duration
	AdminUser                          string
	AdminPasswordHash                  string
	AdminAllowedOrigin                 string
	AdminSessionTTL                    time.Duration
	AdminLockoutDuration               time.Duration
	AdminMaxLoginAttempts              int
	AdminMFARequired                   bool
	AdminMFAEncryptionKey              string
	LogFormat                          string
	LogLevel                           string
	BackupRemoteEnabled                bool
	BackupRemoteProvider               string
	BackupRemoteEndpoint               string
	BackupRemoteRegion                 string
	BackupRemoteBucket                 string
	BackupRemotePrefix                 string
	BackupRemoteAccessKey              string
	BackupRemoteSecretKey              string
	BackupRemoteRetentionCount         int
	BackupRemoteTimeout                time.Duration
}

// Constantes para validação de segurança de senha administrativa (VZ-014).
const (
	AdminPasswordBcryptMinCost = 12
	AdminPasswordBcryptMaxCost = 14
)

// IsAdminEnabled retorna true se a área administrativa estiver configurada e habilitada.
func (c *Config) IsAdminEnabled() bool {
	return c.AdminUser != "" && c.AdminPasswordHash != ""
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

	openRouterVerificationModel := os.Getenv("OPENROUTER_VERIFICATION_MODEL")
	if openRouterVerificationModel == "" {
		openRouterVerificationModel = "openai/gpt-4.1-mini"
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

	monitorWindow, err := parseDurationMax("MONITOR_WINDOW", os.Getenv("MONITOR_WINDOW"), 24*time.Hour, 1*time.Minute, 720*time.Hour)
	if err != nil {
		return nil, err
	}

	monitorMaxCandidates, err := parseInt("MONITOR_MAX_CANDIDATES_PER_RUN", os.Getenv("MONITOR_MAX_CANDIDATES_PER_RUN"), 10, 1, 100)
	if err != nil {
		return nil, err
	}

	monitorMaxVerifications, err := parseInt("MONITOR_MAX_VERIFICATIONS_PER_RUN", os.Getenv("MONITOR_MAX_VERIFICATIONS_PER_RUN"), 10, 1, 100)
	if err != nil {
		return nil, err
	}

	monitorMaxCostPerRunMicros, err := parseCostMicroUSD("MONITOR_MAX_COST_PER_RUN_USD", os.Getenv("MONITOR_MAX_COST_PER_RUN_USD"), 250000, 100, 1000000000)
	if err != nil {
		return nil, err
	}

	monitorMaxCostPerDayMicros, err := parseCostMicroUSD("MONITOR_MAX_COST_PER_DAY_USD", os.Getenv("MONITOR_MAX_COST_PER_DAY_USD"), 1000000, 100, 10000000000)
	if err != nil {
		return nil, err
	}

	monitorLockTTL, err := parseDurationMax("MONITOR_LOCK_TTL", os.Getenv("MONITOR_LOCK_TTL"), 10*time.Minute, 5*time.Second, 24*time.Hour)
	if err != nil {
		return nil, err
	}

	adminUserRaw := os.Getenv("ADMIN_USER")
	adminHashRaw := os.Getenv("ADMIN_PASSWORD_HASH")

	var adminUser, adminPasswordHash, adminAllowedOrigin string
	if adminUserRaw != "" || adminHashRaw != "" {
		if adminUserRaw == "" || adminHashRaw == "" {
			return nil, fmt.Errorf("config: ADMIN_USER e ADMIN_PASSWORD_HASH devem ser configurados em conjunto")
		}

		user := strings.TrimSpace(adminUserRaw)
		if user == "" {
			return nil, fmt.Errorf("config: ADMIN_USER inválido: não pode ser vazio após remoção de espaços")
		}
		if len(user) > 128 {
			return nil, fmt.Errorf("config: ADMIN_USER inválido: comprimento excede limite de 128 caracteres")
		}
		if strings.Contains(user, ":") {
			return nil, fmt.Errorf("config: ADMIN_USER inválido: não pode conter o caractere ':'")
		}
		for _, r := range user {
			if unicode.IsControl(r) {
				return nil, fmt.Errorf("config: ADMIN_USER inválido: não pode conter caracteres de controle")
			}
		}

		hash := strings.TrimSpace(adminHashRaw)
		if hash == "" {
			return nil, fmt.Errorf("config: ADMIN_PASSWORD_HASH inválido: não pode ser vazio")
		}

		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil {
			return nil, fmt.Errorf("config: ADMIN_PASSWORD_HASH inválido: deve conter um hash bcrypt válido")
		}
		if cost < AdminPasswordBcryptMinCost || cost > AdminPasswordBcryptMaxCost {
			return nil, fmt.Errorf("config: ADMIN_PASSWORD_HASH inválido: custo bcrypt fora da faixa permitida (%d a %d)", AdminPasswordBcryptMinCost, AdminPasswordBcryptMaxCost)
		}

		originRaw := os.Getenv("ADMIN_ALLOWED_ORIGIN")
		originCanonical, err := canonicalizeAllowedOrigin(originRaw)
		if err != nil {
			return nil, err
		}

		adminUser = user
		adminPasswordHash = hash
		adminAllowedOrigin = originCanonical
	}

	adminSessionTTL, err := parseDurationMax("ADMIN_SESSION_TTL", os.Getenv("ADMIN_SESSION_TTL"), 8*time.Hour, 15*time.Minute, 168*time.Hour)
	if err != nil {
		return nil, err
	}

	adminLockoutDuration, err := parseDurationMax("ADMIN_LOCKOUT_DURATION", os.Getenv("ADMIN_LOCKOUT_DURATION"), 15*time.Minute, 1*time.Minute, 24*time.Hour)
	if err != nil {
		return nil, err
	}

	adminMaxLoginAttempts, err := parseInt("ADMIN_MAX_LOGIN_ATTEMPTS", os.Getenv("ADMIN_MAX_LOGIN_ATTEMPTS"), 5, 1, 20)
	if err != nil {
		return nil, err
	}

	adminMFARequired := true
	if mfaReqStr := os.Getenv("ADMIN_MFA_REQUIRED"); mfaReqStr != "" {
		if mfaReqStr == "false" || mfaReqStr == "0" {
			adminMFARequired = false
		}
	}

	adminMFAKey := strings.TrimSpace(os.Getenv("ADMIN_MFA_ENCRYPTION_KEY"))
	if adminMFAKey == "" {
		adminMFAKey = strings.TrimSpace(os.Getenv("MFA_ENCRYPTION_KEY"))
	}
	if adminUser != "" && adminMFAKey == "" {
		return nil, fmt.Errorf("config: ADMIN_MFA_ENCRYPTION_KEY é obrigatório quando a área administrativa está habilitada")
	}
	if adminMFAKey != "" {
		if len(adminMFAKey) != 32 && len(adminMFAKey) != 64 {
			return nil, fmt.Errorf("config: ADMIN_MFA_ENCRYPTION_KEY inválida: chave operacional deve ter exatamente 32 bytes ou 64 caracteres hexadecimais (256 bits)")
		}
		if len(adminMFAKey) == 64 {
			if _, err := hex.DecodeString(adminMFAKey); err != nil {
				return nil, fmt.Errorf("config: ADMIN_MFA_ENCRYPTION_KEY inválida: string hexadecimal de 64 caracteres contém formato inválido")
			}
		}
	}

	logFormat := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if logFormat == "" {
		if env == "production" {
			logFormat = "json"
		} else {
			logFormat = "text"
		}
	} else if logFormat != "text" && logFormat != "json" {
		return nil, fmt.Errorf("config: LOG_FORMAT inválido %q: deve ser 'text' ou 'json'", logFormat)
	}

	logLevel := strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevel == "" {
		logLevel = "INFO"
	} else {
		switch logLevel {
		case "DEBUG", "INFO", "WARN", "ERROR":
			// válido
		default:
			return nil, fmt.Errorf("config: LOG_LEVEL inválido %q: deve ser 'DEBUG', 'INFO', 'WARN' ou 'ERROR'", logLevel)
		}
	}

	backupRemoteEnabled := false
	if enabledStr := os.Getenv("BACKUP_REMOTE_ENABLED"); enabledStr != "" {
		if enabledStr == "true" || enabledStr == "1" {
			backupRemoteEnabled = true
		} else if enabledStr != "false" && enabledStr != "0" {
			return nil, fmt.Errorf("config: BACKUP_REMOTE_ENABLED inválido %q: deve ser 'true', 'false', '1' ou '0'", enabledStr)
		}
	}

	backupRemoteProvider := strings.ToLower(strings.TrimSpace(os.Getenv("BACKUP_REMOTE_PROVIDER")))
	if backupRemoteProvider == "" {
		backupRemoteProvider = "s3"
	}

	backupRemoteEndpoint := strings.TrimSpace(os.Getenv("BACKUP_REMOTE_ENDPOINT"))
	if backupRemoteEndpoint == "" {
		backupRemoteEndpoint = "https://s3.us-east-1.amazonaws.com"
	}

	backupRemoteRegion := strings.TrimSpace(os.Getenv("BACKUP_REMOTE_REGION"))
	if backupRemoteRegion == "" {
		backupRemoteRegion = "us-east-1"
	}

	backupRemoteBucket := strings.TrimSpace(os.Getenv("BACKUP_REMOTE_BUCKET"))
	backupRemotePrefix := strings.Trim(strings.TrimSpace(os.Getenv("BACKUP_REMOTE_PREFIX")), "/")
	if backupRemotePrefix == "" {
		backupRemotePrefix = "backups"
	}

	backupRemoteAccessKey := strings.TrimSpace(os.Getenv("BACKUP_REMOTE_ACCESS_KEY"))
	backupRemoteSecretKey := strings.TrimSpace(os.Getenv("BACKUP_REMOTE_SECRET_KEY"))

	backupRemoteRetentionCount, err := parseInt("BACKUP_REMOTE_RETENTION_COUNT", os.Getenv("BACKUP_REMOTE_RETENTION_COUNT"), 14, 1, 1000)
	if err != nil {
		return nil, err
	}

	backupRemoteTimeout, err := parseDurationMax("BACKUP_REMOTE_TIMEOUT", os.Getenv("BACKUP_REMOTE_TIMEOUT"), 60*time.Second, 1*time.Second, 10*time.Minute)
	if err != nil {
		return nil, err
	}

	if backupRemoteEnabled {
		if backupRemoteBucket == "" {
			return nil, fmt.Errorf("config: BACKUP_REMOTE_BUCKET é obrigatório quando BACKUP_REMOTE_ENABLED=true")
		}
		if backupRemoteAccessKey == "" {
			return nil, fmt.Errorf("config: BACKUP_REMOTE_ACCESS_KEY é obrigatório quando BACKUP_REMOTE_ENABLED=true")
		}
		if backupRemoteSecretKey == "" {
			return nil, fmt.Errorf("config: BACKUP_REMOTE_SECRET_KEY é obrigatório quando BACKUP_REMOTE_ENABLED=true")
		}
		endpointURL, err := url.Parse(backupRemoteEndpoint)
		if err != nil || endpointURL.Scheme == "" || endpointURL.Host == "" {
			return nil, fmt.Errorf("config: BACKUP_REMOTE_ENDPOINT inválido %q: deve ser uma URL válida com esquema (ex.: https://s3.us-east-1.amazonaws.com)", backupRemoteEndpoint)
		}
		if endpointURL.Scheme != "https" {
			return nil, fmt.Errorf("config: BACKUP_REMOTE_ENDPOINT inválido %q: esquema deve ser https para transporte seguro", backupRemoteEndpoint)
		}
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
		OpenRouterVerificationModel:        openRouterVerificationModel,
		OpenRouterTimeout:                  openRouterTimeout,
		OpenRouterWebSearchEngine:          openRouterEngine,
		OpenRouterWebSearchMaxResults:      openRouterMaxResults,
		OpenRouterWebSearchMaxTotalResults: openRouterMaxTotalResults,
		OpenRouterWebSearchMaxUses:         openRouterMaxUses,
		OpenRouterMaxToolCalls:             openRouterMaxToolCalls,
		MonitorWindow:                      monitorWindow,
		MonitorMaxCandidatesPerRun:         monitorMaxCandidates,
		MonitorMaxVerificationsPerRun:      monitorMaxVerifications,
		MonitorMaxCostPerRunUSD:            research.MicroUSDToFloat(monitorMaxCostPerRunMicros),
		MonitorMaxCostPerRunMicroUSD:       monitorMaxCostPerRunMicros,
		MonitorMaxCostPerDayUSD:            research.MicroUSDToFloat(monitorMaxCostPerDayMicros),
		MonitorMaxCostPerDayMicroUSD:       monitorMaxCostPerDayMicros,
		MonitorLockTTL:                     monitorLockTTL,
		AdminUser:                          adminUser,
		AdminPasswordHash:                  adminPasswordHash,
		AdminAllowedOrigin:                 adminAllowedOrigin,
		AdminSessionTTL:                    adminSessionTTL,
		AdminLockoutDuration:               adminLockoutDuration,
		AdminMaxLoginAttempts:              adminMaxLoginAttempts,
		AdminMFARequired:                   adminMFARequired,
		AdminMFAEncryptionKey:              adminMFAKey,
		LogFormat:                          logFormat,
		LogLevel:                           logLevel,
		BackupRemoteEnabled:                backupRemoteEnabled,
		BackupRemoteProvider:               backupRemoteProvider,
		BackupRemoteEndpoint:               backupRemoteEndpoint,
		BackupRemoteRegion:                 backupRemoteRegion,
		BackupRemoteBucket:                 backupRemoteBucket,
		BackupRemotePrefix:                 backupRemotePrefix,
		BackupRemoteAccessKey:              backupRemoteAccessKey,
		BackupRemoteSecretKey:              backupRemoteSecretKey,
		BackupRemoteRetentionCount:         backupRemoteRetentionCount,
		BackupRemoteTimeout:                backupRemoteTimeout,
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

func parseDurationMax(envKey, val string, defaultVal, minVal, maxVal time.Duration) (time.Duration, error) {
	if val == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválido %q: %w", envKey, val, err)
	}
	if d < minVal || d > maxVal {
		return 0, fmt.Errorf("config: %s inválido %v: deve estar entre %v e %v", envKey, d, minVal, maxVal)
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

func parseCostMicroUSD(envKey, val string, defaultMicros, minMicros, maxMicros int64) (int64, error) {
	if val == "" {
		return defaultMicros, nil
	}
	micros, err := research.ParseDecimalCostStringToMicroUSD(val)
	if err != nil {
		return 0, fmt.Errorf("config: %s inválido %q: %w", envKey, val, err)
	}
	if micros < minMicros || micros > maxMicros {
		return 0, fmt.Errorf("config: %s inválido %q: deve estar entre $%.4f e $%.4f", envKey, val, float64(minMicros)/1000000.0, float64(maxMicros)/1000000.0)
	}
	return micros, nil
}

func canonicalizeAllowedOrigin(originRaw string) (string, error) {
	originTrimmed := strings.TrimSpace(originRaw)
	if originTrimmed == "" {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN é obrigatório quando a área administrativa está habilitada")
	}

	u, err := url.Parse(originTrimmed)
	if err != nil {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: %w", originRaw, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: esquema deve ser http ou https", originRaw)
	}
	if u.User != nil {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: não deve conter credenciais de usuário", originRaw)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: deve conter apenas esquema e host (ex.: http://localhost:8090)", originRaw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: host não pode ser vazio", originRaw)
	}

	var host string
	var portNum int
	var isIPv6 bool

	h, p, err := net.SplitHostPort(u.Host)
	if err == nil {
		if p == "" {
			return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: porta não pode ser vazia", originRaw)
		}
		pn, err := strconv.Atoi(p)
		if err != nil || pn <= 0 || pn > 65535 || strconv.Itoa(pn) != p {
			return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: porta %q deve ser um número entre 1 e 65535", originRaw, p)
		}
		portNum = pn
		host = strings.ToLower(h)
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
		if ip := net.ParseIP(host); ip != nil {
			if ip.To4() == nil {
				isIPv6 = true
			}
		} else if strings.Contains(host, ":") {
			return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: endereço de host IPv6 inválido", originRaw)
		}
	} else {
		if strings.HasSuffix(u.Host, ":") {
			return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: porta não pode ser vazia", originRaw)
		}
		rawHost := strings.ToLower(u.Host)
		if strings.HasPrefix(rawHost, "[") && strings.HasSuffix(rawHost, "]") {
			host = rawHost[1 : len(rawHost)-1]
			ip := net.ParseIP(host)
			if ip == nil || ip.To4() != nil {
				return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: endereço IPv6 inválido dentro de colchetes", originRaw)
			}
			isIPv6 = true
		} else if strings.Contains(rawHost, ":") {
			return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: host com dois-pontos deve especificar porta válida ou ser IPv6 entre colchetes", originRaw)
		} else {
			host = rawHost
		}
	}

	if host == "" {
		return "", fmt.Errorf("config: ADMIN_ALLOWED_ORIGIN inválido %q: host não pode ser vazio", originRaw)
	}

	// Elimina portas padrão durante a canonicalização
	includePort := false
	if portNum > 0 {
		if (scheme == "http" && portNum == 80) || (scheme == "https" && portNum == 443) {
			includePort = false
		} else {
			includePort = true
		}
	}

	if isIPv6 {
		if includePort {
			return fmt.Sprintf("%s://[%s]:%d", scheme, host, portNum), nil
		}
		return fmt.Sprintf("%s://[%s]", scheme, host), nil
	}

	if includePort {
		return fmt.Sprintf("%s://%s:%d", scheme, host, portNum), nil
	}
	return fmt.Sprintf("%s://%s", scheme, host), nil
}
