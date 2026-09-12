package domain_test

import (
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
