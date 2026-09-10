package sourcecheck

import (
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

// TechnicalReason representa o motivo técnico normalizado para a classificação do resultado.
type TechnicalReason string

const (
	ReasonOK               TechnicalReason = "ok"
	ReasonHTTP404          TechnicalReason = "http_404"
	ReasonHTTP410          TechnicalReason = "http_410"
	ReasonHTTP400          TechnicalReason = "http_400"
	ReasonHTTP401          TechnicalReason = "http_401"
	ReasonHTTP403          TechnicalReason = "http_403"
	ReasonHTTP429          TechnicalReason = "http_429"
	ReasonHTTP5xx          TechnicalReason = "http_5xx"
	ReasonHTTP4xx          TechnicalReason = "http_4xx"
	ReasonTimeout          TechnicalReason = "timeout"
	ReasonCanceled         TechnicalReason = "canceled"
	ReasonNetworkError     TechnicalReason = "network_error"
	ReasonDNSNXDomain      TechnicalReason = "dns_nxdomain"
	ReasonDNSTemporary     TechnicalReason = "dns_temporary"
	ReasonTLSError         TechnicalReason = "tls_error"
	ReasonSSRFBlocked      TechnicalReason = "ssrf_blocked"
	ReasonInvalidURL       TechnicalReason = "invalid_url"
	ReasonRedirectBlocked  TechnicalReason = "redirect_blocked"
	ReasonTooManyRedirects TechnicalReason = "too_many_redirects"
)

// CheckResult contém o resultado estruturado de uma verificação GET segura e limitada.
type CheckResult struct {
	// URL solicitada
	URL string `json:"url"`
	// SanitizedURL é a URL sem parâmetros sensíveis para registro seguro em log
	SanitizedURL string `json:"sanitized_url"`
	// Status de acessibilidade editorial conforme domain.SourceAccessStatus
	Status domain.SourceAccessStatus `json:"status"`
	// TechnicalReason especifica a causa técnica detalhada do status
	TechnicalReason TechnicalReason `json:"technical_reason"`
	// HTTPStatus é o código de status HTTP retornado (0 se falha de conexão/DNS/SSRF)
	HTTPStatus int `json:"http_status,omitempty"`
	// NormalizedErrorCode é o código textual normalizado para persistência em sources
	NormalizedErrorCode string `json:"normalized_error_code,omitempty"`
	// CheckedAt é o timestamp UTC em que a verificação foi executada
	CheckedAt time.Time `json:"checked_at"`
	// Duration é o tempo total gasto na requisição
	Duration time.Duration `json:"duration"`
	// FinalURL é a URL final atingida após eventuais redirecionamentos permitidos
	FinalURL string `json:"final_url,omitempty"`
	// RedirectCount é o total de redirecionamentos seguidos com sucesso
	RedirectCount int `json:"redirect_count"`
	// IsSSRFBlocked indica se a tentativa foi abortada por violação de segurança de rede
	IsSSRFBlocked bool `json:"is_ssrf_blocked"`
	// IsInconclusive indica se o resultado é transitório ou incompleto (não autoriza publicação nem comprova indisponibilidade)
	IsInconclusive bool `json:"is_inconclusive"`
	// ErrorMessage armazena mensagem amigável de erro em caso de falha
	ErrorMessage string `json:"error_message,omitempty"`
}
