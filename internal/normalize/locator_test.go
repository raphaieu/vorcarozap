package normalize_test

import (
	"strings"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/normalize"
)

func TestLocator_ValidCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Páginas simples
		{"ponto p", "p. 42", "Pág. 42"},
		{"p sem ponto", "p 42", "Pág. 42"},
		{"pag com ponto", "pag. 42", "Pág. 42"},
		{"pág acentuado", "pág. 42", "Pág. 42"},
		{"pagina por extenso", "página 42", "Pág. 42"},
		{"pagina sem acento", "pagina 42", "Pág. 42"},
		{"maiúscula", "PÁG. 42", "Pág. 42"},
		{"número romano", "pág. iv", "Pág. iv"},
		{"número com sufixo", "pág. 42-A", "Pág. 42-A"},

		// Intervalos de páginas
		{"pp com hífen", "pp. 42-44", "Págs. 42–44"},
		{"págs com hífen", "págs. 42-44", "Págs. 42–44"},
		{"págs com espaços em volta do hífen", "págs. 42 - 44", "Págs. 42–44"},
		{"p com intervalo vira Págs", "p. 42-44", "Págs. 42–44"},
		{"páginas a intervalo", "páginas 42 a 44", "Págs. 42–44"},
		{"intervalo já com en-dash", "págs. 42–44", "Págs. 42–44"},
		{"intervalo com em-dash", "págs. 42—44", "Págs. 42–44"},

		// Folhas (fls)
		{"folha simples", "fl. 12", "Fl. 12"},
		{"folha por extenso", "folha 12", "Fl. 12"},
		{"folha com verso", "fl. 12v", "Fl. 12v"},
		{"folhas intervalo", "fls. 12-15", "Fls. 12–15"},
		{"folhas extenso", "folhas 12 a 15", "Fls. 12–15"},
		{"fl com intervalo vira Fls", "fl. 12-15", "Fls. 12–15"},

		// Figuras
		{"figura simples", "fig. 12", "Fig. 12"},
		{"figura extenso", "figura 12", "Fig. 12"},
		{"figuras intervalo", "figs. 12-14", "Figs. 12–14"},
		{"figura com letra", "fig. 3A", "Fig. 3A"},

		// Tabelas
		{"tabela simples", "tab. 3", "Tabela 3"},
		{"tabela extenso", "tabela 3", "Tabela 3"},
		{"tabelas intervalo", "tabs. 3-5", "Tabelas 3–5"},
		{"tabelas extenso", "tabelas 3 a 5", "Tabelas 3–5"},

		// Anexos e Itens
		{"anexo simples", "anexo B", "Anexo B"},
		{"anx abreviado", "anx. 1", "Anexo 1"},
		{"anexos intervalo", "anexos 1-3", "Anexos 1–3"},
		{"item simples", "item 4", "item 4"},
		{"it abreviado", "it. 4", "item 4"},

		// Artigos, Parágrafos e Seções
		{"artigo simples", "art. 5", "Art. 5"},
		{"artigo extenso", "artigo 5", "Art. 5"},
		{"artigo ordinal", "art. 5º", "Art. 5º"},
		{"artigos intervalo", "arts. 5-8", "Arts. 5–8"},
		{"parágrafo símbolo", "§ 1º", "§ 1º"},
		{"parágrafos múltiplos", "§§ 1-3", "§§ 1–3"},
		{"seção simples", "seção 2.1", "Seção 2.1"},
		{"secao sem acento", "secao 3", "Seção 3"},

		// Combinações compostas
		{"página e figura", "p. 42, figura 12", "Pág. 42, Fig. 12"},
		{"página e figura com abreviação", "pág. 42, fig. 12", "Pág. 42, Fig. 12"},
		{"páginas e tabela", "págs. 42-44, tabela 3", "Págs. 42–44, Tabela 3"},
		{"anexo e item", "Anexo B, item 4", "Anexo B, item 4"},
		{"folhas e anexo", "fls. 10-12, anexo 1", "Fls. 10–12, Anexo 1"},
		{"separador ponto e vírgula", "pág. 5; tabela 2", "Pág. 5, Tabela 2"},
		{"artigo e parágrafo", "art. 5º, § 1º", "Art. 5º, § 1º"},

		// Preservação de texto livre seguro
		{"texto livre capítulo", "Capítulo IV", "Capítulo IV"},
		{"texto livre nota de rodapé", "Nota de rodapé 2", "Nota de rodapé 2"},
		{"conclusão técnica", "Conclusão pericial, item 3", "Conclusão pericial, item 3"},

		// Casos vazios ou apenas espaços
		{"string vazia", "", ""},
		{"espaços em branco", "   \t  \n ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalize.Locator(tt.input)
			if err != nil {
				t.Fatalf("Locator(%q) retornou erro inesperado: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("Locator(%q) = %q, esperado %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLocator_SecurityAndRejection(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		errContains string
	}{
		{
			name:        "excesso de tamanho",
			input:       strings.Repeat("a", 201),
			errContains: "excede o limite máximo",
		},
		{
			name:        "tag HTML script",
			input:       "<script>alert(1)</script>",
			errContains: "não pode conter tags HTML",
		},
		{
			name:        "tag HTML b",
			input:       "pág. <b>42</b>",
			errContains: "não pode conter tags HTML",
		},
		{
			name:        "tag img xss",
			input:       "<img src=x onerror=alert(1)>",
			errContains: "não pode conter tags HTML",
		},
		{
			name:        "esquema javascript",
			input:       "javascript:alert('xss')",
			errContains: "esquema ou conteúdo executável inseguro",
		},
		{
			name:        "esquema data",
			input:       "data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
			errContains: "esquema ou conteúdo executável inseguro",
		},
		{
			name:        "esquema vbscript",
			input:       "vbscript:msgbox(1)",
			errContains: "esquema ou conteúdo executável inseguro",
		},
		{
			name:        "esquema file",
			input:       "file:///etc/passwd",
			errContains: "esquema ou conteúdo executável inseguro",
		},
		{
			name:        "path traversal unix",
			input:       "../../etc/passwd",
			errContains: "navegação de diretório",
		},
		{
			name:        "path traversal windows",
			input:       "..\\..\\windows\\system32",
			errContains: "navegação de diretório",
		},
		{
			name:        "caracteres de controle",
			input:       "pág. 42\x00\x01",
			errContains: "caracteres de controle inválidos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalize.Locator(tt.input)
			if err == nil {
				t.Fatalf("Locator(%q) deveria ter falhado com erro contendo %q, mas obteve sucesso", tt.input, tt.errContains)
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Locator(%q) erro = %q, esperado conter %q", tt.input, err.Error(), tt.errContains)
			}
		})
	}
}

func TestLocator_IdempotenceAndDeterminism(t *testing.T) {
	inputs := []string{
		"p. 42",
		"págs. 42-44",
		"p. 42, figura 12",
		"fls. 12-15",
		"Anexo B, item 4",
		"Art. 5º, § 1º",
	}

	for _, in := range inputs {
		firstPass, err := normalize.Locator(in)
		if err != nil {
			t.Fatalf("falha no primeiro passo para %q: %v", in, err)
		}
		secondPass, err := normalize.Locator(firstPass)
		if err != nil {
			t.Fatalf("falha no segundo passo para %q: %v", firstPass, err)
		}
		if firstPass != secondPass {
			t.Errorf("idempotência violada: pass1=%q != pass2=%q", firstPass, secondPass)
		}
	}
}

func TestSafeLocator(t *testing.T) {
	if got := normalize.SafeLocator("p. 42"); got != "Pág. 42" {
		t.Errorf("SafeLocator(p. 42) = %q, want 'Pág. 42'", got)
	}
	if got := normalize.SafeLocator("<script>alert(1)</script>"); got != "" {
		t.Errorf("SafeLocator(malicious) = %q, want empty string", got)
	}
	if got := normalize.SafeLocator(""); got != "" {
		t.Errorf("SafeLocator('') = %q, want empty string", got)
	}
}

func TestCompareLocators(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int // -1, 0, 1
	}{
		{"iguais idênticos", "Pág. 42", "Pág. 42", 0},
		{"número simples menor", "Pág. 5", "Pág. 12", -1},
		{"número simples maior", "Pág. 42", "Pág. 5", 1},
		{"número de 3 dígitos", "Pág. 99", "Pág. 100", -1},
		{"mesma página vs intervalo", "Pág. 42", "Págs. 42–44", -1},
		{"intervalos diferentes início igual", "Págs. 42–44", "Págs. 42–48", -1},
		{"intervalos diferentes início maior", "Págs. 50–52", "Págs. 42–48", 1},
		{"folhas numéricas", "Fl. 2", "Fl. 10", -1},
		{"página vs figura", "Pág. 42", "Fig. 1", -1},
		{"figura vs tabela", "Fig. 10", "Tabela 2", -1},
		{"tabela vs anexo", "Tabela 5", "Anexo A", -1},
		{"subcomponente igual no início", "Pág. 10, Fig. 2", "Pág. 10, Fig. 5", -1},
		{"subcomponente menor em página", "Pág. 5, Fig. 10", "Pág. 10, Fig. 1", -1},
		{"anexos alfabéticos", "Anexo A", "Anexo B", -1},
		{"vazio vs preenchido", "", "Pág. 1", 1},
		{"preenchido vs vazio", "Pág. 1", "", -1},
		{"ambos vazios", "", "", 0},
		{"espaços apenas", "   ", "", 0},
		{"artigos e parágrafos", "Art. 5º", "Art. 10", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize.CompareLocators(tt.a, tt.b)
			if (tt.expected < 0 && got >= 0) || (tt.expected > 0 && got <= 0) || (tt.expected == 0 && got != 0) {
				t.Errorf("CompareLocators(%q, %q) = %d, esperado sinal %d", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}
