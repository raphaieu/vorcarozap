package pages

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// Human translations e formatadores para a área administrativa
func FormatSourceAccessStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "reachable":
		return "Acessível (Confirmada)"
	case "unreachable":
		return "Inacessível"
	case "cited_by_provider":
		return "Citada pelo Provedor"
	case "not_checked":
		return "Não Verificada"
	default:
		return status
	}
}

func FormatEditorialStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "quarantined":
		return "Quarentena"
	case "published":
		return "Publicado"
	case "rejected":
		return "Rejeitado"
	case "archived":
		return "Arquivado"
	default:
		return status
	}
}

func FormatClaimOrigin(origin string) string {
	switch strings.ToLower(strings.TrimSpace(origin)) {
	case "curated_seed":
		return "Carga Curada"
	case "openrouter":
		return "OpenRouter"
	case "admin":
		return "Administrativo"
	default:
		return origin
	}
}

func FormatEvidenceRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "supports":
		return "Sustenta vínculo"
	case "contradicts":
		return "Contesta vínculo (Defesa)"
	case "contextualizes":
		return "Contextualiza"
	default:
		return role
	}
}

func FormatEvidenceSourceStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "Ativo"
	case "rejected":
		return "Desaprovado / Inativo"
	default:
		return status
	}
}

func FormatPolicyAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "publish":
		return "Publicar"
	case "quarantine":
		return "Quarentena"
	case "reject":
		return "Rejeitar"
	default:
		return action
	}
}

func FormatConfidence(c float64) string {
	if c <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", c*100)
}

func parseStringArrayJSON(raw string) []string {
	var list []string
	if strings.TrimSpace(raw) == "" {
		return list
	}
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

// Admin Dashboard VM
type AdminDashboardVM struct {
	TotalSources            int64
	SourcesReachable        int64
	SourcesUnreachable      int64
	SourcesNotChecked       int64
	SourcesCitedByProvider  int64
	TotalClaims             int64
	ClaimsPublished         int64
	ClaimsQuarantined       int64
	ClaimsRejected          int64
	ClaimsArchived          int64
	TotalEvidenceSources    int64
	EvidenceSourcesActive   int64
	EvidenceSourcesRejected int64
	TotalCandidates         int64
	CandidatesQuarantined   int64
	CandidatesPublished     int64
	CandidatesRejected      int64
	CandidatesArchived      int64
	CandidatesDuplicates    int64
	TotalRuns               int64
	RunsCompleted           int64
	RunsPartial             int64
	RunsRunning             int64
	RunsFailed              int64
	RunsPending             int64
	TotalEntities           int64
}

func ToAdminDashboardVM(counts *sqlc.GetAdminOverviewCountsRow) AdminDashboardVM {
	if counts == nil {
		return AdminDashboardVM{}
	}
	return AdminDashboardVM{
		TotalSources:            counts.TotalSources,
		SourcesReachable:        counts.SourcesReachable,
		SourcesUnreachable:      counts.SourcesUnreachable,
		SourcesNotChecked:       counts.SourcesNotChecked,
		SourcesCitedByProvider:  counts.SourcesCitedByProvider,
		TotalClaims:             counts.TotalClaims,
		ClaimsPublished:         counts.ClaimsPublished,
		ClaimsQuarantined:       counts.ClaimsQuarantined,
		ClaimsRejected:          counts.ClaimsRejected,
		ClaimsArchived:          counts.ClaimsArchived,
		TotalEvidenceSources:    counts.TotalEvidenceSources,
		EvidenceSourcesActive:   counts.EvidenceSourcesActive,
		EvidenceSourcesRejected: counts.EvidenceSourcesRejected,
		TotalCandidates:         counts.TotalCandidates,
		CandidatesQuarantined:   counts.CandidatesQuarantined,
		CandidatesPublished:     counts.CandidatesPublished,
		CandidatesRejected:      counts.CandidatesRejected,
		CandidatesArchived:      counts.CandidatesArchived,
		CandidatesDuplicates:    counts.CandidatesDuplicates,
		TotalRuns:               counts.TotalRuns,
		RunsCompleted:           counts.RunsCompleted,
		RunsPartial:             counts.RunsPartial,
		RunsRunning:             counts.RunsRunning,
		RunsFailed:              counts.RunsFailed,
		RunsPending:             counts.RunsPending,
		TotalEntities:           counts.TotalEntities,
	}
}

// ExternalLinkVM encapsula e higieniza links para recursos externos, prevenindo esquemas inseguros (ex: javascript:).
type ExternalLinkVM struct {
	RawURL  string // URL original bruta para exibição textual caso inválida
	SafeURL string // URL absoluta http/https higienizada
	IsValid bool   // Verdadeiro somente se a URL tiver esquema http/https, host não vazio e sem credenciais (userinfo)
}

// SanitizeExternalLink valida se uma URL é segura para navegação externa.
func SanitizeExternalLink(raw string) ExternalLinkVM {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ExternalLinkVM{RawURL: "", SafeURL: "", IsValid: false}
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return ExternalLinkVM{RawURL: trimmed, SafeURL: "", IsValid: false}
	}

	scheme := strings.ToLower(u.Scheme)
	// Aceita exclusivamente http ou https absoluto, com host não-vazio e sem credenciais na URL (userinfo)
	if (scheme != "http" && scheme != "https") || u.Host == "" || u.User != nil {
		return ExternalLinkVM{RawURL: trimmed, SafeURL: "", IsValid: false}
	}

	return ExternalLinkVM{
		RawURL:  trimmed,
		SafeURL: u.String(),
		IsValid: true,
	}
}

