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

var (
	digitsRegex = regexp.MustCompile(`\d+`)
)

// getLocatorComponentRank classifica a hierarquia de segmentos documentais para ordenação canônica:
// 1. Páginas / Folhas (conteúdo principal sequencial)
// 2. Artigos / Parágrafos / Seções
// 3. Figuras / Imagens
// 4. Tabelas
// 5. Anexos / Itens / Outros
func getLocatorComponentRank(comp string) int {
	c := strings.ToLower(comp)
	switch {
	case strings.HasPrefix(c, "pág") || strings.HasPrefix(c, "pp") || strings.HasPrefix(c, "fl"):
		return 1
	case strings.HasPrefix(c, "art") || strings.HasPrefix(c, "§") || strings.HasPrefix(c, "seç"):
		return 2
	case strings.HasPrefix(c, "fig"):
		return 3
	case strings.HasPrefix(c, "tab"):
		return 4
	case strings.HasPrefix(c, "anex") || strings.HasPrefix(c, "it"):
		return 5
	default:
		return 6
	}
}

// CompareLocators compara dois localizadores documentais retornando:
// -1 se a precede b
//
//	0 se a e b são equivalentes na ordenação
//	1 se a sucede b
//
// A ordenação é determinística e natural:
// - Localizadores vazios são posicionados no final.
// - Segmentos de páginas/folhas precedem figuras, tabelas e anexos.
// - Números são ordenados pelo seu valor inteiro (ex: 5 < 12 < 42).
// - Intervalos são desempatados pelo limite final (ex: 42–44 < 42–48).
// - Subcomponentes múltiplos ("Pág. 10, Fig. 2" vs "Pág. 10, Fig. 5") são comparados em cascata.
func CompareLocators(a, b string) int {
	if a == b {
		return 0
	}

	normA := SafeLocator(a)
	normB := SafeLocator(b)

	// Itens vazios vão para o final
	if normA == "" && normB != "" {
		return 1
	}
	if normA != "" && normB == "" {
		return -1
	}
	if normA == "" && normB == "" {
		return strings.Compare(strings.TrimSpace(a), strings.TrimSpace(b))
	}

	// Divide em múltiplos componentes separados por vírgula ou ponto-e-vírgula
	splitComps := func(s string) []string {
		var parts []string
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
			t := strings.TrimSpace(p)
			if t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 && s != "" {
			parts = append(parts, s)
		}
		return parts
	}

	partsA := splitComps(normA)
	partsB := splitComps(normB)

	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}

	for i := 0; i < minLen; i++ {
		compA := partsA[i]
		compB := partsB[i]

		rankA := getLocatorComponentRank(compA)
		rankB := getLocatorComponentRank(compB)

		if rankA != rankB {
			if rankA < rankB {
				return -1
			}
			return 1
		}

		// Extrai dígitos para comparação numérica
		numsA := digitsRegex.FindAllString(compA, -1)
		numsB := digitsRegex.FindAllString(compB, -1)

		if len(numsA) > 0 && len(numsB) > 0 {
			// Compara o primeiro número
			var valA1, valB1 int
			fmt.Sscanf(numsA[0], "%d", &valA1)
			fmt.Sscanf(numsB[0], "%d", &valB1)

			if valA1 != valB1 {
				if valA1 < valB1 {
					return -1
				}
				return 1
			}

			// Se ambos têm segundo número (intervalo)
			if len(numsA) > 1 && len(numsB) > 1 {
				var valA2, valB2 int
				fmt.Sscanf(numsA[1], "%d", &valA2)
				fmt.Sscanf(numsB[1], "%d", &valB2)

				if valA2 != valB2 {
					if valA2 < valB2 {
						return -1
					}
					return 1
				}
			} else if len(numsA) > 1 {
				// a é intervalo (42–44), b é número único (42) -> número único precede intervalo
				return 1
			} else if len(numsB) > 1 {
				return -1
			}
		}

		// Comparação lexicográfica do componente caso numérico não resolva
		if compA != compB {
			return strings.Compare(compA, compB)
		}
	}

	if len(partsA) != len(partsB) {
		if len(partsA) < len(partsB) {
			return -1
		}
		return 1
	}

	return 0
}
