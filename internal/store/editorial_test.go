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
