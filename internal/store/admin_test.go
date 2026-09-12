package store_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestAdminOverviewCounts(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Inserir entidades, fontes, claims, candidatos, runs
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "ent-1", "slug-1", "Entidade 1", "Politica", 5)
	insertEntity(t, db, "ent-2", "slug-2", "Entidade 2", "Empresarial", 4)

	insertCase(t, db, "case-1", "case-slug-1", "Caso 1")
	insertRel(t, db, "rel-1", "ent-1", "ent-2", "case-1", "contato", "Resumo")

	insertSource(t, db, "src-1", "Fonte Reachable")
	_, err := db.ExecContext(ctx, "UPDATE sources SET source_access_status = 'reachable' WHERE id = 'src-1'")
	if err != nil {
		t.Fatalf("erro ao atualizar source: %v", err)
	}

	insertSource(t, db, "src-2", "Fonte Unreachable")
	_, err = db.ExecContext(ctx, "UPDATE sources SET source_access_status = 'unreachable' WHERE id = 'src-2'")
	if err != nil {
		t.Fatalf("erro ao atualizar source: %v", err)
	}

	insertClaim(t, db, "clm-1", "rel-1", "A", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev-1", "clm-1", "es-1", "src-1", "supports", "active")

	insertClaim(t, db, "clm-2", "rel-1", "D", "possible_link", 0, "quarantined", now)
	insertEvidenceAndSource(t, db, "ev-2", "clm-2", "es-2", "src-2", "supports", "active")

	insertClaim(t, db, "clm-3", "rel-1", "E", "context_only", 0, "rejected", now)
	insertEvidenceAndSource(t, db, "ev-3", "clm-3", "es-3", "src-1", "contradicts", "rejected")

	// Inserir monitoring run
	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES ('run-1', 'completed', 'Daniel Vorcaro', 'openai/gpt-4o', ?)
	`, now)
	if err != nil {
		t.Fatalf("erro ao inserir run: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES ('run-2', 'failed', 'Banco Master', 'openai/gpt-4o', ?)
	`, now)
	if err != nil {
		t.Fatalf("erro ao inserir run: %v", err)
	}

	// Inserir candidatos
	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			proposition, suggested_grade, source_url, canonical_url, technical_confidence,
			editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id, created_at, updated_at
		) VALUES
		('cand-1', 'run-1', 'v1:fp1', 'Entidade 1', 'entidade 1', 'Proposição 1', 'A', 'https://src1.com', 'https://src1.com', 0.95, 'published', 0, '', NULL, ?, ?),
		('cand-2', 'run-1', 'v1:fp2', 'Entidade 2', 'entidade 2', 'Proposição 2', 'D', 'https://src2.com', 'https://src2.com', 0.70, 'quarantined', 0, '', NULL, ?, ?),
		('cand-3', 'run-1', 'v1:fp1', 'Entidade 1', 'entidade 1', 'Proposição 1', 'A', 'https://src1.com', 'https://src1.com', 0.95, 'quarantined', 1, 'same_run_duplicate', 'cand-1', ?, ?)
	`, now, now, now, now, now, now)
	if err != nil {
		t.Fatalf("erro ao inserir candidatos: %v", err)
	}

	counts, err := store.GetAdminOverviewCounts(ctx, db)
	if err != nil {
		t.Fatalf("GetAdminOverviewCounts falhou: %v", err)
	}

	if counts.TotalSources != 2 {
		t.Errorf("TotalSources: esperado 2, obtido %d", counts.TotalSources)
	}
	if counts.SourcesReachable != 1 {
		t.Errorf("SourcesReachable: esperado 1, obtido %d", counts.SourcesReachable)
	}
	if counts.SourcesUnreachable != 1 {
		t.Errorf("SourcesUnreachable: esperado 1, obtido %d", counts.SourcesUnreachable)
	}

	if counts.TotalClaims != 3 {
		t.Errorf("TotalClaims: esperado 3, obtido %d", counts.TotalClaims)
	}
	if counts.ClaimsPublished != 1 {
		t.Errorf("ClaimsPublished: esperado 1, obtido %d", counts.ClaimsPublished)
	}
	if counts.ClaimsQuarantined != 1 {
		t.Errorf("ClaimsQuarantined: esperado 1, obtido %d", counts.ClaimsQuarantined)
	}
	if counts.ClaimsRejected != 1 {
		t.Errorf("ClaimsRejected: esperado 1, obtido %d", counts.ClaimsRejected)
	}

	if counts.TotalEvidenceSources != 3 {
		t.Errorf("TotalEvidenceSources: esperado 3, obtido %d", counts.TotalEvidenceSources)
	}
	if counts.EvidenceSourcesActive != 2 {
		t.Errorf("EvidenceSourcesActive: esperado 2, obtido %d", counts.EvidenceSourcesActive)
	}
	if counts.EvidenceSourcesRejected != 1 {
		t.Errorf("EvidenceSourcesRejected: esperado 1, obtido %d", counts.EvidenceSourcesRejected)
	}

	if counts.TotalCandidates != 3 {
		t.Errorf("TotalCandidates: esperado 3, obtido %d", counts.TotalCandidates)
	}
	if counts.CandidatesPublished != 1 {
		t.Errorf("CandidatesPublished: esperado 1, obtido %d", counts.CandidatesPublished)
	}
	if counts.CandidatesQuarantined != 2 {
		t.Errorf("CandidatesQuarantined: esperado 2, obtido %d", counts.CandidatesQuarantined)
	}
	if counts.CandidatesDuplicates != 1 {
		t.Errorf("CandidatesDuplicates: esperado 1, obtido %d", counts.CandidatesDuplicates)
	}

	if counts.TotalRuns != 2 {
		t.Errorf("TotalRuns: esperado 2, obtido %d", counts.TotalRuns)
	}
	if counts.RunsCompleted != 1 {
		t.Errorf("RunsCompleted: esperado 1, obtido %d", counts.RunsCompleted)
	}
	if counts.RunsFailed != 1 {
		t.Errorf("RunsFailed: esperado 1, obtido %d", counts.RunsFailed)
	}

	if counts.TotalEntities != 2 {
		t.Errorf("TotalEntities: esperado 2, obtido %d", counts.TotalEntities)
	}
}

func TestAdminCandidates_ListingFilteringAndDetail(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "ent-a", "ent-a", "Entidade Alfa", "Politica", 5)
	insertEntity(t, db, "ent-b", "ent-b", "Entidade Beta", "Empresarial", 4)
	insertCase(t, db, "case-a", "case-a", "Operação Master")

	_, err := db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (id, status, query, discovery_provider, discovery_model, verification_provider, verification_model, created_at)
		VALUES ('run-alpha', 'completed', 'Investigação Especial', 'openrouter', 'anthropic/claude-3.5-sonnet', 'openrouter', 'openai/gpt-4o', ?)
	`, now)
	if err != nil {
		t.Fatalf("erro ao inserir run: %v", err)
	}

	// Inserir candidatos com variados status, graus e dados relacionais
	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			target_entity_name, normalized_target_entity_name, relationship_type,
			proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
			technical_confidence, editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
			resolved_subject_entity_id, resolved_target_entity_id, structural_gate_passed, semantic_gate_passed, policy_action,
			created_at, updated_at
		) VALUES
		('c-1', 'run-alpha', 'v1:fp1', 'Entidade Alfa', 'entidade alfa', 'Entidade Beta', 'entidade beta', 'societário', 'Alfa comprou participação em Beta com 100% de desconto', 'A', 'https://noticias.com/1', 'https://noticias.com/1', 'Notícia 1', 'Jornal X', 0.98, 'published', 0, '', NULL, 'ent-a', 'ent-b', 1, 1, 'publish', ?, ?),
		('c-2', 'run-alpha', 'v1:fp2', 'Entidade Alfa', 'entidade alfa', '', '', '', 'Alfa citada indiretamente', 'E', 'https://noticias.com/2', 'https://noticias.com/2', 'Notícia 2', 'Jornal Y', 0.60, 'quarantined', 0, '', NULL, 'ent-a', NULL, 1, 0, 'quarantine', ?, ?),
		('c-3', 'run-alpha', 'v1:fp1', 'Entidade Alfa', 'entidade alfa', 'Entidade Beta', 'entidade beta', 'societário', 'Alfa comprou participação em Beta com 100% de desconto', 'A', 'https://noticias.com/1', 'https://noticias.com/1', 'Notícia 1', 'Jornal X', 0.98, 'quarantined', 1, 'same_run_duplicate', 'c-1', 'ent-a', 'ent-b', 1, 1, 'quarantine', ?, ?),
		('c-4', 'run-alpha', 'v1:fp3', 'Entidade Beta', 'entidade beta', '', '', '', 'Beta teve pedido rejeitado', 'D', 'https://noticias.com/4', 'https://noticias.com/4', 'Notícia 4', 'Jornal Z', 0.75, 'rejected', 0, '', NULL, 'ent-b', NULL, 0, 0, 'quarantine', ?, ?)
	`, now, now, now, now, now, now, now, now)
	if err != nil {
		t.Fatalf("erro ao inserir candidatos: %v", err)
	}

	// Inserir avaliação semântica para c-1
	_, err = db.ExecContext(ctx, `
		INSERT INTO semantic_evaluations (
			id, monitoring_candidate_id, provider, model, schema_version,
			identity_match, claim_supported, claim_overstates_source, attribution_explicit,
			grade_compatible, contains_illicit_inference, uncertainties, recommended_action,
			raw_response, prompt_tokens, completion_tokens, total_tokens, cost_microusd, cost, created_at
		) VALUES (
			'se-1', 'c-1', 'openrouter', 'openai/gpt-4o', 'v1',
			1, 1, 0, 1, 1, 0, '[]', 'publish',
			'{"raw":"response_secret"}', 500, 150, 650, 3250, 0.00325, ?
		)
	`, now)
	if err != nil {
		t.Fatalf("erro ao inserir semantic evaluation: %v", err)
	}

	// 1. Listagem completa
	resAll, err := store.ListAdminCandidates(ctx, db, store.AdminCandidateFilter{})
	if err != nil {
		t.Fatalf("ListAdminCandidates falhou: %v", err)
	}
	if resAll.TotalCount != 4 {
		t.Errorf("TotalCount: esperado 4, obtido %d", resAll.TotalCount)
	}
	if len(resAll.Candidates) != 4 {
		t.Errorf("len(Candidates): esperado 4, obtido %d", len(resAll.Candidates))
	}

	// 2. Filtro por status
	resQuarantined, err := store.ListAdminCandidates(ctx, db, store.AdminCandidateFilter{Status: "quarantined"})
	if err != nil {
		t.Fatalf("ListAdminCandidates quarantined falhou: %v", err)
	}
	if resQuarantined.TotalCount != 2 {
		t.Errorf("Quarantined count: esperado 2, obtido %d", resQuarantined.TotalCount)
	}

	// 3. Filtro por grau
	resGradeA, err := store.ListAdminCandidates(ctx, db, store.AdminCandidateFilter{Grade: "A"})
	if err != nil {
		t.Fatalf("ListAdminCandidates Grade A falhou: %v", err)
	}
	if resGradeA.TotalCount != 2 {
		t.Errorf("Grade A count: esperado 2, obtido %d", resGradeA.TotalCount)
	}

	// 4. Busca textual com escape de caracteres especiais
	resSearch, err := store.ListAdminCandidates(ctx, db, store.AdminCandidateFilter{Search: "100%"})
	if err != nil {
		t.Fatalf("ListAdminCandidates search falhou: %v", err)
	}
	if resSearch.TotalCount != 2 {
		t.Errorf("Search '100%%' count: esperado 2, obtido %d", resSearch.TotalCount)
	}

	// 5. Paginação
	resPage, err := store.ListAdminCandidates(ctx, db, store.AdminCandidateFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListAdminCandidates paginado falhou: %v", err)
	}
	if len(resPage.Candidates) != 2 || resPage.TotalPages != 2 {
		t.Errorf("Paginação: esperado 2 itens e 2 páginas, obtido %d itens e %d páginas", len(resPage.Candidates), resPage.TotalPages)
	}

	// 6. Detalhe de candidato existente com avaliação
	detail, err := store.GetAdminCandidateDetail(ctx, db, "c-1")
	if err != nil {
		t.Fatalf("GetAdminCandidateDetail c-1 falhou: %v", err)
	}
	if detail.Candidate.ID != "c-1" {
		t.Errorf("detail ID: esperado c-1, obtido %s", detail.Candidate.ID)
	}
	if detail.Candidate.ResolvedSubjectName != "Entidade Alfa" {
		t.Errorf("resolved subject name: esperado 'Entidade Alfa', obtido %q", detail.Candidate.ResolvedSubjectName)
	}
	if len(detail.Evaluations) != 1 {
		t.Fatalf("len(Evaluations): esperado 1, obtido %d", len(detail.Evaluations))
	}
	if detail.Evaluations[0].RecommendedAction != "publish" {
		t.Errorf("RecommendedAction: esperado 'publish', obtido %q", detail.Evaluations[0].RecommendedAction)
	}

	// 7. Detalhe de candidato inexistente -> ErrNotFound
	_, err404 := store.GetAdminCandidateDetail(ctx, db, "c-inexistente")
	if !errors.Is(err404, store.ErrNotFound) {
		t.Errorf("esperado ErrNotFound para candidato inexistente, obtido %v", err404)
	}
}

func TestAdminEvidenceSources_ListingAndFiltering(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "ent-1", "slug-1", "Carlos Andrade", "Politica", 5)
	insertEntity(t, db, "ent-2", "slug-2", "Banco Master", "Empresarial", 5)
	insertCase(t, db, "case-1", "caso-principal", "Caso Principal")
	insertRel(t, db, "rel-1", "ent-1", "ent-2", "case-1", "societário", "Resumo sociedade")

	insertSource(t, db, "src-1", "Diário Oficial da União")
	insertSource(t, db, "src-2", "Portal G1")

	insertClaim(t, db, "clm-1", "rel-1", "A", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev-1", "clm-1", "es-1", "src-1", "supports", "active")
	insertEvidenceAndSource(t, db, "ev-1b", "clm-1", "es-2", "src-2", "contradicts", "active")

	insertClaim(t, db, "clm-2", "rel-1", "D", "possible_link", 0, "quarantined", now)
	insertEvidenceAndSource(t, db, "ev-2", "clm-2", "es-3", "src-2", "supports", "rejected")

	// 1. Listagem completa
	resAll, err := store.ListAdminEvidenceSources(ctx, db, store.AdminEvidenceFilter{})
	if err != nil {
		t.Fatalf("ListAdminEvidenceSources falhou: %v", err)
	}
	if resAll.TotalCount != 3 {
		t.Errorf("TotalCount: esperado 3, obtido %d", resAll.TotalCount)
	}

	// 2. Filtro por papel (role = contradicts)
	resContradicts, err := store.ListAdminEvidenceSources(ctx, db, store.AdminEvidenceFilter{Role: "contradicts"})
	if err != nil {
		t.Fatalf("ListAdminEvidenceSources role contradicts falhou: %v", err)
	}
	if resContradicts.TotalCount != 1 {
		t.Errorf("Contradicts count: esperado 1, obtido %d", resContradicts.TotalCount)
	}

	// 3. Filtro por claim_status (quarantined)
	resQuarantined, err := store.ListAdminEvidenceSources(ctx, db, store.AdminEvidenceFilter{ClaimStatus: "quarantined"})
	if err != nil {
		t.Fatalf("ListAdminEvidenceSources quarantined falhou: %v", err)
	}
	if resQuarantined.TotalCount != 1 {
		t.Errorf("Claim status quarantined count: esperado 1, obtido %d", resQuarantined.TotalCount)
	}

	// 4. Filtro por evidence_source_status (rejected)
	resEsRejected, err := store.ListAdminEvidenceSources(ctx, db, store.AdminEvidenceFilter{EvidenceSourceStatus: "rejected"})
	if err != nil {
		t.Fatalf("ListAdminEvidenceSources es rejected falhou: %v", err)
	}
	if resEsRejected.TotalCount != 1 {
		t.Errorf("ES rejected count: esperado 1, obtido %d", resEsRejected.TotalCount)
	}

	// 5. Busca textual
	resSearch, err := store.ListAdminEvidenceSources(ctx, db, store.AdminEvidenceFilter{Search: "Carlos Andrade"})
	if err != nil {
		t.Fatalf("ListAdminEvidenceSources search falhou: %v", err)
	}
	if resSearch.TotalCount != 3 {
		t.Errorf("Search 'Carlos Andrade' count: esperado 3, obtido %d", resSearch.TotalCount)
	}
}

func TestAdminSources_ListingAndFiltering(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "ent-1", "slug-1", "Pessoa", "Politica", 3)
	insertCase(t, db, "case-1", "caso-1", "Caso 1")
	insertRel(t, db, "rel-1", "ent-1", "", "case-1", "contato", "Resumo")
	insertClaim(t, db, "clm-1", "rel-1", "A", "supports_link", 1, "published", now)

	insertSource(t, db, "src-1", "Folha de S.Paulo - Matéria Especial")
	_, _ = db.ExecContext(ctx, "UPDATE sources SET source_access_status = 'reachable', source_type = 'article' WHERE id = 'src-1'")

	insertSource(t, db, "src-2", "Nota Oficial do Banco")
	_, _ = db.ExecContext(ctx, "UPDATE sources SET source_access_status = 'cited_by_provider', source_type = 'official_statement' WHERE id = 'src-2'")

	insertEvidenceAndSource(t, db, "ev-1", "clm-1", "es-1", "src-1", "supports", "active")
	insertEvidenceAndSource(t, db, "ev-1b", "clm-1", "es-2", "src-1", "supports", "rejected")

	// 1. Listagem completa
	resAll, err := store.ListAdminSources(ctx, db, store.AdminSourceFilter{})
	if err != nil {
		t.Fatalf("ListAdminSources falhou: %v", err)
	}
	if resAll.TotalCount != 2 {
		t.Errorf("TotalCount: esperado 2, obtido %d", resAll.TotalCount)
	}

	// Verifica contagem de usos
	var src1Row sqlc.ListAdminSourcesRow
	for _, s := range resAll.Sources {
		if s.ID == "src-1" {
			src1Row = s
			break
		}
	}
	if src1Row.TotalUses != 2 || src1Row.ActiveUses != 1 {
		t.Errorf("src-1 usos: esperado total=2 e active=1, obtido total=%d e active=%d", src1Row.TotalUses, src1Row.ActiveUses)
	}

	// 2. Filtro por access_status
	resReachable, err := store.ListAdminSources(ctx, db, store.AdminSourceFilter{AccessStatus: "reachable"})
	if err != nil {
		t.Fatalf("ListAdminSources reachable falhou: %v", err)
	}
	if resReachable.TotalCount != 1 || resReachable.Sources[0].ID != "src-1" {
		t.Errorf("Reachable filter: esperado src-1, obtido %+v", resReachable.Sources)
	}

	// 3. Filtro por source_type
	resStatement, err := store.ListAdminSources(ctx, db, store.AdminSourceFilter{SourceType: "official_statement"})
	if err != nil {
		t.Fatalf("ListAdminSources statement falhou: %v", err)
	}
	if resStatement.TotalCount != 1 || resStatement.Sources[0].ID != "src-2" {
		t.Errorf("SourceType filter: esperado src-2, obtido %+v", resStatement.Sources)
	}

	// 4. Busca textual
	resSearch, err := store.ListAdminSources(ctx, db, store.AdminSourceFilter{Search: "Folha"})
	if err != nil {
		t.Fatalf("ListAdminSources search falhou: %v", err)
	}
	if resSearch.TotalCount != 1 {
		t.Errorf("Search 'Folha' count: esperado 1, obtido %d", resSearch.TotalCount)
	}
}

func TestAdminIsolationFromPublicQueries(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	// Inserir entidade exclusivamente com claim em quarentena
	insertEntity(t, db, "ent-quarantined-only", "ent-quarantined", "Entidade Quarentenada", "Politica", 3)
	insertCase(t, db, "case-q", "case-q", "Caso Q")
	insertRel(t, db, "rel-q", "ent-quarantined-only", "", "case-q", "mencao", "Mencao")
	insertSource(t, db, "src-q", "Fonte Q")
	insertClaim(t, db, "clm-q", "rel-q", "E", "possible_link", 0, "quarantined", now)
	insertEvidenceAndSource(t, db, "ev-q", "clm-q", "es-q", "src-q", "supports", "active")

	// 1. Consulta pública NÃO deve encontrar
	publicRes, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{})
	if err != nil {
		t.Fatalf("ListPublicEntities falhou: %v", err)
	}
	if publicRes.TotalCount != 0 {
		t.Errorf("Público não deveria ver entidade com claim em quarentena, obteve %d", publicRes.TotalCount)
	}

	// 2. Consulta administrativa de evidências DEVE encontrar
	adminEvRes, err := store.ListAdminEvidenceSources(ctx, db, store.AdminEvidenceFilter{})
	if err != nil {
		t.Fatalf("ListAdminEvidenceSources falhou: %v", err)
	}
	if adminEvRes.TotalCount != 1 {
		t.Errorf("Admin deve ver evidence_source de claim em quarentena, obteve %d", adminEvRes.TotalCount)
	}
	if !strings.Contains(adminEvRes.EvidenceSources[0].SubjectEntityName, "Entidade Quarentenada") {
		t.Errorf("Nome da entidade em quarentena não encontrado no admin")
	}
}

func TestAdminSourceFilter_SourceTypeAllowlist(t *testing.T) {
	// 1. Tipos canônicos permitidos devem ser preservados e válidos
	canonicalOptions := store.GetAdminSourceTypeOptions()
	if len(canonicalOptions) != 6 {
		t.Fatalf("esperado 6 opções canônicas de source_type, obtido %d", len(canonicalOptions))
	}

	expectedValues := map[string]string{
		"article":            "Artigo / Notícia",
		"official_statement": "Nota Oficial / Comunicado",
		"court_document":     "Peça Judicial / Decisão",
		"police_report":      "Relatório Policial / Pericial",
		"interview":          "Entrevista",
		"social_media":       "Rede Social",
	}

	for _, opt := range canonicalOptions {
		expectedLabel, ok := expectedValues[opt.Value]
		if !ok {
			t.Errorf("opção inesperada na lista canônica: %s", opt.Value)
		}
		if opt.Label != expectedLabel {
			t.Errorf("label para %s: esperado %q, obtido %q", opt.Value, expectedLabel, opt.Label)
		}
		if !store.IsAllowedAdminSourceType(opt.Value) {
			t.Errorf("tipo canônico %q deve ser reconhecido por IsAllowedAdminSourceType", opt.Value)
		}
		f := store.SanitizeAdminSourceFilter(store.AdminSourceFilter{SourceType: opt.Value})
		if f.SourceType != opt.Value {
			t.Errorf("tipo permitido %q deveria ser preservado, obtido %q", opt.Value, f.SourceType)
		}
	}

	// 2. Tipos desconhecidos devem ser descartados (resultando em filtro seguro vazio)
	unknownTypes := []string{
		"blog",
		"forum",
		"arbitrary_type",
		"'; DROP TABLE sources; --",
		"UNKNOWN",
	}
	for _, unk := range unknownTypes {
		if store.IsAllowedAdminSourceType(unk) {
			t.Errorf("tipo desconhecido %q não deveria ser permitido", unk)
		}
		f := store.SanitizeAdminSourceFilter(store.AdminSourceFilter{SourceType: unk})
		if f.SourceType != "" {
			t.Errorf("tipo desconhecido %q deveria ser descartado para '', obtido %q", unk, f.SourceType)
		}
	}

	// 3. Tipos excessivamente longos devem ser descartados
	longType := strings.Repeat("a", 500)
	fLong := store.SanitizeAdminSourceFilter(store.AdminSourceFilter{SourceType: longType})
	if fLong.SourceType != "" {
		t.Errorf("tipo excessivamente longo deveria ser descartado para '', obtido %q", fLong.SourceType)
	}

	// 4. Teste de listagem no banco com tipo desconhecido não causa erro e age como busca sem filtro
	db, ctx := setupTestDB(t)
	insertSource(t, db, "src-al-1", "Fonte 1")
	_, _ = db.ExecContext(ctx, "UPDATE sources SET source_type = 'article' WHERE id = 'src-al-1'")
	insertSource(t, db, "src-al-2", "Fonte 2")
	_, _ = db.ExecContext(ctx, "UPDATE sources SET source_type = 'interview' WHERE id = 'src-al-2'")

	res, err := store.ListAdminSources(ctx, db, store.AdminSourceFilter{SourceType: "invalid_unsupported_type"})
	if err != nil {
		t.Fatalf("ListAdminSources com tipo desconhecido não deveria falhar: %v", err)
	}
	if res.TotalCount != 2 {
		t.Errorf("tipo desconhecido deve retornar todos os itens (filtro vazio seguro), esperado 2, obtido %d", res.TotalCount)
	}
}

func TestGetAdminClaimDetail(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "ent-sub-clm", "slug-sub-clm", "Sujeito Claim", "Politica", 5)
	insertEntity(t, db, "ent-tgt-clm", "slug-tgt-clm", "Alvo Claim", "Empresarial", 4)
	insertCase(t, db, "case-clm-1", "case-clm-slug-1", "Caso do Claim")
	insertRel(t, db, "rel-clm-1", "ent-sub-clm", "ent-tgt-clm", "case-clm-1", "societario", "Resumo Relacao")
	insertSource(t, db, "src-clm-1", "Fonte do Claim")

	insertClaim(t, db, "clm-detail-1", "rel-clm-1", "A", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev-clm-1", "clm-detail-1", "es-clm-1", "src-clm-1", "supports", "active")
	insertEvidenceAndSource(t, db, "ev-clm-2", "clm-detail-1", "es-clm-2", "src-clm-1", "contradicts", "active")

	// Inserir decisão prévia
	q := sqlc.New(db)
	_, err := q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-clm-1",
		ClaimID:              sql.NullString{String: "clm-detail-1", Valid: true},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "approve",
		Reason:               "Aprovação prévia auditada.",
		Actor:                "admin_seed",
		CandidateFingerprint: "v1:fp-clm-1",
		CreatedAt:            now,
	})
	if err != nil {
		t.Fatalf("falha ao criar decisão de teste: %v", err)
	}

	detail, err := store.GetAdminClaimDetail(ctx, db, "clm-detail-1")
	if err != nil {
		t.Fatalf("GetAdminClaimDetail falhou: %v", err)
	}

	if detail.Claim.ID != "clm-detail-1" {
		t.Errorf("Claim.ID = %q, esperado 'clm-detail-1'", detail.Claim.ID)
	}
	if detail.Claim.SubjectEntityName != "Sujeito Claim" {
		t.Errorf("SubjectEntityName = %q, esperado 'Sujeito Claim'", detail.Claim.SubjectEntityName)
	}
	if detail.Claim.TargetEntityName != "Alvo Claim" {
		t.Errorf("TargetEntityName = %q, esperado 'Alvo Claim'", detail.Claim.TargetEntityName)
	}
	if len(detail.EvidenceSources) != 2 {
		t.Errorf("EvidenceSources = %d, esperado 2", len(detail.EvidenceSources))
	}
	if len(detail.Decisions) != 1 {
		t.Errorf("Decisions = %d, esperado 1", len(detail.Decisions))
	}
	if detail.Decisions[0].Action != "approve" || detail.Decisions[0].Actor != "admin_seed" {
		t.Errorf("Decisão inesperada: %+v", detail.Decisions[0])
	}

	// Claim inexistente retorna ErrNotFound
	_, err = store.GetAdminClaimDetail(ctx, db, "inexistente")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("esperava ErrNotFound para claim inexistente, obtido %v", err)
	}

	// ID vazio retorna ErrNotFound
	_, err = store.GetAdminClaimDetail(ctx, db, "   ")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("esperava ErrNotFound para id vazio, obtido %v", err)
	}
}

func TestGetAdminEvidenceSourceDetail(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	insertEntity(t, db, "ent-sub-es", "slug-sub-es", "Sujeito ES", "Politica", 5)
	insertEntity(t, db, "ent-tgt-es", "slug-tgt-es", "Alvo ES", "Empresarial", 4)
	insertCase(t, db, "case-es-1", "case-es-slug-1", "Caso do ES")
	insertRel(t, db, "rel-es-1", "ent-sub-es", "ent-tgt-es", "case-es-1", "societario", "Resumo Relacao ES")
	insertSource(t, db, "src-es-1", "Fonte do ES")

	insertClaim(t, db, "clm-es-1", "rel-es-1", "A", "supports_link", 1, "published", now)
	insertEvidenceAndSource(t, db, "ev-es-1", "clm-es-1", "es-detail-1", "src-es-1", "supports", "active")
	insertEvidenceAndSource(t, db, "ev-es-2", "clm-es-1", "es-detail-2", "src-es-1", "supports", "active")

	// Inserir decisão prévia para o evidence_source
	q := sqlc.New(db)
	_, err := q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-es-1",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: "es-detail-1", Valid: true},
		Action:               "reject",
		Reason:               "Trecho impreciso e fora de contexto.",
		Actor:                "moderador_teste",
		CandidateFingerprint: "",
		CreatedAt:            now,
	})
	if err != nil {
		t.Fatalf("falha ao criar decisão de moderação para evidence_source: %v", err)
	}

	detail, err := store.GetAdminEvidenceSourceDetail(ctx, db, "es-detail-1")
	if err != nil {
		t.Fatalf("GetAdminEvidenceSourceDetail falhou: %v", err)
	}

	if detail.EvidenceSource.EvidenceSourceID != "es-detail-1" {
		t.Errorf("EvidenceSourceID = %q, esperado 'es-detail-1'", detail.EvidenceSource.EvidenceSourceID)
	}
	if detail.EvidenceSource.ClaimID != "clm-es-1" {
		t.Errorf("ClaimID = %q, esperado 'clm-es-1'", detail.EvidenceSource.ClaimID)
	}
	if detail.EvidenceSource.SubjectEntityName != "Sujeito ES" {
		t.Errorf("SubjectEntityName = %q, esperado 'Sujeito ES'", detail.EvidenceSource.SubjectEntityName)
	}
	if detail.ActiveSupportsCount != 2 {
		t.Errorf("ActiveSupportsCount = %d, esperado 2", detail.ActiveSupportsCount)
	}
	if detail.OtherActiveSupportsCount != 1 {
		t.Errorf("OtherActiveSupportsCount = %d, esperado 1", detail.OtherActiveSupportsCount)
	}
	if len(detail.Decisions) != 1 {
		t.Errorf("Decisions = %d, esperado 1", len(detail.Decisions))
	}
	if detail.Decisions[0].Action != "reject" || detail.Decisions[0].Actor != "moderador_teste" {
		t.Errorf("Decisão inesperada: %+v", detail.Decisions[0])
	}

	// EvidenceSource inexistente retorna ErrNotFound
	_, err = store.GetAdminEvidenceSourceDetail(ctx, db, "inexistente")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("esperava ErrNotFound para evidence_source inexistente, obtido %v", err)
	}

	// ID vazio retorna ErrNotFound
	_, err = store.GetAdminEvidenceSourceDetail(ctx, db, "   ")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("esperava ErrNotFound para id vazio, obtido %v", err)
	}
}
