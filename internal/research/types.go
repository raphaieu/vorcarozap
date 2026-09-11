package research

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Erros sentinela do pacote research.
var (
	ErrMissingAPIKey    = errors.New("research: chave de API não configurada")
	ErrNotImplemented   = errors.New("research: funcionalidade não implementada nesta fase")
	ErrEmptyQuery       = errors.New("research: consulta de descoberta vazia")
	ErrEmptyResponse    = errors.New("research: resposta do provedor vazia ou sem escolhas válidas")
	ErrInvalidResponse  = errors.New("research: resposta do provedor inválida ou corrompida")
	ErrResponseTooLarge = errors.New("research: resposta do provedor excedeu o limite máximo de tamanho")
	ErrInvalidCandidate = errors.New("research: candidato com dados estruturados inválidos ou fora dos limites")
	ErrInvalidVerifyIn  = errors.New("research: parâmetros de entrada para verificação semântica inválidos")
)

// DiscoverInput encapsula os parâmetros de consulta para descoberta de novas informações e evidências.
type DiscoverInput struct {
	Query string `json:"query"`
}

// Citation representa uma citação/fonte técnica retornada pela busca ou ancorada pelo modelo.
type Citation struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
}

// CandidateExtraction encapsula uma alegação factual estruturada extraída pelo modelo.
type CandidateExtraction struct {
	EntityName          string  `json:"entity_name"`
	TargetEntityName    string  `json:"target_entity_name,omitempty"`
	CaseName            string  `json:"case_name,omitempty"`
	RelationshipType    string  `json:"relationship_type,omitempty"`
	Proposition         string  `json:"proposition"`
	SuggestedGrade      string  `json:"suggested_grade"`
	SourceURL           string  `json:"source_url"`
	SourceTitle         string  `json:"source_title"`
	PublisherOrAuthor   string  `json:"publisher_or_author"`
	PublishedAt         string  `json:"published_at"`
	Excerpt             string  `json:"excerpt"`
	Locator             string  `json:"locator"`
	ContextLimits       string  `json:"context_limits"`
	TechnicalConfidence float64 `json:"technical_confidence"`
}

// Validate valida os invariantes estruturais básicos da extração.
func (c *CandidateExtraction) Validate() error {
	if strings.TrimSpace(c.EntityName) == "" {
		return fmt.Errorf("%w: entity_name não pode ser vazio", ErrInvalidCandidate)
	}
	if strings.TrimSpace(c.Proposition) == "" {
		return fmt.Errorf("%w: proposition não pode ser vazia", ErrInvalidCandidate)
	}
	switch c.SuggestedGrade {
	case "A", "B", "C", "D", "E":
		// grau válido
	default:
		return fmt.Errorf("%w: suggested_grade inválido %q (deve ser A, B, C, D ou E)", ErrInvalidCandidate, c.SuggestedGrade)
	}
	if strings.TrimSpace(c.SourceURL) == "" {
		return fmt.Errorf("%w: source_url não pode ser vazia", ErrInvalidCandidate)
	}
	if c.TechnicalConfidence < 0.0 || c.TechnicalConfidence > 1.0 {
		return fmt.Errorf("%w: technical_confidence %.2f fora do intervalo [0.0, 1.0]", ErrInvalidCandidate, c.TechnicalConfidence)
	}
	return nil
}

// DiscoverResult encapsula a resposta técnica da etapa de descoberta com candidatos estruturados.
type DiscoverResult struct {
	Candidates       []CandidateExtraction `json:"candidates"`
	Content          string                `json:"content"`
	Model            string                `json:"model"`
	Citations        []Citation            `json:"citations"`
	PromptTokens     int                   `json:"prompt_tokens"`
	CompletionTokens int                   `json:"completion_tokens"`
	TotalTokens      int                   `json:"total_tokens"`
	WebSearchCalls   int                   `json:"web_search_calls"`
}

// VerifyInput define a entrada para a verificação semântica de compatibilidade documental (gate semântico).
type VerifyInput struct {
	SubjectName       string `json:"subject_name"`
	TargetEntityName  string `json:"target_entity_name,omitempty"`
	CaseName          string `json:"case_name,omitempty"`
	RelationshipType  string `json:"relationship_type,omitempty"`
	Proposition       string `json:"proposition"`
	Excerpt           string `json:"excerpt"`
	SourceTitle       string `json:"source_title,omitempty"`
	PublisherOrAuthor string `json:"publisher_or_author,omitempty"`
	SourceURL         string `json:"source_url"`
	Grade             string `json:"grade"`
	ContextLimits     string `json:"context_limits,omitempty"`
}

// Validate valida os parâmetros de entrada para o gate semântico.
func (v *VerifyInput) Validate() error {
	if strings.TrimSpace(v.SubjectName) == "" {
		return fmt.Errorf("%w: subject_name não pode ser vazio", ErrInvalidVerifyIn)
	}
	if strings.TrimSpace(v.Proposition) == "" {
		return fmt.Errorf("%w: proposition não pode ser vazia", ErrInvalidVerifyIn)
	}
	if strings.TrimSpace(v.Excerpt) == "" {
		return fmt.Errorf("%w: excerpt não pode ser vazio", ErrInvalidVerifyIn)
	}
	if strings.TrimSpace(v.SourceURL) == "" {
		return fmt.Errorf("%w: source_url não pode ser vazia", ErrInvalidVerifyIn)
	}
	switch v.Grade {
	case "A", "B", "C", "D", "E":
		// grau válido
	default:
		return fmt.Errorf("%w: grade inválido %q (deve ser A, B, C, D ou E)", ErrInvalidVerifyIn, v.Grade)
	}
	return nil
}

// VerifyResult define a avaliação semântica estruturada retornada pela LLM.
type VerifyResult struct {
	IdentityMatch            bool     `json:"identity_match"`
	ClaimSupported           bool     `json:"claim_supported"`
	ClaimOverstatesSource    bool     `json:"claim_overstates_source"`
	AttributionExplicit      bool     `json:"attribution_explicit"`
	GradeCompatible          bool     `json:"grade_compatible"`
	ContainsIllicitInference bool     `json:"contains_illicit_inference"`
	Uncertainties            []string `json:"uncertainties"`
	RecommendedAction        string   `json:"recommended_action"`
}

// Validate valida o resultado retornado pelo modelo de verificação.
func (r *VerifyResult) Validate() error {
	switch r.RecommendedAction {
	case "publish", "quarantine", "reject":
		// ação permitida
	default:
		return fmt.Errorf("%w: recommended_action inválida %q (deve ser publish, quarantine ou reject)", ErrInvalidResponse, r.RecommendedAction)
	}
	if r.Uncertainties == nil {
		return fmt.Errorf("%w: uncertainties não pode ser nulo", ErrInvalidResponse)
	}
	for i, u := range r.Uncertainties {
		if strings.TrimSpace(u) == "" {
			return fmt.Errorf("%w: uncertainties[%d] não pode ser vazio ou conter apenas espaços", ErrInvalidResponse, i)
		}
	}
	return nil
}

// ResearchProvider define o contrato abstrato para provedores de pesquisa e inteligência artificial.
// O domínio e a esteira de monitoramento dependem exclusivamente desta interface.
type ResearchProvider interface {
	Discover(ctx context.Context, input DiscoverInput) (*DiscoverResult, error)
	Verify(ctx context.Context, input VerifyInput) (*VerifyResult, error)
}
