package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
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

// SourceType define a natureza e o tipo canônico de uma fonte documental registrada.
type SourceType string

const (
	SourceTypeArticle           SourceType = "article"
	SourceTypeOfficialStatement SourceType = "official_statement"
	SourceTypeCourtDocument     SourceType = "court_document"
	SourceTypePoliceReport      SourceType = "police_report"
	SourceTypeInterview         SourceType = "interview"
	SourceTypeSocialMedia       SourceType = "social_media"
)

// CanonicalSourceTypes define a lista canônica, ordenada e imutável de todos os tipos de fonte reconhecidos.
var CanonicalSourceTypes = []SourceType{
	SourceTypeArticle,
	SourceTypeOfficialStatement,
	SourceTypeCourtDocument,
	SourceTypePoliceReport,
	SourceTypeInterview,
	SourceTypeSocialMedia,
}

func (t SourceType) IsValid() bool {
	switch t {
	case SourceTypeArticle, SourceTypeOfficialStatement, SourceTypeCourtDocument,
		SourceTypePoliceReport, SourceTypeInterview, SourceTypeSocialMedia:
		return true
	default:
		return false
	}
}

// IsPrimaryDocument indica se o tipo de fonte é um documento primário oficial (peça judicial, laudo pericial ou comunicado oficial).
func (t SourceType) IsPrimaryDocument() bool {
	switch t {
	case SourceTypeCourtDocument, SourceTypePoliceReport, SourceTypeOfficialStatement:
		return true
	default:
		return false
	}
}

// Label retorna o rótulo descritivo em português para o tipo de fonte.
func (t SourceType) Label() string {
	switch t {
	case SourceTypeArticle:
		return "Artigo / Notícia"
	case SourceTypeOfficialStatement:
		return "Nota Oficial / Comunicado"
	case SourceTypeCourtDocument:
		return "Peça Judicial / Decisão"
	case SourceTypePoliceReport:
		return "Relatório Policial / Pericial"
	case SourceTypeInterview:
		return "Entrevista"
	case SourceTypeSocialMedia:
		return "Rede Social"
	default:
		return string(t)
	}
}

// NatureLabel retorna a classificação documental da fonte (primária vs secundária).
func (t SourceType) NatureLabel() string {
	if t.IsPrimaryDocument() {
		return "Documento Oficial / Primário"
	}
	switch t {
	case SourceTypeArticle, SourceTypeInterview:
		return "Reportagem / Apuração Jornalística"
	case SourceTypeSocialMedia:
		return "Publicação em Rede Social"
	default:
		return "Referência Documental"
	}
}

// DocumentSequenceItem representa um item atômico na sequência contextual de um documento.
type DocumentSequenceItem struct {
	ID                  string
	EvidenceID          string
	Excerpt             string
	Locator             string
	Role                EvidenceSourceRole
	Status              EvidenceSourceStatus
	ClaimID             string
	ClaimProposition    string
	ClaimGrade          EvidenceGrade
	ClaimDisposition    ClaimDisposition
	ClaimStatus         ClaimStatus
	ClaimMetricEligible bool
	RelationshipType    string
	RelationshipSummary string
	ContextLimits       string
	SubjectEntityID     string
	SubjectEntityName   string
	SubjectEntitySlug   string
	TargetEntityID      string
	TargetEntityName    string
	TargetEntitySlug    string
	CaseID              string
	CaseName            string
	CaseSlug            string
	CreatedAt           string
	UpdatedAt           string
}

// DocumentSourceDetail representa uma fonte documental completa com sua sequência contextual de usos.
type DocumentSourceDetail struct {
	ID                    string
	Title                 string
	PublisherOrAuthor     string
	OriginalURL           string
	CanonicalURL          string
	PublishedAt           string
	AccessedAt            string
	SourceType            SourceType
	SourceAccessStatus    SourceAccessStatus
	SourceAccessCheckedAt string
	HTTPStatus            int
	NormalizedErrorCode   string
	CreatedAt             string
	UpdatedAt             string
	Sequence              []DocumentSequenceItem
	EditorialHistory      []PublicEditorialEvent
}

