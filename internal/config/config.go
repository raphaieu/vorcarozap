package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contém os parâmetros operacionais da aplicação.
type Config struct {
	Port         int
	Env          string
	DBPath       string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
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

	return &Config{
		Port:         port,
		Env:          env,
		DBPath:       dbPath,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
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
