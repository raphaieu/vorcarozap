package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/research"
)

const (
	defaultBaseURL                  = "https://openrouter.ai/api/v1/chat/completions"
	defaultDiscoveryModel           = "openai/gpt-4.1-mini"
	defaultTimeout                  = 30 * time.Second
	defaultWebSearchEngine          = "auto"
	defaultWebSearchMaxResults      = 5
	defaultWebSearchMaxTotalResults = 15
	defaultWebSearchMaxUses         = 3
	defaultMaxToolCalls             = 5
	maxResponseBodyBytes            = 5 * 1024 * 1024 // 5MB limite de leitura de resposta
)

// ClientConfig define os parâmetros para inicialização do cliente OpenRouter.
type ClientConfig struct {
	APIKey                   string
	BaseURL                  string
	DiscoveryModel           string
	Timeout                  time.Duration
	WebSearchEngine          string
	WebSearchMaxResults      int
	WebSearchMaxTotalResults int
	WebSearchMaxUses         int
	MaxToolCalls             int
	HTTPClient               *http.Client
}

// Client implementa a interface research.ResearchProvider utilizando a API OpenRouter via net/http.
type Client struct {
	apiKey                   string
	baseURL                  string
	discoveryModel           string
	timeout                  time.Duration
	webSearchEngine          string
	webSearchMaxResults      int
	webSearchMaxTotalResults int
	webSearchMaxUses         int
	maxToolCalls             int
	httpClient               *http.Client
}

