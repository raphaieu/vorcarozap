package importer

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

var (
	spaceRegex     = regexp.MustCompile(`\s+`)
	slugCleanRegex = regexp.MustCompile(`[^a-z0-9]+`)
)

// NormalizeString remove espaços nas pontas e compacta sequências de espaços internos.
func NormalizeString(s string) string {
	s = strings.TrimSpace(s)
	return spaceRegex.ReplaceAllString(s, " ")
}

// NormalizeUnicode aplica a forma de normalização NFC (Canonical Decomposition, followed by Canonical Composition)
// garantindo representação consistente de caracteres acentuados sem descartar os acentos.
func NormalizeUnicode(s string) string {
	return norm.NFC.String(s)
}

// NormalizeName produz a forma canônica de comparação e indexação de nomes:
// normalização Unicode NFC, compactação de espaços e conversão para minúsculas.
func NormalizeName(name string) string {
	s := NormalizeString(name)
	s = NormalizeUnicode(s)
	return strings.ToLower(s)
}

// removeDiacritics decompõe caracteres acentuados (NFD) e remove marcas não espaçadas (Mn),
// convertendo letras acentuadas para sua base ASCII (ex: 'á' -> 'a', 'ç' -> 'c').
func removeDiacritics(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return result
}

// GenerateSlug gera um identificador legível por humanos determinístico e estável para URL:
// minúsculo, sem acentos, caracteres não-alfanuméricos substituídos por hífen,
// sem hífens consecutivos ou nas extremidades.
func GenerateSlug(text string) string {
	s := NormalizeString(text)
	s = removeDiacritics(s)
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

// CanonicalURL aplica normalização conservadora de URLs conforme ADR-006 e seção A.6:
// - aceita somente esquemas 'http' e 'https';
// - exige hostname válido e não vazio;
// - hostname convertido para minúsculas;
// - remove fragmento (#...);
// - remove portas padrão (:80 para http, :443 para https);
// - preserva query strings legítimas intactas;
// - não faz requests de rede, redirects ou descoberta de canonical tags.
func CanonicalURL(raw string) (string, error) {
	raw = NormalizeString(raw)
	if raw == "" {
		return "", fmt.Errorf("url vazia")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("falha ao analisar url: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("esquema inválido %q: permitido apenas http ou https", u.Scheme)
	}
	u.Scheme = scheme

	host := strings.ToLower(u.Host)
	if host == "" {
		return "", fmt.Errorf("hostname ausente")
	}

	// Remove portas padrão redundantes
	if scheme == "http" && strings.HasSuffix(host, ":80") {
		host = strings.TrimSuffix(host, ":80")
	} else if scheme == "https" && strings.HasSuffix(host, ":443") {
		host = strings.TrimSuffix(host, ":443")
	}
	u.Host = host

	// Remove fragmento
	u.Fragment = ""

	// Normaliza path vazio
	if u.Path == "" {
		u.Path = "/"
	}

	return u.String(), nil
}

// ParseRelevance converte e valida a relevância numérica na escala 1..5.
func ParseRelevance(raw string) (domain.Relevance, error) {
	clean := NormalizeString(raw)
	if clean == "" {
		return 0, fmt.Errorf("relevância vazia")
	}

	val, err := strconv.Atoi(clean)
	if err != nil {
		return 0, fmt.Errorf("relevância não numérica %q: %w", raw, err)
	}

	rel, err := domain.ValidateRelevance(val)
	if err != nil {
		return 0, err
	}

	return rel, nil
}
