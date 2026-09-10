package sourcecheck_test

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/sourcecheck"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

type mockResolver struct {
	lookupFunc func(ctx context.Context, network, host string) ([]net.IP, error)
}

func (m *mockResolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	if m.lookupFunc != nil {
		return m.lookupFunc(ctx, network, host)
	}
	return nil, errors.New("lookupFunc não configurado")
}

func TestVerifier_HTTPMethodIsGET(t *testing.T) {
	receivedMethod := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	cfg := sourcecheck.DefaultVerifierConfig()
	cfg.AllowLocalIPsForTesting = true
	v := sourcecheck.NewVerifier(cfg)

	res := v.Check(context.Background(), ts.URL)
	if res.Status != domain.SourceAccessReachable {
		t.Fatalf("esperado reachable, obtido %s (erro: %s)", res.Status, res.ErrorMessage)
	}
	if receivedMethod != http.MethodGet {
		t.Fatalf("o verificador deve usar obrigatoriamente HTTP GET, obteve %q (nunca HEAD)", receivedMethod)
	}
}

func TestVerifier_ClassificationTableDriven(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		expectedStatus domain.SourceAccessStatus
		expectedReason sourcecheck.TechnicalReason
		expectedCode   string
		inconclusive   bool
	}{
		{
			name: "200 OK vira reachable",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: domain.SourceAccessReachable,
			expectedReason: sourcecheck.ReasonOK,
			expectedCode:   "",
			inconclusive:   false,
		},
		{
			name: "404 Not Found vira unreachable definitivo",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			expectedStatus: domain.SourceAccessUnreachable,
			expectedReason: sourcecheck.ReasonHTTP404,
			expectedCode:   "http_404",
			inconclusive:   false,
		},
		{
			name: "410 Gone vira unreachable definitivo",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusGone)
			},
			expectedStatus: domain.SourceAccessUnreachable,
			expectedReason: sourcecheck.ReasonHTTP410,
			expectedCode:   "http_410",
			inconclusive:   false,
		},
		{
			name: "400 Bad Request vira unreachable definitivo",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			expectedStatus: domain.SourceAccessUnreachable,
			expectedReason: sourcecheck.ReasonHTTP400,
			expectedCode:   "http_400",
			inconclusive:   false,
		},
		{
			name: "401 Unauthorized vira not_checked inconclusivo (paywall/login)",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			expectedStatus: domain.SourceAccessNotChecked,
			expectedReason: sourcecheck.ReasonHTTP401,
			expectedCode:   "http_401",
			inconclusive:   true,
		},
		{
			name: "403 Forbidden vira not_checked inconclusivo (anti-bot)",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			expectedStatus: domain.SourceAccessNotChecked,
			expectedReason: sourcecheck.ReasonHTTP403,
			expectedCode:   "http_403",
			inconclusive:   true,
		},
		{
			name: "429 Too Many Requests vira not_checked inconclusivo (rate limit)",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			expectedStatus: domain.SourceAccessNotChecked,
			expectedReason: sourcecheck.ReasonHTTP429,
			expectedCode:   "http_429",
			inconclusive:   true,
		},
		{
			name: "500 Internal Server Error vira not_checked inconclusivo (falha temporária)",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedStatus: domain.SourceAccessNotChecked,
			expectedReason: sourcecheck.ReasonHTTP5xx,
			expectedCode:   "http_500",
			inconclusive:   true,
		},
		{
			name: "503 Service Unavailable vira not_checked inconclusivo",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			expectedStatus: domain.SourceAccessNotChecked,
			expectedReason: sourcecheck.ReasonHTTP5xx,
			expectedCode:   "http_503",
			inconclusive:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(tt.handler)
			defer ts.Close()

			cfg := sourcecheck.DefaultVerifierConfig()
			cfg.AllowLocalIPsForTesting = true
			v := sourcecheck.NewVerifier(cfg)

			res := v.Check(context.Background(), ts.URL)
			if res.Status != tt.expectedStatus {
				t.Errorf("Status = %s; esperado %s", res.Status, tt.expectedStatus)
			}
			if res.TechnicalReason != tt.expectedReason {
				t.Errorf("TechnicalReason = %s; esperado %s", res.TechnicalReason, tt.expectedReason)
			}
			if res.NormalizedErrorCode != tt.expectedCode {
				t.Errorf("NormalizedErrorCode = %q; esperado %q", res.NormalizedErrorCode, tt.expectedCode)
			}
			if res.IsInconclusive != tt.inconclusive {
				t.Errorf("IsInconclusive = %v; esperado %v", res.IsInconclusive, tt.inconclusive)
			}
		})
	}
}

