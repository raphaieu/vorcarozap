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