// IntegritySummary retorna o resumo humano da integridade técnica observada para a fonte.
func (d DocumentSourceDetail) IntegritySummary() string {
	switch d.SourceAccessStatus {
	case SourceAccessReachable:
		return "Conteúdo observado acessível na checagem técnica"
	case SourceAccessUnreachable:
		return "Fonte indisponível ou inacessível na checagem técnica"
	case SourceAccessCitedByProvider:
		return "Citada pelo provedor na descoberta de monitoramento"
	default:
		return "Acessibilidade técnica pendente de verificação"
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

// ModerationAction define uma ação humana deliberada de moderação editorial (VZ-016).
type ModerationAction string

const (
	ModerationActionApprove ModerationAction = "approve"
	ModerationActionReject  ModerationAction = "reject"
	ModerationActionRestore ModerationAction = "restore"
)

func (a ModerationAction) IsValid() bool {
	switch a {
	case ModerationActionApprove, ModerationActionReject, ModerationActionRestore:
		return true
	default:
		return false
	}
}

// ValidateClaimTransition valida se a transição de estado solicitada para uma alegação é permitida pelo modelo editorial.
// Retorna o novo ClaimStatus resultante ou erro caso a transição seja proibida.
func ValidateClaimTransition(current ClaimStatus, action ModerationAction) (ClaimStatus, error) {
	if !current.IsValid() {
		return "", fmt.Errorf("domain: estado atual do claim inválido %q", current)
	}
	if !action.IsValid() {
		return "", fmt.Errorf("domain: ação de moderação inválida %q", action)
	}

	switch action {
	case ModerationActionApprove:
		if current == ClaimStatusQuarantined {
			return ClaimStatusPublished, nil
		}
		return "", fmt.Errorf("domain: ação 'approve' não é permitida para claim com status %q (permitida apenas para %q)", current, ClaimStatusQuarantined)

	case ModerationActionReject:
		if current == ClaimStatusPublished || current == ClaimStatusQuarantined {
			return ClaimStatusRejected, nil
		}
		return "", fmt.Errorf("domain: ação 'reject' não é permitida para claim com status %q (permitida apenas para %q e %q)", current, ClaimStatusPublished, ClaimStatusQuarantined)

	case ModerationActionRestore:
		if current == ClaimStatusRejected || current == ClaimStatusArchived {
			return ClaimStatusQuarantined, nil
		}
		return "", fmt.Errorf("domain: ação 'restore' não é permitida para claim com status %q (permitida apenas para %q e %q)", current, ClaimStatusRejected, ClaimStatusArchived)

	default:
		return "", fmt.Errorf("domain: transição não implementada para a ação %q", action)
	}
}

// ValidateEvidenceSourceTransition valida se a transição de estado solicitada para um uso de evidência (evidence_source) é permitida.
// Retorna o novo EvidenceSourceStatus resultante ou erro caso a transição seja proibida.
func ValidateEvidenceSourceTransition(current EvidenceSourceStatus, action ModerationAction) (EvidenceSourceStatus, error) {
	if !current.IsValid() {
		return "", fmt.Errorf("domain: estado atual do evidence_source inválido %q", current)
	}
	if !action.IsValid() {
		return "", fmt.Errorf("domain: ação de moderação inválida %q", action)
	}

	switch action {
	case ModerationActionReject:
		if current == EvidenceSourceStatusActive {
			return EvidenceSourceStatusRejected, nil
		}
		return "", fmt.Errorf("domain: ação 'reject' não é permitida para evidence_source com status %q (permitida apenas para %q)", current, EvidenceSourceStatusActive)

	case ModerationActionRestore:
		if current == EvidenceSourceStatusRejected {
			return EvidenceSourceStatusActive, nil
		}
		return "", fmt.Errorf("domain: ação 'restore' não é permitida para evidence_source com status %q (permitida apenas para %q)", current, EvidenceSourceStatusRejected)

	default:
		return "", fmt.Errorf("domain: ação de moderação %q não é suportada para evidence_source", action)
	}
}

const (
	MaxModerationReasonLength = 1000
)

// ValidateModerationReason valida e normaliza o motivo da decisão de moderação.
// O motivo é obrigatório, não pode ser vazio após trim, não pode exceder MaxModerationReasonLength caracteres
// e não pode conter tags HTML.
func ValidateModerationReason(rawReason string) (string, error) {
	trimmed := strings.TrimSpace(rawReason)
	if trimmed == "" {
		return "", fmt.Errorf("domain: motivo da moderação é obrigatório e não pode ser vazio")
	}
	if len(trimmed) > MaxModerationReasonLength {
		return "", fmt.Errorf("domain: motivo da moderação excede o limite de %d caracteres (obtido: %d)", MaxModerationReasonLength, len(trimmed))
	}
	// Bloqueio de tags HTML básicas para integridade editorial
	if strings.Contains(trimmed, "<") || strings.Contains(trimmed, ">") {
		return "", fmt.Errorf("domain: motivo da moderação não pode conter tags ou caracteres '<' e '>'")
	}
	return trimmed, nil
}

// ValidateModerationActor valida e normaliza o identificador do operador de moderação.
func ValidateModerationActor(rawActor string) (string, error) {
	trimmed := strings.TrimSpace(rawActor)
	if trimmed == "" {
		return "", fmt.Errorf("domain: identificador do operador (actor) é obrigatório e não pode ser vazio")
	}
	if len(trimmed) > 128 {
		return "", fmt.Errorf("domain: identificador do operador (actor) excede o limite de 128 caracteres")
	}
	return trimmed, nil
}

// PublicEditorialTargetType define o tipo de objeto alvo de uma alteração editorial pública.
type PublicEditorialTargetType string

const (
	PublicEditorialTargetClaim          PublicEditorialTargetType = "claim"
	PublicEditorialTargetEvidenceSource PublicEditorialTargetType = "evidence_source"
)

// PublicEditorialAction define a ação editorial normalizada para consumo público.
type PublicEditorialAction string

const (
	PublicEditorialActionApprove        PublicEditorialAction = "approve"
	PublicEditorialActionReject         PublicEditorialAction = "reject"
	PublicEditorialActionRestore        PublicEditorialAction = "restore"
	PublicEditorialActionInitialPublish PublicEditorialAction = "initial_publication"
)

// PublicEditorialEvent representa um evento auditável e redigido no histórico público de transparência editorial.
// Não expõe nomes de usuários (actor), fingerprints, raw_response ou texto confidencial de itens não públicos.
type PublicEditorialEvent struct {
	ID               string                    `json:"id"`
	CreatedAt        string                    `json:"created_at"`
	TargetType       PublicEditorialTargetType `json:"target_type"`
	TargetID         string                    `json:"target_id"`
	Action           PublicEditorialAction     `json:"action"`
	ActionLabel      string                    `json:"action_label"`
	TargetLabel      string                    `json:"target_label"`
	Summary          string                    `json:"summary"`
	ImpactLabel      string                    `json:"impact_label"`
	IsTargetPublic   bool                      `json:"is_target_public"`
	ClaimID          string                    `json:"claim_id,omitempty"`
	ClaimProposition string                    `json:"claim_proposition,omitempty"`
	ClaimGrade       EvidenceGrade             `json:"claim_grade,omitempty"`
	SourceID         string                    `json:"source_id,omitempty"`
	SourceTitle      string                    `json:"source_title,omitempty"`
	Locator          string                    `json:"locator,omitempty"`
}

// StatementType define a natureza estruturada de uma manifestação de defesa ou contraditório (VZ-025).
type StatementType string

const (
	StatementTypeCorrection        StatementType = "correction"
	StatementTypeRebuttal          StatementType = "rebuttal"
	StatementTypeClarification     StatementType = "clarification"
	StatementTypeAdditionalContext StatementType = "additional_context"
)

// CanonicalStatementTypes enumera todos os tipos canônicos de manifestação.
var CanonicalStatementTypes = []StatementType{
	StatementTypeCorrection,
	StatementTypeRebuttal,
	StatementTypeClarification,
	StatementTypeAdditionalContext,
}

func (t StatementType) IsValid() bool {
	switch t {
	case StatementTypeCorrection, StatementTypeRebuttal, StatementTypeClarification, StatementTypeAdditionalContext:
		return true
	default:
		return false
	}
}

// Label retorna a descrição legível em português do tipo de manifestação.
func (t StatementType) Label() string {
	switch t {
	case StatementTypeCorrection:
		return "Correção / Retificação Factual"
	case StatementTypeRebuttal:
		return "Contestação / Defesa Formal"
	case StatementTypeClarification:
		return "Esclarecimento Institucional"
	case StatementTypeAdditionalContext:
		return "Contexto Adicional / Complemento"
	default:
		return string(t)
	}
}

// StatementStatus define o estado operacional e editorial de uma manifestação (VZ-025).
type StatementStatus string

const (
	StatementStatusQuarantined StatementStatus = "quarantined"
	StatementStatusUnderReview StatementStatus = "under_review"
	StatementStatusAccepted    StatementStatus = "accepted"
	StatementStatusRejected    StatementStatus = "rejected"
	StatementStatusArchived    StatementStatus = "archived"
)

func (s StatementStatus) IsValid() bool {
	switch s {
	case StatementStatusQuarantined, StatementStatusUnderReview, StatementStatusAccepted,
		StatementStatusRejected, StatementStatusArchived:
		return true
	default:
		return false
	}
}

// Label retorna a descrição amigável em português do estado da manifestação.
func (s StatementStatus) Label() string {
	switch s {
	case StatementStatusQuarantined:
		return "Quarentena (Pendente de Análise)"
	case StatementStatusUnderReview:
		return "Em Revisão Editorial"
	case StatementStatusAccepted:
		return "Aceita e Publicada"
	case StatementStatusRejected:
		return "Rejeitada"
	case StatementStatusArchived:
		return "Arquivada"
	default:
		return string(s)
	}
}

// StatementModerationAction define as ações permitidas na moderação de manifestações (VZ-025).
type StatementModerationAction string

const (
	StatementActionAccept  StatementModerationAction = "accept"
	StatementActionReject  StatementModerationAction = "reject"
	StatementActionReview  StatementModerationAction = "review"
	StatementActionArchive StatementModerationAction = "archive"
)

func (a StatementModerationAction) IsValid() bool {
	switch a {
	case StatementActionAccept, StatementActionReject, StatementActionReview, StatementActionArchive:
		return true
	default:
		return false
	}
}

// Label retorna o rótulo descritivo da ação de moderação em português.
func (a StatementModerationAction) Label() string {
	switch a {
	case StatementActionAccept:
		return "Aceitar e Publicar"
	case StatementActionReject:
		return "Rejeitar Manifestação"
	case StatementActionReview:
		return "Colocar em Revisão"
	case StatementActionArchive:
		return "Arquivar Manifestação"
	default:
		return string(a)
	}
}

// Constantes de limites para validação de manifestações de contraditório (VZ-025).
const (
	MaxStatementTitleLength       = 200
	MinStatementContentLength     = 10
	MaxStatementContentLength     = 5000
	MaxStatementSourceURLLength   = 1000
	MaxStatementContactInfoLength = 255
	MaxStatementReasonLength      = 1000
	MaxStatementActorLength       = 128
)

// DefenseStatementSubmission encapsula os dados brutos submetidos para uma manifestação.
type DefenseStatementSubmission struct {
	ClaimID       string        `json:"claim_id"`
	StatementType StatementType `json:"statement_type"`
	Title         string        `json:"title"`
	Content       string        `json:"content"`
	SourceURL     string        `json:"source_url"`
	ContactInfo   string        `json:"contact_info"`
}

// PublicDefenseStatement representa a projeção pública e segura de uma manifestação aceita.
// O campo de contato (contact_info) NUNCA é exposto neste modelo.
type PublicDefenseStatement struct {
	ID                 string        `json:"id"`
	ClaimID            string        `json:"claim_id"`
	StatementType      StatementType `json:"statement_type"`
	StatementTypeLabel string        `json:"statement_type_label"`
	Title              string        `json:"title"`
	Content            string        `json:"content"`
	SourceURL          string        `json:"source_url,omitempty"`
	CreatedAt          string        `json:"created_at"`
}

// DefenseStatementDecision representa uma deliberação imutável de moderação sobre uma manifestação.
type DefenseStatementDecision struct {
	ID          string                    `json:"id"`
	StatementID string                    `json:"statement_id"`
	Action      StatementModerationAction `json:"action"`
	ActionLabel string                    `json:"action_label"`
	Reason      string                    `json:"reason"`
	Actor       string                    `json:"actor"`
	CreatedAt   string                    `json:"created_at"`
}

// AdminDefenseStatementDetail representa o modelo administrativo completo para inspeção e deliberação.
type AdminDefenseStatementDetail struct {
	ID                 string                     `json:"id"`
	ClaimID            string                     `json:"claim_id"`
	ClaimProposition   string                     `json:"claim_proposition"`
	ClaimGrade         EvidenceGrade              `json:"claim_grade"`
	ClaimStatus        ClaimStatus                `json:"claim_status"`
	SubjectEntityName  string                     `json:"subject_entity_name"`
	SubjectEntitySlug  string                     `json:"subject_entity_slug"`
	StatementType      StatementType              `json:"statement_type"`
	StatementTypeLabel string                     `json:"statement_type_label"`
	Title              string                     `json:"title"`
	Content            string                     `json:"content"`
	SourceURL          string                     `json:"source_url"`
	ContactInfo        string                     `json:"contact_info"` // Dado confidencial exclusivo do admin
	Status             StatementStatus            `json:"status"`
	StatusLabel        string                     `json:"status_label"`
	CreatedAt          string                     `json:"created_at"`
	UpdatedAt          string                     `json:"updated_at"`
	Decisions          []DefenseStatementDecision `json:"decisions"`
}

// ValidateStatementSubmission valida e normaliza as entradas de uma submissão de manifestação.
func ValidateStatementSubmission(sub DefenseStatementSubmission) (DefenseStatementSubmission, error) {
	claimID := strings.TrimSpace(sub.ClaimID)
	if claimID == "" {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: claim_id é obrigatório")
	}

	if !sub.StatementType.IsValid() {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: tipo de manifestação inválido %q", sub.StatementType)
	}

	title := strings.TrimSpace(sub.Title)
	if title == "" {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: título da manifestação é obrigatório")
	}
	if len(title) > MaxStatementTitleLength {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: título da manifestação excede o limite de %d caracteres (obtido: %d)", MaxStatementTitleLength, len(title))
	}
	if strings.Contains(title, "<") || strings.Contains(title, ">") {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: título da manifestação não pode conter caracteres '<' ou '>'")
	}

	content := strings.TrimSpace(sub.Content)
	if len(content) < MinStatementContentLength {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: conteúdo da manifestação muito curto (mínimo de %d caracteres, obtido: %d)", MinStatementContentLength, len(content))
	}
	if len(content) > MaxStatementContentLength {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: conteúdo da manifestação excede o limite de %d caracteres (obtido: %d)", MaxStatementContentLength, len(content))
	}
	if strings.Contains(content, "<") || strings.Contains(content, ">") {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: conteúdo da manifestação não pode conter tags ou caracteres '<' e '>'")
	}

	sourceURL := strings.TrimSpace(sub.SourceURL)
	if sourceURL != "" {
		if len(sourceURL) > MaxStatementSourceURLLength {
			return DefenseStatementSubmission{}, fmt.Errorf("domain: URL da fonte de referência excede %d caracteres", MaxStatementSourceURLLength)
		}
		if strings.Contains(sourceURL, "<") || strings.Contains(sourceURL, ">") {
			return DefenseStatementSubmission{}, fmt.Errorf("domain: URL da fonte não pode conter caracteres '<' ou '>'")
		}
		parsedURL, err := parseStrictHTTPURL(sourceURL)
		if err != nil {
			return DefenseStatementSubmission{}, fmt.Errorf("domain: URL da fonte inválida: %w", err)
		}
		sourceURL = parsedURL
	}

	contactInfo := strings.TrimSpace(sub.ContactInfo)
	if len(contactInfo) > MaxStatementContactInfoLength {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: informações de contato excedem %d caracteres", MaxStatementContactInfoLength)
	}
	if strings.Contains(contactInfo, "<") || strings.Contains(contactInfo, ">") {
		return DefenseStatementSubmission{}, fmt.Errorf("domain: informações de contato não podem conter caracteres '<' ou '>'")
	}

	return DefenseStatementSubmission{
		ClaimID:       claimID,
		StatementType: sub.StatementType,
		Title:         title,
		Content:       content,
		SourceURL:     sourceURL,
		ContactInfo:   contactInfo,
	}, nil
}

// parseStrictHTTPURL valida se a string é uma URL http ou https válida e bem formada.
// Rejeita esquemas não-HTTP, credenciais embutidas (userinfo), ausência de host e caracteres de controle.
func parseStrictHTTPURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > MaxStatementSourceURLLength {
		return "", fmt.Errorf("URL excede comprimento máximo de %d caracteres", MaxStatementSourceURLLength)
	}

	// Verificar caracteres de controle ASCII (0-31 e 127) e quebras de linha
	for i := 0; i < len(trimmed); i++ {
		b := trimmed[i]
		if b < 32 || b == 127 {
			return "", fmt.Errorf("URL contém caracteres de controle proibidos")
		}
	}

	u, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return "", fmt.Errorf("URL malformada: %w", err)
	}

	// Esquema estrito: apenas http ou https
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("esquema de URL inválido (apenas http e https são permitidos)")
	}

	// Host obrigatório e não vazio
	host := strings.TrimSpace(u.Host)
	if host == "" {
		return "", fmt.Errorf("URL sem host/domínio válido")
	}

	hostname := u.Hostname()
	if hostname == "" || strings.HasPrefix(hostname, "-") || strings.HasSuffix(hostname, "-") || strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return "", fmt.Errorf("URL sem host/domínio válido")
	}

	// Proibir userinfo (http://user:pass@example.com)
	if u.User != nil {
		return "", fmt.Errorf("URL não pode conter credenciais de usuário (userinfo)")
	}

	// Normalizar esquema
	u.Scheme = scheme
	return u.String(), nil
}

