package importer

import (
	"fmt"
	"strconv"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/normalize"
)

// NormalizeString remove espaços nas pontas e compacta sequências de espaços internos.
func NormalizeString(s string) string {
	return normalize.String(s)
}

// NormalizeUnicode aplica a forma de normalização NFC (Canonical Decomposition, followed by Canonical Composition)
// garantindo representação consistente de caracteres acentuados sem descartar os acentos.
func NormalizeUnicode(s string) string {
	return normalize.Unicode(s)
}

// NormalizeName produz a forma canônica de comparação e indexação de nomes:
// normalização Unicode NFC, compactação de espaços e conversão para minúsculas.
func NormalizeName(name string) string {
	return normalize.Name(name)
}

// GenerateSlug gera um identificador legível por humanos determinístico e estável para URL:
// minúsculo, sem acentos, caracteres não-alfanuméricos substituídos por hífen,
// sem hífens consecutivos ou nas extremidades.
func GenerateSlug(text string) string {
	return normalize.Slug(text)
}

// DisambiguateSlug gera um slug estável quando houver colisão de slugs entre nomes distintos.
func DisambiguateSlug(baseSlug string, index int) string {
	return normalize.DisambiguateSlug(baseSlug, index)
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
	return normalize.CanonicalURL(raw)
}

// ParseRelevance converte e valida a relevância numérica na escala 1..5.
func ParseRelevance(raw string) (domain.Relevance, error) {
	clean := normalize.String(raw)
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
