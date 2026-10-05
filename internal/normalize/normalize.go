package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	spaceRegex     = regexp.MustCompile(`\s+`)
	slugCleanRegex = regexp.MustCompile(`[^a-z0-9]+`)
)

// String remove espaços nas extremidades e compacta sequências de espaços internos em um único espaço em branco.
func String(s string) string {
	s = strings.TrimSpace(s)
	return spaceRegex.ReplaceAllString(s, " ")
}

// Unicode aplica a forma de normalização canônica NFC (Canonical Decomposition, followed by Canonical Composition),
// preservando a acentuação e padronizando representações compostas.
func Unicode(s string) string {
	return norm.NFC.String(s)
}

// Name produz a forma canônica de comparação e indexação de nomes de pessoas e instituições:
// normalização Unicode NFC, compactação de espaços e conversão para minúsculas.
func Name(name string) string {
	s := String(name)
	s = Unicode(s)
	return strings.ToLower(s)
}

// RemoveDiacritics decompõe caracteres acentuados (NFD) e remove marcas não espaçadas (Mn),
// convertendo letras acentuadas para sua base ASCII (ex: 'á' -> 'a', 'ç' -> 'c').
func RemoveDiacritics(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return result
}

// Slug gera um identificador legível por humanos determinístico e estável para URL:
// minúsculo, sem acentos, caracteres não-alfanuméricos substituídos por hífen,
// sem hífens consecutivos ou nas extremidades.
func Slug(text string) string {
	s := String(text)
	s = RemoveDiacritics(s)
	s = strings.ToLower(s)
	s = slugCleanRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "item"
	}
	return s
}

// DisambiguateSlug gera um slug estável quando houver colisão de slugs entre nomes distintos.
func DisambiguateSlug(baseSlug string, index int) string {
	if index <= 1 {
		return baseSlug
	}
	return fmt.Sprintf("%s-%d", baseSlug, index)
}

// CanonicalURL aplica normalização conservadora de URLs conforme ADR-006 e ADR-013:
// - aceita somente esquemas 'http' e 'https';
// - exige hostname válido e não vazio;
// - rejeita credenciais embutidas (userinfo);
// - hostname e esquema convertidos para minúsculas;
// - remove fragmento (#...);
// - remove portas padrão (:80 para http, :443 para https);
// - preserva query strings legítimas intactas;
// - normaliza caminho vazio para "/";
// - não faz requests de rede, redirects ou descoberta de canonical tags.
func CanonicalURL(raw string) (string, error) {
	raw = String(raw)
	if raw == "" {
		return "", fmt.Errorf("normalize: url vazia")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("normalize: falha ao analisar url")
	}

	if u.User != nil {
		return "", fmt.Errorf("normalize: credenciais embutidas na url não são permitidas")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("normalize: esquema inválido %q: permitido apenas http ou https", u.Scheme)
	}
	u.Scheme = scheme

	host := strings.ToLower(u.Host)
	if host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("normalize: hostname ausente")
	}

	// Remove portas padrão redundantes
	if scheme == "http" && strings.HasSuffix(host, ":80") {
		host = strings.TrimSuffix(host, ":80")
	} else if scheme == "https" && strings.HasSuffix(host, ":443") {
		host = strings.TrimSuffix(host, ":443")
	}
	u.Host = host

	// Remove fragmento (#...)
	u.Fragment = ""

	// Normaliza path vazio para /
	if u.Path == "" {
		u.Path = "/"
	}

	return u.String(), nil
}

var sensitiveQueryParams = map[string]struct{}{
	// Tokens & Auth
	"token":         {},
	"access_token":  {},
	"accesstoken":   {},
	"auth_token":    {},
	"authtoken":     {},
	"api_token":     {},
	"apitoken":      {},
	"id_token":      {},
	"idtoken":       {},
	"bearer":        {},
	"bearer_token":  {},
	"bearertoken":   {},
	"refresh_token": {},
	"refreshtoken":  {},
	"session":       {},
	"session_token": {},
	"sessiontoken":  {},
	"session_id":    {},
	"sessionid":     {},
	"jwt":           {},
	"auth":          {},
	"authorization": {},
	"code":          {},
	"assertion":     {},
	"passcode":      {},
	"otp":           {},
	"cf_token":      {},
	"cftoken":       {},
	"token_hash":    {},
	"secure_token":  {},
	// Passwords & Secrets
	"password":      {},
	"pass":          {},
	"passwd":        {},
	"pwd":           {},
	"secret":        {},
	"api_secret":    {},
	"apisecret":     {},
	"client_secret": {},
	"clientsecret":  {},
	"shared_secret": {},
	"sharedsecret":  {},
	"credential":    {},
	"credentials":   {},
	// Keys
	"key":         {},
	"apikey":      {},
	"api_key":     {},
	"access_key":  {},
	"accesskey":   {},
	"auth_key":    {},
	"authkey":     {},
	"secret_key":  {},
	"secretkey":   {},
	"app_key":     {},
	"appkey":      {},
	"private_key": {},
	"privatekey":  {},
	// Signatures & Signed URLs (AWS S3, GCP Storage, Azure SAS, Cloudflare)
	"signature":             {},
	"sig":                   {},
	"sign":                  {},
	"signed_url":            {},
	"hmac":                  {},
	"sas_token":             {},
	"sastoken":              {},
	"se":                    {},
	"sp":                    {},
	"sv":                    {},
	"sr":                    {},
	"skoid":                 {},
	"sktid":                 {},
	"x_amz_signature":       {},
	"x_amz_credential":      {},
	"x_amz_security_token":  {},
	"x_amz_algorithm":       {},
	"x_amz_date":            {},
	"x_goog_signature":      {},
	"x_goog_credential":     {},
	"x_goog_security_token": {},
	"x_goog_algorithm":      {},
	"x_goog_date":           {},
}

