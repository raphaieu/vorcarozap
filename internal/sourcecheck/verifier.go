package sourcecheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

var (
	ErrTooManyRedirects = errors.New("limite de redirecionamentos excedido")
	ErrRedirectSSRF     = errors.New("redirecionamento bloqueado por política SSRF")
)

type redirectTrackerKey struct{}

// VerifierConfig define os parâmetros operacionais e limites do verificador de fontes.
type VerifierConfig struct {
	TotalTimeout            time.Duration
	DialTimeout             time.Duration
	ResponseHeaderTimeout   time.Duration
	MaxResponseHeaderBytes  int64
	MaxRedirects            int
	MaxBodyBytes            int64
	UserAgent               string
	Resolver                Resolver
	AllowLocalIPsForTesting bool
}

// DefaultVerifierConfig retorna a configuração conservadora padrão para produção.
func DefaultVerifierConfig() VerifierConfig {
	return VerifierConfig{
		TotalTimeout:            5 * time.Second,
		DialTimeout:             3 * time.Second,
		ResponseHeaderTimeout:   3 * time.Second,
		MaxResponseHeaderBytes:  32 * 1024, // 32 KB limite conservador de cabeçalhos
		MaxRedirects:            3,
		MaxBodyBytes:            64 * 1024, // 64 KB
		UserAgent:               "VorcaroZAP-SourceChecker/1.0 (+https://github.com/raphaieu/vorcarozap)",
		Resolver:                net.DefaultResolver,
		AllowLocalIPsForTesting: false,
	}
}

// Verifier executa verificações HTTP GET reais, seguras e com limites rigorosos.
type Verifier struct {
	cfg       VerifierConfig
	validator *SSRFValidator
	client    *http.Client
}

// NewVerifier inicializa o verificador com transporte seguro contra SSRF e DNS rebinding.
func NewVerifier(cfg VerifierConfig) *Verifier {
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = 5 * time.Second
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	if cfg.ResponseHeaderTimeout <= 0 {
		cfg.ResponseHeaderTimeout = 3 * time.Second
	}
	if cfg.MaxResponseHeaderBytes <= 0 {
		cfg.MaxResponseHeaderBytes = 32 * 1024
	}
	if cfg.MaxRedirects <= 0 {
		cfg.MaxRedirects = 3
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 64 * 1024
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "VorcaroZAP-SourceChecker/1.0 (+https://github.com/raphaieu/vorcarozap)"
	}
	if cfg.Resolver == nil {
		cfg.Resolver = net.DefaultResolver
	}

	validator := NewSSRFValidator(cfg.AllowLocalIPsForTesting)

	dialer := &net.Dialer{
		Timeout:   cfg.DialTimeout,
		KeepAlive: 0, // Conexões descartáveis para evitar reutilização associada a rebinding
	}

	transport := &http.Transport{
		// Impede que proxies herdados do ambiente (HTTP_PROXY / HTTPS_PROXY) contornem as regras de segurança
		Proxy: nil,
		// DialContext resolve o host e conecta diretamente ao IP validado, evitando DNS rebinding
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			validIPs, err := validator.ResolveAndValidateHost(ctx, cfg.Resolver, host)
			if err != nil {
				return nil, err
			}

			// Conecta ao primeiro IP validado
			targetAddr := net.JoinHostPort(validIPs[0].String(), port)
			return dialer.DialContext(ctx, network, targetAddr)
		},
		ResponseHeaderTimeout:  cfg.ResponseHeaderTimeout,
		TLSHandshakeTimeout:    cfg.DialTimeout,
		MaxResponseHeaderBytes: cfg.MaxResponseHeaderBytes,
		DisableKeepAlives:      true,
		MaxIdleConns:           0,
		// Preserva validação TLS completa e SNI usando os certificados do sistema
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	v := &Verifier{
		cfg:       cfg,
		validator: validator,
	}

	v.client = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Atualiza a contagem efetiva de redirecionamentos no contexto
			if tracker, ok := req.Context().Value(redirectTrackerKey{}).(*int); ok && tracker != nil {
				*tracker = len(via)
			}

			// via inclui a requisição inicial; bloqueia apenas quando ultrapassar cfg.MaxRedirects saltos
			if len(via) > cfg.MaxRedirects {
				return ErrTooManyRedirects
			}

			// Revalida a cada salto o novo destino contra SSRF e credenciais
			_, newHost, _, err := validator.ParseAndValidateURL(req.URL.String())
			if err != nil {
				return fmt.Errorf("%w: %v", ErrRedirectSSRF, err)
			}

			if _, err := validator.ResolveAndValidateHost(req.Context(), cfg.Resolver, newHost); err != nil {
				// Só classifica como violação de segurança SSRF se o IP/host for explicitamente bloqueado
				if errors.Is(err, ErrBlockedIP) || errors.Is(err, ErrBlockedHostname) {
					return fmt.Errorf("%w: %v", ErrRedirectSSRF, err)
				}
				// Preserva o erro original de DNS temporário, timeout ou cancelamento
				return err
			}

			// Remove Referer para evitar vazamento de parâmetros sensíveis entre domínios
			req.Header.Del("Referer")
			// Remove quaisquer cabeçalhos de autenticação ou cookies
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")

			return nil
		},
	}

	return v
}

