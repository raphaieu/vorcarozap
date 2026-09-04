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
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout: esperado 5s, obtido %v", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout: esperado 10s, obtido %v", cfg.WriteTimeout)
	}
	if cfg.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout: esperado 60s, obtido %v", cfg.IdleTimeout)
	}
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