// Candidates View Models
type AdminCandidateItemVM struct {
	ID                   string
	MonitoringRunID      string
	Fingerprint          string
	EntityName           string
	TargetEntityName     string
	CaseName             string
	RelationshipType     string
	Proposition          string
	SuggestedGrade       string
	SuggestedGradeHuman  string
	SourceURL            ExternalLinkVM
	CanonicalURL         ExternalLinkVM
	SourceTitle          string
	PublisherOrAuthor    string
	PublishedAtHuman     string
	Excerpt              string
	Locator              string
	ContextLimits        string
	TechnicalConfidence  float64
	ConfidenceHuman      string
	EditorialStatus      string
	EditorialStatusHuman string
	IsDuplicate          bool
	DuplicateReason      string
	CanonicalCandidateID string
	StructuralGatePassed sql.NullInt64
	SemanticGatePassed   sql.NullInt64
	PolicyAction         string
	PolicyActionHuman    string
	CreatedAtHuman       string
	UpdatedAtHuman       string
	RunQuery             string
	RunDiscoveryModel    string
}

type AdminCandidateFilterVM struct {
	Status      string
	Grade       string
	RunID       string
	Period      string
	PeriodSince string
	Search      string
	Page        int
	PageSize    int
	TotalPages  int
	TotalCount  int64
}

