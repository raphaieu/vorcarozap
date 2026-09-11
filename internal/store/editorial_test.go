package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func setupTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao executar migrations: %v", err)
	}

	return db, ctx
}

func TestMigrationRollbackAndReapply(t *testing.T) {
	db, ctx := setupTestDB(t)

	// Valida que a tabela entities existe após o Migrate
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM entities").Scan(&count); err != nil {
		t.Fatalf("falha ao consultar entities após migrate: %v", err)
	}

	// Executa rollback da migration 00007
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00007: %v", err)
	}

	// Executa rollback da migration 00006
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00006: %v", err)
	}

	// Executa rollback da migration 00005
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00005: %v", err)
	}

	// Executa rollback da migration 00004
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00004: %v", err)
	}

	// Executa rollback da migration 00003
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00003: %v", err)
	}

	// Executa rollback da migration 00002
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00002: %v", err)
	}

	// A tabela entities não deve mais existir
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM entities").Scan(&count); err == nil {
		t.Fatal("esperava erro ao consultar tabela entities após rollback, mas a tabela ainda existe")
	}

	// Re-aplica as migrations
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao re-aplicar migrations após rollback: %v", err)
	}

	// Tabela entities deve existir novamente
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM entities").Scan(&count); err != nil {
		t.Fatalf("falha ao consultar entities após re-migrate: %v", err)
	}
}

func TestEntityConstraints(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Criação válida
	e1, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-1",
		Type:               "person",
		Name:               "Daniel Vorcaro",
		NormalizedName:     "daniel vorcaro",
		Slug:               "daniel-vorcaro",
		RoleOrContext:      "Empresário",
		Summary:            "Controlador do Banco Master",
		Relevance:          4,
		RelevanceRationale: "Investigado em inquérito de repercussão nacional",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade válida: %v", err)
	}
	if e1.Slug != "daniel-vorcaro" {
		t.Errorf("slug inesperado: %s", e1.Slug)
	}

	// 2. Rejeição de relevância fora de 1..5
	tableRelevance := []int{0, 6, -1, 10}
	for _, rel := range tableRelevance {
		_, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
			ID:                 "ent-invalid-rel",
			Type:               "person",
			Name:               "Pessoa Invalida",
			NormalizedName:     "pessoa invalida",
			Slug:               "pessoa-invalida",
			RoleOrContext:      "",
			Summary:            "",
			Relevance:          int64(rel),
			RelevanceRationale: "Qualquer justificativa",
			CreatedAt:          now,
			UpdatedAt:          now,
		})
		if err == nil {
			t.Errorf("esperava erro ao inserir relevância %d, obteve sucesso", rel)
		}
	}

	// 3. Rejeição de EntityType inválido
	_, err = queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-invalid-type",
		Type:               "corporation",
		Name:               "Nome",
		NormalizedName:     "nome",
		Slug:               "nome-corp",
		RoleOrContext:      "",
		Summary:            "",
		Relevance:          3,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err == nil {
		t.Error("esperava erro ao inserir tipo de entidade 'corporation', obteve sucesso")
	}

	// 4. Unicidade de slug
	_, err = queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-dup-slug",
		Type:               "person",
		Name:               "Outro Daniel",
		NormalizedName:     "outro daniel",
		Slug:               "daniel-vorcaro",
		RoleOrContext:      "",
		Summary:            "",
		Relevance:          3,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err == nil {
		t.Error("esperava erro de unicidade de slug, obteve sucesso")
	}

	// 5. Unicidade de alias por entidade
	_, err = queries.CreateEntityAlias(ctx, sqlc.CreateEntityAliasParams{
		ID:              "alias-1",
		EntityID:        "ent-1",
		Alias:           "Dani",
		NormalizedAlias: "dani",
		CreatedAt:       now,
	})
	if err != nil {
		t.Fatalf("falha ao criar alias: %v", err)
	}

	_, err = queries.CreateEntityAlias(ctx, sqlc.CreateEntityAliasParams{
		ID:              "alias-2",
		EntityID:        "ent-1",
		Alias:           "Dani",
		NormalizedAlias: "dani",
		CreatedAt:       now,
	})
	if err == nil {
		t.Error("esperava erro de unicidade de alias normalizado para a mesma entidade, obteve sucesso")
	}
}