// ValidateStatementTransition valida a máquina de estados editorial de uma manifestação.
func ValidateStatementTransition(current StatementStatus, action StatementModerationAction) (StatementStatus, error) {
	if !current.IsValid() {
		return "", fmt.Errorf("domain: estado atual da manifestação inválido %q", current)
	}
	if !action.IsValid() {
		return "", fmt.Errorf("domain: ação de moderação inválida %q", action)
	}

	switch action {
	case StatementActionAccept:
		if current == StatementStatusQuarantined || current == StatementStatusUnderReview {
			return StatementStatusAccepted, nil
		}
		return "", fmt.Errorf("domain: ação 'accept' não é permitida para manifestação com status %q (permitida para %q ou %q)", current, StatementStatusQuarantined, StatementStatusUnderReview)

	case StatementActionReject:
		if current == StatementStatusQuarantined || current == StatementStatusUnderReview || current == StatementStatusAccepted {
			return StatementStatusRejected, nil
		}
		return "", fmt.Errorf("domain: ação 'reject' não é permitida para manifestação com status %q", current)

	case StatementActionReview:
		if current == StatementStatusQuarantined {
			return StatementStatusUnderReview, nil
		}
		return "", fmt.Errorf("domain: ação 'review' não é permitida para manifestação com status %q (permitida apenas para %q)", current, StatementStatusQuarantined)

	case StatementActionArchive:
		if current == StatementStatusAccepted || current == StatementStatusRejected || current == StatementStatusQuarantined {
			return StatementStatusArchived, nil
		}
		return "", fmt.Errorf("domain: ação 'archive' não é permitida para manifestação com status %q", current)

	default:
		return "", fmt.Errorf("domain: transição não implementada para a ação %q", action)
	}
}

