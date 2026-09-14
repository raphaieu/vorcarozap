package normalize

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	// MaxLocatorLength define o limite máximo conservador de tamanho para localizadores documentais.
	MaxLocatorLength = 200
)

var (
	// Insecure tokens
	unsafeSchemeRegex  = regexp.MustCompile(`(?i)(javascript|data|vbscript|file):`)
	pathTraversalRegex = regexp.MustCompile(`(\.\./|\.\.\\)`)

	// Range separators inside numbers: e.g. 42-44, 42 - 44, 42—44, 42 a 44
	rangeHyphenRegex = regexp.MustCompile(`(\d+)\s*(?:[-–—]|\ba\b)\s*(\d+)`)

	// Regexes for individual component prefixes (case-insensitive, ordered longer first with word boundaries)
	pageRangeRegex   = regexp.MustCompile(`(?i)^(?:p[aá]ginas|p[aá]gs\.?|pp\.?)\s+(.+)`)
	singlePageRegex  = regexp.MustCompile(`(?i)^(?:p[aá]gina\b|p[aá]g\b\.?|pp?\b\.?)\s*(.+)`)
	leafRangeRegex   = regexp.MustCompile(`(?i)^(?:folhas|fls\b\.?)\s+(.+)`)
	singleLeafRegex  = regexp.MustCompile(`(?i)^(?:folha\b|fls?\b\.?)\s*(.+)`)
	figureRangeRegex = regexp.MustCompile(`(?i)^(?:figuras|figs\b\.?)\s+(.+)`)
	singleFigRegex   = regexp.MustCompile(`(?i)^(?:figura\b|figs?\b\.?)\s*(.+)`)
	tableRangeRegex  = regexp.MustCompile(`(?i)^(?:tabelas|tabs\b\.?)\s+(.+)`)
	singleTableRegex = regexp.MustCompile(`(?i)^(?:tabela\b|tabs?\b\.?)\s*(.+)`)
	annexRangeRegex  = regexp.MustCompile(`(?i)^(?:anexos\b|anxs\b\.?)\s*(.+)`)
	singleAnnexRegex = regexp.MustCompile(`(?i)^(?:anexo\b|anx\b\.?)\s*(.+)`)
	itemRegex        = regexp.MustCompile(`(?i)^(?:itens\b|item\b|it\b\.?)\s*(.+)`)
	sectionRegex     = regexp.MustCompile(`(?i)^(?:se[cç][oõ]es\b|se[cç][aã]o\b|secs?\b\.?)\s*(.+)`)
	articleRegex     = regexp.MustCompile(`(?i)^(?:artigos\b|artigo\b|arts?\b\.?)\s*(.+)`)
	paragraphRegex   = regexp.MustCompile(`(?i)^(?:par[aá]grafos\b|par[aá]grafo\b|parags?\b\.?|§§?)\s*(.+)`)
)

// ValidateLocatorSecurity verifica se a string bruta de localizador cumpre os limites de tamanho
// e não contém HTML, scripts, esquemas perigosos ou tentativas de path traversal.
func ValidateLocatorSecurity(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	if len(trimmed) > MaxLocatorLength {
		return fmt.Errorf("normalize: localizador excede o limite máximo de %d caracteres (obtido: %d)", MaxLocatorLength, len(trimmed))
	}

	// Bloqueio de tags HTML e caracteres de marcação
	if strings.Contains(trimmed, "<") || strings.Contains(trimmed, ">") {
		return errors.New("normalize: localizador não pode conter tags HTML ou caracteres '<' e '>'")
	}

	// Bloqueio de esquemas executáveis
	if unsafeSchemeRegex.MatchString(trimmed) {
		return errors.New("normalize: localizador contém esquema ou conteúdo executável inseguro")
	}

	// Bloqueio de path traversal
	if pathTraversalRegex.MatchString(trimmed) {
		return errors.New("normalize: localizador não pode conter sequências de navegação de diretório")
	}

	// Bloqueio de caracteres de controle invisíveis
	for _, r := range trimmed {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return errors.New("normalize: localizador contém caracteres de controle inválidos")
		}
	}

	return nil
}