func TestVerifier_Redirects(t *testing.T) {
	t.Run("Exatamente 3 saltos permitidos com contagem correta e destino alcancado", func(t *testing.T) {
		var ts *httptest.Server
		ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, ts.URL+"/jump1", http.StatusFound)
			case "/jump1":
				http.Redirect(w, r, ts.URL+"/jump2", http.StatusFound)
			case "/jump2":
				http.Redirect(w, r, ts.URL+"/final", http.StatusFound)
			case "/final":
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("destino alcançado"))
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.MaxRedirects = 3
		cfg.AllowLocalIPsForTesting = true
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), ts.URL+"/start")
		if res.Status != domain.SourceAccessReachable {
			t.Fatalf("esperado reachable com 3 saltos, obtido %s: %s", res.Status, res.ErrorMessage)
		}
		if !strings.HasSuffix(res.FinalURL, "/final") {
			t.Errorf("FinalURL esperada /final, obtido %q", res.FinalURL)
		}
		if res.RedirectCount != 3 {
			t.Errorf("RedirectCount esperado 3, obtido %d", res.RedirectCount)
		}
	})

	t.Run("Quarto salto excede MaxRedirects=3 e vira not_checked inconclusivo", func(t *testing.T) {
		var ts *httptest.Server
		ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, ts.URL+"/jump1", http.StatusFound)
			case "/jump1":
				http.Redirect(w, r, ts.URL+"/jump2", http.StatusFound)
			case "/jump2":
				http.Redirect(w, r, ts.URL+"/jump3", http.StatusFound)
			case "/jump3":
				http.Redirect(w, r, ts.URL+"/final", http.StatusFound)
			case "/final":
				w.WriteHeader(http.StatusOK)
			default:
				http.NotFound(w, r)
			}
		}))
		defer ts.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.MaxRedirects = 3
		cfg.AllowLocalIPsForTesting = true
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), ts.URL+"/start")
		if res.Status != domain.SourceAccessNotChecked {
			t.Errorf("esperado not_checked ao tentar 4 saltos, obtido %s", res.Status)
		}
		if res.TechnicalReason != sourcecheck.ReasonTooManyRedirects {
			t.Errorf("esperado too_many_redirects, obtido %s", res.TechnicalReason)
		}
		if !res.IsInconclusive {
			t.Errorf("resultado com redirecionamentos excessivos deve ser inconclusivo")
		}
	})

	t.Run("Cabecalho Referer e credenciais sao removidos em redirecionamento entre hosts", func(t *testing.T) {
		receivedReferer := "NIL"
		receivedAuth := "NIL"

		serverDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedReferer = r.Header.Get("Referer")
			receivedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer serverDest.Close()

		serverOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, serverDest.URL+"/target", http.StatusFound)
		}))
		defer serverOrigin.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.AllowLocalIPsForTesting = true
		v := sourcecheck.NewVerifier(cfg)

		// URL inicial contém parâmetro de token sensível na query
		initialURL := serverOrigin.URL + "/source-doc?token=segredo-confidencial-123"
		res := v.Check(context.Background(), initialURL)
		if res.Status != domain.SourceAccessReachable {
			t.Fatalf("esperado reachable, obtido %s: %s", res.Status, res.ErrorMessage)
		}

		if receivedReferer != "" {
			t.Errorf("Referer deveria ter sido removido no salto para evitar vazamento de token, obteve %q", receivedReferer)
		}
		if receivedAuth != "" {
			t.Errorf("Authorization deveria ter sido removido no salto, obteve %q", receivedAuth)
		}
	})

	t.Run("Falha temporaria de DNS em redirecionamento nao vira bloqueio SSRF definitivo", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://dominio-dns-oscilando.com/noticia", http.StatusFound)
		}))
		defer ts.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.AllowLocalIPsForTesting = true
		cfg.Resolver = &mockResolver{
			lookupFunc: func(ctx context.Context, network, host string) ([]net.IP, error) {
				if host == "dominio-dns-oscilando.com" {
					return nil, &net.DNSError{
						Err:       "connection timed out",
						Name:      host,
						IsTimeout: true,
					}
				}
				// Para o servidor local inicial
				return net.DefaultResolver.LookupIP(ctx, network, host)
			},
		}
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), ts.URL+"/start")
		if res.Status != domain.SourceAccessNotChecked {
			t.Errorf("falha temporária de DNS em redirect deve ser not_checked, obtido %s", res.Status)
		}
		if res.TechnicalReason != sourcecheck.ReasonDNSTemporary {
			t.Errorf("TechnicalReason esperado dns_temporary, obtido %s", res.TechnicalReason)
		}
		if res.IsSSRFBlocked {
			t.Errorf("falha de DNS em redirect NÃO pode ser classificada como SSRFBlocked")
		}
	})

	t.Run("Redirecionamento para destino bloqueado por SSRF é interceptado", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Tenta redirecionar para endpoint de metadados da nuvem
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		}))
		defer ts.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.AllowLocalIPsForTesting = true
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), ts.URL+"/redirect-to-metadata")
		if res.Status != domain.SourceAccessUnreachable {
			t.Errorf("redirecionamento malicioso deve ser unreachable, obteve %s", res.Status)
		}
		if !res.IsSSRFBlocked {
			t.Errorf("esperado IsSSRFBlocked = true")
		}
	})
}

