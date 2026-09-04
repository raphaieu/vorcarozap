package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/metrics"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

type mockQuerier struct {
	overviewRow   sqlc.GetPublicOverviewMetricsRow
	overviewErr   error
	networkRow    sqlc.GetEligibleNetworkMetricsRow
	networkErr    error
	gradeRows     []sqlc.ListEligibleClaimsByGradeRow
	gradeErr      error
	relevanceRows []sqlc.ListEligibleEntitiesByRelevanceRow
	relevanceErr  error
	categoryRows  []sqlc.ListEligibleEntitiesByCategoryRow
	categoryErr   error
	recentRows    []sqlc.ListRecentEligibleClaimsRow
	recentErr     error
}

func (m *mockQuerier) GetPublicOverviewMetrics(ctx context.Context) (sqlc.GetPublicOverviewMetricsRow, error) {
	return m.overviewRow, m.overviewErr
}

func (m *mockQuerier) GetEligibleNetworkMetrics(ctx context.Context) (sqlc.GetEligibleNetworkMetricsRow, error) {
	return m.networkRow, m.networkErr
}

func (m *mockQuerier) ListEligibleClaimsByGrade(ctx context.Context) ([]sqlc.ListEligibleClaimsByGradeRow, error) {
	return m.gradeRows, m.gradeErr
}

func (m *mockQuerier) ListEligibleEntitiesByRelevance(ctx context.Context) ([]sqlc.ListEligibleEntitiesByRelevanceRow, error) {
	return m.relevanceRows, m.relevanceErr
}

func (m *mockQuerier) ListEligibleEntitiesByCategory(ctx context.Context) ([]sqlc.ListEligibleEntitiesByCategoryRow, error) {
	return m.categoryRows, m.categoryErr
}

func (m *mockQuerier) ListRecentEligibleClaims(ctx context.Context, arg sqlc.ListRecentEligibleClaimsParams) ([]sqlc.ListRecentEligibleClaimsRow, error) {
	return m.recentRows, m.recentErr
}

func TestGetPublicMetrics_EmptyState(t *testing.T) {
	mock := &mockQuerier{
		overviewRow: sqlc.GetPublicOverviewMetricsRow{
			TotalEntities: 0,
			TotalClaims:   0,
			TotalSources:  0,
			LatestUpdate:  nil,
		},
		networkRow: sqlc.GetEligibleNetworkMetricsRow{
			EligibleEntities: 0,
			EligibleClaims:   0,
		},
		gradeRows:     []sqlc.ListEligibleClaimsByGradeRow{},
		relevanceRows: []sqlc.ListEligibleEntitiesByRelevanceRow{},
		categoryRows:  []sqlc.ListEligibleEntitiesByCategoryRow{},
		recentRows:    []sqlc.ListRecentEligibleClaimsRow{},
	}

	m, err := metrics.GetPublicMetrics(context.Background(), mock, "2026-09-03", 5)
	if err != nil {
		t.Fatalf("esperava sucesso em estado vazio, obteve erro: %v", err)
	}

	if m.Overview.TotalEntities != 0 || m.Overview.TotalClaims != 0 || m.Overview.TotalSources != 0 {
		t.Errorf("overview esperava tudo zero, obteve %+v", m.Overview)
	}
	if m.Overview.LatestUpdate != "" {
		t.Errorf("latest update deveria ser vazio, obteve %q", m.Overview.LatestUpdate)
	}
	if m.Network.EligibleEntities != 0 || m.Network.EligibleClaims != 0 {
		t.Errorf("network esperava tudo zero, obteve %+v", m.Network)
	}

	// Todos os 5 graus devem estar presentes com count 0 e pct 0.0
	if len(m.GradeDist) != 5 {
		t.Fatalf("esperava 5 graus na distribuição, obteve %d", len(m.GradeDist))
	}
	for _, g := range m.GradeDist {
		if g.Count != 0 || g.Percentage != 0.0 {
			t.Errorf("grau %s deveria ter count=0 e pct=0.0, obteve count=%d pct=%v", g.Grade, g.Count, g.Percentage)
		}
	}

	// Todas as 5 relevâncias devem estar presentes com count 0 e pct 0.0
	if len(m.RelevanceDist) != 5 {
		t.Fatalf("esperava 5 relevâncias na distribuição, obteve %d", len(m.RelevanceDist))
	}
	for _, r := range m.RelevanceDist {
		if r.Count != 0 || r.Percentage != 0.0 {
			t.Errorf("relevância %d deveria ter count=0 e pct=0.0, obteve count=%d pct=%v", r.Relevance, r.Count, r.Percentage)
		}
	}

	if m.CategoryDist == nil {
		t.Error("categoryDist não deveria ser nil")
	}
	if len(m.CategoryDist) != 0 {
		t.Errorf("categoryDist deveria ser vazio, obteve %d", len(m.CategoryDist))
	}

	if m.RecentClaims == nil {
		t.Error("recentClaims não deveria ser nil")
	}
	if len(m.RecentClaims) != 0 {
		t.Errorf("recentClaims deveria ser vazio, obteve %d", len(m.RecentClaims))
	}

	if m.CutoffDate != "2026-09-03" {
		t.Errorf("cutoff esperava '2026-09-03', obteve %q", m.CutoffDate)
	}
}

