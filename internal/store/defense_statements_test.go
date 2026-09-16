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

func setupStoreTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "store_statements_test.db")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao executar migrations: %v", err)
	}

	return db, context.Background()
}

func seedStoreTestData(t *testing.T, db *sql.DB, ctx context.Context) (string, string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// Entidade
	_, err := db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-1', 'person', 'Daniel Vorcaro', 'daniel vorcaro', 'daniel-vorcaro', 'Empresário', 'Resumo', 5, 'Relevância alta', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir entidade: %v", err)
	}

	// Caso
	_, err = db.ExecContext(ctx, `
		INSERT INTO cases (id, name, slug, description, created_at, updated_at)
		VALUES ('case-1', 'Caso Master', 'caso-master', 'Descrição do caso', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir caso: %v", err)
	}

	// Relação
	_, err = db.ExecContext(ctx, `
		INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, created_at, updated_at)
		VALUES ('rel-1', 'ent-1', 'case-1', 'Investigado', 'Resumo da relação', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir relação: %v", err)
	}

	// Claim 1 publicado
	claim1ID := "claim-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES (?, 'rel-1', 'Alegação investigada', 'Ministério Público', 'curated_seed', 'A', 'supports_link', 1, 'published', ?, ?)
	`, claim1ID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim 1: %v", err)
	}

	// Claim 2 quarentenado
	claim2ID := "claim-2"
	_, err = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES (?, 'rel-1', 'Alegação sob quarentena', 'Polícia Federal', 'openrouter', 'D', 'possible_link', 0, 'quarantined', ?, ?)
	`, claim2ID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim 2: %v", err)
	}

	// Source e Evidence para Claim 1 (para satisfazer public_claims_view)
	_, err = db.ExecContext(ctx, `
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES ('src-1', 'Fonte Teste', 'Autor X', 'https://exemplo.com/doc', 'https://exemplo.com/doc', 'article', 'reachable', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir source: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence (id, claim_id, summary, created_at, updated_at)
		VALUES ('evi-1', ?, 'Resumo da evidência', ?, ?)
	`, claim1ID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidência: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, role, status, created_at, updated_at)
		VALUES ('es-1', 'evi-1', 'src-1', 'Trecho literal comprobatório', 'supports', 'active', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidence_source: %v", err)
	}

	return claim1ID, claim2ID
}

