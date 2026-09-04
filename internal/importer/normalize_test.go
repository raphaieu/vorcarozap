package importer

import (
	"testing"
)

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"Daniel Vorcaro", "Daniel Vorcaro"},
		{"  Daniel    Vorcaro  ", "Daniel Vorcaro"},
		{"Nome\n\tcom   espaços\r\nvariados", "Nome com espaços variados"},
	}

	for _, tt := range tests {
		got := NormalizeString(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeString(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeUnicode(t *testing.T) {
	// Testa que a normalização NFC preserva os acentos
	input := "João da Silva — Diretor"
	got := NormalizeUnicode(input)
	if got != input {
		t.Errorf("NormalizeUnicode(%q) = %q; acentos não deveriam ser descartados", input, got)
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  DANIEL VORCARO  ", "daniel vorcaro"},
		{"Álvaro   Dias", "álvaro dias"},
		{"Érico   Veríssimo", "érico veríssimo"},
	}

	for _, tt := range tests {
		got := NormalizeName(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeName(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}
}

func TestGenerateSlugAndDisambiguation(t *testing.T) {
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
		got := GenerateSlug(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateSlug(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}

	// Teste de desambiguação
	base := GenerateSlug("Daniel Vorcaro")
	if got := DisambiguateSlug(base, 1); got != "daniel-vorcaro" {
		t.Errorf("esperava slug base para index 1, obteve %q", got)
	}
	if got := DisambiguateSlug(base, 2); got != "daniel-vorcaro-2" {
		t.Errorf("esperava daniel-vorcaro-2, obteve %q", got)
	}
	if got := DisambiguateSlug(base, 3); got != "daniel-vorcaro-3" {
		t.Errorf("esperava daniel-vorcaro-3, obteve %q", got)
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
			name:     "HTTP padrão",
			input:    "http://Example.COM:80/noticia",
			expected: "http://example.com/noticia",
		},
		{
			name:     "HTTPS padrão com remoção de porta 443 e fragmento",
			input:    "https://Noticias.UOL.com.br:443/politica/artigo.html#comentarios",
			expected: "https://noticias.uol.com.br/politica/artigo.html",
		},
		{
			name:     "Preservação de query string essencial",
			input:    "https://g1.globo.com/busca/?q=vorcaro&order=recent",
			expected: "https://g1.globo.com/busca/?q=vorcaro&order=recent",
		},
		{
			name:     "Porta não-padrão preservada",
			input:    "https://api.example.com:8443/data",
			expected: "https://api.example.com:8443/data",
		},
		{
			name:     "Root path vazio vira slash",
			input:    "https://banco.master",
			expected: "https://banco.master/",
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

func TestParseRelevance(t *testing.T) {
	tests := []struct {
		input       string
		expected    int
		expectError bool
	}{
		{"1", 1, false},
		{" 2 ", 2, false},
		{"3", 3, false},
		{"4", 4, false},
		{"5", 5, false},
		{"0", 0, true},
		{"6", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		rel, err := ParseRelevance(tt.input)
		if tt.expectError {
			if err == nil {
				t.Errorf("esperava erro para relevância %q, obteve %d", tt.input, rel)
			}
		} else {
			if err != nil {
				t.Errorf("erro inesperado para relevância %q: %v", tt.input, err)
			}
			if int(rel) != tt.expected {
				t.Errorf("ParseRelevance(%q) = %d; esperado %d", tt.input, rel, tt.expected)
			}
		}
	}
}