// ValidateStatementReason valida e normaliza a justificativa da decisão de moderação da manifestação.
func ValidateStatementReason(rawReason string) (string, error) {
	trimmed := strings.TrimSpace(rawReason)
	if trimmed == "" {
		return "", fmt.Errorf("domain: justificativa da moderação é obrigatória e não pode ser vazia")
	}
	if len(trimmed) > MaxStatementReasonLength {
		return "", fmt.Errorf("domain: justificativa da moderação excede o limite de %d caracteres (obtido: %d)", MaxStatementReasonLength, len(trimmed))
	}
	if strings.Contains(trimmed, "<") || strings.Contains(trimmed, ">") {
		return "", fmt.Errorf("domain: justificativa da moderação não pode conter tags ou caracteres '<' e '>'")
	}
	return trimmed, nil
}

// ValidateStatementActor valida e normaliza o identificador do operador de moderação da manifestação.
func ValidateStatementActor(rawActor string) (string, error) {
	trimmed := strings.TrimSpace(rawActor)
	if trimmed == "" {
		return "", fmt.Errorf("domain: identificador do operador (actor) é obrigatório e não pode ser vazio")
	}
	if len(trimmed) > MaxStatementActorLength {
		return "", fmt.Errorf("domain: identificador do operador (actor) excede o limite de %d caracteres", MaxStatementActorLength)
	}
	return trimmed, nil
}

