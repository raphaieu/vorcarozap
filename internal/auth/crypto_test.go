package auth_test

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/auth"
)

func TestCrypto_EncryptDecrypt_Success(t *testing.T) {
	key, err := auth.DeriveKey("12345678901234567890123456789012") // exatamente 32 bytes
	if err != nil {
		t.Fatalf("falha ao derivar chave: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("tamanho de chave esperado 32, obtido %d", len(key))
	}

	secret := "JBSWY3DPEHPK3PXP"
	encrypted, err := auth.EncryptString(secret, key)
	if err != nil {
		t.Fatalf("falha ao criptografar: %v", err)
	}

	if encrypted == secret || encrypted == "" {
		t.Fatalf("ciphertext inválido ou não cifrado: %s", encrypted)
	}

	decrypted, err := auth.DecryptString(encrypted, key)
	if err != nil {
		t.Fatalf("falha ao descriptografar: %v", err)
	}

	if decrypted != secret {
		t.Fatalf("esperado %s, obtido %s", secret, decrypted)
	}
}

func TestCrypto_TamperedCiphertext_Fails(t *testing.T) {
	key, err := auth.DeriveKey("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("falha ao derivar chave: %v", err)
	}
	secret := "MFA_SECRET_SUPER_CONFIDENCIAL"

	encrypted, err := auth.EncryptString(secret, key)
	if err != nil {
		t.Fatalf("falha ao criptografar: %v", err)
	}

	// Altera os últimos bytes do ciphertext (tag de autenticação)
	bytes, err := hex.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("falha ao decodificar hex: %v", err)
	}
	bytes[len(bytes)-1] ^= 0xFF
	tamperedHex := hex.EncodeToString(bytes)

	_, err = auth.DecryptString(tamperedHex, key)
	if err != auth.ErrDecryptionFailed {
		t.Errorf("esperava ErrDecryptionFailed para payload adulterado, obtido: %v", err)
	}
}

func TestCrypto_WrongKey_Fails(t *testing.T) {
	key1, _ := auth.DeriveKey("11111111111111111111111111111111")
	key2, _ := auth.DeriveKey("22222222222222222222222222222222")
	secret := "SECRET123"

	encrypted, err := auth.EncryptString(secret, key1)
	if err != nil {
		t.Fatalf("falha ao criptografar: %v", err)
	}

	_, err = auth.DecryptString(encrypted, key2)
	if err != auth.ErrDecryptionFailed {
		t.Errorf("esperava ErrDecryptionFailed para chave incorreta, obtido: %v", err)
	}
}

func TestCrypto_DeriveKey_Validation(t *testing.T) {
	tests := []struct {
		name      string
		rawKey    string
		expectErr bool
		errIs     error
	}{
		{
			name:      "chave hex valida de 64 caracteres",
			rawKey:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expectErr: false,
		},
		{
			name:      "chave ascii valida de exatamente 32 bytes",
			rawKey:    "12345678901234567890123456789012",
			expectErr: false,
		},
		{
			name:      "chave vazia falha",
			rawKey:    "",
			expectErr: true,
			errIs:     auth.ErrInvalidKey,
		},
		{
			name:      "chave curta de 16 bytes falha",
			rawKey:    "1234567890123456",
			expectErr: true,
			errIs:     auth.ErrInvalidKey,
		},
		{
			name:      "chave de 31 bytes falha",
			rawKey:    "1234567890123456789012345678901",
			expectErr: true,
			errIs:     auth.ErrInvalidKey,
		},
		{
			name:      "chave de 33 bytes falha",
			rawKey:    "123456789012345678901234567890123",
			expectErr: true,
			errIs:     auth.ErrInvalidKey,
		},
		{
			name:      "chave de 64 caracteres com hex invalido falha",
			rawKey:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789zzzzzz",
			expectErr: true,
			errIs:     auth.ErrInvalidKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, err := auth.DeriveKey(tt.rawKey)
			if (err != nil) != tt.expectErr {
				t.Fatalf("DeriveKey(%q): esperado erro=%v, obtido err=%v", tt.rawKey, tt.expectErr, err)
			}
			if tt.expectErr {
				if tt.errIs != nil && !errors.Is(err, tt.errIs) {
					t.Errorf("DeriveKey(%q): esperado erro %v, obtido %v", tt.rawKey, tt.errIs, err)
				}
				return
			}
			if len(k) != 32 {
				t.Errorf("DeriveKey(%q): esperado tamanho 32, obtido %d", tt.rawKey, len(k))
			}
		})
	}
}
