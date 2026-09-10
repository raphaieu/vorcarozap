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
