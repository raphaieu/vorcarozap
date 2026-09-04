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
