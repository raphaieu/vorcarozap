package pages_test

import (
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
	"github.com/raphaieu/vorcarozap/web/pages"
)

func TestSanitizeExternalLink(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantValid   bool
		wantSafeURL string
		wantRawURL  string
	}{
		{
			name:        "URL válida simples",
			input:       "https://noticias.uol.com.br/politica/artigo.html",
			wantValid:   true,
			wantSafeURL: "https://noticias.uol.com.br/politica/artigo.html",
			wantRawURL:  "https://noticias.uol.com.br/politica/artigo.html",
		},
		{
			name:        "URL com query legítima preservada",
			input:       "https://g1.globo.com/busca/?q=vorcaro&order=recent&page=2",
			wantValid:   true,
			wantSafeURL: "https://g1.globo.com/busca/?q=vorcaro&order=recent&page=2",
			wantRawURL:  "https://g1.globo.com/busca/?q=vorcaro&order=recent&page=2",
		},
		{
			name:        "URL com token em query - Omissão total do link",
			input:       "https://documentos.exemplo.com/laudo.pdf?token=segredo12345",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "", // RawURL não pode vazar o token em telas admin
		},
		{
			name:        "URL com apiKey em query - Omissão total do link",
			input:       "https://api.exemplo.com/doc?apiKey=super_secret_key",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "",
		},
		{
			name:        "URL assinada AWS S3 - Omissão total do link",
			input:       "https://s3.amazonaws.com/bucket/doc.pdf?X-Amz-Signature=abcdef&X-Amz-Credential=AKIAEXAMPLE",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "",
		},
		{
			name:        "URL com credenciais no userinfo - Omissão total",
			input:       "https://usuario:senha123@noticias.exemplo.com/materia",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "",
		},
		{
			name:        "Esquema javascript inseguro sem segredos - Preserva RawURL para diagnóstico escapado",
			input:       "javascript:alert(1)",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "javascript:alert(1)",
		},
		{
			name:        "Esquema ftp sem segredos",
			input:       "ftp://arquivos.exemplo.com/documento.pdf",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "ftp://arquivos.exemplo.com/documento.pdf",
		},
		{
			name:        "String vazia",
			input:       "",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "",
		},
		{
			name:        "Apenas espaços",
			input:       "    ",
			wantValid:   false,
			wantSafeURL: "",
			wantRawURL:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pages.SanitizeExternalLink(tt.input)
			if got.IsValid != tt.wantValid {
				t.Errorf("SanitizeExternalLink(%q).IsValid = %v; esperado %v", tt.input, got.IsValid, tt.wantValid)
			}
			if got.SafeURL != tt.wantSafeURL {
				t.Errorf("SanitizeExternalLink(%q).SafeURL = %q; esperado %q", tt.input, got.SafeURL, tt.wantSafeURL)
			}
			if got.RawURL != tt.wantRawURL {
				t.Errorf("SanitizeExternalLink(%q).RawURL = %q; esperado %q", tt.input, got.RawURL, tt.wantRawURL)
			}
		})
	}
}

