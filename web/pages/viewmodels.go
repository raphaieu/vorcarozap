package pages

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// Human translations
func FormatGrade(grade string) string {
	switch strings.ToUpper(strings.TrimSpace(grade)) {
	case "A":
		return "Grau A — Documento ou manifestação direta"
	case "B":
		return "Grau B — Reportagem fundamentada ou confirmação independente"
	case "C":
		return "Grau C — Associação documentada com significado incompleto"
	case "D":
		return "Grau D — Alegação atribuída sem confirmação independente"
	case "E":
		return "Grau E — Pista ou menção indireta"
	default:
		return grade
	}
}

func FormatGradeShort(grade string) string {
	switch strings.ToUpper(strings.TrimSpace(grade)) {
	case "A":
		return "Grau A (Documento direto)"
	case "B":
		return "Grau B (Reportagem fundamentada)"
	case "C":
		return "Grau C (Associação documentada)"
	case "D":
		return "Grau D (Alegação atribuída)"
	case "E":
		return "Grau E (Pista ou menção)"
	default:
		return grade
	}
}

func FormatRelevance(rel int64) string {
	switch rel {
	case 1:
		return "1 — Local ou circunstancial"
	case 2:
		return "2 — Setorial ou regional"
	case 3:
		return "3 — Nacional moderada"
	case 4:
		return "4 — Alta relevância nacional"
	case 5:
		return "5 — Estratégica ou internacional"
	default:
		return fmt.Sprintf("%d", rel)
	}
}

func FormatDisposition(disp string) string {
	switch strings.TrimSpace(disp) {
	case "supports_link":
		return "Sustenta vínculo"
	case "possible_link":
		return "Vínculo potencial"
	case "contradicts_link":
		return "Contesta vínculo"
	case "context_only":
		return "Apenas contexto"
	case "correction":
		return "Correção / esclarecimento"
	default:
		return disp
	}
}

func FormatDate(isoDate string) string {
	if strings.TrimSpace(isoDate) == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, isoDate)
	if err != nil {
		t, err = time.Parse(time.RFC3339, isoDate)
	}
	if err != nil {
		t, err = time.Parse("2006-01-02", isoDate)
	}
	if err != nil {
		return isoDate
	}
	return t.Format("02/01/2006")
}

// View models
type EntityCardVM struct {
	ID                  string
	Slug                string
	Name                string
	Category            string
	RoleOrContext       string
	Reach               string
	Relevance           int64
	RelevanceHuman      string
	RelevanceRationale  string
	PublicClaimsCount   int64
	HighestGrade        string
	HighestGradeHuman   string
	LastPublicUpdatedAt string
	LastUpdatedHuman    string
	ShortSynthesis      string
}

type FilterParamsVM struct {
	Search      string
	Category    string
	Grade       string
	Relevance   int
	Period      string
	PeriodSince string
	OrderBy     string
	OrderDir    string
	Page        int
	PageSize    int
	TotalPages  int
	TotalCount  int64
	Categories  []string
}

// BuildQueryString gera a query string para paginação preservando todos os filtros.
func (f FilterParamsVM) BuildQueryString(page int) string {
	v := url.Values{}
	if f.Search != "" {
		v.Set("q", f.Search)
	}
	if f.Category != "" {
		v.Set("category", f.Category)
	}
	if f.Grade != "" {
		v.Set("grade", f.Grade)
	}
	if f.Relevance > 0 {
		v.Set("relevance", fmt.Sprintf("%d", f.Relevance))
	}
	if f.Period != "" && f.Period != "all" {
		v.Set("period", f.Period)
	}
	if f.OrderBy != "" && f.OrderBy != store.OrderFieldName {
		v.Set("sort", f.OrderBy)
	}
	if f.OrderDir != "" && f.OrderDir != store.OrderDirAsc {
		v.Set("dir", f.OrderDir)
	}
	if page > 1 {
		v.Set("page", fmt.Sprintf("%d", page))
	}
	qs := v.Encode()
	if qs != "" {
		return "?" + qs
	}
	return ""
}

type EntityListVM struct {
	Entities []EntityCardVM
	Filter   FilterParamsVM
}

type PublicSourceVM struct {
	ID                string
	Title             string
	PublisherOrAuthor string
	OriginalURL       string
	CanonicalURL      string
	PublishedAtHuman  string
	AccessedAtHuman   string
	SourceType        string
	Excerpt           string
	Locator           string
	Role              string
	IsTitleEnriched   bool
	IsAuthorEnriched  bool
}

type PublicClaimVM struct {
	ID                  string
	RelationshipType    string
	RelationshipSummary string
	ContextLimits       string
	TargetName          string
	TargetSlug          string
	CaseName            string
	CaseSlug            string
	Proposition         string
	Attribution         string
	Grade               string
	GradeHuman          string
	Disposition         string
	DispositionHuman    string
	ContextStatus       string
	UpdatedAtHuman      string
	SupportsSources     []PublicSourceVM
	ContradictsSources  []PublicSourceVM
	ContextSources      []PublicSourceVM
}

