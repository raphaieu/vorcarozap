package contradiction_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/contradiction"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func setupTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "contradiction_test.db")

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

func seedTestData(t *testing.T, db *sql.DB, ctx context.Context) string {
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

	// Claim publicado com suporte ativo
	claimID := "claim-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES (?, 'rel-1', 'Alegação investigada', 'Ministério Público', 'curated_seed', 'A', 'supports_link', 1, 'published', ?, ?)
	`, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim: %v", err)
	}

	// Source
	_, err = db.ExecContext(ctx, `
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES ('src-1', 'Fonte Noticiosa', 'Veículo X', 'https://exemplo.com/noticia', 'https://exemplo.com/noticia', 'article', 'reachable', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir source: %v", err)
	}

	// Evidence
	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence (id, claim_id, summary, created_at, updated_at)
		VALUES ('evi-1', ?, 'Resumo da evidência', ?, ?)
	`, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidência: %v", err)
	}

	// Evidence Source
	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, role, status, created_at, updated_at)
		VALUES ('es-1', 'evi-1', 'src-1', 'Trecho literal comprobatório', 'supports', 'active', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidence_source: %v", err)
	}

	return claimID
}

func TestSubmitStatement_SuccessInQuarantine(t *testing.T) {
	db, ctx := setupTestDB(t)
	claimID := seedTestData(t, db, ctx)
	svc := contradiction.NewService(db)

	res, err := svc.SubmitStatement(ctx, contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: domain.StatementTypeRebuttal,
		Title:         "Nota de Esclarecimento da Defesa",
		Content:       "O investigado esclarece categoricamente que não possui participação no fato.",
		SourceURL:     "https://defesa.com/nota-oficial.pdf",
		ContactInfo:   "assessoria@defesa.com",
	})
	if err != nil {
		t.Fatalf("SubmitStatement falhou: %v", err)
	}

	if res.ID == "" {
		t.Errorf("ID gerado não pode ser vazio")
	}
	if res.Status != domain.StatementStatusQuarantined {
		t.Errorf("status inicial deve ser quarantined, obtido: %q", res.Status)
	}

	// Verificar diretamente no banco
	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementByID(ctx, res.ID)
	if err != nil {
		t.Fatalf("falha ao consultar manifestação persistida: %v", err)
	}

	if stmt.Status != "quarantined" {
		t.Errorf("status no banco deve ser quarantined, obtido: %q", stmt.Status)
	}
	if stmt.ContactInfo != "assessoria@defesa.com" {
		t.Errorf("contact_info no banco incorreto: %q", stmt.ContactInfo)
	}

	// Verificar isolamento: não deve constar na listagem pública de aceitas
	pubStmts, err := q.ListPublicDefenseStatementsByClaimID(ctx, claimID)
	if err != nil {
		t.Fatalf("falha ao listar públicas: %v", err)
	}
	if len(pubStmts) != 0 {
		t.Errorf("manifestação em quarentena NÃO deve aparecer publicamente, obtido %d itens", len(pubStmts))
	}
}

func TestSubmitStatement_NonExistentClaim(t *testing.T) {
	db, ctx := setupTestDB(t)
	svc := contradiction.NewService(db)

	_, err := svc.SubmitStatement(ctx, contradiction.SubmitStatementParams{
		ClaimID:       "claim-inexistente",
		StatementType: domain.StatementTypeCorrection,
		Title:         "Título Válido",
		Content:       "Conteúdo suficiente para validação de tamanho.",
	})
	if !errors.Is(err, contradiction.ErrClaimNotFound) {
		t.Fatalf("esperado ErrClaimNotFound, obtido: %v", err)
	}
}

func TestSubmitStatement_PrivateOrQuarantinedOrSupportlessClaim(t *testing.T) {
	db, ctx := setupTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	// Inserir entidade, caso e relação
	_, _ = db.ExecContext(ctx, `INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at) VALUES ('ent-x', 'person', 'Investigado X', 'investigado x', 'investigado-x', 'Papel', 'Resumo', 3, 'Razao', ?, ?)`, now, now)
	_, _ = db.ExecContext(ctx, `INSERT INTO cases (id, name, slug, description, created_at, updated_at) VALUES ('case-x', 'Caso X', 'caso-x', 'Desc', ?, ?)`, now, now)
	_, _ = db.ExecContext(ctx, `INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, created_at, updated_at) VALUES ('rel-x', 'ent-x', 'case-x', 'Investigado', 'Resumo', ?, ?)`, now, now)

	// 1. Claim em quarentena
	quarantinedClaimID := "claim-quarantined"
	_, _ = db.ExecContext(ctx, `INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at) VALUES (?, 'rel-x', 'Alegação privada em quarentena', 'MPF', 'curated_seed', 'A', 'supports_link', 0, 'quarantined', ?, ?)`, quarantinedClaimID, now, now)

	// 2. Claim rejeitado
	rejectedClaimID := "claim-rejected"
	_, _ = db.ExecContext(ctx, `INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at) VALUES (?, 'rel-x', 'Alegação rejeitada', 'MPF', 'curated_seed', 'A', 'supports_link', 0, 'rejected', ?, ?)`, rejectedClaimID, now, now)

	// 3. Claim published mas SEM nenhum evidence_source ativo com role='supports'
	noSupportClaimID := "claim-no-support"
	_, _ = db.ExecContext(ctx, `INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at) VALUES (?, 'rel-x', 'Alegação publicada sem suporte', 'MPF', 'curated_seed', 'A', 'supports_link', 1, 'published', ?, ?)`, noSupportClaimID, now, now)

	svc := contradiction.NewService(db)

	for _, cid := range []string{quarantinedClaimID, rejectedClaimID, noSupportClaimID} {
		t.Run("claim_"+cid, func(t *testing.T) {
			_, err := svc.SubmitStatement(ctx, contradiction.SubmitStatementParams{
				ClaimID:       cid,
				StatementType: domain.StatementTypeCorrection,
				Title:         "Título Válido",
				Content:       "Conteúdo suficiente para validação de tamanho.",
			})
			if !errors.Is(err, contradiction.ErrClaimNotFound) {
				t.Fatalf("submissão para claim não público %q deve retornar ErrClaimNotFound, obtido: %v", cid, err)
			}
		})
	}
}

func TestModerateStatement_TransitionsAndAudit(t *testing.T) {
	db, ctx := setupTestDB(t)
	claimID := seedTestData(t, db, ctx)
	svc := contradiction.NewService(db)

	// 1. Submeter manifestação inicial
	subRes, err := svc.SubmitStatement(ctx, contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: domain.StatementTypeRebuttal,
		Title:         "Contestação Formal",
		Content:       "Texto detalhado com argumentos de defesa devidamente fundamentados.",
		ContactInfo:   "advogado@exemplo.com",
	})
	if err != nil {
		t.Fatalf("submissão falhou: %v", err)
	}

	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementForModeration(ctx, subRes.ID)
	if err != nil {
		t.Fatalf("falha ao consultar statement: %v", err)
	}

	// 2. Colocar em revisão (quarantined -> under_review)
	modRes1, err := svc.ModerateStatement(ctx, contradiction.ModerateStatementParams{
		StatementID:       subRes.ID,
		ExpectedUpdatedAt: stmt.UpdatedAt,
		Action:            domain.StatementActionReview,
		Reason:            "Documento recebido e em análise pela equipe jurídica.",
		Actor:             "operador_admin",
	})
	if err != nil {
		t.Fatalf("moderação review falhou: %v", err)
	}
	if modRes1.NewStatus != domain.StatementStatusUnderReview {
		t.Errorf("status esperado under_review, obtido %q", modRes1.NewStatus)
	}

	// 3. Aceitar manifestação (under_review -> accepted)
	modRes2, err := svc.ModerateStatement(ctx, contradiction.ModerateStatementParams{
		StatementID:       subRes.ID,
		ExpectedUpdatedAt: modRes1.UpdatedAt,
		Action:            domain.StatementActionAccept,
		Reason:            "Manifestação acompanhada de certidão comprobatória aceita.",
		Actor:             "operador_admin",
	})
	if err != nil {
		t.Fatalf("moderação accept falhou: %v", err)
	}
	if modRes2.NewStatus != domain.StatementStatusAccepted {
		t.Errorf("status esperado accepted, obtido %q", modRes2.NewStatus)
	}

	// 4. Verificar que agora aparece publicamente
	pubStmts, err := q.ListPublicDefenseStatementsByClaimID(ctx, claimID)
	if err != nil {
		t.Fatalf("falha ao listar públicas: %v", err)
	}
	if len(pubStmts) != 1 {
		t.Fatalf("esperado 1 manifestação pública aceita, obtido %d", len(pubStmts))
	}
	if pubStmts[0].Title != "Contestação Formal" {
		t.Errorf("título público incorreto: %q", pubStmts[0].Title)
	}

	// 5. Verificar decisões imutáveis de auditoria
	decisions, err := q.ListDefenseStatementDecisionsByStatementID(ctx, subRes.ID)
	if err != nil {
		t.Fatalf("falha ao listar decisões: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("esperado 2 decisões registradas, obtido %d", len(decisions))
	}
	if decisions[0].Action != "accept" || decisions[1].Action != "review" {
		t.Errorf("ordenação ou ações incorretas: %v, %v", decisions[0].Action, decisions[1].Action)
	}
}

func TestModerateStatement_OCCConflict(t *testing.T) {
	db, ctx := setupTestDB(t)
	claimID := seedTestData(t, db, ctx)
	svc := contradiction.NewService(db)

	subRes, err := svc.SubmitStatement(ctx, contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: domain.StatementTypeCorrection,
		Title:         "Retificação de Dados",
		Content:       "Texto detalhado com esclarecimentos sobre os autos do processo.",
	})
	if err != nil {
		t.Fatalf("submissão falhou: %v", err)
	}

	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementForModeration(ctx, subRes.ID)
	if err != nil {
		t.Fatalf("falha ao consultar statement: %v", err)
	}

	// Tentativa concorrente 1
	var wg sync.WaitGroup
	var err1, err2 error
	var res1, res2 *contradiction.ModerateStatementResult

	wg.Add(2)
	go func() {
		defer wg.Done()
		res1, err1 = svc.ModerateStatement(ctx, contradiction.ModerateStatementParams{
			StatementID:       subRes.ID,
			ExpectedUpdatedAt: stmt.UpdatedAt,
			Action:            domain.StatementActionAccept,
			Reason:            "Aprovado por moderador A",
			Actor:             "moderador_a",
		})
	}()

	go func() {
		defer wg.Done()
		res2, err2 = svc.ModerateStatement(ctx, contradiction.ModerateStatementParams{
			StatementID:       subRes.ID,
			ExpectedUpdatedAt: stmt.UpdatedAt,
			Action:            domain.StatementActionReject,
			Reason:            "Rejeitado por moderador B",
			Actor:             "moderador_b",
		})
	}()

	wg.Wait()

	// Exatamente uma das duas deve ter sucesso e a outra deve falhar com ErrConflict
	successes := 0
	conflicts := 0

	if err1 == nil && res1 != nil {
		successes++
	} else if errors.Is(err1, contradiction.ErrConflict) {
		conflicts++
	}

	if err2 == nil && res2 != nil {
		successes++
	} else if errors.Is(err2, contradiction.ErrConflict) {
		conflicts++
	}

	if successes != 1 || conflicts != 1 {
		t.Fatalf("esperado exatamente 1 sucesso e 1 conflito de concorrência, obtido %d sucessos e %d conflitos (err1: %v, err2: %v)", successes, conflicts, err1, err2)
	}
}

func TestModerateStatement_MultiGoroutineOCCRace(t *testing.T) {
	db, ctx := setupTestDB(t)
	claimID := seedTestData(t, db, ctx)
	svc := contradiction.NewService(db)

	subRes, err := svc.SubmitStatement(ctx, contradiction.SubmitStatementParams{
		ClaimID:       claimID,
		StatementType: domain.StatementTypeCorrection,
		Title:         "Retificação de Dados",
		Content:       "Texto detalhado com esclarecimentos para teste multi-goroutine OCC.",
	})
	if err != nil {
		t.Fatalf("submissão falhou: %v", err)
	}

	q := sqlc.New(db)
	stmt, err := q.GetDefenseStatementForModeration(ctx, subRes.ID)
	if err != nil {
		t.Fatalf("falha ao consultar statement: %v", err)
	}

	const concurrentWorkers = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	conflictCount := 0
	otherErrors := 0

	wg.Add(concurrentWorkers)
	for i := 0; i < concurrentWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()
			_, modErr := svc.ModerateStatement(ctx, contradiction.ModerateStatementParams{
				StatementID:       subRes.ID,
				ExpectedUpdatedAt: stmt.UpdatedAt,
				Action:            domain.StatementActionAccept,
				Reason:            "Aprovado por goroutine de teste",
				Actor:             "moderador_concorrente",
			})
			mu.Lock()
			defer mu.Unlock()
			if modErr == nil {
				successCount++
			} else if errors.Is(modErr, contradiction.ErrConflict) {
				conflictCount++
			} else {
				otherErrors++
			}
		}(i)
	}

	wg.Wait()

	if successCount != 1 {
		t.Fatalf("esperado exatamente 1 sucesso entre %d goroutines concorrentes, obtido %d (conflitos: %d, outros: %d)", concurrentWorkers, successCount, conflictCount, otherErrors)
	}
	if conflictCount != concurrentWorkers-1 {
		t.Fatalf("esperado exatamente %d conflitos, obtido %d (outros erros: %d)", concurrentWorkers-1, conflictCount, otherErrors)
	}
}
