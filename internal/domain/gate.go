package domain

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Constantes de limites textuais do gate estrutural.
const (
	MaxEntityNameLen       = 200
	MaxRelationshipTypeLen = 100
	MaxPropositionLen      = 1000
	MaxExcerptLen          = 2000
	MaxLocatorLen          = 200
	MaxContextLimitsLen    = 1000
	MaxSourceURLLen        = 2000
	MaxSourceTitleLen      = 300
	MaxPublisherAuthorLen  = 200
)

var (
	cpfRegex       = regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b|\b\d{11}\b`)
	phoneRegex     = regexp.MustCompile(`(?:\+?55\s?)?(?:\(?\d{2}\)?\s?)(?:9\d{4}|\d{4})[-\s]?\d{4}\b`)
	emailRegex     = regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	rgExplicitReg  = regexp.MustCompile(`(?i)\b(?:RG|Identidade)\s*[:.\-]?\s*[0-9A-Za-z.\-]{6,14}\b`)
	cepRegex       = regexp.MustCompile(`\b\d{5}-\d{3}\b`)
	addressRegex   = regexp.MustCompile(`(?i)\b(?:Rua|Avenida|Av\.|Alameda|Al\.|Travessa|Tv\.|Rodovia|Rod\.)\s+[A-Za-zÀ-ÿ0-9\s.'-]{2,40},\s*(?:n[ºo°.]?\s*)?\d+\b`)
	apartmentRegex = regexp.MustCompile(`(?i)\b(?:Apto|Apartamento|Bloco)\s+\d+\b`)
	isoDateRegex   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// StructuralGateInput encapsula todos os dados do candidato necessários para o gate estrutural puro em Go.
type StructuralGateInput struct {
	Fingerprint                   string
	FingerprintVersion            int64
	IsDuplicate                   bool
	EntityName                    string
	NormalizedEntityName          string
	TargetEntityName              string
	NormalizedTargetEntityName    string
	CaseName                      string
	NormalizedCaseName            string
	RelationshipType              string
	Proposition                   string
	SuggestedGrade                EvidenceGrade
	SourceURL                     string
	CanonicalURL                  string
	SourceTitle                   string
	PublisherOrAuthor             string
	PublishedAt                   string
	Excerpt                       string
	Locator                       string
	ContextLimits                 string
	SubjectResolvedID             string
	SubjectAmbiguous              bool
	TargetResolvedID              string
	TargetAmbiguous               bool
	CaseResolvedID                string
	CaseAmbiguous                 bool
	PreviouslyRejectedFingerprint bool
	SourceAccessStatus            SourceAccessStatus
}

// StructuralGateResult representa o veredito determinístico do gate estrutural em Go.
type StructuralGateResult struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons"`
}

// EvaluateStructuralGate executa a validação pura, determinística e sem I/O do gate estrutural.
func EvaluateStructuralGate(input StructuralGateInput) StructuralGateResult {
	var reasons []string

	// 1. Candidato canônico e versão de fingerprint
	if input.IsDuplicate {
		reasons = append(reasons, "duplicate_candidate")
	}
	if input.FingerprintVersion != 1 {
		reasons = append(reasons, "invalid_fingerprint_version")
	}
	if strings.TrimSpace(input.Fingerprint) == "" {
		reasons = append(reasons, "empty_fingerprint")
	}

	// 2. URL canônica
	cURL := strings.TrimSpace(input.CanonicalURL)
	if cURL == "" {
		reasons = append(reasons, "empty_canonical_url")
	} else if len(cURL) > MaxSourceURLLen {
		reasons = append(reasons, "canonical_url_too_long")
	} else {
		u, err := url.Parse(cURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Host) == "" {
			reasons = append(reasons, "invalid_canonical_url")
		}
	}

	// 3. Sujeito
	subj := strings.TrimSpace(input.EntityName)
	if subj == "" {
		reasons = append(reasons, "empty_subject_name")
	} else if len(subj) > MaxEntityNameLen {
		reasons = append(reasons, "subject_name_too_long")
	}

	// 4. Título da fonte e Publicador/Autor
	title := strings.TrimSpace(input.SourceTitle)
	if title == "" {
		reasons = append(reasons, "empty_source_title")
	} else if len(title) > MaxSourceTitleLen {
		reasons = append(reasons, "source_title_too_long")
	}

	pub := strings.TrimSpace(input.PublisherOrAuthor)
	if pub == "" {
		reasons = append(reasons, "empty_publisher_or_author")
	} else if len(pub) > MaxPublisherAuthorLen {
		reasons = append(reasons, "publisher_or_author_too_long")
	}

	// 5. Trecho literal e Proposição
	excerpt := strings.TrimSpace(input.Excerpt)
	if excerpt == "" {
		reasons = append(reasons, "empty_excerpt")
	} else if len(excerpt) > MaxExcerptLen {
		reasons = append(reasons, "excerpt_too_long")
	}

	prop := strings.TrimSpace(input.Proposition)
	if prop == "" {
		reasons = append(reasons, "empty_proposition")
	} else if len(prop) > MaxPropositionLen {
		reasons = append(reasons, "proposition_too_long")
	}

	// 6. Tipo de relacionamento
	relType := strings.TrimSpace(input.RelationshipType)
	if relType == "" {
		reasons = append(reasons, "empty_relationship_type")
	} else if len(relType) > MaxRelationshipTypeLen {
		reasons = append(reasons, "relationship_type_too_long")
	}

	// 7. Limites de locator e context_limits
	if len(input.Locator) > MaxLocatorLen {
		reasons = append(reasons, "locator_too_long")
	}
	if len(input.ContextLimits) > MaxContextLimitsLen {
		reasons = append(reasons, "context_limits_too_long")
	}

	// 8. Data de publicação (quando informada deve ser ISO YYYY-MM-DD válida)
	if pubAt := strings.TrimSpace(input.PublishedAt); pubAt != "" {
		if !isoDateRegex.MatchString(pubAt) {
			reasons = append(reasons, "invalid_published_at_format")
		} else if _, err := time.Parse("2006-01-02", pubAt); err != nil {
			reasons = append(reasons, "invalid_published_at_date")
		}
	}

	// 9. Grau A–E
	if !input.SuggestedGrade.IsValid() {
		reasons = append(reasons, "invalid_grade")
	}

	// 10. Alvo textual: XOR estrito entre target_entity_name e case_name
	hasTarget := strings.TrimSpace(input.TargetEntityName) != ""
	hasCase := strings.TrimSpace(input.CaseName) != ""
	if (hasTarget && hasCase) || (!hasTarget && !hasCase) {
		reasons = append(reasons, "target_case_xor_violated")
	}

	// 11. Resolução determinística e unívoca de entidades/casos
	if input.SubjectAmbiguous {
		reasons = append(reasons, "ambiguous_subject_entity")
	} else if input.SubjectResolvedID == "" {
		reasons = append(reasons, "unresolved_subject_entity")
	}

	if hasTarget {
		if input.TargetAmbiguous {
			reasons = append(reasons, "ambiguous_target_entity")
		} else if input.TargetResolvedID == "" {
			reasons = append(reasons, "unresolved_target_entity")
		}
	}

	if hasCase {
		if input.CaseAmbiguous {
			reasons = append(reasons, "ambiguous_case")
		} else if input.CaseResolvedID == "" {
			reasons = append(reasons, "unresolved_case")
		}
	}

	// 12. Proibição de autorrelação
	if hasTarget {
		if input.TargetResolvedID != "" && input.SubjectResolvedID != "" && input.TargetResolvedID == input.SubjectResolvedID {
			reasons = append(reasons, "self_relation_prohibited")
		}
		normSubj := strings.TrimSpace(strings.ToLower(input.NormalizedEntityName))
		normTarget := strings.TrimSpace(strings.ToLower(input.NormalizedTargetEntityName))
		if normSubj != "" && normSubj == normTarget {
			reasons = append(reasons, "self_relation_prohibited")
		}
	}

	// 13. Detecção de PII proibida/desnecessária em todos os campos materializáveis/publicáveis
	if ContainsProhibitedPII(input.EntityName) ||
		ContainsProhibitedPII(input.TargetEntityName) ||
		ContainsProhibitedPII(input.CaseName) ||
		ContainsProhibitedPII(input.RelationshipType) ||
		ContainsProhibitedPII(input.Proposition) ||
		ContainsProhibitedPII(input.Excerpt) ||
		ContainsProhibitedPII(input.Locator) ||
		ContainsProhibitedPII(input.ContextLimits) ||
		ContainsProhibitedPII(input.SourceTitle) ||
		ContainsProhibitedPII(input.PublisherOrAuthor) {
		reasons = append(reasons, "contains_unnecessary_pii")
	}

	// 14. Fingerprint previamente rejeitado
	if input.PreviouslyRejectedFingerprint {
		reasons = append(reasons, "fingerprint_previously_rejected")
	}

	// 15. Acessibilidade da fonte via GET seguro
	if input.SourceAccessStatus != SourceAccessReachable {
		switch input.SourceAccessStatus {
		case SourceAccessUnreachable:
			reasons = append(reasons, "source_unreachable")
		case SourceAccessNotChecked:
			reasons = append(reasons, "source_not_checked")
		case SourceAccessCitedByProvider:
			reasons = append(reasons, "source_cited_by_provider_not_verified")
		default:
			reasons = append(reasons, "source_not_reachable")
		}
	}

	return StructuralGateResult{
		Passed:  len(reasons) == 0,
		Reasons: reasons,
	}
}

// ContainsProhibitedPII analisa o texto em busca de dados pessoais desnecessários e sensíveis.
func ContainsProhibitedPII(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}

	if cpfRegex.MatchString(text) {
		matches := cpfRegex.FindAllString(text, -1)
		for _, m := range matches {
			digits := strings.ReplaceAll(strings.ReplaceAll(m, ".", ""), "-", "")
			if len(digits) == 11 && isValidCPFDigits(digits) {
				return true
			}
		}
	}

	if phoneRegex.MatchString(text) {
		return true
	}

	if emailRegex.MatchString(text) {
		return true
	}

	if rgExplicitReg.MatchString(text) {
		return true
	}

	if cepRegex.MatchString(text) {
		return true
	}

	if addressRegex.MatchString(text) {
		return true
	}

	if apartmentRegex.MatchString(text) {
		return true
	}

	return false
}

func isValidCPFDigits(d string) bool {
	if len(d) != 11 {
		return false
	}

	// Rejeita sequências de todos os dígitos iguais
	allSame := true
	for i := 1; i < 11; i++ {
		if d[i] != d[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}

	// Cálculo dos dígitos verificadores
	var sum1, sum2 int
	for i := 0; i < 9; i++ {
		val, err := strconv.Atoi(string(d[i]))
		if err != nil {
			return false
		}
		sum1 += val * (10 - i)
		sum2 += val * (11 - i)
	}

	r1 := (sum1 * 10) % 11
	if r1 == 10 {
		r1 = 0
	}
	d9, _ := strconv.Atoi(string(d[9]))
	if r1 != d9 {
		return false
	}

	sum2 += d9 * 2
	r2 := (sum2 * 10) % 11
	if r2 == 10 {
		r2 = 0
	}
	d10, _ := strconv.Atoi(string(d[10]))
	return r2 == d10
}