// UserRole define o papel institucional atribuído a uma conta administrativa (VZ-026).
type UserRole string

const (
	RoleAdmin    UserRole = "admin"
	RoleEditor   UserRole = "editor"
	RoleReviewer UserRole = "reviewer"
	RoleAuditor  UserRole = "auditor"
)

func (r UserRole) IsValid() bool {
	switch r {
	case RoleAdmin, RoleEditor, RoleReviewer, RoleAuditor:
		return true
	default:
		return false
	}
}

// Permission enumera as permissões operacionais do painel administrativo.
type Permission string

const (
	PermViewDashboard            Permission = "view_dashboard"
	PermViewCandidates           Permission = "view_candidates"
	PermViewSources              Permission = "view_sources"
	PermViewEvidences            Permission = "view_evidences"
	PermViewClaims               Permission = "view_claims"
	PermViewManifestations       Permission = "view_manifestations"
	PermViewManifestationContact Permission = "view_manifestation_contact"
	PermModerateClaims           Permission = "moderate_claims"
	PermModerateEvidenceSources  Permission = "moderate_evidence_sources"
	PermModerateManifestations   Permission = "moderate_manifestations"
	PermManageUsers              Permission = "manage_users"
	PermViewAuditLogs            Permission = "view_audit_logs"
)

