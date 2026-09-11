package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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
	Query       string `json:"query"`
	WindowStart string `json:"window_start,omitempty"`
	WindowEnd   string `json:"window_end,omitempty"`
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
	Cost             float64               `json:"cost"`
	CostMicros       int64                 `json:"cost_micros"`
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
	Model                    string   `json:"model,omitempty"`
	PromptTokens             int      `json:"prompt_tokens,omitempty"`
	CompletionTokens         int      `json:"completion_tokens,omitempty"`
	TotalTokens              int      `json:"total_tokens,omitempty"`
	Cost                     float64  `json:"cost,omitempty"`
	CostMicros               int64    `json:"cost_micros,omitempty"`
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

// MicroUSDMultiplier define o multiplicador para conversão de USD para micro-USD ($1.00 USD = 1.000.000 micro-USD).
const MicroUSDMultiplier = 1000000.0

// ParseCostJSONToMicroUSD analisa defensivamente o campo "usage.cost" do payload JSON do OpenRouter.
// Rejeita custo ausente, null, string, negativo, NaN, infinito, formato inválido e overflow (fail-closed).
// Custo numérico 0 ou 0.0 explícito é aceito e retorna 0.
// Qualquer fração positiva é arredondada estritamente para cima em microdólares inteiros (µUSD)
// através de aritmética racional exata, sem operações de ponto flutuante float64.
func ParseCostJSONToMicroUSD(raw json.RawMessage) (int64, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return 0, fmt.Errorf("%w: campo 'usage.cost' ausente no payload", ErrInvalidResponse)
	}
	if trimmed == "null" {
		return 0, fmt.Errorf("%w: campo 'usage.cost' não pode ser null", ErrInvalidResponse)
	}
	if strings.HasPrefix(trimmed, "\"") {
		return 0, fmt.Errorf("%w: campo 'usage.cost' deve ser numérico, recebido string %s", ErrInvalidResponse, trimmed)
	}
	return ParseDecimalCostStringToMicroUSD(trimmed)
}

// ParseDecimalCostStringToMicroUSD converte exatamente uma representação textual decimal de custo em USD
// para microdólares inteiros (µUSD) aplicando arredondamento estrito para cima (ceiling) em frações submicro,
// sem passar por float64 ou aritmética de ponto flutuante imprecisa.
func ParseDecimalCostStringToMicroUSD(s string) (int64, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("%w: valor decimal de custo vazio", ErrInvalidResponse)
	}

	var r big.Rat
	if _, ok := r.SetString(trimmed); !ok {
		return 0, fmt.Errorf("%w: formato de custo decimal inválido %q", ErrInvalidResponse, s)
	}

	if r.Sign() < 0 {
		return 0, fmt.Errorf("%w: custo não pode ser negativo (%s)", ErrInvalidResponse, s)
	}
	if r.Sign() == 0 {
		return 0, nil
	}

	// Multiplica por 1.000.000 (1 USD = 1.000.000 micro-USD)
	microRat := new(big.Rat).Mul(&r, big.NewRat(1000000, 1))
	num := microRat.Num()
	denom := microRat.Denom()

	quot := new(big.Int).Quo(num, denom)
	rem := new(big.Int).Rem(num, denom)

	// Arredondamento estrito para cima se houver resto fracionário positivo
	if rem.Sign() > 0 {
		quot.Add(quot, big.NewInt(1))
	}

	if !quot.IsInt64() {
		return 0, fmt.Errorf("%w: custo excede o limite máximo representável (%s)", ErrInvalidResponse, s)
	}

	return quot.Int64(), nil
}

// ParseCostToMicroUSD converte defensivamente um custo decimal em USD para microdólares inteiros (micro-USD).
// Mantido para compatibilidade e testes locais, utilizando a conversão racional exata.
func ParseCostToMicroUSD(costUSD float64) (int64, error) {
	if costUSD < 0 {
		return 0, fmt.Errorf("%w: custo de LLM não pode ser negativo (%v)", ErrInvalidResponse, costUSD)
	}
	if costUSD == 0 {
		return 0, nil
	}
	// Formata com alta precisão para converter via big.Rat sem imprecisão
	str := fmt.Sprintf("%.12f", costUSD)
	return ParseDecimalCostStringToMicroUSD(str)
}

// MicroUSDToFloat converte o valor em micro-USD de volta para float64 em USD.
func MicroUSDToFloat(micros int64) float64 {
	if micros <= 0 {
		return 0.0
	}
	return float64(micros) / MicroUSDMultiplier
}