func TestRelationshipsConstraints(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Setup entidades e caso
	_, _ = queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID: "ent-a", Type: "person", Name: "Pessoa A", NormalizedName: "pessoa a",
		Slug: "pessoa-a", Relevance: 3, RelevanceRationale: "Motivo A", CreatedAt: now, UpdatedAt: now,
	})
	_, _ = queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID: "ent-b", Type: "person", Name: "Pessoa B", NormalizedName: "pessoa b",
		Slug: "pessoa-b", Relevance: 3, RelevanceRationale: "Motivo B", CreatedAt: now, UpdatedAt: now,
	})
	_, _ = queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID: "case-1", Name: "Operação Teste", Slug: "operacao-teste",
		Description: "Descrição", CreatedAt: now, UpdatedAt: now,
	})

	// 1. Relacionamento válido Entidade -> Entidade
	relEntity, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-1",
		SubjectEntityID:  "ent-a",
		TargetEntityID:   sql.NullString{String: "ent-b", Valid: true},
		CaseID:           sql.NullString{},
		RelationshipType: "sociedade",
		Summary:          "Sócios em empresa",
		ContextLimits:    "Período de 2020 a 2024",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relacionamento entidade-entidade: %v", err)
	}
	if relEntity.ID != "rel-1" {
		t.Errorf("ID inesperado: %s", relEntity.ID)
	}

	// 2. Relacionamento válido Entidade -> Caso
	relCase, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-2",
		SubjectEntityID:  "ent-a",
		TargetEntityID:   sql.NullString{},
		CaseID:           sql.NullString{String: "case-1", Valid: true},
		RelationshipType: "investigado",
		Summary:          "Citado em inquérito",
		ContextLimits:    "Autos nº 123",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relacionamento entidade-caso: %v", err)
	}
	if relCase.ID != "rel-2" {
		t.Errorf("ID inesperado: %s", relCase.ID)
	}

	// 3. XOR: Ambos preenchidos deve falhar
	_, err = queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-invalid-xor-both",
		SubjectEntityID:  "ent-a",
		TargetEntityID:   sql.NullString{String: "ent-b", Valid: true},
		CaseID:           sql.NullString{String: "case-1", Valid: true},
		RelationshipType: "invalido",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err == nil {
		t.Error("esperava erro XOR quando ambos target_entity_id e case_id estão preenchidos, obteve sucesso")
	}

	// 4. XOR: Nenhum preenchido deve falhar
	_, err = queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-invalid-xor-none",
		SubjectEntityID:  "ent-a",
		TargetEntityID:   sql.NullString{},
		CaseID:           sql.NullString{},
		RelationshipType: "invalido",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err == nil {
		t.Error("esperava erro XOR quando nenhum target está preenchido, obteve sucesso")
	}

	// 5. Autorrelação inválida: subject == target
	_, err = queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-self",
		SubjectEntityID:  "ent-a",
		TargetEntityID:   sql.NullString{String: "ent-a", Valid: true},
		CaseID:           sql.NullString{},
		RelationshipType: "autorrelacao",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err == nil {
		t.Error("esperava erro de autorrelação (subject_entity_id == target_entity_id), obteve sucesso")
	}
}