type EntityDetailVM struct {
	ID                 string
	Slug               string
	Name               string
	Category           string
	RoleOrContext      string
	Reach              string
	Summary            string
	Relevance          int64
	RelevanceHuman     string
	RelevanceRationale string
	UpdatedAtHuman     string
	Claims             []PublicClaimVM
}

func ToEntityCardVM(row sqlc.ListPublicEntitiesRow) EntityCardVM {
	return EntityCardVM{
		ID:                  row.ID,
		Slug:                row.Slug,
		Name:                row.Name,
		Category:            row.Category,
		RoleOrContext:       row.RoleOrContext,
		Reach:               row.Reach,
		Relevance:           row.Relevance,
		RelevanceHuman:      FormatRelevance(row.Relevance),
		RelevanceRationale:  row.RelevanceRationale,
		PublicClaimsCount:   row.PublicClaimsCount,
		HighestGrade:        row.HighestGrade,
		HighestGradeHuman:   FormatGradeShort(row.HighestGrade),
		LastPublicUpdatedAt: row.LastPublicUpdatedAt,
		LastUpdatedHuman:    FormatDate(row.LastPublicUpdatedAt),
		ShortSynthesis:      row.ShortSynthesis,
	}
}

func ToPublicSourceVM(row sqlc.ListPublicEvidenceSourcesByClaimIDRow) PublicSourceVM {
	title := strings.TrimSpace(row.Title)
	publisher := strings.TrimSpace(row.PublisherOrAuthor)
	return PublicSourceVM{
		ID:                row.SourceID,
		Title:             title,
		PublisherOrAuthor: publisher,
		OriginalURL:       row.OriginalUrl,
		CanonicalURL:      row.CanonicalUrl,
		PublishedAtHuman:  FormatDate(row.PublishedAt.String),
		AccessedAtHuman:   FormatDate(row.AccessedAt.String),
		SourceType:        row.SourceType,
		Excerpt:           strings.TrimSpace(row.Excerpt),
		Locator:           strings.TrimSpace(row.Locator),
		Role:              row.Role,
		IsTitleEnriched:   title != "",
		IsAuthorEnriched:  publisher != "",
	}
}

func ToEntityDetailVM(detail *store.PublicEntityDetail) EntityDetailVM {
	var claimsVM []PublicClaimVM
	var lastUpdated string

	for _, c := range detail.Claims {
		var sup, contr, ctxSources []PublicSourceVM
		for _, s := range c.Sources {
			sVM := ToPublicSourceVM(s)
			switch s.Role {
			case "supports":
				sup = append(sup, sVM)
			case "contradicts":
				contr = append(contr, sVM)
			case "contextualizes":
				ctxSources = append(ctxSources, sVM)
			}
		}

		if c.Claim.UpdatedAt > lastUpdated {
			lastUpdated = c.Claim.UpdatedAt
		}

		claimsVM = append(claimsVM, PublicClaimVM{
			ID:                  c.Claim.ClaimID,
			RelationshipType:    c.Claim.RelationshipType,
			RelationshipSummary: c.Claim.RelationshipSummary,
			ContextLimits:       c.Claim.ContextLimits,
			TargetName:          c.Claim.TargetEntityName.String,
			TargetSlug:          c.Claim.TargetEntitySlug.String,
			CaseName:            c.Claim.CaseName.String,
			CaseSlug:            c.Claim.CaseSlug.String,
			Proposition:         c.Claim.Proposition,
			Attribution:         c.Claim.Attribution,
			Grade:               c.Claim.Grade,
			GradeHuman:          FormatGrade(c.Claim.Grade),
			Disposition:         c.Claim.Disposition,
			DispositionHuman:    FormatDisposition(c.Claim.Disposition),
			ContextStatus:       c.Claim.ContextStatus,
			UpdatedAtHuman:      FormatDate(c.Claim.UpdatedAt),
			SupportsSources:     sup,
			ContradictsSources:  contr,
			ContextSources:      ctxSources,
		})
	}

	if lastUpdated == "" {
		lastUpdated = detail.Entity.UpdatedAt
	}

	return EntityDetailVM{
		ID:                 detail.Entity.ID,
		Slug:               detail.Entity.Slug,
		Name:               detail.Entity.Name,
		Category:           detail.Entity.Category,
		RoleOrContext:      detail.Entity.RoleOrContext,
		Reach:              detail.Entity.Reach,
		Summary:            detail.Entity.Summary,
		Relevance:          detail.Entity.Relevance,
		RelevanceHuman:     FormatRelevance(detail.Entity.Relevance),
		RelevanceRationale: detail.Entity.RelevanceRationale,
		UpdatedAtHuman:     FormatDate(lastUpdated),
		Claims:             claimsVM,
	}
}
