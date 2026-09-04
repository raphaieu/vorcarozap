package domain

import (
	"fmt"
)

// EntityType define a natureza de uma entidade registrada.
type EntityType string

const (
	EntityTypePerson       EntityType = "person"
	EntityTypeOrganization EntityType = "organization"
)

func (t EntityType) IsValid() bool {
	switch t {
	case EntityTypePerson, EntityTypeOrganization:
		return true
	default:
		return false
	}
}

// Relevance representa o alcance factual da entidade na escala 1 a 5.
// 1: local/circunstancial
// 2: setorial/regional
// 3: nacional moderada
// 4: alta nacional
// 5: estratégica/internacional
type Relevance int

const (
	RelevanceMin Relevance = 1
	RelevanceMax Relevance = 5
)

func (r Relevance) IsValid() bool {
	return r >= RelevanceMin && r <= RelevanceMax
}

// ClaimOrigin define a procedência de uma alegação.
type ClaimOrigin string

const (
	ClaimOriginOpenRouter  ClaimOrigin = "openrouter"
	ClaimOriginCuratedSeed ClaimOrigin = "curated_seed"
	ClaimOriginAdmin       ClaimOrigin = "admin"
)

func (o ClaimOrigin) IsValid() bool {
	switch o {
	case ClaimOriginOpenRouter, ClaimOriginCuratedSeed, ClaimOriginAdmin:
		return true
	default:
		return false
	}
}

// EvidenceGrade representa a força pública da alegação na escala A a E.
// Pertence exclusivamente ao claim.
type EvidenceGrade string

const (
	EvidenceGradeA EvidenceGrade = "A"
	EvidenceGradeB EvidenceGrade = "B"
	EvidenceGradeC EvidenceGrade = "C"
	EvidenceGradeD EvidenceGrade = "D"
	EvidenceGradeE EvidenceGrade = "E"
)

func (g EvidenceGrade) IsValid() bool {
	switch g {
	case EvidenceGradeA, EvidenceGradeB, EvidenceGradeC, EvidenceGradeD, EvidenceGradeE:
		return true
	default:
		return false
	}
}

// ClaimDisposition define o papel editorial da alegação em relação ao vínculo.
type ClaimDisposition string

const (
	DispositionSupportsLink    ClaimDisposition = "supports_link"
	DispositionPossibleLink    ClaimDisposition = "possible_link"
	DispositionContradictsLink ClaimDisposition = "contradicts_link"
	DispositionContextOnly     ClaimDisposition = "context_only"
	DispositionCorrection      ClaimDisposition = "correction"
)

func (d ClaimDisposition) IsValid() bool {
	switch d {
	case DispositionSupportsLink, DispositionPossibleLink, DispositionContradictsLink,
		DispositionContextOnly, DispositionCorrection:
		return true
	default:
		return false
	}
}

// ClaimStatus define o estado de visibilidade editorial da alegação.
type ClaimStatus string

const (
	ClaimStatusQuarantined ClaimStatus = "quarantined"
	ClaimStatusPublished   ClaimStatus = "published"
	ClaimStatusRejected    ClaimStatus = "rejected"
	ClaimStatusArchived    ClaimStatus = "archived"
)

func (s ClaimStatus) IsValid() bool {
	switch s {
	case ClaimStatusQuarantined, ClaimStatusPublished, ClaimStatusRejected, ClaimStatusArchived:
		return true
	default:
		return false
	}
}

// EvidenceSourceRole define o papel do trecho da fonte em relação à evidência.
type EvidenceSourceRole string

const (
	RoleSupports       EvidenceSourceRole = "supports"
	RoleContradicts    EvidenceSourceRole = "contradicts"
	RoleContextualizes EvidenceSourceRole = "contextualizes"
)

func (r EvidenceSourceRole) IsValid() bool {
	switch r {
	case RoleSupports, RoleContradicts, RoleContextualizes:
		return true
	default:
		return false
	}
}

// EvidenceSourceStatus define o estado editorial da citação/uso da fonte.
type EvidenceSourceStatus string

const (
	EvidenceSourceStatusActive   EvidenceSourceStatus = "active"
	EvidenceSourceStatusRejected EvidenceSourceStatus = "rejected"
)

func (s EvidenceSourceStatus) IsValid() bool {
	switch s {
	case EvidenceSourceStatusActive, EvidenceSourceStatusRejected:
		return true
	default:
		return false
	}
}

// SourceAccessStatus define o resultado da verificação de acessibilidade da fonte.
type SourceAccessStatus string

const (
	SourceAccessCitedByProvider SourceAccessStatus = "cited_by_provider"
	SourceAccessReachable       SourceAccessStatus = "reachable"
	SourceAccessUnreachable     SourceAccessStatus = "unreachable"
	SourceAccessNotChecked      SourceAccessStatus = "not_checked"
)

func (s SourceAccessStatus) IsValid() bool {
	switch s {
	case SourceAccessCitedByProvider, SourceAccessReachable, SourceAccessUnreachable, SourceAccessNotChecked:
		return true
	default:
		return false
	}
}

// ImportRunStatus define o estado operacional de uma execução de importação.
type ImportRunStatus string

const (
	ImportRunStatusPending   ImportRunStatus = "pending"
	ImportRunStatusRunning   ImportRunStatus = "running"
	ImportRunStatusDryRun    ImportRunStatus = "dry_run"
	ImportRunStatusCompleted ImportRunStatus = "completed"
	ImportRunStatusPartial   ImportRunStatus = "partial"
	ImportRunStatusFailed    ImportRunStatus = "failed"
)

func (s ImportRunStatus) IsValid() bool {
	switch s {
	case ImportRunStatusPending, ImportRunStatusRunning, ImportRunStatusDryRun,
		ImportRunStatusCompleted, ImportRunStatusPartial, ImportRunStatusFailed:
		return true
	default:
		return false
	}
}

// ValidateRelevance valida se o valor de relevância está no intervalo 1..5.
func ValidateRelevance(r int) (Relevance, error) {
	rel := Relevance(r)
	if !rel.IsValid() {
		return 0, fmt.Errorf("domain: relevância inválida %d: deve estar entre 1 e 5", r)
	}
	return rel, nil
}