func TestClaimsAndEvidenceIntegrity(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Setup base
	_, _ = queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID: "ent-1", Type: "person", Name: "Daniel Vorcaro", NormalizedName: "daniel vorcaro",
		Slug: "daniel-vorcaro", Relevance: 4, RelevanceRationale: "Justificativa", CreatedAt: now, UpdatedAt: now,
	})
	_, _ = queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID: "ent-2", Type: "organization", Name: "Banco Master", NormalizedName: "banco master",
		Slug: "banco-master", Relevance: 4, RelevanceRationale: "Instituição bancária", CreatedAt: now, UpdatedAt: now,
	})
	_, _ = queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID: "rel-1", SubjectEntityID: "ent-1", TargetEntityID: sql.NullString{String: "ent-2", Valid: true},
		RelationshipType: "controlador", Summary: "Controle acionário", CreatedAt: now, UpdatedAt: now,
	})
	importRun, err := queries.CreateImportRun(ctx, sqlc.CreateImportRunParams{
		ID:             "run-1",
		FilePath:       "_notes/mapa.xlsx",
		FileHash:       "sha256-abc123",
		Origin:         "curated_seed",
		MappingVersion: "import-mapping-v1",
		Status:         "completed",
		IsDryRun:       0,
		SummaryCounts:  `{"entities": 2}`,
		SummaryReport:  "Importação executada com sucesso",
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("falha ao criar import_run: %v", err)
	}

	// 1. Idempotência identificável de import_run por hash e versão
	runFound, err := queries.GetImportRunByHashAndVersion(ctx, sqlc.GetImportRunByHashAndVersionParams{
		FileHash:       "sha256-abc123",
		MappingVersion: "import-mapping-v1",
	})
	if err != nil {
		t.Fatalf("falha ao buscar import_run por hash e versão: %v", err)
	}
	if runFound.ID != importRun.ID {
		t.Errorf("esperava run %s, obteve %s", importRun.ID, runFound.ID)
	}

	// 2. Claim com grau válido A–E e elegibilidade métrica
	claim1, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "claim-1",
		RelationshipID: "rel-1",
		Proposition:    "Daniel Vorcaro é o acionista controlador do Banco Master.",
		Attribution:    "Registro no Banco Central",
		Origin:         "curated_seed",
		Grade:          "A",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		ImportRunID:    sql.NullString{String: "run-1", Valid: true},
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim válido: %v", err)
	}
	if claim1.Grade != "A" || claim1.MetricEligible != 1 {
		t.Errorf("claim inválido: grade=%s metric=%d", claim1.Grade, claim1.MetricEligible)
	}

	// 3. Rejeição de metric_eligible fora de 0/1
	_, err = db.ExecContext(ctx, `INSERT INTO claims (
		id, relationship_id, proposition, origin, grade, disposition, metric_eligible, status, created_at, updated_at
	) VALUES ('claim-bad-metric', 'rel-1', 'Prop', 'admin', 'A', 'supports_link', 2, 'published', ?, ?)`, now, now)
	if err == nil {
		t.Error("esperava erro para metric_eligible = 2, obteve sucesso")
	}

	// 4. Rejeição de enum grade inválido
	_, err = db.ExecContext(ctx, `INSERT INTO claims (
		id, relationship_id, proposition, origin, grade, disposition, metric_eligible, status, created_at, updated_at
	) VALUES ('claim-bad-grade', 'rel-1', 'Prop', 'admin', 'F', 'supports_link', 1, 'published', ?, ?)`, now, now)
	if err == nil {
		t.Error("esperava erro para grade = 'F', obteve sucesso")
	}

	// 5. Rejeição de enum status inválido
	_, err = db.ExecContext(ctx, `INSERT INTO claims (
		id, relationship_id, proposition, origin, grade, disposition, metric_eligible, status, created_at, updated_at
	) VALUES ('claim-bad-status', 'rel-1', 'Prop', 'admin', 'A', 'supports_link', 1, 'invalid_status', ?, ?)`, now, now)
	if err == nil {
		t.Error("esperava erro para status inválido, obteve sucesso")
	}

	// 6. Foreign key inexistente deve falhar
	_, err = queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "claim-bad-fk",
		RelationshipID: "rel-nao-existe",
		Proposition:    "Prop",
		Origin:         "admin",
		Grade:          "A",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err == nil {
		t.Error("esperava erro de foreign key para relationship_id inexistente, obteve sucesso")
	}

	// 7. Evidência e Fonte
	evidence1, err := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-1",
		ClaimID:      "claim-1",
		Summary:      "Certidão de composição acionária",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("falha ao criar evidência: %v", err)
	}

	source1, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-1",
		Title:              "Composição acionária Banco Master",
		PublisherOrAuthor:  "Banco Central do Brasil",
		OriginalUrl:        "https://www.bcb.gov.br/composicao/12345",
		CanonicalUrl:       "https://www.bcb.gov.br/composicao/12345",
		SourceType:         "official_record",
		SourceAccessStatus: "not_checked",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar source: %v", err)
	}

	// 8. Múltiplos evidence_sources para a mesma source com trechos diferentes
	es1, err := queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-1",
		EvidenceID: evidence1.ID,
		SourceID:   source1.ID,
		Excerpt:    "Daniel Vorcaro detém 80% das ações com direito a voto.",
		Locator:    "Página 1, Seção 2",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("falha ao criar evidence_source 1: %v", err)
	}

	es2, err := queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-2",
		EvidenceID: evidence1.ID,
		SourceID:   source1.ID,
		Excerpt:    "Posse homologada pelo comitê em 15 de março.",
		Locator:    "Página 3, Parágrafo 4",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("falha ao criar evidence_source 2 para a mesma source: %v", err)
	}
	if es2.ID != "es-2" {
		t.Errorf("esperava es-2, obteve %s", es2.ID)
	}

	// 9. Consulta de suportes ativos retorna ambos
	supports, err := queries.ListActiveSupportsByClaimID(ctx, "claim-1")
	if err != nil {
		t.Fatalf("falha ao listar suportes ativos: %v", err)
	}
	if len(supports) != 2 {
		t.Fatalf("esperava 2 suportes ativos, obteve %d", len(supports))
	}

	// 10. Atualiza um evidence_source para status = 'rejected'
	_, err = db.ExecContext(ctx, "UPDATE evidence_sources SET status = 'rejected' WHERE id = 'es-2'")
	if err != nil {
		t.Fatalf("falha ao atualizar status de es-2: %v", err)
	}

	// Consulta de suportes ativos deve ignorar o rejeitado
	supportsAfterRejection, err := queries.ListActiveSupportsByClaimID(ctx, "claim-1")
	if err != nil {
		t.Fatalf("falha ao listar suportes ativos após rejeição: %v", err)
	}
	if len(supportsAfterRejection) != 1 {
		t.Fatalf("esperava 1 suporte ativo após rejeição, obteve %d", len(supportsAfterRejection))
	}
	if supportsAfterRejection[0].EvidenceSourceID != es1.ID {
		t.Errorf("esperava suporte ativo es-1, obteve %s", supportsAfterRejection[0].EvidenceSourceID)
	}
}

