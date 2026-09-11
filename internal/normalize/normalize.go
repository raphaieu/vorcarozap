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
		return "", fmt.Errorf("normalize: falha ao analisar url: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("normalize: esquema inválido %q: permitido apenas http ou https", u.Scheme)
	}
	u.Scheme = scheme

	host := strings.ToLower(u.Host)
	if host == "" {
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