func (f AdminCandidateFilterVM) BuildQueryString(page int) string {
	v := url.Values{}
	if f.Status != "" {
		v.Set("status", f.Status)
	}
	if f.Grade != "" {
		v.Set("grade", f.Grade)
	}
	if f.RunID != "" {
		v.Set("run_id", f.RunID)
	}
	if f.Period != "" && f.Period != "all" {
		v.Set("period", f.Period)
	}
	if f.Search != "" {
		v.Set("q", f.Search)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	qs := v.Encode()
	if qs != "" {
		return "?" + qs
	}
	return ""
}

type AdminCandidateListVM struct {
	Candidates []AdminCandidateItemVM
	Filter     AdminCandidateFilterVM
}

func ToAdminCandidateItemVM(c sqlc.ListAdminCandidatesRow) AdminCandidateItemVM {
	return AdminCandidateItemVM{
		ID:                   c.ID,
		MonitoringRunID:      c.MonitoringRunID,
		Fingerprint:          c.Fingerprint,
		EntityName:           c.EntityName,
		TargetEntityName:     c.TargetEntityName,
		CaseName:             c.CaseName,
		RelationshipType:     c.RelationshipType,
		Proposition:          c.Proposition,
		SuggestedGrade:       c.SuggestedGrade,
		SuggestedGradeHuman:  FormatGrade(c.SuggestedGrade),
		SourceURL:            SanitizeExternalLink(c.SourceUrl),
		CanonicalURL:         SanitizeExternalLink(c.CanonicalUrl),
		SourceTitle:          c.SourceTitle,
		PublisherOrAuthor:    c.PublisherOrAuthor,
		PublishedAtHuman:     FormatDate(c.PublishedAt),
		Excerpt:              c.Excerpt,
		Locator:              c.Locator,
		ContextLimits:        c.ContextLimits,
		TechnicalConfidence:  c.TechnicalConfidence,
		ConfidenceHuman:      FormatConfidence(c.TechnicalConfidence),
		EditorialStatus:      c.EditorialStatus,
		EditorialStatusHuman: FormatEditorialStatus(c.EditorialStatus),
		IsDuplicate:          c.IsDuplicate == 1,
		DuplicateReason:      c.DuplicateReason,
		CanonicalCandidateID: c.CanonicalCandidateID,
		StructuralGatePassed: c.StructuralGatePassed,
		SemanticGatePassed:   c.SemanticGatePassed,
		PolicyAction:         c.PolicyAction,
		PolicyActionHuman:    FormatPolicyAction(c.PolicyAction),
		CreatedAtHuman:       FormatDate(c.CreatedAt),
		UpdatedAtHuman:       FormatDate(c.UpdatedAt),
		RunQuery:             c.RunQuery,
		RunDiscoveryModel:    c.RunDiscoveryModel,
	}
}

// Candidate Detail VM
type SemanticEvaluationItemVM struct {
	ID                       string
	MonitoringCandidateID    string
	Provider                 string
	Model                    string
	SchemaVersion            string
	IdentityMatch            bool
	ClaimSupported           bool
	ClaimOverstatesSource    bool
	AttributionExplicit      bool
	GradeCompatible          bool
	ContainsIllicitInference bool
	Uncertainties            []string
	RecommendedAction        string
	RecommendedActionHuman   string
	CreatedAtHuman           string
}

type AdminCandidateDetailVM struct {
	Candidate                 AdminCandidateItemVM
	StructuralGateReasons     []string
	SemanticGateReasons       []string
	PolicyReasons             []string
	Evaluations               []SemanticEvaluationItemVM
	ResolvedSubjectName       string
	ResolvedSubjectSlug       string
	ResolvedTargetName        string
	ResolvedTargetSlug        string
	ResolvedCaseName          string
	ResolvedCaseSlug          string
	PublishedClaimID          string
	PublishedClaimStatus      string
	PublishedClaimStatusHuman string
	PublishedClaimGrade       string
	PublishedClaimGradeHuman  string
	PublishedClaimDisposition string
	PublishedClaimDispHuman   string
	RunQuery                  string
	RunStatus                 string
	RunDiscoveryProvider      string
	RunDiscoveryModel         string
	RunVerificationProvider   string
	RunVerificationModel      string
	RunCreatedAtHuman         string
}

func ToAdminCandidateDetailVM(detail *store.AdminCandidateDetail) AdminCandidateDetailVM {
	if detail == nil {
		return AdminCandidateDetailVM{}
	}

	c := detail.Candidate
	candidateItem := AdminCandidateItemVM{
		ID:                   c.ID,
		MonitoringRunID:      c.MonitoringRunID,
		Fingerprint:          c.Fingerprint,
		EntityName:           c.EntityName,
		TargetEntityName:     c.TargetEntityName,
		CaseName:             c.CaseName,
		RelationshipType:     c.RelationshipType,
		Proposition:          c.Proposition,
		SuggestedGrade:       c.SuggestedGrade,
		SuggestedGradeHuman:  FormatGrade(c.SuggestedGrade),
		SourceURL:            SanitizeExternalLink(c.SourceUrl),
		CanonicalURL:         SanitizeExternalLink(c.CanonicalUrl),
		SourceTitle:          c.SourceTitle,
		PublisherOrAuthor:    c.PublisherOrAuthor,
		PublishedAtHuman:     FormatDate(c.PublishedAt),
		Excerpt:              c.Excerpt,
		Locator:              c.Locator,
		ContextLimits:        c.ContextLimits,
		TechnicalConfidence:  c.TechnicalConfidence,
		ConfidenceHuman:      FormatConfidence(c.TechnicalConfidence),
		EditorialStatus:      c.EditorialStatus,
		EditorialStatusHuman: FormatEditorialStatus(c.EditorialStatus),
		IsDuplicate:          c.IsDuplicate == 1,
		DuplicateReason:      c.DuplicateReason,
		CanonicalCandidateID: c.CanonicalCandidateID,
		StructuralGatePassed: c.StructuralGatePassed,
		SemanticGatePassed:   c.SemanticGatePassed,
		PolicyAction:         c.PolicyAction,
		PolicyActionHuman:    FormatPolicyAction(c.PolicyAction),
		CreatedAtHuman:       FormatDate(c.CreatedAt),
		UpdatedAtHuman:       FormatDate(c.UpdatedAt),
		RunQuery:             c.RunQuery,
		RunDiscoveryModel:    c.RunDiscoveryModel,
	}

	var evalsVM []SemanticEvaluationItemVM
	for _, se := range detail.Evaluations {
		evalsVM = append(evalsVM, SemanticEvaluationItemVM{
			ID:                       se.ID,
			MonitoringCandidateID:    se.MonitoringCandidateID,
			Provider:                 se.Provider,
			Model:                    se.Model,
			SchemaVersion:            se.SchemaVersion,
			IdentityMatch:            se.IdentityMatch == 1,
			ClaimSupported:           se.ClaimSupported == 1,
			ClaimOverstatesSource:    se.ClaimOverstatesSource == 1,
			AttributionExplicit:      se.AttributionExplicit == 1,
			GradeCompatible:          se.GradeCompatible == 1,
			ContainsIllicitInference: se.ContainsIllicitInference == 1,
			Uncertainties:            parseStringArrayJSON(se.Uncertainties),
			RecommendedAction:        se.RecommendedAction,
			RecommendedActionHuman:   FormatPolicyAction(se.RecommendedAction),
			CreatedAtHuman:           FormatDate(se.CreatedAt),
		})
	}

	return AdminCandidateDetailVM{
		Candidate:                 candidateItem,
		StructuralGateReasons:     parseStringArrayJSON(c.StructuralGateReasons),
		SemanticGateReasons:       parseStringArrayJSON(c.SemanticGateReasons),
		PolicyReasons:             parseStringArrayJSON(c.PolicyReasons),
		Evaluations:               evalsVM,
		ResolvedSubjectName:       c.ResolvedSubjectName,
		ResolvedSubjectSlug:       c.ResolvedSubjectSlug,
		ResolvedTargetName:        c.ResolvedTargetName,
		ResolvedTargetSlug:        c.ResolvedTargetSlug,
		ResolvedCaseName:          c.ResolvedCaseName,
		ResolvedCaseSlug:          c.ResolvedCaseSlug,
		PublishedClaimID:          c.PublishedClaimID,
		PublishedClaimStatus:      c.PublishedClaimStatus,
		PublishedClaimStatusHuman: FormatEditorialStatus(c.PublishedClaimStatus),
		PublishedClaimGrade:       c.PublishedClaimGrade,
		PublishedClaimGradeHuman:  FormatGradeShort(c.PublishedClaimGrade),
		PublishedClaimDisposition: c.PublishedClaimDisposition,
		PublishedClaimDispHuman:   FormatDisposition(c.PublishedClaimDisposition),
		RunQuery:                  c.RunQuery,
		RunStatus:                 c.RunStatus,
		RunDiscoveryProvider:      c.RunDiscoveryProvider,
		RunDiscoveryModel:         c.RunDiscoveryModel,
		RunVerificationProvider:   c.RunVerificationProvider,
		RunVerificationModel:      c.RunVerificationModel,
		RunCreatedAtHuman:         FormatDate(c.RunCreatedAt),
	}
}

// Evidence Sources View Models
type AdminEvidenceItemVM struct {
	EvidenceSourceID           string
	EvidenceID                 string
	SourceID                   string
	Excerpt                    string
	Locator                    string
	Role                       string
	RoleHuman                  string
	EvidenceSourceStatus       string
	EvidenceSourceStatusHuman  string
	EvidenceSourceCreatedHuman string
	EvidenceSummary            string
	EvidenceType               string
	ClaimID                    string
	ClaimProposition           string
	ClaimAttribution           string
	ClaimOrigin                string
	ClaimOriginHuman           string
	ClaimGrade                 string
	ClaimGradeHuman            string
	ClaimDisposition           string
	ClaimDispositionHuman      string
	ClaimMetricEligible        bool
	ClaimStatus                string
	ClaimStatusHuman           string
	ClaimContextStatus         string
	ClaimCreatedHuman          string
	RelationshipID             string
	RelationshipType           string
	RelationshipSummary        string
	RelationshipContextLimits  string
	SubjectEntityID            string
	SubjectEntityName          string
	SubjectEntitySlug          string
	TargetEntityID             string
	TargetEntityName           string
	TargetEntitySlug           string
	CaseID                     string
	CaseName                   string
	CaseSlug                   string
	SourceTitle                string
	SourcePublisher            string
	SourceOriginalURL          ExternalLinkVM
	SourceCanonicalURL         ExternalLinkVM
	SourceType                 string
	SourceAccessStatus         string
	SourceAccessStatusHuman    string
	SourceCheckedAtHuman       string
	SourceHTTPStatus           sql.NullInt64
	SourceErrorCode            string
}

type AdminEvidenceFilterVM struct {
	ClaimStatus          string
	EvidenceSourceStatus string
	Origin               string
	Grade                string
	Role                 string
	Period               string
	PeriodSince          string
	Search               string
	Page                 int
	PageSize             int
	TotalPages           int
	TotalCount           int64
}

func (f AdminEvidenceFilterVM) BuildQueryString(page int) string {
	v := url.Values{}
	if f.ClaimStatus != "" {
		v.Set("claim_status", f.ClaimStatus)
	}
	if f.EvidenceSourceStatus != "" {
		v.Set("es_status", f.EvidenceSourceStatus)
	}
	if f.Origin != "" {
		v.Set("origin", f.Origin)
	}
	if f.Grade != "" {
		v.Set("grade", f.Grade)
	}
	if f.Role != "" {
		v.Set("role", f.Role)
	}
	if f.Period != "" && f.Period != "all" {
		v.Set("period", f.Period)
	}
	if f.Search != "" {
		v.Set("q", f.Search)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	qs := v.Encode()
	if qs != "" {
		return "?" + qs
	}
	return ""
}

type AdminEvidenceListVM struct {
	EvidenceSources []AdminEvidenceItemVM
	Filter          AdminEvidenceFilterVM
}

func ToAdminEvidenceItemVM(row sqlc.ListAdminEvidenceSourcesRow) AdminEvidenceItemVM {
	return AdminEvidenceItemVM{
		EvidenceSourceID:           row.EvidenceSourceID,
		EvidenceID:                 row.EvidenceID,
		SourceID:                   row.SourceID,
		Excerpt:                    row.Excerpt,
		Locator:                    row.Locator,
		Role:                       row.Role,
		RoleHuman:                  FormatEvidenceRole(row.Role),
		EvidenceSourceStatus:       row.EvidenceSourceStatus,
		EvidenceSourceStatusHuman:  FormatEvidenceSourceStatus(row.EvidenceSourceStatus),
		EvidenceSourceCreatedHuman: FormatDate(row.EvidenceSourceCreatedAt),
		EvidenceSummary:            row.EvidenceSummary,
		EvidenceType:               row.EvidenceType,
		ClaimID:                    row.ClaimID,
		ClaimProposition:           row.ClaimProposition,
		ClaimAttribution:           row.ClaimAttribution,
		ClaimOrigin:                row.ClaimOrigin,
		ClaimOriginHuman:           FormatClaimOrigin(row.ClaimOrigin),
		ClaimGrade:                 row.ClaimGrade,
		ClaimGradeHuman:            FormatGradeShort(row.ClaimGrade),
		ClaimDisposition:           row.ClaimDisposition,
		ClaimDispositionHuman:      FormatDisposition(row.ClaimDisposition),
		ClaimMetricEligible:        row.ClaimMetricEligible == 1,
		ClaimStatus:                row.ClaimStatus,
		ClaimStatusHuman:           FormatEditorialStatus(row.ClaimStatus),
		ClaimContextStatus:         row.ClaimContextStatus,
		ClaimCreatedHuman:          FormatDate(row.ClaimCreatedAt),
		RelationshipID:             row.RelationshipID,
		RelationshipType:           row.RelationshipType,
		RelationshipSummary:        row.RelationshipSummary,
		RelationshipContextLimits:  row.RelationshipContextLimits,
		SubjectEntityID:            row.SubjectEntityID,
		SubjectEntityName:          row.SubjectEntityName,
		SubjectEntitySlug:          row.SubjectEntitySlug,
		TargetEntityID:             row.TargetEntityID,
		TargetEntityName:           row.TargetEntityName,
		TargetEntitySlug:           row.TargetEntitySlug,
		CaseID:                     row.CaseID,
		CaseName:                   row.CaseName,
		CaseSlug:                   row.CaseSlug,
		SourceTitle:                row.SourceTitle,
		SourcePublisher:            row.SourcePublisherOrAuthor,
		SourceOriginalURL:          SanitizeExternalLink(row.SourceOriginalUrl),
		SourceCanonicalURL:         SanitizeExternalLink(row.SourceCanonicalUrl),
		SourceType:                 row.SourceType,
		SourceAccessStatus:         row.SourceAccessStatus,
		SourceAccessStatusHuman:    FormatSourceAccessStatus(row.SourceAccessStatus),
		SourceCheckedAtHuman:       FormatDate(row.SourceAccessCheckedAt),
		SourceHTTPStatus:           row.SourceHttpStatus,
		SourceErrorCode:            row.SourceNormalizedErrorCode,
	}
}

// Sources View Models
type AdminSourceItemVM struct {
	ID                      string
	Title                   string
	PublisherOrAuthor       string
	OriginalURL             ExternalLinkVM
	CanonicalURL            ExternalLinkVM
	PublishedAtHuman        string
	AccessedAtHuman         string
	CreatedAtHuman          string
	SourceType              string
	SourceAccessStatus      string
	SourceAccessStatusHuman string
	SourceCheckedAtHuman    string
	HTTPStatus              sql.NullInt64
	NormalizedErrorCode     string
	TotalUses               int64
	ActiveUses              int64
}

type AdminSourceFilterVM struct {
	AccessStatus string
	SourceType   string
	Period       string
	PeriodSince  string
	Search       string
	Page         int
	PageSize     int
	TotalPages   int
	TotalCount   int64
}

func (f AdminSourceFilterVM) BuildQueryString(page int) string {
	v := url.Values{}
	if f.AccessStatus != "" {
		v.Set("access_status", f.AccessStatus)
	}
	if f.SourceType != "" {
		v.Set("source_type", f.SourceType)
	}
	if f.Period != "" && f.Period != "all" {
		v.Set("period", f.Period)
	}
	if f.Search != "" {
		v.Set("q", f.Search)
	}
	if page > 1 {
		v.Set("page", strconv.Itoa(page))
	}
	qs := v.Encode()
	if qs != "" {
		return "?" + qs
	}
	return ""
}

// AdminSourceTypeOptionVM representa uma opção de tipo de fonte para o select administrativo.
type AdminSourceTypeOptionVM struct {
	Value string
	Label string
}

// GetAdminSourceTypeOptionsVM converte as opções canônicas de tipo de fonte para o view-model.
func GetAdminSourceTypeOptionsVM() []AdminSourceTypeOptionVM {
	opts := store.GetAdminSourceTypeOptions()
	vms := make([]AdminSourceTypeOptionVM, len(opts))
	for i, opt := range opts {
		vms[i] = AdminSourceTypeOptionVM{
			Value: opt.Value,
			Label: opt.Label,
		}
	}
	return vms
}

type AdminSourceListVM struct {
	Sources           []AdminSourceItemVM
	Filter            AdminSourceFilterVM
	SourceTypeOptions []AdminSourceTypeOptionVM
}

func ToAdminSourceItemVM(s sqlc.ListAdminSourcesRow) AdminSourceItemVM {
	return AdminSourceItemVM{
		ID:                      s.ID,
		Title:                   s.Title,
		PublisherOrAuthor:       s.PublisherOrAuthor,
		OriginalURL:             SanitizeExternalLink(s.OriginalUrl),
		CanonicalURL:            SanitizeExternalLink(s.CanonicalUrl),
		PublishedAtHuman:        FormatDate(s.PublishedAt),
		AccessedAtHuman:         FormatDate(s.AccessedAt),
		CreatedAtHuman:          FormatDate(s.CreatedAt),
		SourceType:              s.SourceType,
		SourceAccessStatus:      s.SourceAccessStatus,
		SourceAccessStatusHuman: FormatSourceAccessStatus(s.SourceAccessStatus),
		SourceCheckedAtHuman:    FormatDate(s.SourceAccessCheckedAt),
		HTTPStatus:              s.HttpStatus,
		NormalizedErrorCode:     s.NormalizedErrorCode,
		TotalUses:               s.TotalUses,
		ActiveUses:              s.ActiveUses,
	}
}
