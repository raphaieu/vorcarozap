package metrics

import (
	"context"
	"fmt"
	"math"

	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// PublicOverview resume o acervo público visível total da plataforma.
type PublicOverview struct {
	TotalEntities int64
	TotalClaims   int64
	TotalSources  int64
	LatestUpdate  string
}

// EligibleNetwork resume a rede documental qualificada para métricas (metric_eligible = true).
type EligibleNetwork struct {
	EligibleEntities int64
	EligibleClaims   int64
}

// GradeCount representa a contagem e proporção de alegações elegíveis por grau documental.
type GradeCount struct {
	Grade      string
	Count      int64
	Percentage float64
}

// RelevanceCount representa a contagem e proporção de entidades elegíveis por nível de relevância pública.
type RelevanceCount struct {
	Relevance  int64
	Count      int64
	Percentage float64
}

// CategoryCount representa a contagem e proporção de entidades elegíveis por categoria.
type CategoryCount struct {
	Category   string
	Count      int64
	Percentage float64
}

// RecentClaim representa uma alegação elegível recente para exibição cronológica.
type RecentClaim struct {
	ClaimID         string
	EntityID        string
	EntityName      string
	EntitySlug      string
	EntityCategory  string
	EntityRelevance int64
	ClaimType       string
	Grade           string
	ShortSynthesis  string
	UpdatedAt       string
}

// PublicMetrics consolida todas as métricas públicas agregadas da plataforma.
type PublicMetrics struct {
	Overview      PublicOverview
	Network       EligibleNetwork
	GradeDist     []GradeCount
	RelevanceDist []RelevanceCount
	CategoryDist  []CategoryCount
	RecentClaims  []RecentClaim
	CutoffDate    string
}

// Querier define a interface de acesso a dados necessária para a engine de métricas.
type Querier interface {
	GetPublicOverviewMetrics(ctx context.Context) (sqlc.GetPublicOverviewMetricsRow, error)
	GetEligibleNetworkMetrics(ctx context.Context) (sqlc.GetEligibleNetworkMetricsRow, error)
	ListEligibleClaimsByGrade(ctx context.Context) ([]sqlc.ListEligibleClaimsByGradeRow, error)
	ListEligibleEntitiesByRelevance(ctx context.Context) ([]sqlc.ListEligibleEntitiesByRelevanceRow, error)
	ListEligibleEntitiesByCategory(ctx context.Context) ([]sqlc.ListEligibleEntitiesByCategoryRow, error)
	ListRecentEligibleClaims(ctx context.Context, arg sqlc.ListRecentEligibleClaimsParams) ([]sqlc.ListRecentEligibleClaimsRow, error)
}

// StandardGrades lista a ordem canônica dos graus documentais.
var StandardGrades = []string{"A", "B", "C", "D", "E"}

// StandardRelevances lista os níveis de relevância canônicos de 1 a 5.
var StandardRelevances = []int64{1, 2, 3, 4, 5}

// GetPublicMetrics calcula todas as métricas públicas agregadas a partir do banco de dados.
// É estritamente resiliente a estados vazios e não realiza divisões por zero.
func GetPublicMetrics(ctx context.Context, q Querier, cutoff string, recentLimit int64) (*PublicMetrics, error) {
	if recentLimit <= 0 {
		recentLimit = 5
	}

	overviewRow, err := q.GetPublicOverviewMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics: falha ao obter overview: %w", err)
	}

	networkRow, err := q.GetEligibleNetworkMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics: falha ao obter rede elegivel: %w", err)
	}

	rawGrades, err := q.ListEligibleClaimsByGrade(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics: falha ao listar graus: %w", err)
	}

	rawRelevances, err := q.ListEligibleEntitiesByRelevance(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics: falha ao listar relevancias: %w", err)
	}

	rawCategories, err := q.ListEligibleEntitiesByCategory(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics: falha ao listar categorias: %w", err)
	}

	rawRecent, err := q.ListRecentEligibleClaims(ctx, sqlc.ListRecentEligibleClaimsParams{
		Since:      "",
		ClaimLimit: recentLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("metrics: falha ao listar recentes: %w", err)
	}

	// 1. Overview
	var latestUpdateStr string
	if overviewRow.LatestUpdate != nil {
		latestUpdateStr = fmt.Sprint(overviewRow.LatestUpdate)
	}
	overview := PublicOverview{
		TotalEntities: overviewRow.TotalEntities,
		TotalClaims:   overviewRow.TotalClaims,
		TotalSources:  overviewRow.TotalSources,
		LatestUpdate:  latestUpdateStr,
	}

	// 2. Network
	network := EligibleNetwork{
		EligibleEntities: networkRow.EligibleEntities,
		EligibleClaims:   networkRow.EligibleClaims,
	}

	// 3. Distribuição de Graus (Garante A a E sempre presentes)
	gradeMap := make(map[string]int64)
	for _, g := range rawGrades {
		gradeMap[g.Grade] = g.Count
	}
	gradeDist := make([]GradeCount, len(StandardGrades))
	for i, g := range StandardGrades {
		cnt := gradeMap[g]
		var pct float64
		if network.EligibleClaims > 0 {
			pct = roundToOneDecimal((float64(cnt) / float64(network.EligibleClaims)) * 100.0)
		}
		gradeDist[i] = GradeCount{
			Grade:      g,
			Count:      cnt,
			Percentage: pct,
		}
	}

	// 4. Distribuição de Relevância (Garante 1 a 5 sempre presentes)
	relMap := make(map[int64]int64)
	for _, r := range rawRelevances {
		relMap[r.Relevance] = r.Count
	}
	relevanceDist := make([]RelevanceCount, len(StandardRelevances))
	for i, rel := range StandardRelevances {
		cnt := relMap[rel]
		var pct float64
		if network.EligibleEntities > 0 {
			pct = roundToOneDecimal((float64(cnt) / float64(network.EligibleEntities)) * 100.0)
		}
		relevanceDist[i] = RelevanceCount{
			Relevance:  rel,
			Count:      cnt,
			Percentage: pct,
		}
	}

	// 5. Distribuição de Categorias
	categoryDist := make([]CategoryCount, 0, len(rawCategories))
	for _, c := range rawCategories {
		var pct float64
		if network.EligibleEntities > 0 {
			pct = roundToOneDecimal((float64(c.Count) / float64(network.EligibleEntities)) * 100.0)
		}
		categoryDist = append(categoryDist, CategoryCount{
			Category:   c.Category,
			Count:      c.Count,
			Percentage: pct,
		})
	}

	// 6. Alegações Recentes
	recentClaims := make([]RecentClaim, 0, len(rawRecent))
	for _, r := range rawRecent {
		recentClaims = append(recentClaims, RecentClaim{
			ClaimID:         r.ClaimID,
			EntityID:        r.EntityID,
			EntityName:      r.EntityName,
			EntitySlug:      r.EntitySlug,
			EntityCategory:  r.EntityCategory,
			EntityRelevance: r.EntityRelevance,
			ClaimType:       r.ClaimType,
			Grade:           r.Grade,
			ShortSynthesis:  r.ShortSynthesis,
			UpdatedAt:       r.UpdatedAt,
		})
	}

	return &PublicMetrics{
		Overview:      overview,
		Network:       network,
		GradeDist:     gradeDist,
		RelevanceDist: relevanceDist,
		CategoryDist:  categoryDist,
		RecentClaims:  recentClaims,
		CutoffDate:    cutoff,
	}, nil
}

func roundToOneDecimal(v float64) float64 {
	return math.Round(v*10) / 10
}
