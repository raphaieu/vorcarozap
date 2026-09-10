package sourcecheck

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrInvalidScheme       = errors.New("esquema inválido: apenas http e https são permitidos")
	ErrEmbeddedCredentials = errors.New("credenciais embutidas na URL são proibidas")
	ErrEmptyHost           = errors.New("hostname da URL não pode ser vazio")
	ErrDisallowedPort      = errors.New("porta não permitida para verificação pública")
	ErrBlockedHostname     = errors.New("hostname bloqueado por política de segurança SSRF")
	ErrBlockedIP           = errors.New("endereço IP bloqueado por política de segurança SSRF")
	ErrNoValidIPResolved   = errors.New("nenhum endereço IP público válido resolvido")
)

// Resolver define a interface para resolução de nomes DNS.
type Resolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// SSRFValidator aplica regras rígidas de segurança contra SSRF e acessos indevidos à rede interna.
type SSRFValidator struct {
	AllowedPorts            map[int]bool
	AllowLocalIPsForTesting bool
	blockedPrefixes         []netip.Prefix
}

// NewSSRFValidator inicializa o validador com as listas canônicas de redes não públicas e portas permitidas.
func NewSSRFValidator(allowLocalIPsForTesting bool) *SSRFValidator {
	v := &SSRFValidator{
		AllowedPorts: map[int]bool{
			80:   true,
			443:  true,
			8080: true,
			8443: true,
		},
		AllowLocalIPsForTesting: allowLocalIPsForTesting,
	}

	// Faixas de IP reservadas, privadas, especiais e metadados de nuvem
	cidrs := []string{
		// IPv4
		"0.0.0.0/8",          // Current network (RFC 1122)
		"10.0.0.0/8",         // Private network (RFC 1918)
		"100.64.0.0/10",      // Shared Address Space / CGNAT (RFC 6598)
		"127.0.0.0/8",        // Loopback (RFC 1122)
		"169.254.0.0/16",     // Link-Local / Cloud Metadata (RFC 3927)
		"172.16.0.0/12",      // Private network (RFC 1918)
		"192.0.0.0/24",       // IETF Protocol Assignments (RFC 6890)
		"192.0.2.0/24",       // TEST-NET-1 (RFC 5737)
		"192.88.99.0/24",     // 6to4 Relay Anycast (RFC 7526)
		"192.168.0.0/16",     // Private network (RFC 1918)
		"198.18.0.0/15",      // Benchmarking (RFC 2544)
		"198.51.100.0/24",    // TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",     // TEST-NET-3 (RFC 5737)
		"224.0.0.0/4",        // Multicast (RFC 5771)
		"240.0.0.0/4",        // Reserved for Future Use (RFC 1112)
		"255.255.255.255/32", // Limited Broadcast (RFC 919)

		// IPv6
		"::/128",        // Unspecified
		"::1/128",       // Loopback
		"64:ff9b::/96",  // IPv4-IPv6 Translation (RFC 6052)
		"100::/64",      // Discard prefix (RFC 6666)
		"2001:db8::/32", // Documentation (RFC 3849)
		"2002::/16",     // 6to4 (RFC 3056)
		"fc00::/7",      // Unique Local Address (ULA, RFC 4193)
		"fe80::/10",     // Link-Local Unicast
		"ff00::/8",      // Multicast
	}

	for _, cidr := range cidrs {
		if prefix, err := netip.ParsePrefix(cidr); err == nil {
			v.blockedPrefixes = append(v.blockedPrefixes, prefix)
		}
	}

	return v
}