// NewClient cria e valida uma nova instância de Client.
func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	discoveryModel := cfg.DiscoveryModel
	if discoveryModel == "" {
		discoveryModel = defaultDiscoveryModel
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	engine := cfg.WebSearchEngine
	if engine == "" {
		engine = defaultWebSearchEngine
	}

	maxResults := cfg.WebSearchMaxResults
	if maxResults <= 0 {
		maxResults = defaultWebSearchMaxResults
	}

	maxTotalResults := cfg.WebSearchMaxTotalResults
	if maxTotalResults <= 0 {
		maxTotalResults = defaultWebSearchMaxTotalResults
	}

	maxUses := cfg.WebSearchMaxUses
	if maxUses <= 0 {
		maxUses = defaultWebSearchMaxUses
	}

	maxToolCalls := cfg.MaxToolCalls
	if maxToolCalls <= 0 {
		maxToolCalls = defaultMaxToolCalls
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	return &Client{
		apiKey:                   strings.TrimSpace(cfg.APIKey),
		baseURL:                  baseURL,
		discoveryModel:           discoveryModel,
		timeout:                  timeout,
		webSearchEngine:          engine,
		webSearchMaxResults:      maxResults,
		webSearchMaxTotalResults: maxTotalResults,
		webSearchMaxUses:         maxUses,
		maxToolCalls:             maxToolCalls,
		httpClient:               httpClient,
	}, nil
}

// Ensure Client implements research.ResearchProvider at compile time.
var _ research.ResearchProvider = (*Client)(nil)

// Discover executa uma pesquisa na web com Structured Outputs e a server tool openrouter:web_search.
func (c *Client) Discover(ctx context.Context, input research.DiscoverInput) (*research.DiscoverResult, error) {
	if c.apiKey == "" {
		return nil, research.ErrMissingAPIKey
	}

	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, research.ErrEmptyQuery
	}

	reqPayload := chatCompletionRequest{
		Model: c.discoveryModel,
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: SystemDiscoveryPrompt,
			},
			{
				Role:    "user",
				Content: buildUserDiscoveryPrompt(query),
			},
		},
		ResponseFormat: &responseFormatDefinition{
			Type: "json_schema",
			JSONSchema: &jsonSchemaPayload{
				Name:   "candidate_extraction",
				Strict: true,
				Schema: buildCandidatesJSONSchema(),
			},
		},
		Tools: []toolDefinition{
			{
				Type: "openrouter:web_search",
				Parameters: &webSearchParameters{
					Engine:          c.webSearchEngine,
					MaxResults:      c.webSearchMaxResults,
					MaxTotalResults: c.webSearchMaxTotalResults,
					MaxUses:         c.webSearchMaxUses,
				},
			},
		},
		MaxToolCalls: c.maxToolCalls,
		Stream:       false,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("openrouter: falha ao serializar payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("openrouter: falha ao criar requisição HTTP: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://vorcarozap.local")
	httpReq.Header.Set("X-OpenRouter-Title", "VorcaroZAP")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("openrouter: falha na comunicação HTTP: %w", sanitizeError(err, c.apiKey))
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("openrouter: falha ao ler corpo da resposta: %w", sanitizeError(err, c.apiKey))
	}
	if int64(len(bodyBytes)) > maxResponseBodyBytes {
		return nil, fmt.Errorf("%w: corpo da resposta excedeu 5 MiB", research.ErrResponseTooLarge)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErrResp struct {
			Error struct {
				Message string `json:"message"`
				Code    any    `json:"code"`
			} `json:"error"`
		}
		errMsg := ""
		if err := json.Unmarshal(bodyBytes, &apiErrResp); err == nil && apiErrResp.Error.Message != "" {
			errMsg = sanitizeBodySnippet(apiErrResp.Error.Message, c.apiKey)
		} else {
			errMsg = sanitizeBodySnippet(string(bodyBytes), c.apiKey)
		}
		return nil, fmt.Errorf("openrouter: status %d inesperado: %s", resp.StatusCode, errMsg)
	}

	var respPayload chatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &respPayload); err != nil {
		return nil, fmt.Errorf("%w: falha ao decodificar JSON: %v", research.ErrInvalidResponse, sanitizeError(err, c.apiKey))
	}

	if len(respPayload.Choices) == 0 {
		return nil, research.ErrEmptyResponse
	}

	firstChoice := respPayload.Choices[0]
	rawContent := strings.TrimSpace(firstChoice.Message.Content)

	citations := extractCitations(firstChoice.Message)

	if rawContent == "" {
		if len(citations) == 0 {
			return nil, research.ErrEmptyResponse
		}
		return nil, fmt.Errorf("%w: resposta do provedor contém apenas citações sem payload JSON de candidatos", research.ErrInvalidResponse)
	}

	// Decodifica e valida o schema estruturado de candidatos (fail closed)
	var candidatesWrapper struct {
		Candidates []research.CandidateExtraction `json:"candidates"`
	}

	if err := json.Unmarshal([]byte(rawContent), &candidatesWrapper); err != nil {
		return nil, fmt.Errorf("%w: resposta não adere ao schema JSON de candidatos: %v", research.ErrInvalidResponse, sanitizeError(err, c.apiKey))
	}
	if candidatesWrapper.Candidates == nil {
		return nil, fmt.Errorf("%w: campo 'candidates' ausente no JSON estruturado", research.ErrInvalidResponse)
	}
	for i, cand := range candidatesWrapper.Candidates {
		if err := cand.Validate(); err != nil {
			return nil, fmt.Errorf("%w: candidato[%d] inválido: %v", research.ErrInvalidCandidate, i, err)
		}
	}

	webSearchCalls := 0
	if respPayload.Usage.ServerToolUse != nil {
		webSearchCalls = respPayload.Usage.ServerToolUse.WebSearchRequests
	}

	modelUsed := respPayload.Model
	if modelUsed == "" {
		modelUsed = c.discoveryModel
	}

	return &research.DiscoverResult{
		Candidates:       candidatesWrapper.Candidates,
		Content:          rawContent,
		Model:            modelUsed,
		Citations:        citations,
		PromptTokens:     respPayload.Usage.PromptTokens,
		CompletionTokens: respPayload.Usage.CompletionTokens,
		TotalTokens:      respPayload.Usage.TotalTokens,
		WebSearchCalls:   webSearchCalls,
	}, nil
}

// Verify representa o gate semântico futuro (VZ-012). Não implementado nesta fase.
func (c *Client) Verify(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
	return nil, research.ErrNotImplemented
}

func buildCandidatesJSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entity_name": map[string]any{
							"type":        "string",
							"description": "Nome da pessoa ou organização mencionada",
						},
						"proposition": map[string]any{
							"type":        "string",
							"description": "Proposição factual atribuível documentada na fonte",
						},
						"suggested_grade": map[string]any{
							"type":        "string",
							"enum":        []string{"A", "B", "C", "D", "E"},
							"description": "Grau de força documental sugerido (A a E)",
						},
						"source_url": map[string]any{
							"type":        "string",
							"description": "URL pública e verificável da fonte",
						},
						"source_title": map[string]any{
							"type":        "string",
							"description": "Título da matéria ou documento",
						},
						"publisher_or_author": map[string]any{
							"type":        "string",
							"description": "Veículo publicador ou autor",
						},
						"published_at": map[string]any{
							"type":        "string",
							"description": "Data de publicação se disponível (formato YYYY-MM-DD ou textual)",
						},
						"excerpt": map[string]any{
							"type":        "string",
							"description": "Trecho comprobatório extraído da fonte",
						},
						"locator": map[string]any{
							"type":        "string",
							"description": "Localizador específico (página, parágrafo, seção) se houver",
						},
						"context_limits": map[string]any{
							"type":        "string",
							"description": "Limites de contexto, ressalvas ou escopo da informação",
						},
						"technical_confidence": map[string]any{
							"type":        "number",
							"description": "Confiança técnica da extração no intervalo de 0.0 a 1.0",
						},
					},
					"required": []string{
						"entity_name",
						"proposition",
						"suggested_grade",
						"source_url",
						"source_title",
						"publisher_or_author",
						"published_at",
						"excerpt",
						"locator",
						"context_limits",
						"technical_confidence",
					},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"candidates"},
		"additionalProperties": false,
	}
}

