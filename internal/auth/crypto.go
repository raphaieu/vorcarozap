package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var (
	ErrDecryptionFailed = errors.New("crypto: falha na decriptação do segredo")
	ErrInvalidKey       = errors.New("crypto: chave de criptografia inválida (requer 32 bytes)")
)

// DeriveKey deriva uma chave simétrica AES-256 de 32 bytes a partir de uma chave textual de 32 bytes ou hexadecimal de 64 caracteres.
// Falha fechado caso a chave seja vazia, tenha tamanho incorreto ou contenha formato hexadecimal inválido.
func DeriveKey(rawKey string) ([]byte, error) {
	if rawKey == "" {
		return nil, ErrInvalidKey
	}
	// Se for hex de 64 caracteres, decodifica estritamente para 32 bytes
	if len(rawKey) == 64 {
		keyBytes, err := hex.DecodeString(rawKey)
		if err == nil && len(keyBytes) == 32 {
			return keyBytes, nil
		}
		return nil, fmt.Errorf("%w: string hexadecimal de 64 caracteres contém caracteres inválidos", ErrInvalidKey)
	}
	// Se for exatamente 32 bytes ASCII
	if len(rawKey) == 32 {
		return []byte(rawKey), nil
	}
	return nil, fmt.Errorf("%w: chave operacional deve ter exatamente 32 bytes ou 64 caracteres hexadecimais (256 bits)", ErrInvalidKey)
}

// EncryptString criptografa uma string em texto plano usando AES-256-GCM.
// Retorna a string em formato hexadecimal contendo nonce (12 bytes) + ciphertext + auth tag (16 bytes).
func EncryptString(plaintext string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKey
	}
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: erro ao instanciar cifra AES: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: erro ao instanciar GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: falha ao gerar nonce criptográfico: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptString descriptografa uma string hexadecimal gerada por EncryptString.
func DecryptString(ciphertextHex string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKey
	}
	if ciphertextHex == "" {
		return "", nil
	}

	data, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", fmt.Errorf("crypto: payload hexadecimal inválido: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: erro ao instanciar cifra AES: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: erro ao instanciar GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrDecryptionFailed
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