// ParseAndValidateURL valida o formato, esquema, credenciais, porta e hostname da URL.
func (v *SSRFValidator) ParseAndValidateURL(rawURL string) (*url.URL, string, int, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, "", 0, errors.New("url vazia")
	}

	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, "", 0, fmt.Errorf("formato de url inválido: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, "", 0, ErrInvalidScheme
	}

	if u.User != nil {
		return nil, "", 0, ErrEmbeddedCredentials
	}

	hostPart := u.Host
	if hostPart == "" {
		return nil, "", 0, ErrEmptyHost
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, "", 0, ErrEmptyHost
	}

	portStr := u.Port()
	var port int
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			return nil, "", 0, ErrDisallowedPort
		}
		port = p
	} else {
		if scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}

	if !v.AllowLocalIPsForTesting && !v.AllowedPorts[port] {
		return nil, "", 0, fmt.Errorf("%w: porta %d", ErrDisallowedPort, port)
	}

	if err := v.validateHostname(hostname); err != nil {
		return nil, "", 0, err
	}

	return u, hostname, port, nil
}

// validateHostname bloqueia domínios internos e nomes de metadados conhecidos.
func (v *SSRFValidator) validateHostname(hostname string) error {
	h := strings.ToLower(strings.TrimSuffix(hostname, "."))

	if !v.AllowLocalIPsForTesting {
		if h == "localhost" || strings.HasSuffix(h, ".localhost") {
			return fmt.Errorf("%w: %s", ErrBlockedHostname, hostname)
		}
		if strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".lan") || strings.HasSuffix(h, ".arpa") {
			return fmt.Errorf("%w: %s", ErrBlockedHostname, hostname)
		}
		if h == "metadata.google.internal" || h == "metadata" || h == "instance-data" {
			return fmt.Errorf("%w: endpoint de metadados %s", ErrBlockedHostname, hostname)
		}
	}

	return nil
}

// ValidateIP verifica se um endereço IP é público e seguro.
func (v *SSRFValidator) ValidateIP(ip net.IP) error {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok || !addr.IsValid() {
		return ErrBlockedIP
	}

	// Normaliza IPv4-mapped IPv6 (ex: ::ffff:127.0.0.1 -> 127.0.0.1)
	addr = addr.Unmap()

	if v.AllowLocalIPsForTesting && addr.IsLoopback() {
		return nil
	}

	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return fmt.Errorf("%w: %s", ErrBlockedIP, addr)
	}

	for _, prefix := range v.blockedPrefixes {
		if prefix.Contains(addr) {
			return fmt.Errorf("%w: %s pertence à faixa reservada %s", ErrBlockedIP, addr, prefix)
		}
	}

	return nil
}

// ResolveAndValidateHost resolve o hostname e garante que todos os IPs pertençam a destinos públicos permitidos.
// Retorna a lista de IPs validados.
func (v *SSRFValidator) ResolveAndValidateHost(ctx context.Context, resolver Resolver, host string) ([]net.IP, error) {
	// Se host já for um IP literal
	if ip := net.ParseIP(host); ip != nil {
		if err := v.ValidateIP(ip); err != nil {
			return nil, err
		}
		return []net.IP{ip}, nil
	}

	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}

	if len(ips) == 0 {
		return nil, ErrNoValidIPResolved
	}

	var validIPs []net.IP
	for _, ip := range ips {
		if err := v.ValidateIP(ip); err != nil {
			// Fail-closed: se qualquer IP resolvido for privado/proibido, rejeita imediatamente
			return nil, fmt.Errorf("ip resolvido inválido (%s): %w", ip.String(), err)
		}
		validIPs = append(validIPs, ip)
	}

	return validIPs, nil
}

// SanitizeURLForLogging limpa credenciais e mascara parâmetros potencialmente sensíveis na query string.
func SanitizeURLForLogging(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[url-invalida]"
	}

	// Remove credenciais e fragmento
	u.User = nil
	u.Fragment = ""

	q := u.Query()
	if len(q) > 0 {
		for key := range q {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "token") || strings.Contains(lowerKey, "key") ||
				strings.Contains(lowerKey, "auth") || strings.Contains(lowerKey, "secret") ||
				strings.Contains(lowerKey, "pass") || strings.Contains(lowerKey, "sig") {
				q.Set(key, "[redacted]")
			}
		}
		u.RawQuery = q.Encode()
	}

	return u.String()
}