func (p Permission) IsValid() bool {
	switch p {
	case PermViewDashboard, PermViewCandidates, PermViewSources, PermViewEvidences,
		PermViewClaims, PermViewManifestations, PermViewManifestationContact,
		PermModerateClaims, PermModerateEvidenceSources, PermModerateManifestations,
		PermManageUsers, PermViewAuditLogs:
		return true
	default:
		return false
	}
}

// HasPermission avalia a matriz de RBAC para determinar se o papel possui a permissão requerida.
func (r UserRole) HasPermission(p Permission) bool {
	switch r {
	case RoleAdmin:
		// Admin possui todas as permissões
		return p.IsValid()

	case RoleEditor:
		// Editor possui acesso editorial completo e pode ver contatos de manifestação,
		// mas não gerencia usuários nem consulta logs restritos de auditoria administrativa.
		switch p {
		case PermViewDashboard, PermViewCandidates, PermViewSources, PermViewEvidences,
			PermViewClaims, PermViewManifestations, PermViewManifestationContact,
			PermModerateClaims, PermModerateEvidenceSources, PermModerateManifestations:
			return true
		default:
			return false
		}

	case RoleReviewer:
		// Reviewer tem acesso de leitura amplo, incluindo contatos de manifestação,
		// sem permissão para mutações ou gestão de usuários/auditoria.
		switch p {
		case PermViewDashboard, PermViewCandidates, PermViewSources, PermViewEvidences,
			PermViewClaims, PermViewManifestations, PermViewManifestationContact:
			return true
		default:
			return false
		}

	case RoleAuditor:
		// Auditor tem acesso de leitura ao painel e à trilha de auditoria administrativa privada,
		// mas não tem acesso a dados de contato pessoal de terceiros nem permissões de mutação.
		switch p {
		case PermViewDashboard, PermViewCandidates, PermViewSources, PermViewEvidences,
			PermViewClaims, PermViewManifestations, PermViewAuditLogs:
			return true
		default:
			return false
		}

	default:
		return false
	}
}

