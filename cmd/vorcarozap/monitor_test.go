package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/config"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
)

type mockMonitorRunner struct {
	runFn func(ctx context.Context, query string) (*monitoring.RunSummary, error)
}

func (m *mockMonitorRunner) Run(ctx context.Context, query string) (*monitoring.RunSummary, error) {
	if m.runFn != nil {
		return m.runFn(ctx, query)
	}
	return nil, errors.New("mock runner not implemented")
}

func TestMonitorCommand_MissingOrEmptyQuery(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "no arguments",
			args: []string{},
		},
		{
			name: "empty query flag",
			args: []string{"--query", ""},
		},
		{
			name: "whitespace query flag",
			args: []string{"--query", "   "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			ctx := context.Background()

			err := runMonitorCommand(ctx, tt.args, &stdout, &stderr, nil, nil)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), "--query é obrigatória") {
				t.Errorf("expected error to mention '--query é obrigatória', got: %v", err)
			}
			if !strings.Contains(stderr.String(), "Uso do comando monitor") {
				t.Errorf("expected stderr to contain usage instructions, got: %s", stderr.String())
			}
		})
	}
}

func TestMonitorCommand_HelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()

	err := runMonitorCommand(ctx, []string{"--help"}, &stdout, &stderr, nil, nil)
	if err != nil {
		t.Fatalf("expected nil error on --help, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "Uso do comando monitor") {
		t.Errorf("expected usage output on stderr, got: %s", stderr.String())
	}
}

func TestMonitorCommand_MissingAPIKeyWithoutInjectedRunner(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	cfg := &config.Config{
		DBPath:           filepath.Join(t.TempDir(), "test.db"),
		OpenRouterAPIKey: "",
	}

	err := runMonitorCommand(ctx, []string{"--query", "investigação"}, &stdout, &stderr, cfg, nil)
	if err == nil {
		t.Fatal("expected error about missing OPENROUTER_API_KEY, got nil")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY é obrigatória") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMonitorCommand_ValidExecutionWithMockRunner(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	tempDB := filepath.Join(t.TempDir(), "test.db")

	cfg := &config.Config{
		DBPath:                  tempDB,
		OpenRouterAPIKey:        "test-key",
		MonitorMaxCostPerDayUSD: 5.00,
	}

	now := time.Now().UTC()
	expectedSummary := &monitoring.RunSummary{
		RunID:                  "run-test-12345",
		Status:                 "completed",
		Query:                  "Daniel Vorcaro Master",
		WindowStart:            now.Add(-24 * time.Hour).Format(time.RFC3339),
		WindowEnd:              now.Format(time.RFC3339),
		TotalCandidates:        5,
		UniqueCandidates:       4,
		DuplicateCandidates:    1,
		VerificationsExecuted:  3,
		PublishedCount:         2,
		QuarantinedCount:       2,
		RejectedCount:          0,
		PromptTokens:           1500,
		CompletionTokens:       450,
		TotalTokens:            1950,
		DiscoveryTokens:        1000,
		VerificationTokens:     950,
		DiscoveryCostUSD:       0.0075,
		DiscoveryCostMicros:    7500,
		VerificationCostUSD:    0.0050,
		VerificationCostMicros: 5000,
		TotalCostUSD:           0.0125,
		TotalCostMicros:        12500,
		DailyTotalCostUSD:      0.0250,
		DailyTotalCostMicros:   25000,
		TechnicalSummary:       "Execução de teste finalizada com sucesso",
	}

	var capturedQuery string
	mockRunner := &mockMonitorRunner{
		runFn: func(ctx context.Context, query string) (*monitoring.RunSummary, error) {
			capturedQuery = query
			return expectedSummary, nil
		},
	}

	runnerFactory := func(c *config.Config, db *sql.DB) (MonitorRunnerInterface, error) {
		return mockRunner, nil
	}

	err := runMonitorCommand(ctx, []string{"--query", "Daniel Vorcaro Master"}, &stdout, &stderr, cfg, runnerFactory)
	if err != nil {
		t.Fatalf("expected runMonitorCommand to succeed, got: %v", err)
	}

	if capturedQuery != "Daniel Vorcaro Master" {
		t.Errorf("expected runner query %q, got %q", "Daniel Vorcaro Master", capturedQuery)
	}

	output := stdout.String()
	expectedOutputs := []string{
		"VorcaroZAP — Relatório de Monitoramento Operacional",
		"Run ID:                  run-test-12345",
		"Status Operacional:      COMPLETED",
		"Consulta:                Daniel Vorcaro Master",
		"Candidatos Descobertos:  5 (4 únicos, 1 duplicados)",
		"Verificações Executadas: 3",
		"Publicações Realizadas:  2",
		"Quarentena Editorial:    2",
		"Tokens Totais:           1950 (Descoberta: 1000, Verificações: 950)",
		"Custo Real (USD):        $0.0125 (Descoberta: $0.007500, Verificações: $0.005000)",
		"Resumo Técnico:          Execução de teste finalizada com sucesso",
	}

	for _, expected := range expectedOutputs {
		if !strings.Contains(output, expected) {
			t.Errorf("expected stdout to contain %q, but got:\n%s", expected, output)
		}
	}
}

func TestMonitorCommand_RunnerError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	tempDB := filepath.Join(t.TempDir(), "test.db")

	cfg := &config.Config{
		DBPath:           tempDB,
		OpenRouterAPIKey: "test-key",
	}

	mockRunner := &mockMonitorRunner{
		runFn: func(ctx context.Context, query string) (*monitoring.RunSummary, error) {
			return nil, errors.New("falha de rede simulada")
		},
	}

	runnerFactory := func(c *config.Config, db *sql.DB) (MonitorRunnerInterface, error) {
		return mockRunner, nil
	}

	err := runMonitorCommand(ctx, []string{"--query", "investigação"}, &stdout, &stderr, cfg, runnerFactory)
	if err == nil {
		t.Fatal("expected error from runner failure, got nil")
	}
	if !strings.Contains(err.Error(), "falha de rede simulada") {
		t.Errorf("expected wrapped runner error, got: %v", err)
	}
}