func TestGetPublicMetrics_CalculationAndProportions(t *testing.T) {
	mock := &mockQuerier{
		overviewRow: sqlc.GetPublicOverviewMetricsRow{
			TotalEntities: 45,
			TotalClaims:   130,
			TotalSources:  88,
			LatestUpdate:  "2026-09-03T18:00:00Z",
		},
		networkRow: sqlc.GetEligibleNetworkMetricsRow{
			EligibleEntities: 40,
			EligibleClaims:   100,
		},
		gradeRows: []sqlc.ListEligibleClaimsByGradeRow{
			{Grade: "A", Count: 30},
			{Grade: "B", Count: 50},
			{Grade: "C", Count: 20},
		},
		relevanceRows: []sqlc.ListEligibleEntitiesByRelevanceRow{
			{Relevance: 3, Count: 10},
			{Relevance: 4, Count: 20},
			{Relevance: 5, Count: 10},
		},
		categoryRows: []sqlc.ListEligibleEntitiesByCategoryRow{
			{Category: "Politica", Count: 25},
			{Category: "Empresarial", Count: 15},
		},
		recentRows: []sqlc.ListRecentEligibleClaimsRow{
			{
				ClaimID:         "claim-1",
				EntityID:        "ent-1",
				EntityName:      "Alice",
				EntitySlug:      "alice",
				EntityCategory:  "Politica",
				EntityRelevance: 4,
				ClaimType:       "contato",
				Grade:           "A",
				ShortSynthesis:  "Reunião institucional",
				UpdatedAt:       "2026-09-03T17:00:00Z",
			},
		},
	}

	m, err := metrics.GetPublicMetrics(context.Background(), mock, "2026-09-03", 5)
	if err != nil {
		t.Fatalf("falha inesperada: %v", err)
	}

	if m.Overview.TotalEntities != 45 || m.Overview.TotalClaims != 130 || m.Overview.TotalSources != 88 {
		t.Errorf("overview incorreto: %+v", m.Overview)
	}
	if m.Overview.LatestUpdate != "2026-09-03T18:00:00Z" {
		t.Errorf("latest update incorreto: %q", m.Overview.LatestUpdate)
	}

	if m.Network.EligibleEntities != 40 || m.Network.EligibleClaims != 100 {
		t.Errorf("network incorreto: %+v", m.Network)
	}

	// Checa graus: A=30%, B=50%, C=20%, D=0%, E=0%
	expectedGrades := map[string]struct {
		count int64
		pct   float64
	}{
		"A": {count: 30, pct: 30.0},
		"B": {count: 50, pct: 50.0},
		"C": {count: 20, pct: 20.0},
		"D": {count: 0, pct: 0.0},
		"E": {count: 0, pct: 0.0},
	}

	for _, g := range m.GradeDist {
		exp, ok := expectedGrades[g.Grade]
		if !ok {
			t.Fatalf("grau inesperado: %s", g.Grade)
		}
		if g.Count != exp.count || g.Percentage != exp.pct {
			t.Errorf("grau %s: esperado count=%d pct=%v, obteve count=%d pct=%v", g.Grade, exp.count, exp.pct, g.Count, g.Percentage)
		}
	}

	// Checa relevâncias: 1=0%, 2=0%, 3=25%, 4=50%, 5=25% (sobre 40 elegíveis)
	expectedRels := map[int64]struct {
		count int64
		pct   float64
	}{
		1: {count: 0, pct: 0.0},
		2: {count: 0, pct: 0.0},
		3: {count: 10, pct: 25.0},
		4: {count: 20, pct: 50.0},
		5: {count: 10, pct: 25.0},
	}

	for _, r := range m.RelevanceDist {
		exp, ok := expectedRels[r.Relevance]
		if !ok {
			t.Fatalf("relevância inesperada: %d", r.Relevance)
		}
		if r.Count != exp.count || r.Percentage != exp.pct {
			t.Errorf("relevância %d: esperado count=%d pct=%v, obteve count=%d pct=%v", r.Relevance, exp.count, exp.pct, r.Count, r.Percentage)
		}
	}

	// Checa categorias
	if len(m.CategoryDist) != 2 {
		t.Fatalf("esperava 2 categorias, obteve %d", len(m.CategoryDist))
	}
	if m.CategoryDist[0].Category != "Politica" || m.CategoryDist[0].Count != 25 || m.CategoryDist[0].Percentage != 62.5 {
		t.Errorf("categoria 0 inesperada: %+v", m.CategoryDist[0])
	}

	// Checa recentes
	if len(m.RecentClaims) != 1 {
		t.Fatalf("esperava 1 recente, obteve %d", len(m.RecentClaims))
	}
	if m.RecentClaims[0].EntityName != "Alice" || m.RecentClaims[0].Grade != "A" {
		t.Errorf("recente 0 inesperado: %+v", m.RecentClaims[0])
	}
}

func TestGetPublicMetrics_QueryErrors(t *testing.T) {
	testErr := errors.New("db error")

	t.Run("Erro no overview", func(t *testing.T) {
		mock := &mockQuerier{overviewErr: testErr}
		_, err := metrics.GetPublicMetrics(context.Background(), mock, "2026-09-03", 5)
		if err == nil {
			t.Fatal("esperava erro mas obteve nil")
		}
	})

	t.Run("Erro na rede", func(t *testing.T) {
		mock := &mockQuerier{networkErr: testErr}
		_, err := metrics.GetPublicMetrics(context.Background(), mock, "2026-09-03", 5)
		if err == nil {
			t.Fatal("esperava erro mas obteve nil")
		}
	})

	t.Run("Erro nos graus", func(t *testing.T) {
		mock := &mockQuerier{gradeErr: testErr}
		_, err := metrics.GetPublicMetrics(context.Background(), mock, "2026-09-03", 5)
		if err == nil {
			t.Fatal("esperava erro mas obteve nil")
		}
	})
}