// RequiresMFA determina se o papel exige compulsoriamente autenticação reforçada por MFA.
func (r UserRole) RequiresMFA() bool {
	switch r {
	case RoleAdmin, RoleEditor:
		return true
	default:
		return false
	}
}

// UserStatus define o estado operacional da conta do usuário.
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusLocked   UserStatus = "locked"
)

func (s UserStatus) IsValid() bool {
	switch s {
	case UserStatusActive, UserStatusDisabled, UserStatusLocked:
		return true
	default:
		return false
	}
}

func (s UserStatus) CanAuthenticate() bool {
	return s == UserStatusActive
}

// Constantes de limites para usuários.
const (
	MinUsernameLength    = 3
	MaxUsernameLength    = 64
	MinDisplayNameLength = 2
	MaxDisplayNameLength = 128
	MinPasswordLength    = 10
	MaxPasswordLength    = 128
)

// ValidateUsername normaliza e valida o nome de usuário (login).
func ValidateUsername(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", fmt.Errorf("domain: nome de usuário é obrigatório")
	}
	if len(trimmed) < MinUsernameLength || len(trimmed) > MaxUsernameLength {
		return "", fmt.Errorf("domain: nome de usuário deve ter entre %d e %d caracteres", MinUsernameLength, MaxUsernameLength)
	}
	if strings.Contains(trimmed, ":") {
		return "", fmt.Errorf("domain: nome de usuário não pode conter o caractere ':'")
	}
	for _, r := range trimmed {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '.' && r != '-' {
			return "", fmt.Errorf("domain: nome de usuário contém caractere inválido %q (permitidos: letras minúsculas, números, '_', '.' e '-')", r)
		}
	}
	return trimmed, nil
}

