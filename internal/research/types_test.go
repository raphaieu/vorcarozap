package research_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/research"
)

func TestParseCostJSONToMicroUSD(t *testing.T) {
	tests := []struct {
		name        string
		raw         json.RawMessage
		expected    int64
		expectErr   bool
		errContains string
	}{
		{
			name:        "ausente / vazio",
			raw:         nil,
			expectErr:   true,
			errContains: "campo 'usage.cost' ausente",
		},
		{
			name:        "vazio string",
			raw:         json.RawMessage(""),
			expectErr:   true,
			errContains: "campo 'usage.cost' ausente",
		},
		{
			name:        "null explícito",
			raw:         json.RawMessage("null"),
			expectErr:   true,
			errContains: "não pode ser null",
		},
		{
			name:        "string em vez de número",
			raw:         json.RawMessage(`"0.05"`),
			expectErr:   true,
			errContains: "deve ser numérico, recebido string",
		},
		{
			name:      "zero inteiro",
			raw:       json.RawMessage("0"),
			expected:  0,
			expectErr: false,
		},
		{
			name:      "zero decimal",
			raw:       json.RawMessage("0.0"),
			expected:  0,
			expectErr: false,
		},
		{
			name:      "zero com múltiplos zeros",
			raw:       json.RawMessage("0.00000000"),
			expected:  0,
			expectErr: false,
		},
		{
			name:        "negativo",
			raw:         json.RawMessage("-0.001"),
			expectErr:   true,
			errContains: "não pode ser negativo",
		},
		{
			name:      "exatamente 1 micro USD (0.000001)",
			raw:       json.RawMessage("0.000001"),
			expected:  1,
			expectErr: false,
		},
		{
			name:      "submicro 0.1 micro USD arredonda para cima (0.0000001)",
			raw:       json.RawMessage("0.0000001"),
			expected:  1,
			expectErr: false,
		},
		{
			name:      "submicro 0.000000000001 arredonda para cima",
			raw:       json.RawMessage("0.000000000001"),
			expected:  1,
			expectErr: false,
		},
		{
			name:      "valor decimal padrão (0.0125)",
			raw:       json.RawMessage("0.0125"),
			expected:  12500,
			expectErr: false,
		},
		{
			name:      "valor 1 USD (1.00)",
			raw:       json.RawMessage("1.00"),
			expected:  1000000,
			expectErr: false,
		},
		{
			name:      "notação científica 1.5e-7 (0.15 micro -> ceil = 1)",
			raw:       json.RawMessage("1.5e-7"),
			expected:  1,
			expectErr: false,
		},
		{
			name:      "notação científica 2.5E-4 (250 micro)",
			raw:       json.RawMessage("2.5E-4"),
			expected:  250,
			expectErr: false,
		},
		{
			name:        "formato inválido abc",
			raw:         json.RawMessage("abc"),
			expectErr:   true,
			errContains: "formato de custo decimal inválido",
		},
		{
			name:        "overflow int64",
			raw:         json.RawMessage("9999999999999999999999999999999999999999"),
			expectErr:   true,
			errContains: "excede o limite máximo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := research.ParseCostJSONToMicroUSD(tt.raw)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("esperava erro contendo %q, mas obteve nil (got=%d)", tt.errContains, got)
				}
				if !errors.Is(err, research.ErrInvalidResponse) {
					t.Errorf("esperava erro wrapping ErrInvalidResponse, obtido: %v", err)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("erro %q não contém substring esperada %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("esperava sucesso, obteve erro: %v", err)
				}
				if got != tt.expected {
					t.Errorf("esperado %d micro-USD, obtido %d", tt.expected, got)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s != "" && (len(s) >= len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr))))))
}