func TestVerifier_SSRFBlocking(t *testing.T) {
	cfg := sourcecheck.DefaultVerifierConfig()
	cfg.AllowLocalIPsForTesting = false // Validação estrita de produção
	v := sourcecheck.NewVerifier(cfg)

	blockedURLs := []struct {
		name string
		url  string
	}{
		{"loopback ipv4", "http://127.0.0.1:80/secret"},
		{"loopback ipv6", "http://[::1]:80/secret"},
		{"ipv4 mapped loopback", "http://[::ffff:127.0.0.1]:80/secret"},
		{"rede privada 10.x", "http://10.0.0.1/admin"},
		{"rede privada 172.16.x", "http://172.16.0.1/admin"},
		{"rede privada 192.168.x", "http://192.168.1.1/router"},
		{"cloud metadata ipv4", "http://169.254.169.254/latest/meta-data/"},
		{"google metadata domain", "http://metadata.google.internal/computeMetadata/v1/"},
		{"localhost domain", "http://localhost:8080/metrics"},
		{"internal domain", "http://service.internal/status"},
		{"credenciais embutidas", "http://user:password@public-domain.com/page"},
		{"esquema proibido file", "file:///etc/passwd"},
		{"esquema proibido ftp", "ftp://ftp.server.com/file"},
		{"porta proibida ssh", "http://public-domain.com:22/"},
		{"porta proibida redis", "http://public-domain.com:6379/"},
	}

	for _, tt := range blockedURLs {
		t.Run(tt.name, func(t *testing.T) {
			res := v.Check(context.Background(), tt.url)
			if res.Status != domain.SourceAccessUnreachable {
				t.Errorf("url %q deveria ser unreachable por segurança, obteve %s", tt.url, res.Status)
			}
			if res.TechnicalReason != sourcecheck.ReasonSSRFBlocked && res.TechnicalReason != sourcecheck.ReasonInvalidURL {
				t.Errorf("url %q motivo esperado SSRFBlocked ou InvalidURL, obteve %s", tt.url, res.TechnicalReason)
			}
		})
	}
}