// ValidateDisplayName valida e normaliza o nome de exibição do usuário.
func ValidateDisplayName(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("domain: nome de exibição é obrigatório")
	}
	if len(trimmed) < MinDisplayNameLength || len(trimmed) > MaxDisplayNameLength {
		return "", fmt.Errorf("domain: nome de exibição deve ter entre %d e %d caracteres", MinDisplayNameLength, MaxDisplayNameLength)
	}
	if strings.Contains(trimmed, "<") || strings.Contains(trimmed, ">") {
		return "", fmt.Errorf("domain: nome de exibição não pode conter tags ou caracteres '<' e '>'")
	}
	return trimmed, nil
}

// ValidatePasswordStrength valida requisitos mínimos de complexidade da senha.
func ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("domain: a senha deve conter no mínimo %d caracteres", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("domain: a senha não pode exceder %d caracteres", MaxPasswordLength)
	}
	return nil
}

// ValidateUserRole valida e converte uma string para UserRole.
func ValidateUserRole(raw string) (UserRole, error) {
	role := UserRole(strings.TrimSpace(raw))
	if !role.IsValid() {
		return "", fmt.Errorf("domain: papel de usuário inválido %q (válidos: admin, editor, reviewer, auditor)", raw)
	}
	return role, nil
}

// ValidateUserStatus valida e converte uma string para UserStatus.
func ValidateUserStatus(raw string) (UserStatus, error) {
	status := UserStatus(strings.TrimSpace(raw))
	if !status.IsValid() {
		return "", fmt.Errorf("domain: status de usuário inválido %q (válidos: active, disabled, locked)", raw)
	}
	return status, nil
}

// AdminUser representa uma conta administrativa do sistema sem dados sensíveis de credenciais.
type AdminUser struct {
	ID                  string     `json:"id"`
	Username            string     `json:"username"`
	DisplayName         string     `json:"display_name"`
	Role                UserRole   `json:"role"`
	Status              UserStatus `json:"status"`
	FailedLoginAttempts int        `json:"failed_login_attempts"`
	MFAFailedAttempts   int        `json:"mfa_failed_attempts"`
	LockedUntil         *string    `json:"locked_until,omitempty"`
	MFAEnabled          bool       `json:"mfa_enabled"`
	MFAEnrolledAt       *string    `json:"mfa_enrolled_at,omitempty"`
	LastLoginAt         *string    `json:"last_login_at,omitempty"`
	CreatedAt           string     `json:"created_at"`
	UpdatedAt           string     `json:"updated_at"`
}

// IsLocked verifica se o usuário está com status locked e se o tempo de bloqueio ainda está ativo.
func (u AdminUser) IsLocked() bool {
	if u.Status != UserStatusLocked {
		return false
	}
	if u.LockedUntil == nil || *u.LockedUntil == "" {
		return true
	}
	lockedTime, err := time.Parse(time.RFC3339Nano, *u.LockedUntil)
	if err != nil {
		return true
	}
	return time.Now().UTC().Before(lockedTime)
}

// AdminSession representa uma sessão ativa no servidor.
type AdminSession struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	MFAVerified    bool   `json:"mfa_verified"`
	IPAddress      string `json:"ip_address"`
	UserAgent      string `json:"user_agent"`
	ExpiresAt      string `json:"expires_at"`
	LastActivityAt string `json:"last_activity_at"`
	CreatedAt      string `json:"created_at"`
}

// AdminAuditLog representa uma entrada imutável na trilha de auditoria administrativa.
type AdminAuditLog struct {
	ID            string  `json:"id"`
	UserID        *string `json:"user_id,omitempty"`
	Username      string  `json:"username"`
	Action        string  `json:"action"`
	ActorID       *string `json:"actor_id,omitempty"`
	ActorUsername string  `json:"actor_username"`
	TargetID      string  `json:"target_id"`
	Details       string  `json:"details"`
	IPAddress     string  `json:"ip_address"`
	CreatedAt     string  `json:"created_at"`
}