// Check executa um HTTP GET seguro e limitado contra a URL informada.
func (v *Verifier) Check(ctx context.Context, rawURL string) CheckResult {
	start := time.Now()
	now := start.UTC()
	sanitizedURL := SanitizeURLForLogging(rawURL)

	res := CheckResult{
		URL:          rawURL,
		SanitizedURL: sanitizedURL,
		CheckedAt:    now,
	}

	// 1. Pré-validação sintática e de segurança da URL
	_, host, _, err := v.validator.ParseAndValidateURL(rawURL)
	if err != nil {
		res.Duration = time.Since(start)
		res.Status = domain.SourceAccessUnreachable
		res.ErrorMessage = err.Error()

		if errors.Is(err, ErrBlockedHostname) || errors.Is(err, ErrBlockedIP) {
			res.TechnicalReason = ReasonSSRFBlocked
			res.NormalizedErrorCode = "ssrf_blocked"
			res.IsSSRFBlocked = true
		} else {
			res.TechnicalReason = ReasonInvalidURL
			res.NormalizedErrorCode = "invalid_url"
		}
		return res
	}

	// 2. Timeout total delimitado e rastreador de saltos
	ctxReq, cancel := context.WithTimeout(ctx, v.cfg.TotalTimeout)
	defer cancel()

	redirectCount := new(int)
	ctxReq = context.WithValue(ctxReq, redirectTrackerKey{}, redirectCount)

	// 3. Montagem da requisição GET real (nunca HEAD)
	req, err := http.NewRequestWithContext(ctxReq, http.MethodGet, rawURL, nil)
	if err != nil {
		res.Duration = time.Since(start)
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonInvalidURL
		res.NormalizedErrorCode = "invalid_url"
		res.ErrorMessage = err.Error()
		return res
	}

	req.Header.Set("User-Agent", v.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")

	// 4. Execução da requisição
	resp, err := v.client.Do(req)
	res.RedirectCount = *redirectCount

	if err != nil {
		res.Duration = time.Since(start)
		res.ErrorMessage = err.Error()
		v.classifyError(err, host, &res)
		return res
	}
	defer resp.Body.Close()

	if resp.Request != nil && resp.Request.URL != nil {
		res.FinalURL = resp.Request.URL.String()
	}

	// 5. Leitura limitada do corpo sem persistência nem scraping
	// Usa LimitReader para não drenar respostas gigantescas ou bombas de descompressão
	bytesRead, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, v.cfg.MaxBodyBytes))
	res.Duration = time.Since(start)
	res.BytesRead = bytesRead
	res.HTTPStatus = resp.StatusCode

	if readErr != nil {
		res.ErrorMessage = fmt.Sprintf("falha na leitura do corpo: %v", readErr)
		v.classifyError(readErr, host, &res)
		return res
	}

	// 6. Classificação do status HTTP
	v.classifyHTTPStatus(resp.StatusCode, &res)
	return res
}

