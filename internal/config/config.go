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

	readTimeout := 5 * time.Second
	if rtStr := os.Getenv("APP_READ_TIMEOUT"); rtStr != "" {
		d, err := time.ParseDuration(rtStr)
		if err != nil {
			return nil, fmt.Errorf("config: APP_READ_TIMEOUT inválido %q: %w", rtStr, err)
		}
		readTimeout = d
	}

	writeTimeout := 10 * time.Second
	if wtStr := os.Getenv("APP_WRITE_TIMEOUT"); wtStr != "" {
		d, err := time.ParseDuration(wtStr)
		if err != nil {
			return nil, fmt.Errorf("config: APP_WRITE_TIMEOUT inválido %q: %w", wtStr, err)
		}
		writeTimeout = d
	}

	idleTimeout := 60 * time.Second
	if itStr := os.Getenv("APP_IDLE_TIMEOUT"); itStr != "" {
		d, err := time.ParseDuration(itStr)
		if err != nil {
			return nil, fmt.Errorf("config: APP_IDLE_TIMEOUT inválido %q: %w", itStr, err)
		}
		idleTimeout = d
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