func TestStore_DefenseStatements(t *testing.T) {
	db, ctx := setupStoreTestDB(t)
	claim1ID, claim2ID := seedStoreTestData(t, db, ctx)
	q := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Inserir 3 manifestações
	// Stmt 1: aceita para claim1
	stmt1ID := "stmt-1"
	err := q.InsertDefenseStatement(ctx, sqlc.InsertDefenseStatementParams{
		ID:            stmt1ID,
		ClaimID:       claim1ID,
		StatementType: "rebuttal",
		Title:         "Contestação ao Claim 1",
		Content:       "Texto detalhado com argumentos de defesa devidamente fundamentados.",
		SourceUrl:     "https://defesa.com/nota.pdf",
		ContactInfo:   "advogado@defesa.com",
		Status:        "accepted",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("falha ao inserir stmt 1: %v", err)
	}

	// Stmt 2: quarentenada para claim1
	stmt2ID := "stmt-2"
	err = q.InsertDefenseStatement(ctx, sqlc.InsertDefenseStatementParams{
		ID:            stmt2ID,
		ClaimID:       claim1ID,
		StatementType: "correction",
		Title:         "Retificação de Data",
		Content:       "Texto retificando a data do evento citado na investigação.",
		SourceUrl:     "",
		ContactInfo:   "contato@exemplo.com",
		Status:        "quarantined",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("falha ao inserir stmt 2: %v", err)
	}

	// Stmt 3: aceita para claim2 (que está em quarentena)
	stmt3ID := "stmt-3"
	err = q.InsertDefenseStatement(ctx, sqlc.InsertDefenseStatementParams{
		ID:            stmt3ID,
		ClaimID:       claim2ID,
		StatementType: "clarification",
		Title:         "Esclarecimento sobre Claim Quarentenado",
		Content:       "Texto explicando o contexto sobre alegação não pública.",
		SourceUrl:     "",
		ContactInfo:   "privado@exemplo.com",
		Status:        "accepted",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("falha ao inserir stmt 3: %v", err)
	}

	// Inserir decisão para stmt 1
	err = q.InsertDefenseStatementDecision(ctx, sqlc.InsertDefenseStatementDecisionParams{
		ID:          "dec-1",
		StatementID: stmt1ID,
		Action:      "accept",
		Reason:      "Manifestação formal comprovada por certidão.",
		Actor:       "admin_test",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao inserir decisão: %v", err)
	}

	// 1. Testar ListAdminDefenseStatements sem filtros
	adminRes, err := store.ListAdminDefenseStatements(ctx, db, store.AdminDefenseStatementFilter{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListAdminDefenseStatements falhou: %v", err)
	}
	if adminRes.TotalCount != 3 {
		t.Errorf("esperado 3 statements no admin, obtido: %d", adminRes.TotalCount)
	}

	// 2. Testar ListAdminDefenseStatements com filtro de status = quarantined
	adminFiltered, err := store.ListAdminDefenseStatements(ctx, db, store.AdminDefenseStatementFilter{
		Status: "quarantined",
	})
	if err != nil {
		t.Fatalf("ListAdminDefenseStatements com filtro falhou: %v", err)
	}
	if adminFiltered.TotalCount != 1 || adminFiltered.Statements[0].ID != stmt2ID {
		t.Errorf("filtro de status falhou, esperado stmt-2, obtido %v", adminFiltered.Statements)
	}

	// 3. Testar GetAdminDefenseStatementDetail
	detail, err := store.GetAdminDefenseStatementDetail(ctx, db, stmt1ID)
	if err != nil {
		t.Fatalf("GetAdminDefenseStatementDetail falhou: %v", err)
	}
	if detail.ID != stmt1ID || detail.ContactInfo != "advogado@defesa.com" || len(detail.Decisions) != 1 {
		t.Errorf("detalhes administrativos incorretos: %+v", detail)
	}

	// 4. Testar GetPublicClaimForManifestation para claim1 (público) e claim2 (quarentenado)
	pubClaimRow, err := q.GetPublicClaimForManifestation(ctx, claim1ID)
	if err != nil {
		t.Fatalf("GetPublicClaimForManifestation falhou para claim público: %v", err)
	}
	if pubClaimRow.ClaimID != claim1ID || pubClaimRow.EntityName != "Daniel Vorcaro" {
		t.Errorf("dados de GetPublicClaimForManifestation incorretos: %+v", pubClaimRow)
	}

	_, err = q.GetPublicClaimForManifestation(ctx, claim2ID)
	if err == nil {
		t.Fatalf("GetPublicClaimForManifestation DEVE falhar para claim em quarentena")
	}

	// 5. Testar ListPublicDefenseStatementsForClaim para claim1 (published + accepted)
	pubStmts1, err := store.ListPublicDefenseStatementsForClaim(ctx, db, claim1ID)
	if err != nil {
		t.Fatalf("ListPublicDefenseStatementsForClaim falhou: %v", err)
	}
	if len(pubStmts1) != 1 || pubStmts1[0].ID != stmt1ID {
		t.Errorf("esperado apenas stmt-1 para claim 1, obtido %v", pubStmts1)
	}

	// 6. Testar ListPublicDefenseStatementsForClaim para claim2 (quarantined claim) -> deve retornar vazio!
	pubStmts2, err := store.ListPublicDefenseStatementsForClaim(ctx, db, claim2ID)
	if err != nil {
		t.Fatalf("ListPublicDefenseStatementsForClaim falhou: %v", err)
	}
	if len(pubStmts2) != 0 {
		t.Errorf("claim não publicado NÃO deve expor manifestações publicamente, obtido: %v", pubStmts2)
	}

	// 7. Testar perda do último suporte de claim1: rejeitar o evidence_source
	_, err = db.ExecContext(ctx, "UPDATE evidence_sources SET status = 'rejected' WHERE id = 'es-1'")
	if err != nil {
		t.Fatalf("falha ao rejeitar evidence_source: %v", err)
	}

	// Agora, claim1 não está mais em public_claims_view:
	_, err = q.GetPublicClaimForManifestation(ctx, claim1ID)
	if err == nil {
		t.Fatalf("GetPublicClaimForManifestation DEVE falhar quando claim perde suporte ativo")
	}

	pubStmts1AfterLoss, err := store.ListPublicDefenseStatementsForClaim(ctx, db, claim1ID)
	if err != nil {
		t.Fatalf("ListPublicDefenseStatementsForClaim falhou: %v", err)
	}
	if len(pubStmts1AfterLoss) != 0 {
		t.Errorf("manifestações de claim sem suporte ativo NÃO devem aparecer publicamente, obtido: %v", pubStmts1AfterLoss)
	}
}