// IsSensitiveQueryParamName avalia se um nome de parâmetro de consulta é sensível
// (token, senha, segredo, chave de API, assinatura criptográfica ou credencial de URL assinada).
// A comparação é insensível a maiúsculas/minúsculas e padroniza separadores ('-' e '_').
func IsSensitiveQueryParamName(paramName string) bool {
	k := strings.ToLower(strings.TrimSpace(paramName))
	if k == "" {
		return false
	}
	kClean := strings.ReplaceAll(k, "-", "_")

	if _, found := sensitiveQueryParams[kClean]; found {
		return true
	}
	if _, found := sensitiveQueryParams[k]; found {
		return true
	}

	// Prefixos conhecidos de URLs assinadas de provedores de nuvem
	if strings.HasPrefix(kClean, "x_amz_") || strings.HasPrefix(kClean, "x_goog_") {
		return true
	}

	// Sufixos comuns de parâmetros sensíveis
	if strings.HasSuffix(kClean, "_token") ||
		strings.HasSuffix(kClean, "_secret") ||
		strings.HasSuffix(kClean, "_password") ||
		strings.HasSuffix(kClean, "_passwd") ||
		strings.HasSuffix(kClean, "_signature") ||
		strings.HasSuffix(kClean, "_sig") ||
		strings.HasSuffix(kClean, "_apikey") ||
		strings.HasSuffix(kClean, "_api_key") ||
		strings.HasSuffix(kClean, "_session") ||
		strings.HasSuffix(kClean, "_auth") {
		return true
	}

	return false
}

// HasSensitiveQueryParams analisa a query string de uma URL e verifica se contém
// parâmetros sensíveis como tokens, senhas, chaves de API, assinaturas ou credenciais.
// A verificação é fail-closed em caso de erro de parsing.
func HasSensitiveQueryParams(u *url.URL) bool {
	if u == nil || u.RawQuery == "" {
		return false
	}

	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return true // Fail-closed em caso de query malformada ou erro de parsing
	}

	for k := range values {
		if IsSensitiveQueryParamName(k) {
			return true
		}
	}

	return false
}

// SafeURL normaliza e sanitiza uma URL para serialização e exibição pública ou administrativa segura.
// Exige esquema http ou https, hostname válido, ausência de credenciais embutidas (userinfo),
// remove fragmentos (#...) e omite integralmente URLs que contenham parâmetros de consulta sensíveis
// (tokens, senhas, chaves de API, assinaturas criptográficas ou URLs assinadas de provedores em nuvem).
// Preserva links com parâmetros de consulta legítimos (como ?id=123, ?page=2, ?q=termo).
// Em caso de URL inválida, vazia, insegura ou com parâmetros sensíveis, retorna string vazia "".
func SafeURL(raw string) string {
	cURL, err := CanonicalURL(raw)
	if err != nil {
		return ""
	}

	u, err := url.Parse(cURL)
	if err != nil {
		return ""
	}

	if HasSensitiveQueryParams(u) {
		return ""
	}

	return cURL
}

// FingerprintV1 calcula o hash determinístico SHA-256 da versão 1 para um candidato de monitoramento.
// Composição exata conforme ADR-013:
// canonical_url + "|" + normalized_entity_name + "|" + normalized_proposition + "|" + normalized_excerpt
func FingerprintV1(canonicalURL, entityName, proposition, excerpt string) string {
	cURL := String(canonicalURL)
	normEntity := Name(entityName)
	normProp := String(Unicode(proposition))
	normExcerpt := String(Unicode(excerpt))

	payload := fmt.Sprintf("%s|%s|%s|%s", cURL, normEntity, normProp, normExcerpt)
	hash := sha256.Sum256([]byte(payload))
	return "v1:" + hex.EncodeToString(hash[:])
}

// ComputeFingerprint calcula o fingerprint versionado do candidato para a versão especificada.
func ComputeFingerprint(version int, canonicalURL, entityName, proposition, excerpt string) (string, error) {
	switch version {
	case 1:
		return FingerprintV1(canonicalURL, entityName, proposition, excerpt), nil
	default:
		return "", fmt.Errorf("normalize: versão de fingerprint desconhecida %d", version)
	}
}
