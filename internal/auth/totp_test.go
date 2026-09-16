package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/auth"
)

func TestGenerateSecret(t *testing.T) {
	s1, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("falha ao gerar segredo: %v", err)
	}
	s2, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("falha ao gerar segundo segredo: %v", err)
	}

	if len(s1) < 16 {
		t.Errorf("segredo muito curto: %q", s1)
	}
	if s1 == s2 {
		t.Errorf("dois segredos gerados não devem ser idênticos: %q", s1)
	}
}

func TestTOTP_RFC6238_TestVectors(t *testing.T) {
	// Segredo RFC 6238 para SHA1: "12345678901234567890" em ASCII
	// Base32 correspondente: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	// RFC 6238 Appendix B: 8 dígitos -> truncado em 6 dígitos (mod 10^6)
	// 59: 94287082 -> 287082
	// 1111111109: 07081804 -> 081804
	// 1111111111: 14050471 -> 050471
	// 1234567890: 89005924 -> 005924
	// 2000000000: 69279037 -> 279037
	testCases := []struct {
		timestamp    int64
		expectedCode string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}

	for _, tc := range testCases {
		tm := time.Unix(tc.timestamp, 0)
		code, err := auth.GenerateCode(secret, tm)
		if err != nil {
			t.Fatalf("falha ao gerar código para timestamp %d: %v", tc.timestamp, err)
		}
		if code != tc.expectedCode {
			t.Errorf("timestamp %d: esperado código %q, obtido %q", tc.timestamp, tc.expectedCode, code)
		}

		// Validação estrita
		valid, step, err := auth.ValidateCode(secret, tc.expectedCode, tm, 0)
		if err != nil {
			t.Fatalf("erro ao validar código: %v", err)
		}
		if !valid {
			t.Errorf("validação falhou para código válido %q no timestamp %d", tc.expectedCode, tc.timestamp)
		}
		if step != tc.timestamp/30 {
			t.Errorf("passo temporal esperado %d, obtido %d", tc.timestamp/30, step)
		}
	}
}

func TestValidateCode_WindowDrift(t *testing.T) {
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("erro ao gerar segredo: %v", err)
	}

	now := time.Now()

	// Código do passo anterior (30 segundos atrás)
	pastCode, err := auth.GenerateCode(secret, now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("erro ao gerar código passado: %v", err)
	}

	// Com janela de 1 passo, deve validar
	valid, _, err := auth.ValidateCode(secret, pastCode, now, 1)
	if err != nil || !valid {
		t.Errorf("código com drift de -30s deveria ser válido com janela 1")
	}

	// Com janela 0 (sem tolerância), deve falhar
	valid0, _, _ := auth.ValidateCode(secret, pastCode, now, 0)
	if valid0 {
		t.Errorf("código com drift de -30s não deveria ser válido com janela 0")
	}

	// Código muito antigo (65 segundos atrás)
	tooOldCode, _ := auth.GenerateCode(secret, now.Add(-65*time.Second))
	validOld, _, _ := auth.ValidateCode(secret, tooOldCode, now, 1)
	if validOld {
		t.Errorf("código antigo (> 60s) não deveria ser aceito")
	}
}

func TestValidateCode_InvalidInputs(t *testing.T) {
	secret, _ := auth.GenerateSecret()
	now := time.Now()

	tests := []string{
		"",
		"12345",
		"1234567",
		"abcdef",
		"12a456",
	}

	for _, input := range tests {
		valid, _, _ := auth.ValidateCode(secret, input, now, 1)
		if valid {
			t.Errorf("entrada %q de comprimento ou formato inválido retornou valid=true", input)
		}
	}
}

func TestGenerateOTPAuthURI(t *testing.T) {
	uri := auth.GenerateOTPAuthURI("VorcaroZAP", "admin.test", "JBSWY3DPEHPK3PXP")

	if !strings.HasPrefix(uri, "otpauth://totp/VorcaroZAP:admin.test?") {
		t.Errorf("prefixo de URI inesperado: %q", uri)
	}
	if !strings.Contains(uri, "secret=JBSWY3DPEHPK3PXP") {
		t.Errorf("URI não contém secret: %q", uri)
	}
	if !strings.Contains(uri, "issuer=VorcaroZAP") {
		t.Errorf("URI não contém issuer: %q", uri)
	}
}