func extractCitations(msg respMessage) []research.Citation {
	var citations []research.Citation

	// 1. Extração preferencial via annotations (padrão OpenRouter para server tools)
	for _, ann := range msg.Annotations {
		if ann.Type == "url_citation" && ann.URLCitation != nil && ann.URLCitation.URL != "" {
			citations = append(citations, research.Citation{
				URL:        ann.URLCitation.URL,
				Title:      strings.TrimSpace(ann.URLCitation.Title),
				Excerpt:    strings.TrimSpace(ann.URLCitation.Content),
				StartIndex: ann.URLCitation.StartIndex,
				EndIndex:   ann.URLCitation.EndIndex,
			})
		}
	}

	// 2. Fallback para campo citations em outros formatos de provedor
	if len(citations) == 0 && len(msg.Citations) > 0 {
		for _, raw := range msg.Citations {
			switch c := raw.(type) {
			case string:
				u := strings.TrimSpace(c)
				if u != "" {
					citations = append(citations, research.Citation{URL: u})
				}
			case map[string]any:
				cit := research.Citation{}
				if u, ok := c["url"].(string); ok {
					cit.URL = strings.TrimSpace(u)
				}
				if t, ok := c["title"].(string); ok {
					cit.Title = strings.TrimSpace(t)
				}
				if e, ok := c["content"].(string); ok {
					cit.Excerpt = strings.TrimSpace(e)
				} else if e, ok := c["excerpt"].(string); ok {
					cit.Excerpt = strings.TrimSpace(e)
				}
				if cit.URL != "" {
					citations = append(citations, cit)
				}
			}
		}
	}

	return citations
}

func sanitizeError(err error, secret string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if secret != "" && strings.Contains(msg, secret) {
		msg = strings.ReplaceAll(msg, secret, "[REDACTED]")
	}
	return errors.New(msg)
}

func sanitizeBodySnippet(body string, secret string) string {
	snippet := strings.TrimSpace(body)
	if secret != "" && strings.Contains(snippet, secret) {
		snippet = strings.ReplaceAll(snippet, secret, "[REDACTED]")
	}
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}
	return snippet
}

// Tipos internos de serialização JSON para a API OpenRouter.

type chatCompletionRequest struct {
	Model          string                    `json:"model"`
	Messages       []chatMessage             `json:"messages"`
	ResponseFormat *responseFormatDefinition `json:"response_format,omitempty"`
	Tools          []toolDefinition          `json:"tools,omitempty"`
	MaxToolCalls   int                       `json:"max_tool_calls,omitempty"`
	Stream         bool                      `json:"stream"`
}

type responseFormatDefinition struct {
	Type       string             `json:"type"`
	JSONSchema *jsonSchemaPayload `json:"json_schema,omitempty"`
}

type jsonSchemaPayload struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type toolDefinition struct {
	Type       string               `json:"type"`
	Parameters *webSearchParameters `json:"parameters,omitempty"`
}

type webSearchParameters struct {
	Engine          string `json:"engine,omitempty"`
	MaxResults      int    `json:"max_results,omitempty"`
	MaxTotalResults int    `json:"max_total_results,omitempty"`
	MaxUses         int    `json:"max_uses,omitempty"`
}

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int         `json:"index"`
	Message      respMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type respMessage struct {
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	Annotations []annotation `json:"annotations,omitempty"`
	Citations   []any        `json:"citations,omitempty"`
}

type annotation struct {
	Type        string       `json:"type"`
	URLCitation *urlCitation `json:"url_citation,omitempty"`
}

type urlCitation struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	Content    string `json:"content,omitempty"`
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
}

type usage struct {
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	ServerToolUse    *serverToolUse `json:"server_tool_use,omitempty"`
}

type serverToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
}