// Locator normaliza e formata de forma determinística e não destrutiva localizadores documentais
// como números de páginas, intervalos, figuras, tabelas, laudos e anexos em português.
//
// Exemplos de transformações:
//   - "p. 42" -> "Pág. 42"
//   - "págs. 42-44" -> "Págs. 42–44"
//   - "p. 42, figura 12" -> "Pág. 42, Fig. 12"
//   - "fls. 12-15" -> "Fls. 12–15"
//   - "tabela 3" -> "Tabela 3"
//   - "anexo B, item 4" -> "Anexo B, item 4"
//   - "art. 5, § 1º" -> "Art. 5, § 1º"
//
// Se o texto não casar com padrões conhecidos mas for seguro, é preservado em sua forma Unicode NFC limpa.
// Retorna erro se o conteúdo for malicioso ou exceder limites de segurança.
func Locator(raw string) (string, error) {
	if err := ValidateLocatorSecurity(raw); err != nil {
		return "", err
	}

	clean := String(norm.NFC.String(raw))
	if clean == "" {
		return "", nil
	}

	// Se contém múltiplas cláusulas separadas por vírgula ou ponto-e-vírgula
	var sep string
	if strings.Contains(clean, ";") {
		sep = ";"
	} else if strings.Contains(clean, ",") {
		sep = ","
	}

	if sep != "" {
		parts := strings.Split(clean, sep)
		var normalizedParts []string
		for _, part := range parts {
			trimmedPart := strings.TrimSpace(part)
			if trimmedPart == "" {
				continue
			}
			normPart := normalizeSingleLocatorComponent(trimmedPart)
			normalizedParts = append(normalizedParts, normPart)
		}
		if len(normalizedParts) > 0 {
			return strings.Join(normalizedParts, ", "), nil
		}
	}

	return normalizeSingleLocatorComponent(clean), nil
}

// SafeLocator normaliza e formata o localizador para contextos de apresentação ou persistência segura.
// Caso a string de entrada contenha conteúdo inseguro ou erro de validação, retorna string vazia "".
func SafeLocator(raw string) string {
	loc, err := Locator(raw)
	if err != nil {
		return ""
	}
	return loc
}

// normalizeSingleLocatorComponent normaliza um segmento individual de localizador.
func normalizeSingleLocatorComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// 1. Normaliza hífens em intervalos de números (ex: 42-44 -> 42–44)
	withRange := rangeHyphenRegex.ReplaceAllString(s, "$1–$2")

	// 2. Páginas
	if isRange(withRange) {
		if m := pageRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
			val := normalizeRangeValue(m[1])
			return "Págs. " + val
		}
	}
	if m := singlePageRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		if isRange(val) {
			return "Págs. " + val
		}
		return "Pág. " + val
	}
	if m := pageRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		return "Págs. " + val
	}

	// 3. Folhas
	if isRange(withRange) {
		if m := leafRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
			val := normalizeRangeValue(m[1])
			return "Fls. " + val
		}
	}
	if m := singleLeafRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		if isRange(val) {
			return "Fls. " + val
		}
		return "Fl. " + val
	}
	if m := leafRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		return "Fls. " + val
	}

	// 4. Figuras
	if isRange(withRange) {
		if m := figureRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
			val := normalizeRangeValue(m[1])
			return "Figs. " + val
		}
	}
	if m := singleFigRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		if isRange(val) {
			return "Figs. " + val
		}
		return "Fig. " + val
	}
	if m := figureRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		return "Figs. " + val
	}

	// 5. Tabelas
	if isRange(withRange) {
		if m := tableRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
			val := normalizeRangeValue(m[1])
			return "Tabelas " + val
		}
	}
	if m := singleTableRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		if isRange(val) {
			return "Tabelas " + val
		}
		return "Tabela " + val
	}
	if m := tableRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := normalizeRangeValue(m[1])
		return "Tabelas " + val
	}

	// 6. Anexos
	if m := singleAnnexRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		return "Anexo " + val
	}
	if m := annexRangeRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		return "Anexos " + val
	}

	// 7. Itens
	if m := itemRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		return "item " + val
	}

	// 8. Artigos
	if m := articleRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		if isRange(val) {
			return "Arts. " + val
		}
		return "Art. " + val
	}

	// 9. Parágrafos (§ / §§)
	if m := paragraphRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		if isRange(val) {
			return "§§ " + val
		}
		return "§ " + val
	}

	// 10. Seções
	if m := sectionRegex.FindStringSubmatch(withRange); len(m) > 1 {
		val := strings.TrimSpace(m[1])
		if isRange(val) {
			return "Seções " + val
		}
		return "Seção " + val
	}

	// Se não casou com nenhum prefixo conhecido, preserva o texto com en-dash em intervalos
	return withRange
}

var rangeMatchRegex = regexp.MustCompile(`(?:\d+\s*[–—]\s*\d+|\d+\s*-\s*\d+|\d+\s+a\s+\d+)`)

func isRange(s string) bool {
	return rangeMatchRegex.MatchString(s)
}

func normalizeRangeValue(val string) string {
	val = strings.TrimSpace(val)
	val = rangeHyphenRegex.ReplaceAllString(val, "$1–$2")
	return val
}