func TestPublicViewModels_URLSanitization(t *testing.T) {
	t.Run("ToPublicSourceVM omite URL canônica se contiver token sensível", func(t *testing.T) {
		row := sqlc.ListPublicEvidenceSourcesByClaimIDRow{
			SourceID:          "src-test-1",
			Title:             "Artigo Investigativo",
			PublisherOrAuthor: "Veículo de Imprensa",
			OriginalUrl:       "https://noticias.com/artigo?token=token_secreto",
			CanonicalUrl:      "https://noticias.com/artigo?token=token_secreto",
			SourceType:        "article",
			Role:              "supports",
		}

		vm := pages.ToPublicSourceVM(row)
		if vm.CanonicalURL != "" {
			t.Errorf("esperava CanonicalURL vazia para URL com token, obtido %q", vm.CanonicalURL)
		}
		if vm.OriginalURL != "" {
			t.Errorf("esperava OriginalURL vazia para URL com token, obtido %q", vm.OriginalURL)
		}
	})

	t.Run("ToPublicSourceVM preserva URL canônica se possuir apenas parâmetros legítimos", func(t *testing.T) {
		row := sqlc.ListPublicEvidenceSourcesByClaimIDRow{
			SourceID:          "src-test-2",
			Title:             "Artigo com Paginação",
			PublisherOrAuthor: "Veículo",
			OriginalUrl:       "https://noticias.com/artigo?id=999&page=3",
			CanonicalUrl:      "https://noticias.com/artigo?id=999&page=3",
			SourceType:        "article",
			Role:              "supports",
		}

		vm := pages.ToPublicSourceVM(row)
		expected := "https://noticias.com/artigo?id=999&page=3"
		if vm.CanonicalURL != expected {
			t.Errorf("esperava CanonicalURL %q, obtido %q", expected, vm.CanonicalURL)
		}
	})

	t.Run("ToPublicDefenseStatementVM omite SourceURL com token sensível", func(t *testing.T) {
		stmt := domain.PublicDefenseStatement{
			ID:            "stmt-1",
			ClaimID:       "clm-1",
			StatementType: domain.StatementTypeClarification,
			Title:         "Nota de Esclarecimento",
			Content:       "Conteúdo da defesa...",
			SourceURL:     "https://defesa.com/nota.pdf?auth_token=super_secret_auth",
			CreatedAt:     "2026-09-03T10:00:00Z",
		}

		vm := pages.ToPublicDefenseStatementVM(stmt)
		if vm.SourceURL != "" {
			t.Errorf("esperava SourceURL vazia para manifestação com token, obtido %q", vm.SourceURL)
		}
	})
}

func TestAdminViewModels_URLSanitization(t *testing.T) {
	t.Run("ToAdminDefenseStatementDetailVM omite SourceURL com token sensível", func(t *testing.T) {
		detail := &domain.AdminDefenseStatementDetail{
			ID:                "stmt-adm-1",
			ClaimID:           "clm-1",
			ClaimProposition:  "Proposição de teste",
			ClaimGrade:        domain.EvidenceGradeA,
			ClaimStatus:       domain.ClaimStatusPublished,
			SubjectEntityName: "Daniel Vorcaro",
			SubjectEntitySlug: "daniel-vorcaro",
			StatementType:     domain.StatementTypeClarification,
			Title:             "Peça de Defesa",
			Content:           "Argumentação jurídica",
			SourceURL:         "https://defesa.com/laudo.pdf?X-Amz-Signature=abc12345&X-Amz-Credential=def",
			Status:            domain.StatementStatusAccepted,
		}

		vm := pages.ToAdminDefenseStatementDetailVM(detail, "", "")
		if vm.SourceURL != "" {
			t.Errorf("esperava SourceURL vazia no admin para manifestação com URL assinada, obtido %q", vm.SourceURL)
		}
	})

	t.Run("ToAdminSourceDetailVM omite CanonicalURL com token sensível", func(t *testing.T) {
		res := &store.AdminSourceDetailResult{
			Source: sqlc.GetAdminSourceDetailByIDRow{
				ID:           "src-adm-1",
				Title:        "Documento com SAS Token",
				CanonicalUrl: "https://blob.core.windows.net/docs/pericia.pdf?sig=secret_sig_token%3D",
				SourceType:   "court_document",
			},
		}

		vm := pages.ToAdminSourceDetailVM(res)
		if vm.CanonicalURL.IsValid {
			t.Errorf("esperava IsValid false para documento com token SAS")
		}
		if vm.CanonicalURL.SafeURL != "" {
			t.Errorf("esperava SafeURL vazia para documento com token SAS, obtido %q", vm.CanonicalURL.SafeURL)
		}
		if vm.CanonicalURL.RawURL != "" {
			t.Errorf("esperava RawURL vazia para documento com token SAS, obtido %q", vm.CanonicalURL.RawURL)
		}
	})
}
