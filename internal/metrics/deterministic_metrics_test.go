package metrics_test

import (
	"context"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/metrics"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// TestDeterministic_Metrics_TableDriven e TestDeterministic_Metrics_RealSQLite comprova todos os invariantes
// de agregação de métricas públicas por estado ativo, elegibilidade métrica e segregação de dados.
func TestDeterministic_Metrics_TableDriven(t *testing.T) {
	t.Run("Imunidade a divisão por zero com base vazia ou zero elegíveis", func(t *testing.T) {
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
			t.Fatalf("esperava sucesso mesmo com zeros: %v", err)
		}

		if len(m.GradeDist) != 5 {
			t.Fatalf("esperava 5 graus na distribuição, obteve %d", len(m.GradeDist))
		}
		for _, g := range m.GradeDist {
			if g.Count != 0 || g.Percentage != 0.0 {
				t.Errorf("grau %s deveria ter count=0 e pct=0.0, obteve %d (%v)", g.Grade, g.Count, g.Percentage)
			}
		}

		if len(m.RelevanceDist) != 5 {
			t.Fatalf("esperava 5 relevâncias na distribuição, obteve %d", len(m.RelevanceDist))
		}
		for _, r := range m.RelevanceDist {
			if r.Count != 0 || r.Percentage != 0.0 {
				t.Errorf("relevância %d deveria ter count=0 e pct=0.0, obteve %d (%v)", r.Relevance, r.Count, r.Percentage)
			}
		}
	})

	t.Run("Integração Real SQLite: Exclusão de não-públicos, não-elegíveis e moderação em tempo real", func(t *testing.T) {
		db, queries := setupMetricsTestDB(t)
		ctx := context.Background()

		// Inserção da topologia de teste
		_, err := db.ExecContext(ctx, `
			INSERT INTO cases (id, name, slug, description)
			VALUES ('case-det-1', 'Operação Métricas Real', 'operacao-metricas-real', 'Teste de integração');

			INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, category, reach)
			VALUES
				('ent-1', 'person', 'Entidade Elegivel A', 'entidade elegivel a', 'entidade-elegivel-a', 'Diretor', 'Resumo', 5, 'R1', 'Finanças', 'Nacional'),
				('ent-2', 'person', 'Entidade Contexto B', 'entidade contexto b', 'entidade-contexto-b', 'Sócio', 'Resumo', 4, 'R2', 'Politica', 'Nacional'),
				('ent-3', 'person', 'Entidade Quarentena C', 'entidade quarentena c', 'entidade-quarentena-c', 'Contato', 'Resumo', 3, 'R3', 'Setor Público', 'Regional'),
				('ent-4', 'person', 'Entidade Rejeitada D', 'entidade rejeitada d', 'entidade-rejeitada-d', 'Representante', 'Resumo', 2, 'R4', 'Outros', 'Regional'),
				('ent-5', 'person', 'Entidade Arquivada E', 'entidade arquivada e', 'entidade-arquivada-e', 'Consultor', 'Resumo', 1, 'R5', 'Outros', 'Regional');

			INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary)
			VALUES
				('rel-1', 'ent-1', 'case-det-1', 'investigado', 'Vínculo 1'),
				('rel-2', 'ent-2', 'case-det-1', 'investigado', 'Vínculo 2'),
				('rel-3', 'ent-3', 'case-det-1', 'investigado', 'Vínculo 3'),
				('rel-4', 'ent-4', 'case-det-1', 'investigado', 'Vínculo 4'),
				('rel-5', 'ent-5', 'case-det-1', 'investigado', 'Vínculo 5');

			INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_access_status)
			VALUES
				('src-1', 'Fonte 1', 'Jornal 1', 'https://noticia1.com', 'https://noticia1.com', 'reachable'),
				('src-2', 'Fonte 2', 'Jornal 2', 'https://noticia2.com', 'https://noticia2.com', 'reachable'),
				('src-3', 'Fonte 3', 'Jornal 3', 'https://noticia3.com', 'https://noticia3.com', 'reachable');

			-- Claim 1: ent-1, published, metric_eligible=1, Grau A, com evidence_source ativo supports
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES ('c1', 'rel-1', 'Proposição 1', 'Jornal 1', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ativo');
			INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-1', 'c1', 'Ev 1');
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES ('es-1', 'ev-1', 'src-1', 'Trecho 1', 'P.1', 'supports', 'active');

			-- Claim 1b: ent-1, published, metric_eligible=1, Grau B, mesmo sujeito ent-1 (NÃO pode duplicar ent-1 em EligibleEntities)
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES ('c1b', 'rel-1', 'Proposição 1b', 'Jornal 1', 'curated_seed', 'B', 'supports_link', 1, 'published', 'ativo');
			INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-1b', 'c1b', 'Ev 1b');
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES ('es-1b', 'ev-1b', 'src-1', 'Trecho 1b', 'P.2', 'supports', 'active');

			-- Claim 2: ent-2, published, metric_eligible=0 (context_only, Grau C), NÃO conta em EligibleClaims/EligibleEntities
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES ('c2', 'rel-2', 'Proposição 2', 'Jornal 2', 'curated_seed', 'C', 'context_only', 0, 'published', 'ativo');
			INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-2', 'c2', 'Ev 2');
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES ('es-2', 'ev-2', 'src-2', 'Trecho 2', 'P.3', 'supports', 'active');

			-- Claim 3: ent-3, quarantined (Grau D, metric_eligible=1), NÃO pode constar em nada público
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES ('c3', 'rel-3', 'Proposição 3', 'Jornal 3', 'openrouter', 'D', 'possible_link', 1, 'quarantined', 'ativo');
			INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-3', 'c3', 'Ev 3');
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES ('es-3', 'ev-3', 'src-3', 'Trecho 3', 'P.4', 'supports', 'active');

			-- Claim 4: ent-4, rejected, NÃO pode constar em nada público
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES ('c4', 'rel-4', 'Proposição 4', 'Jornal 3', 'admin', 'E', 'possible_link', 1, 'rejected', 'ativo');
			INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-4', 'c4', 'Ev 4');
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES ('es-4', 'ev-4', 'src-3', 'Trecho 4', 'P.5', 'supports', 'active');

			-- Claim 5: ent-5, published mas com evidence_source APENAS role='contradicts' (sem supports), NÃO entra na view pública
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES ('c5', 'rel-5', 'Proposição 5', 'Jornal 3', 'curated_seed', 'A', 'contradicts_link', 1, 'published', 'ativo');
			INSERT INTO evidence (id, claim_id, summary) VALUES ('ev-5', 'c5', 'Ev 5');
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES ('es-5', 'ev-5', 'src-3', 'Trecho 5', 'P.6', 'contradicts', 'active');
		`)
		if err != nil {
			t.Fatalf("falha ao popular dados reais no SQLite: %v", err)
		}

		// 1. Cômputo inicial
		m, err := metrics.GetPublicMetrics(ctx, queries, "2026-09-03", 5)
		if err != nil {
			t.Fatalf("falha em GetPublicMetrics: %v", err)
		}

		// Overview: c1, c1b, c2 (3 claims públicos; c3=quarentena, c4=rejeitado, c5=sem supports ativo)
		if m.Overview.TotalClaims != 3 {
			t.Errorf("Overview.TotalClaims: esperado 3, obteve %d", m.Overview.TotalClaims)
		}
		// Entidades públicas: ent-1 e ent-2 (2 entidades)
		if m.Overview.TotalEntities != 2 {
			t.Errorf("Overview.TotalEntities: esperado 2, obteve %d", m.Overview.TotalEntities)
		}

		// Rede Elegível: apenas claims com metric_eligible=1 (c1 e c1b = 2 claims)
		if m.Network.EligibleClaims != 2 {
			t.Errorf("Network.EligibleClaims: esperado 2, obteve %d", m.Network.EligibleClaims)
		}
		// Entidades elegíveis: apenas ent-1 (1 entidade; ent-2 só tem c2 com metric_eligible=0)
		// E múltiplos claims de ent-1 (c1 e c1b) NÃO duplicam a contagem!
		if m.Network.EligibleEntities != 1 {
			t.Errorf("Network.EligibleEntities: esperado 1 (sem duplicação de ent-1), obteve %d", m.Network.EligibleEntities)
		}

		// Distribuição por Grau na rede (c1=A, c1b=B): Grau A=1 (50%), Grau B=1 (50%), C/D/E=0 (0%)
		for _, g := range m.GradeDist {
			if g.Grade == "A" && (g.Count != 1 || g.Percentage != 50.0) {
				t.Errorf("Grau A incorreto: %+v", g)
			}
			if g.Grade == "B" && (g.Count != 1 || g.Percentage != 50.0) {
				t.Errorf("Grau B incorreto: %+v", g)
			}
			if (g.Grade == "C" || g.Grade == "D" || g.Grade == "E") && (g.Count != 0 || g.Percentage != 0.0) {
				t.Errorf("Grau %s deveria ser 0: %+v", g.Grade, g)
			}
		}

		// Distribuição por Relevância na rede: ent-1 possui relevância 5 -> Rel 5=1 (100%), Rel 1-4=0 (0%)
		for _, r := range m.RelevanceDist {
			if r.Relevance == 5 && (r.Count != 1 || r.Percentage != 100.0) {
				t.Errorf("Relevância 5 incorreta: %+v", r)
			}
			if r.Relevance != 5 && (r.Count != 0 || r.Percentage != 0.0) {
				t.Errorf("Relevância %d deveria ser 0: %+v", r.Relevance, r)
			}
		}

		// 2. Moderação: Rejeitar c1 (published -> rejected)
		_, err = db.ExecContext(ctx, "UPDATE claims SET status = 'rejected', updated_at = datetime('now') WHERE id = 'c1'")
		if err != nil {
			t.Fatalf("falha ao rejeitar c1: %v", err)
		}

		mAfterReject, err := metrics.GetPublicMetrics(ctx, queries, "2026-09-03", 5)
		if err != nil {
			t.Fatalf("falha ao consultar métricas após rejeitar c1: %v", err)
		}
		if mAfterReject.Network.EligibleClaims != 1 {
			t.Errorf("Network.EligibleClaims após rejeição: esperado 1 (c1b), obteve %d", mAfterReject.Network.EligibleClaims)
		}
		// Grau A agora tem 0 (0%), Grau B tem 1 (100%)
		for _, g := range mAfterReject.GradeDist {
			if g.Grade == "A" && (g.Count != 0 || g.Percentage != 0.0) {
				t.Errorf("Grau A após rejeição deveria ter zerado: %+v", g)
			}
			if g.Grade == "B" && (g.Count != 1 || g.Percentage != 100.0) {
				t.Errorf("Grau B após rejeição de A deveria ser 100%%: %+v", g)
			}
		}
	})
}