func TestTransactionRollback(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Executa uma transação que simula erro no final
	err := store.ExecTx(ctx, db, func(q *sqlc.Queries) error {
		_, err := q.CreateEntity(ctx, sqlc.CreateEntityParams{
			ID:                 "ent-tx-test",
			Type:               "person",
			Name:               "Pessoa Transacional",
			NormalizedName:     "pessoa transacional",
			Slug:               "pessoa-transacional",
			RoleOrContext:      "Cargo",
			Summary:            "Resumo",
			Relevance:          3,
			RelevanceRationale: "Justificativa",
			CreatedAt:          now,
			UpdatedAt:          now,
		})
		if err != nil {
			return err
		}

		// Simula erro proposital
		return sql.ErrTxDone
	})

	if err == nil {
		t.Fatal("esperava erro ao forçar falha na transação, obteve nil")
	}

	// Valida que a entidade NÃO foi persistida
	queries := sqlc.New(db)
	_, err = queries.GetEntityByID(ctx, "ent-tx-test")
	if err == nil {
		t.Fatal("entidade inserida em transação com falha não deveria existir no banco")
	}
}

func TestMigration00003_Hardening(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Testa idempotência de import_runs: índice único parcial uq_import_runs_applied
	hash := "sha256-test-hash-123"
	version := "import-mapping-v1"

	// Primeiro run aplicado (is_dry_run = 0, status = 'running')
	run1, err := queries.CreateImportRun(ctx, sqlc.CreateImportRunParams{
		ID:             "run-1",
		FilePath:       "/imports/test.xlsx",
		FileHash:       hash,
		Origin:         "curated_seed",
		MappingVersion: version,
		Status:         "running",
		IsDryRun:       0,
		SummaryCounts:  "{}",
		SummaryReport:  "",
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("falha ao criar primeiro import run: %v", err)
	}

	// Segundo run aplicado com mesmo hash e versão DEVE falhar pelo índice único parcial
	_, err = queries.CreateImportRun(ctx, sqlc.CreateImportRunParams{
		ID:             "run-2-dup",
		FilePath:       "/imports/test.xlsx",
		FileHash:       hash,
		Origin:         "curated_seed",
		MappingVersion: version,
		Status:         "running",
		IsDryRun:       0,
		SummaryCounts:  "{}",
		SummaryReport:  "",
		CreatedAt:      now,
	})
	if err == nil {
		t.Fatal("esperava erro de constraint de unicidade ao inserir import_run duplicado em andamento, obteve sucesso")
	}

	// Inserção com is_dry_run = 1 DEVE ser permitida (não é barrada pelo índice parcial)
	_, err = queries.CreateImportRun(ctx, sqlc.CreateImportRunParams{
		ID:             "run-dry",
		FilePath:       "/imports/test.xlsx",
		FileHash:       hash,
		Origin:         "curated_seed",
		MappingVersion: version,
		Status:         "dry_run",
		IsDryRun:       1,
		SummaryCounts:  "{}",
		SummaryReport:  "",
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("dry-run não deveria ser barrado pelo índice único: %v", err)
	}

	// Inserção com status = 'failed' também não deve ser barrada pelo índice parcial
	_, err = queries.CreateImportRun(ctx, sqlc.CreateImportRunParams{
		ID:             "run-failed",
		FilePath:       "/imports/test.xlsx",
		FileHash:       "other-hash",
		Origin:         "curated_seed",
		MappingVersion: version,
		Status:         "failed",
		IsDryRun:       0,
		SummaryCounts:  "{}",
		SummaryReport:  "",
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("failed run não deveria ser barrado: %v", err)
	}

	// Consulta de run aplicado determinística
	appliedRun, err := queries.GetAppliedImportRun(ctx, sqlc.GetAppliedImportRunParams{
		FileHash:       hash,
		MappingVersion: version,
	})
	if err != nil {
		t.Fatalf("falha ao buscar run aplicado: %v", err)
	}
	if appliedRun.ID != run1.ID {
		t.Fatalf("esperava run %s, obteve %s", run1.ID, appliedRun.ID)
	}

	// Atualização do run
	updatedRun, err := queries.UpdateImportRun(ctx, sqlc.UpdateImportRunParams{
		ID:            run1.ID,
		Status:        "completed",
		SummaryCounts: `{"total": 10}`,
		SummaryReport: "ok",
		CompletedAt:   sql.NullString{String: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("falha ao atualizar import run: %v", err)
	}
	if updatedRun.Status != "completed" {
		t.Fatalf("esperava status completed, obteve %s", updatedRun.Status)
	}

	// 2. Testa criação de source com título e autor vazios (curated_seed sem inventar metadados)
	src, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-seed-1",
		Title:              "",
		PublisherOrAuthor:  "",
		OriginalUrl:        "https://example.com/article?id=1",
		CanonicalUrl:       "https://example.com/article?id=1",
		SourceType:         "article",
		SourceAccessStatus: "not_checked",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar source com título/autor vazios: %v", err)
	}
	if src.Title != "" || src.PublisherOrAuthor != "" {
		t.Errorf("título e autor deveriam estar vazios, obteve %q e %q", src.Title, src.PublisherOrAuthor)
	}

	// Unicidade de canonical_url em sources
	_, err = queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-seed-dup",
		Title:              "",
		PublisherOrAuthor:  "",
		OriginalUrl:        "https://example.com/article?id=1",
		CanonicalUrl:       "https://example.com/article?id=1",
		SourceType:         "article",
		SourceAccessStatus: "not_checked",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err == nil {
		t.Fatal("esperava erro de unicidade para canonical_url duplicada em sources")
	}

	// 3. Testa evidence_sources com excerpt vazio (síntese editorial não é trecho literal)
	c, _ := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID: "case-seed-test", Name: "Caso Teste", Slug: "caso-teste", Description: "Desc", CreatedAt: now, UpdatedAt: now,
	})
	ent, _ := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID: "ent-seed-test", Type: "person", Name: "Nome", NormalizedName: "nome", Slug: "nome",
		RoleOrContext: "Cargo", Summary: "Resumo", Relevance: 3, RelevanceRationale: "Justificativa",
		CreatedAt: now, UpdatedAt: now,
	})
	rel, _ := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID: "rel-seed-test", SubjectEntityID: ent.ID, CaseID: sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "vínculo", Summary: "Resumo", ContextLimits: "Limites",
		CreatedAt: now, UpdatedAt: now,
	})
	cl, _ := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID: "cl-seed-test", RelationshipID: rel.ID, Proposition: "Proposição", Origin: "curated_seed",
		Grade: "A", Disposition: "supports_link", MetricEligible: 1, Status: "published",
		CreatedAt: now, UpdatedAt: now,
	})
	ev, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID: "ev-seed-test", ClaimID: cl.ID, Summary: "Resumo da evidência", EvidenceType: "document",
		CreatedAt: now, UpdatedAt: now,
	})
	es, err := queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-seed-test",
		EvidenceID: ev.ID,
		SourceID:   src.ID,
		Excerpt:    "", // vazio: sem citação literal forçada
		Locator:    "",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("falha ao criar evidence_source com excerpt vazio: %v", err)
	}
	if es.Excerpt != "" {
		t.Errorf("esperava excerpt vazio, obteve %q", es.Excerpt)
	}
}