func TestVerifier_DNSResolutionHandling(t *testing.T) {
	t.Run("DNS NXDOMAIN vira unreachable", func(t *testing.T) {
		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.Resolver = &mockResolver{
			lookupFunc: func(ctx context.Context, network, host string) ([]net.IP, error) {
				return nil, &net.DNSError{
					Err:        "no such host",
					Name:       host,
					IsNotFound: true,
				}
			},
		}
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), "http://dominio-inexistente-12345.com/noticia")
		if res.Status != domain.SourceAccessUnreachable {
			t.Errorf("esperado unreachable para NXDOMAIN, obtido %s", res.Status)
		}
		if res.TechnicalReason != sourcecheck.ReasonDNSNXDomain {
			t.Errorf("esperado dns_nxdomain, obtido %s", res.TechnicalReason)
		}
		if res.NormalizedErrorCode != "dns_nxdomain" {
			t.Errorf("esperado dns_nxdomain, obtido %q", res.NormalizedErrorCode)
		}
	})

	t.Run("DNS falha temporária vira not_checked inconclusivo", func(t *testing.T) {
		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.Resolver = &mockResolver{
			lookupFunc: func(ctx context.Context, network, host string) ([]net.IP, error) {
				return nil, &net.DNSError{
					Err:       "server failure",
					Name:      host,
					IsTimeout: true,
				}
			},
		}
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), "http://portal-noticias.com/artigo")
		if res.Status != domain.SourceAccessNotChecked {
			t.Errorf("esperado not_checked para falha temporária de DNS, obtido %s", res.Status)
		}
		if res.TechnicalReason != sourcecheck.ReasonDNSTemporary {
			t.Errorf("esperado dns_temporary, obtido %s", res.TechnicalReason)
		}
		if !res.IsInconclusive {
			t.Errorf("falha temporária de DNS deve ser inconclusiva")
		}
	})

	t.Run("DNS que resolve para IP privado é bloqueado", func(t *testing.T) {
		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.AllowLocalIPsForTesting = false
		cfg.Resolver = &mockResolver{
			lookupFunc: func(ctx context.Context, network, host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("192.168.1.50")}, nil
			},
		}
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), "http://dominio-rebinding.com/teste")
		if res.Status != domain.SourceAccessUnreachable {
			t.Errorf("esperado unreachable para domínio resolvendo em IP privado, obtido %s", res.Status)
		}
		if !res.IsSSRFBlocked {
			t.Errorf("esperado IsSSRFBlocked = true")
		}
	})

	t.Run("Mitigação de DNS Rebinding: intercepta resolução para IP proibido no DialContext", func(t *testing.T) {
		// Demonstra a proteção contra DNS Rebinding:
		// Mesmo que a URL seja sintaticamente permitida e o transporte tente discar,
		// o DialContext resolve e valida o IP antes de abrir a conexão TCP.
		// Se o resolver retornar um IP privado (ataque de rebinding), a conexão é abortada como SSRF.
		mockRes := &mockResolver{
			lookupFunc: func(ctx context.Context, network, host string) ([]net.IP, error) {
				// Simula retorno de IP de rede interna privada na etapa de conexão
				return []net.IP{net.ParseIP("10.0.0.5")}, nil
			},
		}

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.AllowLocalIPsForTesting = false
		cfg.Resolver = mockRes
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), "http://servico-legitimo.com/artigo")
		if res.Status != domain.SourceAccessUnreachable {
			t.Errorf("ataque de DNS rebinding para IP privado deve ser unreachable, obtido %s (erro: %s)", res.Status, res.ErrorMessage)
		}
		if !res.IsSSRFBlocked {
			t.Errorf("esperado IsSSRFBlocked = true quando o IP de conexão é privado")
		}
		if res.TechnicalReason != sourcecheck.ReasonSSRFBlocked {
			t.Errorf("TechnicalReason esperado ssrf_blocked, obtido %s", res.TechnicalReason)
		}
	})
}

