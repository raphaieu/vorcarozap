package research

import (
	"context"
	"errors"
)

// Erros sentinela do pacote research.
var (
	ErrMissingAPIKey    = errors.New("research: chave de API não configurada")
	ErrNotImplemented   = errors.New("research: funcionalidade não implementada nesta fase")
	ErrEmptyQuery       = errors.New("research: consulta de descoberta vazia")
	ErrEmptyResponse    = errors.New("research: resposta do provedor vazia ou sem escolhas válidas")
	ErrInvalidResponse  = errors.New("research: resposta do provedor inválida ou corrompida")
	ErrResponseTooLarge = errors.New("research: resposta do provedor excedeu o limite máximo de tamanho")
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

// DiscoverResult encapsula a resposta técnica da etapa de descoberta.
// Não cria claims, entidades ou fontes diretamente (responsabilidade de fases posteriores).
type DiscoverResult struct {
	Content          string     `json:"content"`
	Model            string     `json:"model"`
	Citations        []Citation `json:"citations"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	TotalTokens      int        `json:"total_tokens"`
	WebSearchCalls   int        `json:"web_search_calls"`
}

// VerifyInput define a entrada para a verificação semântica de compatibilidade documental (gate semântico).
type VerifyInput struct {
	EntityName string `json:"entity_name"`
	ClaimText  string `json:"claim_text"`
	SourceText string `json:"source_text"`
	Grade      string `json:"grade"`
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

// ResearchProvider define o contrato abstrato para provedores de pesquisa e inteligência artificial.
// O domínio e a esteira de monitoramento dependem exclusivamente desta interface.
type ResearchProvider interface {
	Discover(ctx context.Context, input DiscoverInput) (*DiscoverResult, error)
	Verify(ctx context.Context, input VerifyInput) (*VerifyResult, error)
}