func TestMigration00004_SeedFidelity(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mig_00004.db")
	ctx := context.Background()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	// 1. Aplica todas as migrations
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// 2. Reverte 00007, 00006, 00005 e 00004 para simular estado do VZ-005 antes da 00004
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00007: %v", err)
	}
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00006: %v", err)
	}
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00005: %v", err)
	}
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00004: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Insere registros no formato do VZ-005 (onde summary continha alcance e attribution continha situação)
	_, err = db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-legacy', 'person', 'Nome Legado', 'nome legado', 'nome-legado', 'Cargo', 'Nacional', 3, 'Justificativa', ?, ?);
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir entidade legada: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description, created_at, updated_at)
		VALUES ('case-legacy', 'Caso', 'caso-leg', 'Desc', ?, ?);
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir caso: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits, created_at, updated_at)
		VALUES ('rel-legacy', 'ent-legacy', 'case-legacy', 'tipo', 'sum', 'lim', ?, ?);
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir relacionamento: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES ('cl-legacy', 'rel-legacy', 'Proposição', 'Investigado', 'curated_seed', 'A', 'supports_link', 1, 'published', ?, ?);
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim legado: %v", err)
	}

	// 3. Aplica as migrations sobre o banco com dados preexistentes
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations sobre dados legados: %v", err)
	}

	// 4. Valida que reach recebeu 'Nacional' e summary foi limpo
	var reach, summary string
	err = db.QueryRowContext(ctx, "SELECT reach, summary FROM entities WHERE id = 'ent-legacy'").Scan(&reach, &summary)
	if err != nil {
		t.Fatalf("falha ao consultar entidade pós-migração: %v", err)
	}
	if reach != "Nacional" {
		t.Errorf("esperava reach='Nacional', obteve %q", reach)
	}
	if summary != "" {
		t.Errorf("esperava summary='', obteve %q", summary)
	}

	// 5. Valida que context_status recebeu 'Investigado' e attribution foi limpa
	var contextStatus, attribution string
	err = db.QueryRowContext(ctx, "SELECT context_status, attribution FROM claims WHERE id = 'cl-legacy'").Scan(&contextStatus, &attribution)
	if err != nil {
		t.Fatalf("falha ao consultar claim pós-migração: %v", err)
	}
	if contextStatus != "Investigado" {
		t.Errorf("esperava context_status='Investigado', obteve %q", contextStatus)
	}
	if attribution != "" {
		t.Errorf("esperava attribution='', obteve %q", attribution)
	}
}