func TestVerifier_TimeoutsAndCancellation(t *testing.T) {
	t.Run("Servidor lento atinge timeout e vira not_checked inconclusivo", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.TotalTimeout = 50 * time.Millisecond
		cfg.AllowLocalIPsForTesting = true
		v := sourcecheck.NewVerifier(cfg)

		res := v.Check(context.Background(), ts.URL)
		if res.Status != domain.SourceAccessNotChecked {
			t.Errorf("esperado not_checked em timeout, obtido %s", res.Status)
		}
		if res.TechnicalReason != sourcecheck.ReasonTimeout {
			t.Errorf("esperado timeout, obtido %s", res.TechnicalReason)
		}
		if !res.IsInconclusive {
			t.Errorf("timeout deve ser tratado como inconclusivo")
		}
	})

	t.Run("Cancelamento de contexto vira not_checked inconclusivo", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		cfg := sourcecheck.DefaultVerifierConfig()
		cfg.TotalTimeout = 2 * time.Second
		cfg.AllowLocalIPsForTesting = true
		v := sourcecheck.NewVerifier(cfg)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		res := v.Check(ctx, ts.URL)
		if res.Status != domain.SourceAccessNotChecked {
			t.Errorf("esperado not_checked em cancelamento, obtido %s", res.Status)
		}
		if res.TechnicalReason != sourcecheck.ReasonCanceled && res.TechnicalReason != sourcecheck.ReasonTimeout {
			t.Errorf("esperado canceled ou timeout, obtido %s", res.TechnicalReason)
		}
		if !res.IsInconclusive {
			t.Errorf("cancelamento deve ser tratado como inconclusivo")
		}
	})
}

func TestVerifier_BoundedBodyRead(t *testing.T) {
	// Servidor responde com payload grande (500 KB)
	largeData := strings.Repeat("A", 500*1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeData))
	}))
	defer ts.Close()

	cfg := sourcecheck.DefaultVerifierConfig()
	cfg.MaxBodyBytes = 16 * 1024 // Limite de 16 KB
	cfg.AllowLocalIPsForTesting = true
	v := sourcecheck.NewVerifier(cfg)

	res := v.Check(context.Background(), ts.URL)
	if res.Status != domain.SourceAccessReachable {
		t.Fatalf("esperado reachable mesmo para resposta grande, obtido %s", res.Status)
	}
	if res.HTTPStatus != http.StatusOK {
		t.Errorf("esperado status HTTP 200, obtido %d", res.HTTPStatus)
	}
	if res.BytesRead != 16*1024 {
		t.Errorf("BytesRead esperado 16384, obtido %d", res.BytesRead)
	}
}

func TestVerifier_BodyReadTimeoutAfter200(t *testing.T) {
	// Servidor responde imediatamente com 200 OK nos cabeçalhos, faz Flush, mas trava antes/durante a entrega do corpo
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Trava a conexão até o timeout estourar
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte("tarde demais"))
	}))
	defer ts.Close()

	cfg := sourcecheck.DefaultVerifierConfig()
	cfg.TotalTimeout = 100 * time.Millisecond
	cfg.AllowLocalIPsForTesting = true
	v := sourcecheck.NewVerifier(cfg)

	res := v.Check(context.Background(), ts.URL)

	// O timeout durante o corpo DEVE transformar o resultado em not_checked inconclusivo, NUNCA em reachable!
	if res.Status != domain.SourceAccessNotChecked {
		t.Fatalf("timeout durante o corpo DEVE ser not_checked, obtido %s (HTTPStatus=%d)", res.Status, res.HTTPStatus)
	}
	if res.TechnicalReason != sourcecheck.ReasonTimeout {
		t.Errorf("TechnicalReason esperado timeout, obtido %s", res.TechnicalReason)
	}
	if !res.IsInconclusive {
		t.Errorf("resultado com timeout de corpo deve ser marcado como IsInconclusive = true")
	}
	if res.HTTPStatus != http.StatusOK {
		t.Errorf("HTTPStatus deve preservar o status 200 recebido antes da falha, obtido %d", res.HTTPStatus)
	}
	if res.Duration < 80*time.Millisecond {
		t.Errorf("Duration (%v) deve incluir o tempo gasto na leitura do corpo", res.Duration)
	}
}

