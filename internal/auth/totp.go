package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// TOTPPeriod define o intervalo padrão de rotação do TOTP em segundos (RFC 6238).
	TOTPPeriod = 30
	// TOTPDigits define a quantidade de dígitos numéricos do código gerado.
	TOTPDigits = 6
	// TOTPSecretBytes define o tamanho da entropia do segredo (20 bytes = 160 bits).
	TOTPSecretBytes = 20
	// TOTPDefaultWindow define o número de passos de tolerância temporal (±1 passo = ±30s).
	TOTPDefaultWindow = 1
)

var b32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret gera um novo segredo aleatório criptograficamente seguro codificado em Base32.
func GenerateSecret() (string, error) {
	buf := make([]byte, TOTPSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth/totp: falha ao gerar entropia para segredo: %w", err)
	}
	return b32Encoding.EncodeToString(buf), nil
}

// GenerateCode deriva o código TOTP de 6 dígitos para o segredo e instante fornecidos.
func GenerateCode(secret string, t time.Time) (string, error) {
	timestep := t.Unix() / TOTPPeriod
	return generateCodeForTimestep(secret, timestep)
}

func generateCodeForTimestep(secret string, timestep int64) (string, error) {
	cleanSecret := strings.ToUpper(strings.TrimSpace(secret))
	key, err := b32Encoding.DecodeString(cleanSecret)
	if err != nil {
		// Tenta fallback para decodificação padrão com padding se necessário
		key, err = base32.StdEncoding.DecodeString(cleanSecret)
		if err != nil {
			return "", fmt.Errorf("auth/totp: segredo base32 inválido: %w", err)
		}
	}

	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(timestep))

	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	mac := h.Sum(nil)

	offset := mac[len(mac)-1] & 0x0f
	binaryCode := (uint32(mac[offset]&0x7f) << 24) |
		(uint32(mac[offset+1]&0xff) << 16) |
		(uint32(mac[offset+2]&0xff) << 8) |
		(uint32(mac[offset+3] & 0xff))

	otp := binaryCode % 1000000
	return fmt.Sprintf("%06d", otp), nil
}

// ValidateCode valida se o código de 6 dígitos corresponde ao segredo dentro da janela temporal permitida.
// Retorna (true, timestep, nil) se o código for válido, onde timestep é o passo temporal correspondente.
func ValidateCode(secret, code string, t time.Time, windowSteps int) (bool, int64, error) {
	cleanCode := strings.TrimSpace(code)
	if len(cleanCode) != TOTPDigits {
		return false, 0, nil
	}
	if _, err := strconv.Atoi(cleanCode); err != nil {
		return false, 0, nil
	}

	currentTimestep := t.Unix() / TOTPPeriod

	for i := -windowSteps; i <= windowSteps; i++ {
		step := currentTimestep + int64(i)
		expectedCode, err := generateCodeForTimestep(secret, step)
		if err != nil {
			return false, 0, err
		}
		if hmac.Equal([]byte(cleanCode), []byte(expectedCode)) {
			return true, step, nil
		}
	}

	return false, 0, nil
}

// GenerateOTPAuthURI formata a URI padronizada para importação em aplicativos autenticadores.
func GenerateOTPAuthURI(issuer, username, secret string) string {
	cleanIssuer := strings.TrimSpace(issuer)
	cleanUser := strings.TrimSpace(username)
	cleanSecret := strings.ToUpper(strings.TrimSpace(secret))

	label := url.PathEscape(cleanIssuer) + ":" + url.PathEscape(cleanUser)

	v := url.Values{}
	v.Set("secret", cleanSecret)
	v.Set("issuer", cleanIssuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")

	return fmt.Sprintf("otpauth://totp/%s?%s", label, v.Encode())
}