func TestMigration00003_DownFailsOnIncompatibleData(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mig_00003_down.db")
	ctx := context.Background()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// Reverte 00007, 00006, 00005 e 00004 para ficar exatamente na 00003
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00007: %v", err)
	}
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00006: %v", err)
	}
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00005: %v", err)
	}
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter 00004: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Insere uma source com título vazio (válida em 00003, incompatível com 00002)
	_, err = db.ExecContext(ctx, `
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES ('src-empty-title', '', '', 'https://example.com/item', 'https://example.com/item', 'article', 'not_checked', ?, ?);
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir source compatível com 00003: %v", err)
	}

	// O rollback da 00003 DEVE falhar explicitamente e não apagar registros silenciosamente
	err = store.Rollback(ctx, db)
	if err == nil {
		t.Fatal("esperava erro ao tentar reverter migration 00003 contendo dados incompatíveis, mas rollback teve sucesso")
	}

	// Confirma que a tabela sources e o registro incompatível continuam intactos
	var count int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE id = 'src-empty-title'").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("dados não devem ser perdidos na tentativa de rollback falha: count=%d, err=%v", count, err)
	}
}

func TestMigration00007_RollbackAndReapply(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mig_00007_rollback.db")
	ctx := context.Background()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	// Insere registro de candidate com campos v2
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (id, status, query, discovery_provider, discovery_model, created_at)
		VALUES ('run-mig-7', 'completed', 'query', 'openrouter', 'openai/gpt-4.1-mini', ?);
	`, now)
	if err != nil {
		t.Fatalf("falha ao criar run: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
			target_entity_name, normalized_target_entity_name, relationship_type,
			proposition, suggested_grade, source_url, canonical_url, technical_confidence,
			created_at, updated_at
		) VALUES (
			'cand-mig-7', 'run-mig-7', 'v1:abc', 1, 'Daniel Vorcaro', 'daniel vorcaro',
			'Banco Master', 'banco master', 'societario',
			'Controle societário', 'A', 'https://example.com/item', 'https://example.com/item', 0.95,
			?, ?
		);
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir candidato v2: %v", err)
	}

	// Cria avaliação semântica
	_, err = db.ExecContext(ctx, `
		INSERT INTO semantic_evaluations (
			id, monitoring_candidate_id, provider, model, schema_version,
			identity_match, claim_supported, claim_overstates_source, attribution_explicit,
			grade_compatible, contains_illicit_inference, uncertainties, recommended_action,
			created_at
		) VALUES (
			'eval-mig-7', 'cand-mig-7', 'openrouter', 'openai/gpt-4.1-mini', 'v1',
			1, 1, 0, 1, 1, 0, '[]', 'publish', ?
		);
	`, now)
	if err != nil {
		t.Fatalf("falha ao inserir avaliação semântica: %v", err)
	}

	// Reverte migration 00007 (Rollback)
	if err := store.Rollback(ctx, db); err != nil {
		t.Fatalf("falha ao reverter migration 00007: %v", err)
	}

	// Tabela semantic_evaluations deve ter sido removida
	var evalCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM semantic_evaluations").Scan(&evalCount); err == nil {
		t.Fatal("esperava erro ao consultar semantic_evaluations após rollback da 00007, mas tabela ainda existe")
	}

	// Tabela monitoring_candidates deve ter voltado ao formato v1 preservando dados essenciais
	var candEntity, candProp string
	err = db.QueryRowContext(ctx, "SELECT entity_name, proposition FROM monitoring_candidates WHERE id = 'cand-mig-7'").Scan(&candEntity, &candProp)
	if err != nil {
		t.Fatalf("falha ao consultar candidato após rollback 00007: %v", err)
	}
	if candEntity != "Daniel Vorcaro" || candProp != "Controle societário" {
		t.Errorf("dados essenciais do candidato corrompidos após rollback 00007: entity=%q, prop=%q", candEntity, candProp)
	}

	// Re-aplica as migrations
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao re-aplicar migrations após rollback 00007: %v", err)
	}

	// semantic_evaluations deve existir novamente
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM semantic_evaluations").Scan(&evalCount); err != nil {
		t.Fatalf("falha ao consultar semantic_evaluations após re-migrate: %v", err)
	}
}
