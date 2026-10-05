package normalize

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"vazio", "", ""},
		{"apenas espaços", "    ", ""},
		{"espaços laterais", "  Daniel Vorcaro  ", "Daniel Vorcaro"},
		{"múltiplos espaços internos", "Daniel   \t  Vorcaro", "Daniel Vorcaro"},
		{"quebras de linha e tabs", "Nome\n\tcom   espaços\r\nvariados", "Nome com espaços variados"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := String(tt.input)
			if got != tt.expected {
				t.Errorf("String(%q) = %q; esperado %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUnicode(t *testing.T) {
	input := "João da Silva — Diretor de Operações"
	got := Unicode(input)
	if got != input {
		t.Errorf("Unicode(%q) = %q; esperado %q", input, got, input)
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  DANIEL VORCARO  ", "daniel vorcaro"},
		{"Álvaro   Dias", "álvaro dias"},
		{"Érico   Veríssimo", "érico veríssimo"},
		{"Banco   Master   S.A.", "banco master s.a."},
	}

	for _, tt := range tests {
		got := Name(tt.input)
		if got != tt.expected {
			t.Errorf("Name(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}
}

func TestSlugAndDisambiguation(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Daniel Vorcaro", "daniel-vorcaro"},
		{"João da Silva, Jr.", "joao-da-silva-jr"},
		{"Álvaro Dias & Cia.", "alvaro-dias-cia"},
		{"---Nome---Com---Traços---", "nome-com-tracos"},
		{"", "item"},
	}

	for _, tt := range tests {
		got := Slug(tt.input)
		if got != tt.expected {
			t.Errorf("Slug(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}

	base := Slug("Daniel Vorcaro")
	if got := DisambiguateSlug(base, 1); got != "daniel-vorcaro" {
		t.Errorf("DisambiguateSlug(1) = %q; esperado daniel-vorcaro", got)
	}
	if got := DisambiguateSlug(base, 2); got != "daniel-vorcaro-2" {
		t.Errorf("DisambiguateSlug(2) = %q; esperado daniel-vorcaro-2", got)
	}
	if got := DisambiguateSlug(base, 3); got != "daniel-vorcaro-3" {
		t.Errorf("DisambiguateSlug(3) = %q; esperado daniel-vorcaro-3", got)
	}
}

func TestCanonicalURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "URL vazia",
			input:       "",
			expectError: true,
		},
		{
			name:        "Apenas espaços",
			input:       "   ",
			expectError: true,
		},
		{
			name:        "Esquema inválido (ftp)",
			input:       "ftp://example.com/file",
			expectError: true,
		},
		{
			name:        "Sem host",
			input:       "https:///path",
			expectError: true,
		},
		{
			name:     "HTTP padrão com lowercase no host",
			input:    "http://Example.COM:80/noticia",
			expected: "http://example.com/noticia",
		},
		{
			name:     "HTTPS padrão com remoção de porta 443 e fragmento",
			input:    "https://Noticias.UOL.com.br:443/politica/artigo.html#comentarios",
			expected: "https://noticias.uol.com.br/politica/artigo.html",
		},
		{
			name:     "Preservação de query string legítima",
			input:    "https://g1.globo.com/busca/?q=vorcaro&order=recent",
			expected: "https://g1.globo.com/busca/?q=vorcaro&order=recent",
		},
		{
			name:     "Porta não-padrão preservada",
			input:    "https://api.example.com:8443/data",
			expected: "https://api.example.com:8443/data",
		},
		{
			name:        "Rejeição de credenciais embutidas (user:pass)",
			input:       "https://usuario:senha123@noticias.exemplo.com/materia",
			expectError: true,
		},
		{
			name:        "Rejeição de credenciais apenas com usuário",
			input:       "https://admin@noticias.exemplo.com/materia",
			expectError: true,
		},
		{
			name:        "Porta inválida com potencial segredo",
			input:       "https://exemplo.com:SEGREDO/documento",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalURL(tt.input)
			if tt.expectError {
				if err == nil {
					t.Fatalf("esperava erro para %q, obteve sucesso %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado para %q: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("CanonicalURL(%q) = %q; esperado %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsSensitiveQueryParamName(t *testing.T) {
	tests := []struct {
		paramName string
		sensitive bool
	}{
		// Legítimos
		{"id", false},
		{"page", false},
		{"q", false},
		{"search", false},
		{"category", false},
		{"sort", false},
		{"dir", false},
		{"order", false},
		{"ref", false},
		{"utm_source", false},
		{"utm_medium", false},
		{"tab", false},
		{"year", false},
		{"month", false},
		{"view", false},
		{"format", false},

		// Tokens & Auth
		{"token", true},
		{"TOKEN", true},
		{"Token", true},
		{"session", true},
		{"SESSION", true},
		{"Session", true},
		{"session_token", true},
		{"session-token", true},
		{"session_id", true},
		{"sessionid", true},
		{"api_token", true},
		{"api-token", true},
		{"apitoken", true},
		{"bearer", true},
		{"bearer_token", true},
		{"access_token", true},
		{"access-token", true},
		{"accessToken", true},
		{"auth_token", true},
		{"authtoken", true},
		{"id_token", true},
		{"refresh_token", true},
		{"jwt", true},
		{"JWT", true},
		{"auth", true},
		{"authorization", true},
		{"code", true},
		{"assertion", true},
		{"passcode", true},
		{"otp", true},
		{"cf_token", true},
		{"token_hash", true},
		{"secure_token", true},
		{"user_token", true},
		{"download_token", true},
		{"user_session", true},

		// Passwords & Secrets
		{"password", true},
		{"PASSWORD", true},
		{"pass", true},
		{"passwd", true},
		{"pwd", true},
		{"secret", true},
		{"SECRET", true},
		{"api_secret", true},
		{"client_secret", true},
		{"client-secret", true},
		{"shared_secret", true},
		{"credential", true},
		{"credentials", true},
		{"admin_password", true},
		{"master_secret", true},

		// Keys
		{"key", true},
		{"KEY", true},
		{"apikey", true},
		{"apiKey", true},
		{"api_key", true},
		{"API_KEY", true},
		{"api-key", true},
		{"access_key", true},
		{"access-key", true},
		{"secret_key", true},
		{"app_key", true},
		{"private_key", true},
		{"custom_apikey", true},
		{"service_api_key", true},

		// Signatures & Cloud Signed URLs
		{"signature", true},
		{"SIGNATURE", true},
		{"sig", true},
		{"SIG", true},
		{"sign", true},
		{"signed_url", true},
		{"hmac", true},
		{"HMAC", true},
		{"x-amz-signature", true},
		{"X-Amz-Signature", true},
		{"x_amz_signature", true},
		{"X-Amz-Credential", true},
		{"x-amz-security-token", true},
		{"x-amz-algorithm", true},
		{"x-amz-date", true},
		{"x-goog-signature", true},
		{"X-Goog-Signature", true},
		{"x-goog-credential", true},
		{"x-goog-security-token", true},
		{"x-goog-algorithm", true},
		{"request_signature", true},
		{"url_sig", true},
	}

	for _, tt := range tests {
		t.Run(tt.paramName, func(t *testing.T) {
			got := IsSensitiveQueryParamName(tt.paramName)
			if got != tt.sensitive {
				t.Errorf("IsSensitiveQueryParamName(%q) = %v; esperado %v", tt.paramName, got, tt.sensitive)
			}
		})
	}
}

func TestSafeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// URLs Válidas sem parâmetros sensíveis
		{"URL simples válida", "https://example.com/noticia", "https://example.com/noticia"},
		{"URL com query legítima id e page", "https://example.com/noticia?id=123&page=2", "https://example.com/noticia?id=123&page=2"},
		{"URL com busca e ordenação legítimas", "https://g1.globo.com/busca/?q=vorcaro&order=recent", "https://g1.globo.com/busca/?q=vorcaro&order=recent"},
		{"URL com múltiplos parâmetros legítimos e UTM", "https://noticias.com/artigo?categoria=economia&ano=2026&utm_source=twitter&ref=share", "https://noticias.com/artigo?categoria=economia&ano=2026&utm_source=twitter&ref=share"},
		{"URL com fragmento removido e query preservada", "https://example.com/noticia?id=100#secao-1", "https://example.com/noticia?id=100"},
		{"URL com porta não-padrão e query", "https://api.example.com:8443/doc?view=summary", "https://api.example.com:8443/doc?view=summary"},

		// URLs com parâmetros de credenciais / tokens (devem ser omitidas -> "")
		{"URL com token em query", "https://example.com/documento.pdf?token=secret123", ""},
		{"URL com token em maiúsculas", "https://example.com/documento.pdf?TOKEN=secret123", ""},
		{"URL com token percent-encoded", "https://example.com/documento.pdf?%74%6f%6b%65%6e=secret123", ""},
		{"URL com token sem valor", "https://example.com/documento.pdf?token", ""},
		{"URL com session em query", "https://example.com/area?session=valor_ficticio", ""},
		{"URL com SESSION em maiúsculas", "https://example.com/area?SESSION=valor_ficticio", ""},
		{"URL com api_token", "https://example.com/dados?api_token=xyz123", ""},
		{"URL com bearer", "https://example.com/dados?bearer=token_abc", ""},
		{"URL com access_token", "https://example.com/doc.pdf?access_token=xyz987", ""},
		{"URL com auth_token e id legítimo", "https://example.com/doc.pdf?id=123&auth_token=xyz", ""},
		{"URL com apiKey", "https://example.com/dados?apiKey=secret_key_123", ""},
		{"URL com api_key em maiúsculas", "https://example.com/dados?API_KEY=secret_key_123", ""},
		{"URL com api-key codificado %5F", "https://example.com/dados?api%5fkey=secret_key_123", ""},
		{"URL com password em query", "https://example.com/area?password=minhasenha", ""},
		{"URL com secret em query", "https://example.com/api?secret=supersecret", ""},
		{"URL com JWT em query", "https://example.com/relatorio?jwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", ""},
		{"URL com auth em query", "https://example.com/relatorio?auth=bearer123", ""},

		// URLs assinadas de provedores (AWS S3, GCP Storage, Azure SAS)
		{"AWS S3 Presigned URL (X-Amz-Signature)", "https://s3.amazonaws.com/bucket/doc.pdf?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20260903%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20260903T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=abcdef1234567890", ""},
		{"GCP Cloud Storage Signed URL (x-goog-signature)", "https://storage.googleapis.com/meu-bucket/laudo.pdf?x-goog-signature=fedcba0987654321&x-goog-algorithm=GOOG4-RSA-SHA256", ""},
		{"Azure Blob SAS URL (sig)", "https://storageacct.blob.core.windows.net/docs/pericia.pdf?sp=r&st=2026-09-03T00:00:00Z&se=2026-09-04T00:00:00Z&spr=https&sv=2020-08-04&sr=b&sig=M3V4c3NpZ25hdHVyZTEyMw%3D%3D", ""},
		{"URL com hmac genérico", "https://cdn.exemplo.com/arquivo.pdf?hmac=987654321abcdef", ""},
		{"URL com download_token", "https://downloads.exemplo.com/peça.pdf?download_token=abc123xyz", ""},

		// Delimitadores alternativos e formatações
		{"Query com ponto e vírgula e token", "https://example.com/noticia?id=123;token=secret", ""},
		{"Query malformada com percent inválido (fail-closed)", "https://example.com/noticia?%ZZ=123", ""},

		// Casos de segurança e validação básica já existentes
		{"URL com credenciais no userinfo", "https://user:pass@example.com/noticia", ""},
		{"Esquema javascript", "javascript:alert(1)", ""},
		{"Esquema data", "data:text/html,<html>", ""},
		{"Esquema ftp", "ftp://example.com/file", ""},
		{"Vazia", "", ""},
		{"Apenas espaços", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeURL(tt.input)
			if got != tt.expected {
				t.Errorf("SafeURL(%q) = %q; esperado %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFingerprintV1(t *testing.T) {
	cURL := "https://noticias.exemplo.com/materia-1"
	entity := "Daniel Vorcaro"
	prop := "Aquisição de participação societária no Banco Master"
	excerpt := "Conforme ata da assembleia, o empresário adquiriu o controle."

	fp1 := FingerprintV1(cURL, entity, prop, excerpt)

	if !strings.HasPrefix(fp1, "v1:") {
		t.Fatalf("fingerprint deve iniciar com prefixo 'v1:', obtido %q", fp1)
	}

	// 1. Determinismo: mesmas entradas produzem exatamente o mesmo hash
	fp2 := FingerprintV1(cURL, entity, prop, excerpt)
	if fp1 != fp2 {
		t.Fatalf("fingerprint não foi determinístico: %q != %q", fp1, fp2)
	}

	// 2. Normalização de espaçamento e case de entidade não alteram o fingerprint
	fp3 := FingerprintV1(cURL, "  DANIEL   VORCARO  ", "  "+prop+"  ", "  "+excerpt+"  ")
	if fp1 != fp3 {
		t.Errorf("fingerprint deveria ser idêntico com normalização de entidade/espaços: %q != %q", fp1, fp3)
	}

	// 3. Mudança de entidade produz fingerprint diferente (mesma URL)
	fpDiffEntity := FingerprintV1(cURL, "Outra Pessoa", prop, excerpt)
	if fp1 == fpDiffEntity {
		t.Errorf("fingerprints deveriam ser diferentes para entidades distintas na mesma URL")
	}

	// 4. Mudança de proposição produz fingerprint diferente
	fpDiffProp := FingerprintV1(cURL, entity, "Outra alegação factual distinta", excerpt)
	if fp1 == fpDiffProp {
		t.Errorf("fingerprints deveriam ser diferentes para proposições distintas")
	}

	// 5. Mudança de URL produz fingerprint diferente
	fpDiffURL := FingerprintV1("https://outro.site.com/materia", entity, prop, excerpt)
	if fp1 == fpDiffURL {
		t.Errorf("fingerprints deveriam ser diferentes para URLs distintas")
	}
}

func TestComputeFingerprint(t *testing.T) {
	fp, err := ComputeFingerprint(1, "https://exemplo.com", "Entidade", "Prop", "Excerpt")
	if err != nil {
		t.Fatalf("ComputeFingerprint(1) erro inesperado: %v", err)
	}
	if !strings.HasPrefix(fp, "v1:") {
		t.Errorf("ComputeFingerprint(1) = %q; esperado prefixo v1:", fp)
	}

	_, err = ComputeFingerprint(2, "https://exemplo.com", "Entidade", "Prop", "Excerpt")
	if err == nil {
		t.Errorf("esperava erro para versão 2 de fingerprint não suportada")
	}
}
