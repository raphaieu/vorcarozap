package store_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/metrics"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestMetrics_ActiveStateAndEligibility(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)

	// 1. Inserir Entidades
	// e1: Relevância 5, Politica
	// e2: Relevância 4, Empresarial
	// e3: Relevância 3, Jurídico (apenas context_only, metric_eligible = 0)
	// e4: Relevância 2, Outros (somente alegações em quarentena -> não deve aparecer em nada)
	// e5: Relevância 1, Vazia (sem alegações -> não deve aparecer em nada)
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "e1", "alice-politica", "Alice", "Politica", 5)
	insertEntity(t, db, "e2", "bob-empresarial", "Bob", "Empresarial", 4)
	insertEntity(t, db, "e3", "carlos-contexto", "Carlos", "Juridico", 3)
	insertEntity(t, db, "e4", "daniel-quarentena", "Daniel", "Outros", 2)
	insertEntity(t, db, "e5", "elena-vazia", "Elena", "Outros", 1)

	// Inserir Casos e Relacionamentos
	insertCase(t, db, "case1", "caso-principal", "Caso Principal")
	insertRel(t, db, "rel1", "e1", "e2", "case1", "contato", "Contato Alice e Bob")
	insertRel(t, db, "rel2", "e2", "e1", "case1", "sociedade", "Sociedade Bob e Alice")
	insertRel(t, db, "rel3", "e3", "e1", "case1", "historico", "Historico Carlos")
	insertRel(t, db, "rel4", "e4", "e1", "case1", "suspeita", "Suspeita Daniel")

	// Fontes
	insertSource(t, db, "s1", "Fonte 1 - Jornal A")
	insertSource(t, db, "s2", "Fonte 2 - Diário B")
	insertSource(t, db, "s3", "Fonte 3 - Inativa")

	// Alegação 1: Alice -> Bob, Grau A, published, metric_eligible = 1
	insertClaim(t, db, "c1", "rel1", "A", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev1", "c1", "es1", "s1", "supports", "active")

	// Alegação 2: Alice -> Bob, Grau B, published, metric_eligible = 1 (mesma fonte s1)
	insertClaim(t, db, "c2", "rel1", "B", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev2", "c2", "es2", "s1", "supports", "active")

	// Alegação 3: Bob -> Alice, Grau A, published, metric_eligible = 1 (fonte s2)
	insertClaim(t, db, "c3", "rel2", "A", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev3", "c3", "es3", "s2", "supports", "active")

	// Alegação 4: Carlos -> Alice, Grau C, published, metric_eligible = 0 (context_only, fonte s2)
	insertClaim(t, db, "c4", "rel3", "C", "context_only", 0, "published", now)
	insertEvidenceAndSource(t, db, "ev4", "c4", "es4", "s2", "supports", "active")

	// Alegação 5: Daniel -> Alice, Grau D, quarantined, metric_eligible = 1 (fonte s1) -> NÃO pode contar
	insertClaim(t, db, "c5", "rel4", "D", "supports_link", 1, "quarantined", now)
	insertEvidenceAndSource(t, db, "ev5", "c5", "es5", "s1", "supports", "active")

	// Alegação 6: Bob, Grau B, published, metric_eligible = 1 mas fonte rejeitada es6 -> NÃO pode contar
	insertClaim(t, db, "c6", "rel2", "B", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev6", "c6", "es6", "s3", "supports", "rejected")

	// Executa cálculo de métricas
	m, err := metrics.GetPublicMetrics(ctx, queries, "2026-09-03", 5)
	if err != nil {
		t.Fatalf("GetPublicMetrics falhou: %v", err)
	}

	// 1. Overview público:
	// Entidades públicas: Alice (e1), Bob (e2), Carlos (e3) = 3 entidades. (Daniel está em quarentena; Elena não tem claims).
	if m.Overview.TotalEntities != 3 {
		t.Errorf("Overview.TotalEntities: esperado 3, obteve %d", m.Overview.TotalEntities)
	}
	// Claims públicas: c1, c2, c3, c4 = 4 claims. (c5 quarentena, c6 sem fonte ativa supports).
	if m.Overview.TotalClaims != 4 {
		t.Errorf("Overview.TotalClaims: esperado 4, obteve %d", m.Overview.TotalClaims)
	}
	// Fontes públicas distintas ativas: s1 e s2 = 2 fontes.
	if m.Overview.TotalSources != 2 {
		t.Errorf("Overview.TotalSources: esperado 2, obteve %d", m.Overview.TotalSources)
	}

	// 2. Rede Elegível (metric_eligible = 1):
	// Entidades elegíveis: Alice (e1) e Bob (e2) = 2 entidades. (Carlos tem apenas c4 com metric_eligible = 0).
	if m.Network.EligibleEntities != 2 {
		t.Errorf("Network.EligibleEntities: esperado 2, obteve %d", m.Network.EligibleEntities)
	}
	// Claims elegíveis: c1 (Grau A), c2 (Grau B), c3 (Grau A) = 3 claims.
	if m.Network.EligibleClaims != 3 {
		t.Errorf("Network.EligibleClaims: esperado 3, obteve %d", m.Network.EligibleClaims)
	}

	// 3. Distribuição por Grau (sobre as 3 alegações elegíveis):
	// Grau A: 2 (66.7%)
	// Grau B: 1 (33.3%)
	// Grau C: 0 (0.0%)
	// Grau D: 0 (0.0%)
	// Grau E: 0 (0.0%)
	gradeCounts := make(map[string]int64)
	for _, g := range m.GradeDist {
		gradeCounts[g.Grade] = g.Count
	}
	if gradeCounts["A"] != 2 || gradeCounts["B"] != 1 || gradeCounts["C"] != 0 || gradeCounts["D"] != 0 || gradeCounts["E"] != 0 {
		t.Errorf("GradeDist incorreto: %+v", gradeCounts)
	}

	// 4. Distribuição por Relevância (sobre as 2 entidades elegíveis e1 e e2):
	// Rel 5 (Alice): 1 (50.0%)
	// Rel 4 (Bob): 1 (50.0%)
	// Rel 3 (Carlos): 0 (0.0%) - Carlos não é elegível!
	// Rel 2 (Daniel): 0 (0.0%)
	// Rel 1 (Elena): 0 (0.0%)
	relCounts := make(map[int64]int64)
	for _, r := range m.RelevanceDist {
		relCounts[r.Relevance] = r.Count
	}
	if relCounts[5] != 1 || relCounts[4] != 1 || relCounts[3] != 0 || relCounts[2] != 0 || relCounts[1] != 0 {
		t.Errorf("RelevanceDist incorreto: %+v", relCounts)
	}

	// 5. Categorias: Politica=1, Empresarial=1
	if len(m.CategoryDist) != 2 {
		t.Errorf("CategoryDist: esperado 2 categorias, obteve %d", len(m.CategoryDist))
	}

	// 6. Recentes elegíveis: 3 claims (c1, c2, c3)
	if len(m.RecentClaims) != 3 {
		t.Errorf("RecentClaims: esperado 3 claims recentes, obteve %d", len(m.RecentClaims))
	}
}

func insertEntity(t *testing.T, db *sql.DB, id, slug, name, category string, rel int) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO entities (id, type, name, normalized_name, slug, category, role_or_context, reach, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES (?, 'person', ?, ?, ?, ?, 'context', 'nacional', 'summary', ?, 'rationale', ?, ?)
	`, id, name, name, slug, category, rel, now, now)
	if err != nil {
		t.Fatalf("insertEntity %s: %v", id, err)
	}
}

func insertCase(t *testing.T, db *sql.DB, id, slug, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO cases (id, slug, name, description, created_at, updated_at)
		VALUES (?, ?, ?, 'sum', ?, ?)
	`, id, slug, name, now, now)
	if err != nil {
		t.Fatalf("insertCase %s: %v", id, err)
	}
}

func insertRel(t *testing.T, db *sql.DB, id, sub, tgt, caseID, relType, summary string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	var tgtVal, caseVal sql.NullString
	if tgt != "" {
		tgtVal = sql.NullString{String: tgt, Valid: true}
	} else if caseID != "" {
		caseVal = sql.NullString{String: caseID, Valid: true}
	}
	_, err := db.Exec(`
		INSERT INTO relationships (id, subject_entity_id, target_entity_id, case_id, relationship_type, summary, context_limits, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'limits', ?, ?)
	`, id, sub, tgtVal, caseVal, relType, summary, now, now)
	if err != nil {
		t.Fatalf("insertRel %s: %v", id, err)
	}
}

func insertSource(t *testing.T, db *sql.DB, id, title string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES (?, ?, 'Autor', 'http://ex.com/' || ?, 'http://ex.com/' || ?, 'news_article', 'not_checked', ?, ?)
	`, id, title, id, id, now, now)
	if err != nil {
		t.Fatalf("insertSource %s: %v", id, err)
	}
}

func insertClaim(t *testing.T, db *sql.DB, id, relID, grade, disp string, eligible int, status, updatedAt string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, created_at, updated_at)
		VALUES (?, ?, 'proposicao ' || ?, 'attr', 'curated_seed', ?, ?, ?, ?, 'as_is', ?, ?)
	`, id, relID, id, grade, disp, eligible, status, now, updatedAt)
	if err != nil {
		t.Fatalf("insertClaim %s: %v", id, err)
	}
}

func insertEvidenceAndSource(t *testing.T, db *sql.DB, evID, claimID, esID, sourceID, role, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO evidence (id, claim_id, summary, evidence_type, created_at, updated_at)
		VALUES (?, ?, 'ev summary', 'document', ?, ?)
	`, evID, claimID, now, now)
	if err != nil {
		t.Fatalf("insertEvidence %s: %v", evID, err)
	}

	_, err = db.Exec(`
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status, created_at, updated_at)
		VALUES (?, ?, ?, 'excerpt', 'locator', ?, ?, ?, ?)
	`, esID, evID, sourceID, role, status, now, now)
	if err != nil {
		t.Fatalf("insertEvidenceSource %s: %v", esID, err)
	}
}