// classifyError analisa erros de rede/conexão/DNS/SSRF/TLS/timeout.
func (v *Verifier) classifyError(err error, host string, res *CheckResult) {
	// 1. Redirecionamento bloqueado
	if errors.Is(err, ErrRedirectSSRF) {
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonRedirectBlocked
		res.NormalizedErrorCode = "redirect_blocked"
		res.IsSSRFBlocked = true
		return
	}

	// 2. Limite de redirecionamentos excedido
	if errors.Is(err, ErrTooManyRedirects) {
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonTooManyRedirects
		res.NormalizedErrorCode = "too_many_redirects"
		res.IsInconclusive = true
		return
	}

	// 3. Cancelamento explícito de contexto
	if errors.Is(err, context.Canceled) {
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonCanceled
		res.NormalizedErrorCode = "canceled"
		res.IsInconclusive = true
		return
	}

	// 4. Timeout de contexto ou de rede
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "Timeout exceeded") {
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonTimeout
		res.NormalizedErrorCode = "timeout"
		res.IsInconclusive = true
		return
	}

	// 5. Erros de DNS
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			res.Status = domain.SourceAccessUnreachable
			res.TechnicalReason = ReasonDNSNXDomain
			res.NormalizedErrorCode = "dns_nxdomain"
			return
		}
		if dnsErr.IsTimeout || dnsErr.Temporary() {
			res.Status = domain.SourceAccessNotChecked
			res.TechnicalReason = ReasonDNSTemporary
			res.NormalizedErrorCode = "dns_temporary"
			res.IsInconclusive = true
			return
		}
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonDNSNXDomain
		res.NormalizedErrorCode = "dns_error"
		return
	}

	// 6. Bloqueio SSRF no dialer
	if errors.Is(err, ErrBlockedIP) || errors.Is(err, ErrBlockedHostname) || strings.Contains(err.Error(), "bloqueado por política de segurança SSRF") {
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonSSRFBlocked
		res.NormalizedErrorCode = "ssrf_blocked"
		res.IsSSRFBlocked = true
		return
	}

	// Erros de TLS
	var certErr x509.CertificateInvalidError
	var unknownAuthErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuthErr) || errors.As(err, &hostnameErr) ||
		strings.Contains(err.Error(), "certificate") || strings.Contains(err.Error(), "tls:") {
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonTLSError
		res.NormalizedErrorCode = "tls_error"
		return
	}

	// Erro genérico de conexão/reset/timeout de rede
	res.Status = domain.SourceAccessNotChecked
	res.TechnicalReason = ReasonNetworkError
	res.NormalizedErrorCode = "network_error"
	res.IsInconclusive = true
}

// classifyHTTPStatus classifica as respostas HTTP conforme o modelo editorial.
func (v *Verifier) classifyHTTPStatus(statusCode int, res *CheckResult) {
	switch {
	case statusCode >= 200 && statusCode < 300:
		// HTTP 2xx: verificado com sucesso
		res.Status = domain.SourceAccessReachable
		res.TechnicalReason = ReasonOK
		res.NormalizedErrorCode = ""

	case statusCode == http.StatusNotFound:
		// HTTP 404: erro comprovadamente definitivo
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonHTTP404
		res.NormalizedErrorCode = "http_404"

	case statusCode == http.StatusGone:
		// HTTP 410: erro comprovadamente definitivo
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonHTTP410
		res.NormalizedErrorCode = "http_410"

	case statusCode == http.StatusBadRequest:
		// HTTP 400: formato inválido definitivo
		res.Status = domain.SourceAccessUnreachable
		res.TechnicalReason = ReasonHTTP400
		res.NormalizedErrorCode = "http_400"

	case statusCode == http.StatusUnauthorized:
		// HTTP 401: paywall ou exigência de login -> inconclusivo
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonHTTP401
		res.NormalizedErrorCode = "http_401"
		res.IsInconclusive = true

	case statusCode == http.StatusForbidden:
		// HTTP 403: desafio de bot/Cloudflare ou paywall -> inconclusivo
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonHTTP403
		res.NormalizedErrorCode = "http_403"
		res.IsInconclusive = true

	case statusCode == http.StatusTooManyRequests:
		// HTTP 429: rate limit transitório -> inconclusivo
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonHTTP429
		res.NormalizedErrorCode = "http_429"
		res.IsInconclusive = true

	case statusCode >= 500 && statusCode < 600:
		// HTTP 5xx: falha do servidor temporária -> inconclusivo
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonHTTP5xx
		res.NormalizedErrorCode = fmt.Sprintf("http_%d", statusCode)
		res.IsInconclusive = true

	default:
		// Demais status 4xx ou inesperados
		res.Status = domain.SourceAccessNotChecked
		res.TechnicalReason = ReasonHTTP4xx
		res.NormalizedErrorCode = fmt.Sprintf("http_%d", statusCode)
		res.IsInconclusive = true
	}
}
