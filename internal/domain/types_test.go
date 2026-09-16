package domain_test

import (
	"strings"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

func TestEntityType(t *testing.T) {
	tests := []struct {
		val   domain.EntityType
		valid bool
	}{
		{domain.EntityTypePerson, true},
		{domain.EntityTypeOrganization, true},
		{"company", false},
		{"", false},
		{"other", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("EntityType(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestRelevance(t *testing.T) {
	tests := []struct {
		val   int
		valid bool
	}{
		{0, false},
		{1, true},
		{2, true},
		{3, true},
		{4, true},
		{5, true},
		{6, false},
		{-1, false},
	}

	for _, tt := range tests {
		rel := domain.Relevance(tt.val)
		if got := rel.IsValid(); got != tt.valid {
			t.Errorf("Relevance(%d).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}

		_, err := domain.ValidateRelevance(tt.val)
		if (err == nil) != tt.valid {
			t.Errorf("ValidateRelevance(%d) err = %v, esperado válido = %v", tt.val, err, tt.valid)
		}
	}
}

func TestClaimOrigin(t *testing.T) {
	tests := []struct {
		val   domain.ClaimOrigin
		valid bool
	}{
		{domain.ClaimOriginOpenRouter, true},
		{domain.ClaimOriginCuratedSeed, true},
		{domain.ClaimOriginAdmin, true},
		{"web_scraping", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("ClaimOrigin(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestEvidenceGrade(t *testing.T) {
	tests := []struct {
		val   domain.EvidenceGrade
		valid bool
	}{
		{domain.EvidenceGradeA, true},
		{domain.EvidenceGradeB, true},
		{domain.EvidenceGradeC, true},
		{domain.EvidenceGradeD, true},
		{domain.EvidenceGradeE, true},
		{"F", false},
		{"a", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("EvidenceGrade(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestClaimDisposition(t *testing.T) {
	tests := []struct {
		val   domain.ClaimDisposition
		valid bool
	}{
		{domain.DispositionSupportsLink, true},
		{domain.DispositionPossibleLink, true},
		{domain.DispositionContradictsLink, true},
		{domain.DispositionContextOnly, true},
		{domain.DispositionCorrection, true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("ClaimDisposition(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestClaimStatus(t *testing.T) {
	tests := []struct {
		val   domain.ClaimStatus
		valid bool
	}{
		{domain.ClaimStatusQuarantined, true},
		{domain.ClaimStatusPublished, true},
		{domain.ClaimStatusRejected, true},
		{domain.ClaimStatusArchived, true},
		{"pending", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("ClaimStatus(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestEvidenceSourceRole(t *testing.T) {
	tests := []struct {
		val   domain.EvidenceSourceRole
		valid bool
	}{
		{domain.RoleSupports, true},
		{domain.RoleContradicts, true},
		{domain.RoleContextualizes, true},
		{"proves", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("EvidenceSourceRole(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestEvidenceSourceStatus(t *testing.T) {
	tests := []struct {
		val   domain.EvidenceSourceStatus
		valid bool
	}{
		{domain.EvidenceSourceStatusActive, true},
		{domain.EvidenceSourceStatusRejected, true},
		{"inactive", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("EvidenceSourceStatus(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestSourceAccessStatus(t *testing.T) {
	tests := []struct {
		val   domain.SourceAccessStatus
		valid bool
	}{
		{domain.SourceAccessCitedByProvider, true},
		{domain.SourceAccessReachable, true},
		{domain.SourceAccessUnreachable, true},
		{domain.SourceAccessNotChecked, true},
		{"error", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("SourceAccessStatus(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestImportRunStatus(t *testing.T) {
	tests := []struct {
		val   domain.ImportRunStatus
		valid bool
	}{
		{domain.ImportRunStatusPending, true},
		{domain.ImportRunStatusRunning, true},
		{domain.ImportRunStatusDryRun, true},
		{domain.ImportRunStatusCompleted, true},
		{domain.ImportRunStatusPartial, true},
		{domain.ImportRunStatusFailed, true},
		{"quarantined", false},
		{"published", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("ImportRunStatus(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestModerationAction(t *testing.T) {
	tests := []struct {
		val   domain.ModerationAction
		valid bool
	}{
		{domain.ModerationActionApprove, true},
		{domain.ModerationActionReject, true},
		{domain.ModerationActionRestore, true},
		{domain.ModerationAction("archive"), false},
		{"delete", false},
		{"publish", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.val.IsValid(); got != tt.valid {
			t.Errorf("ModerationAction(%q).IsValid() = %v, esperado %v", tt.val, got, tt.valid)
		}
	}
}

func TestValidateClaimTransition(t *testing.T) {
	tests := []struct {
		name       string
		current    domain.ClaimStatus
		action     domain.ModerationAction
		wantTarget domain.ClaimStatus
		wantErr    bool
	}{
		// Approve transitions
		{
			name:       "approve quarantined -> published",
			current:    domain.ClaimStatusQuarantined,
			action:     domain.ModerationActionApprove,
			wantTarget: domain.ClaimStatusPublished,
			wantErr:    false,
		},
		{
			name:       "approve published -> error",
			current:    domain.ClaimStatusPublished,
			action:     domain.ModerationActionApprove,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "approve rejected -> error",
			current:    domain.ClaimStatusRejected,
			action:     domain.ModerationActionApprove,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "approve archived -> error",
			current:    domain.ClaimStatusArchived,
			action:     domain.ModerationActionApprove,
			wantTarget: "",
			wantErr:    true,
		},

		// Reject transitions
		{
			name:       "reject published -> rejected",
			current:    domain.ClaimStatusPublished,
			action:     domain.ModerationActionReject,
			wantTarget: domain.ClaimStatusRejected,
			wantErr:    false,
		},
		{
			name:       "reject quarantined -> rejected",
			current:    domain.ClaimStatusQuarantined,
			action:     domain.ModerationActionReject,
			wantTarget: domain.ClaimStatusRejected,
			wantErr:    false,
		},
		{
			name:       "reject rejected -> error",
			current:    domain.ClaimStatusRejected,
			action:     domain.ModerationActionReject,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "reject archived -> error",
			current:    domain.ClaimStatusArchived,
			action:     domain.ModerationActionReject,
			wantTarget: "",
			wantErr:    true,
		},

		// Restore transitions
		{
			name:       "restore rejected -> quarantined",
			current:    domain.ClaimStatusRejected,
			action:     domain.ModerationActionRestore,
			wantTarget: domain.ClaimStatusQuarantined,
			wantErr:    false,
		},
		{
			name:       "restore archived -> quarantined",
			current:    domain.ClaimStatusArchived,
			action:     domain.ModerationActionRestore,
			wantTarget: domain.ClaimStatusQuarantined,
			wantErr:    false,
		},
		{
			name:       "restore quarantined -> error",
			current:    domain.ClaimStatusQuarantined,
			action:     domain.ModerationActionRestore,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "restore published -> error",
			current:    domain.ClaimStatusPublished,
			action:     domain.ModerationActionRestore,
			wantTarget: "",
			wantErr:    true,
		},

		// Archive action is out of executable scope
		{
			name:       "archive published -> error",
			current:    domain.ClaimStatusPublished,
			action:     domain.ModerationAction("archive"),
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "archive quarantined -> error",
			current:    domain.ClaimStatusQuarantined,
			action:     domain.ModerationAction("archive"),
			wantTarget: "",
			wantErr:    true,
		},

		// Invalid inputs
		{
			name:       "invalid current status",
			current:    domain.ClaimStatus("invalid"),
			action:     domain.ModerationActionApprove,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "invalid action",
			current:    domain.ClaimStatusQuarantined,
			action:     domain.ModerationAction("invalid"),
			wantTarget: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ValidateClaimTransition(tt.current, tt.action)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateClaimTransition(%q, %q) err = %v, wantErr = %v", tt.current, tt.action, err, tt.wantErr)
			}
			if got != tt.wantTarget {
				t.Errorf("ValidateClaimTransition(%q, %q) = %q, esperado %q", tt.current, tt.action, got, tt.wantTarget)
			}
		})
	}
}

func TestValidateModerationReason(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      string
		expectErr bool
	}{
		{
			name:      "valid reason",
			raw:       "Documento comprobatório verificado em cartório.",
			want:      "Documento comprobatório verificado em cartório.",
			expectErr: false,
		},
		{
			name:      "valid reason with trimming",
			raw:       "   Motivo com espaços   ",
			want:      "Motivo com espaços",
			expectErr: false,
		},
		{
			name:      "empty reason",
			raw:       "",
			want:      "",
			expectErr: true,
		},
		{
			name:      "whitespace only reason",
			raw:       "   \t\n   ",
			want:      "",
			expectErr: true,
		},
		{
			name:      "reason with html tags",
			raw:       "Motivo com <script>alert(1)</script>",
			want:      "",
			expectErr: true,
		},
		{
			name:      "reason exceeding max length",
			raw:       string(make([]byte, 1001)),
			want:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ValidateModerationReason(tt.raw)
			if (err != nil) != tt.expectErr {
				t.Fatalf("ValidateModerationReason() err = %v, expectErr = %v", err, tt.expectErr)
			}
			if got != tt.want {
				t.Errorf("ValidateModerationReason() = %q, esperado %q", got, tt.want)
			}
		})
	}
}

func TestValidateModerationActor(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      string
		expectErr bool
	}{
		{
			name:      "valid actor",
			raw:       "admin_editor",
			want:      "admin_editor",
			expectErr: false,
		},
		{
			name:      "valid actor with trimming",
			raw:       "   editor1   ",
			want:      "editor1",
			expectErr: false,
		},
		{
			name:      "empty actor",
			raw:       "",
			want:      "",
			expectErr: true,
		},
		{
			name:      "whitespace only actor",
			raw:       "   ",
			want:      "",
			expectErr: true,
		},
		{
			name:      "actor exceeding limit",
			raw:       string(make([]byte, 129)),
			want:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ValidateModerationActor(tt.raw)
			if (err != nil) != tt.expectErr {
				t.Fatalf("ValidateModerationActor() err = %v, expectErr = %v", err, tt.expectErr)
			}
			if got != tt.want {
				t.Errorf("ValidateModerationActor() = %q, esperado %q", got, tt.want)
			}
		})
	}
}

func TestValidateEvidenceSourceTransition(t *testing.T) {
	tests := []struct {
		name       string
		current    domain.EvidenceSourceStatus
		action     domain.ModerationAction
		wantTarget domain.EvidenceSourceStatus
		wantErr    bool
	}{
		// Reject transitions
		{
			name:       "reject active -> rejected",
			current:    domain.EvidenceSourceStatusActive,
			action:     domain.ModerationActionReject,
			wantTarget: domain.EvidenceSourceStatusRejected,
			wantErr:    false,
		},
		{
			name:       "reject rejected -> error",
			current:    domain.EvidenceSourceStatusRejected,
			action:     domain.ModerationActionReject,
			wantTarget: "",
			wantErr:    true,
		},

		// Restore transitions
		{
			name:       "restore rejected -> active",
			current:    domain.EvidenceSourceStatusRejected,
			action:     domain.ModerationActionRestore,
			wantTarget: domain.EvidenceSourceStatusActive,
			wantErr:    false,
		},
		{
			name:       "restore active -> error",
			current:    domain.EvidenceSourceStatusActive,
			action:     domain.ModerationActionRestore,
			wantTarget: "",
			wantErr:    true,
		},

		// Approve is not supported for evidence_source
		{
			name:       "approve active -> error",
			current:    domain.EvidenceSourceStatusActive,
			action:     domain.ModerationActionApprove,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "approve rejected -> error",
			current:    domain.EvidenceSourceStatusRejected,
			action:     domain.ModerationActionApprove,
			wantTarget: "",
			wantErr:    true,
		},

		// Invalid inputs
		{
			name:       "invalid current status",
			current:    domain.EvidenceSourceStatus("invalid"),
			action:     domain.ModerationActionReject,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "invalid action",
			current:    domain.EvidenceSourceStatusActive,
			action:     domain.ModerationAction("invalid"),
			wantTarget: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ValidateEvidenceSourceTransition(tt.current, tt.action)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateEvidenceSourceTransition(%q, %q) err = %v, wantErr = %v", tt.current, tt.action, err, tt.wantErr)
			}
			if got != tt.wantTarget {
				t.Errorf("ValidateEvidenceSourceTransition(%q, %q) = %q, esperado %q", tt.current, tt.action, got, tt.wantTarget)
			}
		})
	}
}

func TestSourceType(t *testing.T) {
	validTypes := []domain.SourceType{
		domain.SourceTypeArticle,
		domain.SourceTypeCourtDocument,
		domain.SourceTypePoliceReport,
		domain.SourceTypeOfficialStatement,
		domain.SourceTypeInterview,
		domain.SourceTypeSocialMedia,
	}

	for _, st := range validTypes {
		if !st.IsValid() {
			t.Errorf("SourceType %q deveria ser válido", st)
		}
		if st.Label() == "" {
			t.Errorf("SourceType %q não deve ter Label vazia", st)
		}
		if st.NatureLabel() == "" {
			t.Errorf("SourceType %q não deve ter NatureLabel vazia", st)
		}
	}

	invalidType := domain.SourceType("invalid_type")
	if invalidType.IsValid() {
		t.Errorf("SourceType %q não deveria ser válido", invalidType)
	}

	// Validação de documentos primários
	primaryTypes := []domain.SourceType{
		domain.SourceTypeCourtDocument,
		domain.SourceTypePoliceReport,
		domain.SourceTypeOfficialStatement,
	}
	for _, pt := range primaryTypes {
		if !pt.IsPrimaryDocument() {
			t.Errorf("SourceType %q deveria ser reconhecido como documento primário", pt)
		}
	}

	secondaryTypes := []domain.SourceType{
		domain.SourceTypeArticle,
		domain.SourceTypeInterview,
		domain.SourceTypeSocialMedia,
	}
	for _, st := range secondaryTypes {
		if st.IsPrimaryDocument() {
			t.Errorf("SourceType %q NÃO deveria ser reconhecido como documento primário", st)
		}
	}

	// CanonicalSourceTypes deve conter exatamente os 6 tipos canônicos
	if len(domain.CanonicalSourceTypes) != 6 {
		t.Fatalf("esperado 6 tipos em CanonicalSourceTypes, obtido %d", len(domain.CanonicalSourceTypes))
	}
	for _, ct := range domain.CanonicalSourceTypes {
		if !ct.IsValid() {
			t.Errorf("tipo canônico %q deveria ser válido", ct)
		}
	}

	// Tipo desconhecido deve permanecer neutro
	unknown := domain.SourceType("blog_post_desconhecido")
	if unknown.IsValid() {
		t.Errorf("tipo desconhecido não deveria ser válido")
	}
	if unknown.IsPrimaryDocument() {
		t.Errorf("tipo desconhecido não deve ser classificado como primário")
	}
	if unknown.Label() != "blog_post_desconhecido" {
		t.Errorf("Label de tipo desconhecido deve retornar seu valor original, obtido: %q", unknown.Label())
	}
	if unknown.NatureLabel() != "Referência Documental" {
		t.Errorf("NatureLabel de tipo desconhecido deve ser 'Referência Documental', obtido: %q", unknown.NatureLabel())
	}
}

func TestStatementTypeAndStatus(t *testing.T) {
	// Test StatementType
	validTypes := []domain.StatementType{
		domain.StatementTypeCorrection,
		domain.StatementTypeRebuttal,
		domain.StatementTypeClarification,
		domain.StatementTypeAdditionalContext,
	}
	for _, st := range validTypes {
		if !st.IsValid() {
			t.Errorf("StatementType %q deveria ser válido", st)
		}
		if st.Label() == "" {
			t.Errorf("StatementType %q não deveria ter Label vazia", st)
		}
	}
	if domain.StatementType("invalid").IsValid() {
		t.Errorf("StatementType 'invalid' não deveria ser válido")
	}

	// Test StatementStatus
	validStatuses := []domain.StatementStatus{
		domain.StatementStatusQuarantined,
		domain.StatementStatusUnderReview,
		domain.StatementStatusAccepted,
		domain.StatementStatusRejected,
		domain.StatementStatusArchived,
	}
	for _, ss := range validStatuses {
		if !ss.IsValid() {
			t.Errorf("StatementStatus %q deveria ser válido", ss)
		}
		if ss.Label() == "" {
			t.Errorf("StatementStatus %q não deveria ter Label vazia", ss)
		}
	}
	if domain.StatementStatus("invalid").IsValid() {
		t.Errorf("StatementStatus 'invalid' não deveria ser válido")
	}

	// Test StatementModerationAction
	validActions := []domain.StatementModerationAction{
		domain.StatementActionAccept,
		domain.StatementActionReject,
		domain.StatementActionReview,
		domain.StatementActionArchive,
	}
	for _, sa := range validActions {
		if !sa.IsValid() {
			t.Errorf("StatementModerationAction %q deveria ser válida", sa)
		}
		if sa.Label() == "" {
			t.Errorf("StatementModerationAction %q não deveria ter Label vazia", sa)
		}
	}
	if domain.StatementModerationAction("invalid").IsValid() {
		t.Errorf("StatementModerationAction 'invalid' não deveria ser válida")
	}
}

func TestValidateStatementSubmission(t *testing.T) {
	tests := []struct {
		name      string
		sub       domain.DefenseStatementSubmission
		expectErr bool
	}{
		{
			name: "valid complete submission",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-1",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Manifestação de Defesa Oficial",
				Content:       "Esta é uma contestação formal e detalhada com mais de 10 caracteres.",
				SourceURL:     "https://exemplo.com/comunicado.pdf",
				ContactInfo:   "advogado@exemplo.com - Tel: (11) 9999-9999",
			},
			expectErr: false,
		},
		{
			name: "valid minimal submission without optional fields",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-2",
				StatementType: domain.StatementTypeCorrection,
				Title:         "Retificação de Data",
				Content:       "O evento narrado ocorreu em 2024 e não em 2023 conforme certidão.",
			},
			expectErr: false,
		},
		{
			name: "missing claim_id",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "",
				StatementType: domain.StatementTypeClarification,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
			},
			expectErr: true,
		},
		{
			name: "invalid statement_type",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-3",
				StatementType: domain.StatementType("invalid_type"),
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
			},
			expectErr: true,
		},
		{
			name: "empty title",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-4",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "   ",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
			},
			expectErr: true,
		},
		{
			name: "title exceeding limit",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-5",
				StatementType: domain.StatementTypeRebuttal,
				Title:         strings.Repeat("a", 201),
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
			},
			expectErr: true,
		},
		{
			name: "title containing html script tag",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-6",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título com <script>alert(1)</script>",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
			},
			expectErr: true,
		},
		{
			name: "content too short (< 10 chars)",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-7",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Curto",
			},
			expectErr: true,
		},
		{
			name: "content exceeding max length (> 5000 chars)",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-8",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       strings.Repeat("a", 5001),
			},
			expectErr: true,
		},
		{
			name: "content containing html tags",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-9",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto com <b>negrito</b> e <i>tags</i> perigosas.",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url with javascript scheme",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-10",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "javascript:alert(1)",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url without http/https",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-11",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "ftp://exemplo.com/arquivo.pdf",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url empty host http://",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-1",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "http://",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url empty host with query http://?x",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-2",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "http://?x",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url with userinfo credentials",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-3",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "http://user:pass@example.com/nota",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url with control characters",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-4",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "https://example.com/path\x00malicious",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url with newline control characters",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-5",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "https://example.com/path\nheader-injection",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url with data scheme",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-5b",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "data:text/html,<script>alert(1)</script>",
			},
			expectErr: true,
		},
		{
			name: "invalid source_url with invalid host prefix hyphen",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-5c",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "http://-invalid-host/doc.pdf",
			},
			expectErr: true,
		},
		{
			name: "valid source_url with upper case scheme normalized",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-url-6",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				SourceURL:     "HTTP://EXAMPLE.COM/nota.pdf",
			},
			expectErr: false,
		},
		{
			name: "contact_info exceeding limit",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-12",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				ContactInfo:   strings.Repeat("c", 256),
			},
			expectErr: true,
		},
		{
			name: "contact_info with html tags",
			sub: domain.DefenseStatementSubmission{
				ClaimID:       "claim-uuid-13",
				StatementType: domain.StatementTypeRebuttal,
				Title:         "Título Válido",
				Content:       "Texto de esclarecimento válido com tamanho suficiente.",
				ContactInfo:   "<email>teste@exemplo.com</email>",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ValidateStatementSubmission(tt.sub)
			if (err != nil) != tt.expectErr {
				t.Fatalf("ValidateStatementSubmission() err = %v, expectErr = %v", err, tt.expectErr)
			}
			if !tt.expectErr {
				if got.ClaimID != tt.sub.ClaimID {
					t.Errorf("got.ClaimID = %q, want %q", got.ClaimID, tt.sub.ClaimID)
				}
			}
		})
	}
}

func TestValidateStatementTransition(t *testing.T) {
	tests := []struct {
		name       string
		current    domain.StatementStatus
		action     domain.StatementModerationAction
		wantTarget domain.StatementStatus
		wantErr    bool
	}{
		// Accept
		{
			name:       "accept quarantined -> accepted",
			current:    domain.StatementStatusQuarantined,
			action:     domain.StatementActionAccept,
			wantTarget: domain.StatementStatusAccepted,
			wantErr:    false,
		},
		{
			name:       "accept under_review -> accepted",
			current:    domain.StatementStatusUnderReview,
			action:     domain.StatementActionAccept,
			wantTarget: domain.StatementStatusAccepted,
			wantErr:    false,
		},
		{
			name:       "accept rejected -> error",
			current:    domain.StatementStatusRejected,
			action:     domain.StatementActionAccept,
			wantTarget: "",
			wantErr:    true,
		},

		// Reject
		{
			name:       "reject quarantined -> rejected",
			current:    domain.StatementStatusQuarantined,
			action:     domain.StatementActionReject,
			wantTarget: domain.StatementStatusRejected,
			wantErr:    false,
		},
		{
			name:       "reject under_review -> rejected",
			current:    domain.StatementStatusUnderReview,
			action:     domain.StatementActionReject,
			wantTarget: domain.StatementStatusRejected,
			wantErr:    false,
		},
		{
			name:       "reject accepted -> rejected",
			current:    domain.StatementStatusAccepted,
			action:     domain.StatementActionReject,
			wantTarget: domain.StatementStatusRejected,
			wantErr:    false,
		},

		// Review
		{
			name:       "review quarantined -> under_review",
			current:    domain.StatementStatusQuarantined,
			action:     domain.StatementActionReview,
			wantTarget: domain.StatementStatusUnderReview,
			wantErr:    false,
		},
		{
			name:       "review accepted -> error",
			current:    domain.StatementStatusAccepted,
			action:     domain.StatementActionReview,
			wantTarget: "",
			wantErr:    true,
		},

		// Archive
		{
			name:       "archive accepted -> archived",
			current:    domain.StatementStatusAccepted,
			action:     domain.StatementActionArchive,
			wantTarget: domain.StatementStatusArchived,
			wantErr:    false,
		},
		{
			name:       "archive rejected -> archived",
			current:    domain.StatementStatusRejected,
			action:     domain.StatementActionArchive,
			wantTarget: domain.StatementStatusArchived,
			wantErr:    false,
		},
		{
			name:       "archive quarantined -> archived",
			current:    domain.StatementStatusQuarantined,
			action:     domain.StatementActionArchive,
			wantTarget: domain.StatementStatusArchived,
			wantErr:    false,
		},

		// Invalid inputs
		{
			name:       "invalid status",
			current:    domain.StatementStatus("invalid"),
			action:     domain.StatementActionAccept,
			wantTarget: "",
			wantErr:    true,
		},
		{
			name:       "invalid action",
			current:    domain.StatementStatusQuarantined,
			action:     domain.StatementModerationAction("invalid"),
			wantTarget: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ValidateStatementTransition(tt.current, tt.action)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateStatementTransition(%q, %q) err = %v, wantErr = %v", tt.current, tt.action, err, tt.wantErr)
			}
			if got != tt.wantTarget {
				t.Errorf("ValidateStatementTransition(%q, %q) = %q, want %q", tt.current, tt.action, got, tt.wantTarget)
			}
		})
	}
}

func TestValidateStatementReasonAndActor(t *testing.T) {
	// Reason
	validReason, err := domain.ValidateStatementReason("Manifestação documental fundamentada e validada.")
	if err != nil || validReason == "" {
		t.Fatalf("ValidateStatementReason err = %v", err)
	}

	_, err = domain.ValidateStatementReason("")
	if err == nil {
		t.Errorf("ValidateStatementReason com string vazia deveria retornar erro")
	}

	_, err = domain.ValidateStatementReason("Motivo com <script>tag</script>")
	if err == nil {
		t.Errorf("ValidateStatementReason com HTML deveria retornar erro")
	}

	_, err = domain.ValidateStatementReason(strings.Repeat("a", 1001))
	if err == nil {
		t.Errorf("ValidateStatementReason > 1000 caracteres deveria retornar erro")
	}

	// Actor
	validActor, err := domain.ValidateStatementActor("moderador_1")
	if err != nil || validActor == "" {
		t.Fatalf("ValidateStatementActor err = %v", err)
	}

	_, err = domain.ValidateStatementActor("")
	if err == nil {
		t.Errorf("ValidateStatementActor com string vazia deveria retornar erro")
	}

	_, err = domain.ValidateStatementActor(strings.Repeat("a", 129))
	if err == nil {
		t.Errorf("ValidateStatementActor > 128 caracteres deveria retornar erro")
	}
}