func TestSanitizeURLForLogging(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "url simples sem params",
			input:    "https://g1.globo.com/politica/noticia.ghtml",
			expected: "https://g1.globo.com/politica/noticia.ghtml",
		},
		{
			name:     "remove credenciais e fragmento",
			input:    "http://admin:secret@site.com/doc#secao1",
			expected: "http://site.com/doc",
		},
		{
			name:     "mascara token e apikey na query",
			input:    "https://api.site.com/v1/data?token=abc12345&apiKey=xyz999&categoria=politica",
			expected: "https://api.site.com/v1/data?apiKey=%5Bredacted%5D&categoria=politica&token=%5Bredacted%5D",
		},
		{
			name:     "mascara auth e secret",
			input:    "https://portal.com/page?auth_key=supersecret&safe=true",
			expected: "https://portal.com/page?auth_key=%5Bredacted%5D&safe=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourcecheck.SanitizeURLForLogging(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeURLForLogging(%q) = %q; esperado %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCheckAndPersist_Integration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("falha ao abrir sqlite em memória: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao rodar migrations: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("artigo publicado"))
	}))
	defer ts.Close()

	sourceID := uuid.NewString()
	q := sqlc.New(db)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = q.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 sourceID,
		Title:              "Reportagem Original",
		PublisherOrAuthor:  "Jornal Teste",
		OriginalUrl:        ts.URL,
		CanonicalUrl:       ts.URL,
		SourceType:         "article",
		SourceAccessStatus: string(domain.SourceAccessNotChecked),
		CreatedAt:          nowStr,
		UpdatedAt:          nowStr,
	})
	if err != nil {
		t.Fatalf("falha ao criar source no banco: %v", err)
	}

	cfg := sourcecheck.DefaultVerifierConfig()
	cfg.AllowLocalIPsForTesting = true
	v := sourcecheck.NewVerifier(cfg)

	// Executa CheckAndPersist
	res, err := sourcecheck.CheckAndPersist(ctx, v, db, sourceID, ts.URL, domain.SourceAccessNotChecked)
	if err != nil {
		t.Fatalf("CheckAndPersist falhou: %v", err)
	}

	if res.Status != domain.SourceAccessReachable {
		t.Fatalf("esperado status reachable, obtido %s", res.Status)
	}

	// Verifica persistência no SQLite
	src, err := q.GetSourceByID(ctx, sourceID)
	if err != nil {
		t.Fatalf("falha ao buscar source pós-verificação: %v", err)
	}

	if src.SourceAccessStatus != string(domain.SourceAccessReachable) {
		t.Errorf("banco: esperado source_access_status = %q, obteve %q", domain.SourceAccessReachable, src.SourceAccessStatus)
	}
	if !src.SourceAccessCheckedAt.Valid || src.SourceAccessCheckedAt.String == "" {
		t.Errorf("banco: source_access_checked_at não foi gravado")
	}
	if !src.HttpStatus.Valid || src.HttpStatus.Int64 != 200 {
		t.Errorf("banco: esperado http_status = 200, obteve %v", src.HttpStatus)
	}
}
