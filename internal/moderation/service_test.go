package moderation_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/moderation"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func setupTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "moderation_test.db")

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

func seedTestData(t *testing.T, db *sql.DB, ctx context.Context) (string, string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	// Entidade Sujeito
	_, err := db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-subject', 'person', 'Daniel Vorcaro', 'daniel vorcaro', 'daniel-vorcaro', 'Empresário', 'Resumo', 5, 'Relevância alta', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir entidade sujeito: %v", err)
	}

	// Entidade Alvo
	_, err = db.ExecContext(ctx, `
		INSERT INTO entities (id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at)
		VALUES ('ent-target', 'organization', 'Banco Master', 'banco master', 'banco-master', 'Instituição Financeira', 'Resumo', 4, 'Relevância alta', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir entidade alvo: %v", err)
	}

	// Relacionamento
	_, err = db.ExecContext(ctx, `
		INSERT INTO relationships (id, subject_entity_id, target_entity_id, relationship_type, summary, context_limits, created_at, updated_at)
		VALUES ('rel-1', 'ent-subject', 'ent-target', 'societário', 'Controle societário', 'Limites factuais', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir relacionamento: %v", err)
	}

	// Fonte
	_, err = db.ExecContext(ctx, `
		INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_type, source_access_status, created_at, updated_at)
		VALUES ('src-1', 'Matéria de Teste', 'Folha', 'https://folha.uol.com.br/artigo-1', 'https://folha.uol.com.br/artigo-1', 'article', 'reachable', ?, ?)
	`, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir fonte: %v", err)
	}

	return "ent-subject", "rel-1"
}

func insertTestClaim(t *testing.T, db *sql.DB, ctx context.Context, claimID, relID, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := db.ExecContext(ctx, `
		INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, created_at, updated_at)
		VALUES (?, ?, 'Proposição factual do claim', 'Atribuição jornalística', 'openrouter', 'A', 'supports_link', 1, ?, ?, ?)
	`, claimID, relID, status, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir claim %s: %v", claimID, err)
	}
}

func insertTestEvidenceSource(t *testing.T, db *sql.DB, ctx context.Context, esID, claimID, srcID, role, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	evID := "ev-" + esID
	_, err := db.ExecContext(ctx, `
		INSERT INTO evidence (id, claim_id, summary, evidence_type, created_at, updated_at)
		VALUES (?, ?, 'Resumo da evidência', 'document', ?, ?)
	`, evID, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidência: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status, created_at, updated_at)
		VALUES (?, ?, ?, 'Trecho literal comprovatório da fonte', 'p. 1', ?, ?, ?, ?)
	`, esID, evID, srcID, role, status, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir evidence_source: %v", err)
	}
}

func insertTestMonitoringCandidate(t *testing.T, db *sql.DB, ctx context.Context, candID, claimID, fp, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES ('run-test', 'completed', 'teste query', 'openai/gpt-4o', ?)
		ON CONFLICT(id) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("falha ao inserir run: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			proposition, suggested_grade, source_url, canonical_url, technical_confidence,
			editorial_status, is_duplicate, duplicate_reason, published_claim_id, created_at, updated_at
		) VALUES (
			?, 'run-test', ?, 'Daniel Vorcaro', 'daniel vorcaro',
			'Proposição factual', 'A', 'https://folha.uol.com.br/artigo-1', 'https://folha.uol.com.br/artigo-1',
			0.95, ?, 0, '', ?, ?, ?
		)
	`, candID, fp, status, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir candidato: %v", err)
	}
}

func getTestClaimUpdatedAt(t *testing.T, db *sql.DB, ctx context.Context, claimID string) string {
	t.Helper()
	var updatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = ?", claimID).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at do claim %s: %v", claimID, err)
	}
	return updatedAt
}

func TestModerateClaim_ValidTransitions(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	t.Run("approve quarantined -> published", func(t *testing.T) {
		claimID := "claim-approve-1"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-app-1", claimID, "src-1", "supports", "active")
		insertTestMonitoringCandidate(t, db, ctx, "cand-app-1", claimID, "v1:fp-app-1", "quarantined")

		res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "Evidência documental de cartório comprovada com suporte ativo.",
			Actor:             "admin_operator",
		})
		if err != nil {
			t.Fatalf("esperava sucesso ao aprovar, erro: %v", err)
		}

		if res.NewStatus != domain.ClaimStatusPublished {
			t.Errorf("NewStatus = %q, esperado %q", res.NewStatus, domain.ClaimStatusPublished)
		}
		if res.PreviousStatus != domain.ClaimStatusQuarantined {
			t.Errorf("PreviousStatus = %q, esperado %q", res.PreviousStatus, domain.ClaimStatusQuarantined)
		}

		// Verificar claim no banco
		var status string
		err = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&status)
		if err != nil || status != "published" {
			t.Errorf("status no banco = %q (err=%v), esperado 'published'", status, err)
		}

		// Verificar decisão no banco
		var decAction, decActor, decReason, decFP string
		var claimFK sql.NullString
		var esFK sql.NullString
		err = db.QueryRowContext(ctx, `
			SELECT action, actor, reason, candidate_fingerprint, claim_id, evidence_source_id
			FROM moderation_decisions WHERE id = ?
		`, res.DecisionID).Scan(&decAction, &decActor, &decReason, &decFP, &claimFK, &esFK)
		if err != nil {
			t.Fatalf("falha ao consultar decisão de moderação: %v", err)
		}

		if decAction != "approve" || decActor != "admin_operator" || decFP != "v1:fp-app-1" {
			t.Errorf("decisão gravada incorretamente: action=%q, actor=%q, fp=%q", decAction, decActor, decFP)
		}
		if !claimFK.Valid || claimFK.String != claimID {
			t.Errorf("claim_id na decisão = %v, esperado %q", claimFK, claimID)
		}
		if esFK.Valid {
			t.Errorf("evidence_source_id na decisão deve ser nulo (XOR), obtido %v", esFK)
		}
	})

	t.Run("reject published -> rejected", func(t *testing.T) {
		claimID := "claim-reject-pub"
		insertTestClaim(t, db, ctx, claimID, relID, "published")
		insertTestEvidenceSource(t, db, ctx, "es-rej-1", claimID, "src-1", "supports", "active")
		insertTestMonitoringCandidate(t, db, ctx, "cand-rej-1", claimID, "v1:fp-rej-1", "published")

		res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionReject,
			Reason:            "Fonte retratou a matéria original.",
			Actor:             "admin_operator",
		})
		if err != nil {
			t.Fatalf("esperava sucesso ao rejeitar claim publicado, erro: %v", err)
		}

		if res.NewStatus != domain.ClaimStatusRejected {
			t.Errorf("NewStatus = %q, esperado %q", res.NewStatus, domain.ClaimStatusRejected)
		}

		// Verificar que monitoring_candidate associado foi marcado como rejected
		var candStatus string
		err = db.QueryRowContext(ctx, "SELECT editorial_status FROM monitoring_candidates WHERE id = 'cand-rej-1'").Scan(&candStatus)
		if err != nil || candStatus != "rejected" {
			t.Errorf("candidato status = %q (err=%v), esperado 'rejected'", candStatus, err)
		}

		// Fonte permanece intacta
		var srcStatus string
		err = db.QueryRowContext(ctx, "SELECT source_access_status FROM sources WHERE id = 'src-1'").Scan(&srcStatus)
		if err != nil || srcStatus != "reachable" {
			t.Errorf("fonte deve permanecer intacta: status=%q", srcStatus)
		}
	})

	t.Run("reject quarantined -> rejected", func(t *testing.T) {
		claimID := "claim-reject-quar"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-rej-2", claimID, "src-1", "supports", "active")
		insertTestMonitoringCandidate(t, db, ctx, "cand-rej-2", claimID, "v1:fp-rej-2", "quarantined")

		res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionReject,
			Reason:            "Alegação inconsistente com os autos.",
			Actor:             "admin_operator",
		})
		if err != nil {
			t.Fatalf("esperava sucesso ao rejeitar claim em quarentena, erro: %v", err)
		}

		if res.NewStatus != domain.ClaimStatusRejected {
			t.Errorf("NewStatus = %q, esperado %q", res.NewStatus, domain.ClaimStatusRejected)
		}
	})

	t.Run("restore rejected -> quarantined (never directly published)", func(t *testing.T) {
		claimID := "claim-restore-rej"
		insertTestClaim(t, db, ctx, claimID, relID, "rejected")
		insertTestEvidenceSource(t, db, ctx, "es-rest-1", claimID, "src-1", "supports", "active")

		res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionRestore,
			Reason:            "Revisão editorial solicitada para reavaliação de novos documentos.",
			Actor:             "admin_operator",
		})
		if err != nil {
			t.Fatalf("esperava sucesso ao restaurar claim rejeitado, erro: %v", err)
		}

		if res.NewStatus != domain.ClaimStatusQuarantined {
			t.Errorf("NewStatus = %q, esperado 'quarantined' (restore NUNCA publica diretamente)", res.NewStatus)
		}
	})

	t.Run("restore archived -> quarantined", func(t *testing.T) {
		claimID := "claim-restore-arch"
		insertTestClaim(t, db, ctx, claimID, relID, "archived")
		insertTestEvidenceSource(t, db, ctx, "es-rest-2", claimID, "src-1", "supports", "active")

		res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionRestore,
			Reason:            "Desarquivamento para reexame em quarentena.",
			Actor:             "admin_operator",
		})
		if err != nil {
			t.Fatalf("esperava sucesso ao restaurar claim arquivado, erro: %v", err)
		}

		if res.NewStatus != domain.ClaimStatusQuarantined {
			t.Errorf("NewStatus = %q, esperado 'quarantined'", res.NewStatus)
		}
	})
}

func TestModerateClaim_ApproveRequiresActiveSupports(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	t.Run("approve without evidence_sources fails", func(t *testing.T) {
		claimID := "claim-no-es"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")

		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "Tentativa de aprovar claim sem evidência.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrNoActiveSupport) {
			t.Fatalf("esperava ErrNoActiveSupport, obtido: %v", err)
		}

		var status string
		_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&status)
		if status != "quarantined" {
			t.Errorf("claim status = %q, esperado 'quarantined' (sem alteração)", status)
		}
	})

	t.Run("approve with only contradicts role fails", func(t *testing.T) {
		claimID := "claim-only-contradicts"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-contra-1", claimID, "src-1", "contradicts", "active")

		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "Tentativa de aprovar claim apenas com contraditório.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrNoActiveSupport) {
			t.Fatalf("esperava ErrNoActiveSupport, obtido: %v", err)
		}
	})

	t.Run("approve with only rejected supports fails", func(t *testing.T) {
		claimID := "claim-rejected-support"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-rej-supp-1", claimID, "src-1", "supports", "rejected")

		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "Tentativa de aprovar claim com suporte inativo/rejeitado.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrNoActiveSupport) {
			t.Fatalf("esperava ErrNoActiveSupport, obtido: %v", err)
		}
	})
}

func TestModerateClaim_InvalidTransitions(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	invalidCases := []struct {
		name       string
		initStatus string
		action     domain.ModerationAction
	}{
		{"approve on published", "published", domain.ModerationActionApprove},
		{"approve on rejected", "rejected", domain.ModerationActionApprove},
		{"approve on archived", "archived", domain.ModerationActionApprove},
		{"restore on quarantined", "quarantined", domain.ModerationActionRestore},
		{"restore on published", "published", domain.ModerationActionRestore},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			claimID := "claim-" + tc.name
			insertTestClaim(t, db, ctx, claimID, relID, tc.initStatus)
			insertTestEvidenceSource(t, db, ctx, "es-"+tc.name, claimID, "src-1", "supports", "active")

			_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
				ClaimID:           claimID,
				ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
				Action:            tc.action,
				Reason:            "Transição inválida de teste.",
				Actor:             "admin_operator",
			})
			if !errors.Is(err, moderation.ErrInvalidTransition) {
				t.Fatalf("esperava ErrInvalidTransition, obtido: %v", err)
			}

			var status string
			_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&status)
			if status != tc.initStatus {
				t.Errorf("status = %q, esperado %q (sem alteração)", status, tc.initStatus)
			}
		})
	}
}

func TestModerateClaim_ValidationErrors(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-val-test"
	insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
	insertTestEvidenceSource(t, db, ctx, "es-val-1", claimID, "src-1", "supports", "active")

	t.Run("nonexistent claim returns ErrClaimNotFound", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           "claim-inexistente-12345",
			ExpectedUpdatedAt: "2026-09-10T10:00:00Z",
			Action:            domain.ModerationActionApprove,
			Reason:            "Motivo válido.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrClaimNotFound) {
			t.Fatalf("esperava ErrClaimNotFound, obtido: %v", err)
		}
	})

	t.Run("empty expected_updated_at returns ErrMissingExpectedVersion", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: "",
			Action:            domain.ModerationActionApprove,
			Reason:            "Motivo válido.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrMissingExpectedVersion) {
			t.Fatalf("esperava ErrMissingExpectedVersion, obtido: %v", err)
		}

		// Banco de dados deve permanecer intacto
		var status string
		_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&status)
		if status != "quarantined" {
			t.Errorf("status alterado indevidamente: %q", status)
		}
		var decCount int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = ?", claimID).Scan(&decCount)
		if decCount != 0 {
			t.Errorf("decisão persistida indevidamente: %d", decCount)
		}
	})

	t.Run("whitespace expected_updated_at returns ErrMissingExpectedVersion", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: "   \t\n",
			Action:            domain.ModerationActionApprove,
			Reason:            "Motivo válido.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrMissingExpectedVersion) {
			t.Fatalf("esperava ErrMissingExpectedVersion, obtido: %v", err)
		}
	})

	t.Run("divergent expected_updated_at returns ErrConflict", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: "1999-01-01T00:00:00Z",
			Action:            domain.ModerationActionApprove,
			Reason:            "Motivo válido.",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrConflict) {
			t.Fatalf("esperava ErrConflict, obtido: %v", err)
		}

		var decCount int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = ?", claimID).Scan(&decCount)
		if decCount != 0 {
			t.Errorf("decisão persistida indevidamente em conflito: %d", decCount)
		}
	})

	t.Run("empty reason returns ErrInvalidReason", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "   ",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrInvalidReason) {
			t.Fatalf("esperava ErrInvalidReason, obtido: %v", err)
		}
	})

	t.Run("html in reason returns ErrInvalidReason", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "Motivo com <script>alert(1)</script>",
			Actor:             "admin_operator",
		})
		if !errors.Is(err, moderation.ErrInvalidReason) {
			t.Fatalf("esperava ErrInvalidReason, obtido: %v", err)
		}
	})

	t.Run("empty actor returns ErrInvalidActor", func(t *testing.T) {
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: getTestClaimUpdatedAt(t, db, ctx, claimID),
			Action:            domain.ModerationActionApprove,
			Reason:            "Motivo válido.",
			Actor:             "",
		})
		if !errors.Is(err, moderation.ErrInvalidActor) {
			t.Fatalf("esperava ErrInvalidActor, obtido: %v", err)
		}
	})
}

func insertTestMonitoringCandidateWithDuplicate(t *testing.T, db *sql.DB, ctx context.Context, candID, claimID, fp, status string, isDuplicate int, canonicalID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := db.ExecContext(ctx, `
		INSERT INTO monitoring_runs (id, status, query, discovery_model, created_at)
		VALUES ('run-test', 'completed', 'teste query', 'openai/gpt-4o', ?)
		ON CONFLICT(id) DO NOTHING
	`, now)
	if err != nil {
		t.Fatalf("falha ao inserir run: %v", err)
	}

	duplicateReason := ""
	var canonicalCandidateID any = nil
	if isDuplicate == 1 {
		duplicateReason = "existing_fingerprint"
		canonicalCandidateID = canonicalID
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO monitoring_candidates (
			id, monitoring_run_id, fingerprint, entity_name, normalized_entity_name,
			proposition, suggested_grade, source_url, canonical_url, technical_confidence,
			editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id, published_claim_id, created_at, updated_at
		) VALUES (
			?, 'run-test', ?, 'Daniel Vorcaro', 'daniel vorcaro',
			'Proposição factual', 'A', 'https://folha.uol.com.br/artigo-1', 'https://folha.uol.com.br/artigo-1',
			0.95, ?, ?, ?, ?, ?, ?, ?
		)
	`, candID, fp, status, isDuplicate, duplicateReason, canonicalCandidateID, claimID, now, now)
	if err != nil {
		t.Fatalf("falha ao inserir candidato: %v", err)
	}
}

func TestModerateClaim_ConcurrencyConflict(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-concurrent-1"
	insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
	insertTestEvidenceSource(t, db, ctx, "es-conc-1", claimID, "src-1", "supports", "active")

	var initialUpdatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = ?", claimID).Scan(&initialUpdatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at inicial: %v", err)
	}

	var wg sync.WaitGroup
	successCount := 0
	conflictCount := 0
	var mu sync.Mutex

	// Disparar duas tentativas simultâneas com o mesmo updated_at esperado
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
				ClaimID:           claimID,
				ExpectedUpdatedAt: initialUpdatedAt,
				Action:            domain.ModerationActionApprove,
				Reason:            "Aprovação simultânea em teste de concorrência.",
				Actor:             "admin_operator",
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, moderation.ErrConflict) {
				conflictCount++
			}
		}(i)
	}

	wg.Wait()

	if successCount != 1 {
		t.Errorf("esperava exatamente 1 aprovação bem-sucedida, obtido: %d", successCount)
	}
	if conflictCount != 1 {
		t.Errorf("esperava exatamente 1 falha com ErrConflict estrito, obtido: %d", conflictCount)
	}

	// Verificar contagem de decisões gravadas no banco (deve ser exatamente 1)
	var totalDecisions int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = ?", claimID).Scan(&totalDecisions)
	if totalDecisions != 1 {
		t.Errorf("total de decisões gravadas = %d, esperado exatamente 1", totalDecisions)
	}
}

func TestModerateClaim_Reject_CanonicalVsDuplicateCandidate(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-reject-dup-test"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, "es-dup-test", claimID, "src-1", "supports", "active")

	// Candidato canônico
	insertTestMonitoringCandidateWithDuplicate(t, db, ctx, "cand-canon-1", claimID, "v1:fp-canonical", "published", 0, "")
	// Candidato duplicata vinculado ao mesmo claim
	insertTestMonitoringCandidateWithDuplicate(t, db, ctx, "cand-dup-1", claimID, "v1:fp-duplicate", "quarantined", 1, "cand-canon-1")

	var initialUpdatedAt string
	_ = db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = ?", claimID).Scan(&initialUpdatedAt)

	res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
		ClaimID:           claimID,
		ExpectedUpdatedAt: initialUpdatedAt,
		Action:            domain.ModerationActionReject,
		Reason:            "Rejeição com verificação de canônico vs duplicata.",
		Actor:             "admin_operator",
	})
	if err != nil {
		t.Fatalf("esperava sucesso ao rejeitar claim, erro: %v", err)
	}

	if res.NewStatus != domain.ClaimStatusRejected {
		t.Errorf("NewStatus = %q, esperado %q", res.NewStatus, domain.ClaimStatusRejected)
	}

	// Verificar que o candidato canônico mudou para rejected
	var canonStatus string
	err = db.QueryRowContext(ctx, "SELECT editorial_status FROM monitoring_candidates WHERE id = 'cand-canon-1'").Scan(&canonStatus)
	if err != nil || canonStatus != "rejected" {
		t.Errorf("candidato canônico status = %q (err=%v), esperado 'rejected'", canonStatus, err)
	}

	// Verificar que o candidato duplicata NÃO foi modificado (permaneceu 'quarantined')
	var dupStatus string
	err = db.QueryRowContext(ctx, "SELECT editorial_status FROM monitoring_candidates WHERE id = 'cand-dup-1'").Scan(&dupStatus)
	if err != nil || dupStatus != "quarantined" {
		t.Errorf("candidato duplicata status = %q (err=%v), esperado 'quarantined' (inalterado)", dupStatus, err)
	}

	// Verificar que o fingerprint gravado na decisão foi o do canônico
	var decFP string
	err = db.QueryRowContext(ctx, "SELECT candidate_fingerprint FROM moderation_decisions WHERE id = ?", res.DecisionID).Scan(&decFP)
	if err != nil {
		t.Fatalf("falha ao consultar decisão: %v", err)
	}
	if decFP != "v1:fp-canonical" {
		t.Errorf("candidate_fingerprint na decisão = %q, esperado 'v1:fp-canonical'", decFP)
	}
}

func TestModerationDecisions_TargetXORConstraint(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	claimID := "claim-xor-test"
	insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
	insertTestEvidenceSource(t, db, ctx, "es-xor-1", claimID, "src-1", "supports", "active")

	q := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339)

	// Inserção com ambos nulos deve falhar
	_, err := q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-null-both",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "approve",
		Reason:               "Teste ambos nulos",
		Actor:                "admin",
		CandidateFingerprint: "",
		CreatedAt:            now,
	})
	if err == nil {
		t.Errorf("esperava erro de constraint XOR ao inserir decisão sem alvo")
	}

	// Inserção com ambos preenchidos deve falhar
	_, err = q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-both-filled",
		ClaimID:              sql.NullString{String: claimID, Valid: true},
		EvidenceSourceID:     sql.NullString{String: "es-xor-1", Valid: true},
		Action:               "approve",
		Reason:               "Teste ambos preenchidos",
		Actor:                "admin",
		CandidateFingerprint: "",
		CreatedAt:            now,
	})
	if err == nil {
		t.Errorf("esperava erro de constraint XOR ao inserir decisão com dois alvos simultâneos")
	}
}

func getTestEvidenceSourceUpdatedAt(t *testing.T, db *sql.DB, ctx context.Context, esID string) string {
	t.Helper()
	var updatedAt string
	err := db.QueryRowContext(ctx, "SELECT updated_at FROM evidence_sources WHERE id = ?", esID).Scan(&updatedAt)
	if err != nil {
		t.Fatalf("falha ao obter updated_at do evidence_source %s: %v", esID, err)
	}
	return updatedAt
}

func TestModerateEvidenceSource_RejectLastSupport_QuarantinesClaim(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-last-support"
	esID := "es-last-support"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, esID, claimID, "src-1", "supports", "active")
	insertTestMonitoringCandidate(t, db, ctx, "cand-ls-1", claimID, "v1:fp-last-support", "published")

	esUpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)

	res, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  esID,
		ExpectedUpdatedAt: esUpdatedAt,
		Action:            domain.ModerationActionReject,
		Reason:            "Trecho de evidência descontextualizado e desaprovado.",
		Actor:             "admin_editor",
	})
	if err != nil {
		t.Fatalf("ModerateEvidenceSource falhou: %v", err)
	}

	if res.PreviousStatus != domain.EvidenceSourceStatusActive {
		t.Errorf("PreviousStatus = %q, esperado 'active'", res.PreviousStatus)
	}
	if res.NewStatus != domain.EvidenceSourceStatusRejected {
		t.Errorf("NewStatus = %q, esperado 'rejected'", res.NewStatus)
	}
	if !res.ClaimQuarantined {
		t.Errorf("ClaimQuarantined = false, esperado true (perdeu o último suporte)")
	}
	if res.NewClaimStatus != domain.ClaimStatusQuarantined {
		t.Errorf("NewClaimStatus = %q, esperado 'quarantined'", res.NewClaimStatus)
	}

	// Verificar estado do claim no banco
	var claimStatus string
	var metricEligible int
	err = db.QueryRowContext(ctx, "SELECT status, metric_eligible FROM claims WHERE id = ?", claimID).Scan(&claimStatus, &metricEligible)
	if err != nil {
		t.Fatalf("falha ao consultar claim: %v", err)
	}
	if claimStatus != "quarantined" {
		t.Errorf("claim status no banco = %q, esperado 'quarantined'", claimStatus)
	}
	if metricEligible != 0 {
		t.Errorf("claim metric_eligible no banco = %d, esperado 0", metricEligible)
	}

	// Verificar estado do evidence_source no banco
	var esStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM evidence_sources WHERE id = ?", esID).Scan(&esStatus)
	if err != nil {
		t.Fatalf("falha ao consultar evidence_source: %v", err)
	}
	if esStatus != "rejected" {
		t.Errorf("evidence_source status no banco = %q, esperado 'rejected'", esStatus)
	}

	// Verificar auditoria em moderation_decisions (com XOR estrito)
	var decClaimID sql.NullString
	var decESID sql.NullString
	var decAction, decActor, decReason, decFP string
	err = db.QueryRowContext(ctx, `
		SELECT claim_id, evidence_source_id, action, actor, reason, candidate_fingerprint
		FROM moderation_decisions WHERE id = ?
	`, res.DecisionID).Scan(&decClaimID, &decESID, &decAction, &decActor, &decReason, &decFP)
	if err != nil {
		t.Fatalf("falha ao consultar decisão de moderação: %v", err)
	}
	if decClaimID.Valid {
		t.Errorf("claim_id na decisão deveria ser nulo (XOR), obtido %q", decClaimID.String)
	}
	if !decESID.Valid || decESID.String != esID {
		t.Errorf("evidence_source_id na decisão = %+v, esperado %q", decESID, esID)
	}
	if decAction != "reject" || decActor != "admin_editor" || decReason != "Trecho de evidência descontextualizado e desaprovado." {
		t.Errorf("dados da decisão incorretos: action=%q, actor=%q, reason=%q", decAction, decActor, decReason)
	}
	if decFP != "v1:fp-last-support" {
		t.Errorf("candidate_fingerprint na decisão = %q, esperado 'v1:fp-last-support'", decFP)
	}
}

func TestModerateEvidenceSource_RejectNonLastSupport_KeepsClaimPublished(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-multi-support"
	es1ID := "es-multi-1"
	es2ID := "es-multi-2"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, es1ID, claimID, "src-1", "supports", "active")
	insertTestEvidenceSource(t, db, ctx, es2ID, claimID, "src-1", "supports", "active")

	es1UpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, es1ID)

	res, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  es1ID,
		ExpectedUpdatedAt: es1UpdatedAt,
		Action:            domain.ModerationActionReject,
		Reason:            "Rejeitando primeiro suporte; claim ainda possui segundo suporte ativo.",
		Actor:             "admin_editor",
	})
	if err != nil {
		t.Fatalf("ModerateEvidenceSource falhou: %v", err)
	}

	if res.ClaimQuarantined {
		t.Errorf("ClaimQuarantined = true, esperado false (claim ainda tem es-multi-2 ativo)")
	}
	if res.NewClaimStatus != domain.ClaimStatusPublished {
		t.Errorf("NewClaimStatus = %q, esperado 'published'", res.NewClaimStatus)
	}

	// Verificar que o claim permaneceu published no banco
	var claimStatus string
	var metricEligible int
	err = db.QueryRowContext(ctx, "SELECT status, metric_eligible FROM claims WHERE id = ?", claimID).Scan(&claimStatus, &metricEligible)
	if err != nil {
		t.Fatalf("falha ao consultar claim: %v", err)
	}
	if claimStatus != "published" {
		t.Errorf("claim status no banco = %q, esperado 'published'", claimStatus)
	}
	if metricEligible != 1 {
		t.Errorf("claim metric_eligible no banco = %d, esperado 1", metricEligible)
	}
}

func TestModerateEvidenceSource_RejectNonSupportRole_KeepsClaimPublished(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-role-test"
	esSupportID := "es-sup-1"
	esContradictID := "es-contra-1"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, esSupportID, claimID, "src-1", "supports", "active")
	insertTestEvidenceSource(t, db, ctx, esContradictID, claimID, "src-1", "contradicts", "active")

	contraUpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, esContradictID)

	res, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  esContradictID,
		ExpectedUpdatedAt: contraUpdatedAt,
		Action:            domain.ModerationActionReject,
		Reason:            "Rejeitando evidência de defesa contraditória desnecessária.",
		Actor:             "admin_editor",
	})
	if err != nil {
		t.Fatalf("ModerateEvidenceSource falhou: %v", err)
	}

	if res.ClaimQuarantined {
		t.Errorf("ClaimQuarantined = true, esperado false (rejeição de contradicts não afeta suporte)")
	}

	var claimStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&claimStatus)
	if err != nil {
		t.Fatalf("falha ao consultar claim: %v", err)
	}
	if claimStatus != "published" {
		t.Errorf("claim status no banco = %q, esperado 'published'", claimStatus)
	}
}

func TestModerateEvidenceSource_Restore_KeepsClaimInQuarantine(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-restore-test"
	esID := "es-restore-1"
	insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
	insertTestEvidenceSource(t, db, ctx, esID, claimID, "src-1", "supports", "rejected")

	esUpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)

	res, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  esID,
		ExpectedUpdatedAt: esUpdatedAt,
		Action:            domain.ModerationActionRestore,
		Reason:            "Restaurando uso de evidência após validação documental.",
		Actor:             "admin_editor",
	})
	if err != nil {
		t.Fatalf("ModerateEvidenceSource falhou: %v", err)
	}

	if res.PreviousStatus != domain.EvidenceSourceStatusRejected {
		t.Errorf("PreviousStatus = %q, esperado 'rejected'", res.PreviousStatus)
	}
	if res.NewStatus != domain.EvidenceSourceStatusActive {
		t.Errorf("NewStatus = %q, esperado 'active'", res.NewStatus)
	}
	if res.NewClaimStatus != domain.ClaimStatusQuarantined {
		t.Errorf("NewClaimStatus = %q, esperado 'quarantined' (invariante: restauração de evidência nunca republica claim)", res.NewClaimStatus)
	}

	// Verificar no banco que o claim continua em quarentena
	var claimStatus string
	err = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&claimStatus)
	if err != nil {
		t.Fatalf("falha ao consultar claim: %v", err)
	}
	if claimStatus != "quarantined" {
		t.Errorf("claim status no banco = %q, esperado 'quarantined'", claimStatus)
	}
}

func TestModerateEvidenceSource_OCC_ConflictAndConcurrency(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-occ-test"
	esID := "es-occ-1"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, esID, claimID, "src-1", "supports", "active")

	esUpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)

	// 1. Versão esperada vazia deve falhar com ErrMissingExpectedVersion
	_, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  esID,
		ExpectedUpdatedAt: "",
		Action:            domain.ModerationActionReject,
		Reason:            "Teste sem versão esperada",
		Actor:             "admin",
	})
	if !errors.Is(err, moderation.ErrMissingExpectedVersion) {
		t.Errorf("esperava ErrMissingExpectedVersion, obtido %v", err)
	}

	// 2. Versão divergente/stale deve falhar com ErrConflict
	_, err = svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  esID,
		ExpectedUpdatedAt: "2020-01-01T00:00:00Z",
		Action:            domain.ModerationActionReject,
		Reason:            "Teste versão antiga",
		Actor:             "admin",
	})
	if !errors.Is(err, moderation.ErrConflict) {
		t.Errorf("esperava ErrConflict para versão stale, obtido %v", err)
	}

	// 3. Concorrência real: duas goroutines tentando moderar simultaneamente com a mesma versão inicial
	var wg sync.WaitGroup
	results := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, rErr := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            fmt.Sprintf("Decisão concorrente número %d", idx),
				Actor:             "admin_concurrent",
			})
			results[idx] = rErr
		}(i)
	}

	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, resErr := range results {
		if resErr == nil {
			successCount++
		} else if errors.Is(resErr, moderation.ErrConflict) {
			conflictCount++
		} else {
			t.Errorf("erro inesperado na moderação concorrente: %v", resErr)
		}
	}

	if successCount != 1 || conflictCount != 1 {
		t.Errorf("concorrência determinística: esperado 1 sucesso e 1 conflito, obtido %d sucessos e %d conflitos", successCount, conflictCount)
	}

	// Exatamente uma decisão deve ter sido gravada no banco
	var decCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE evidence_source_id = ?", esID).Scan(&decCount)
	if err != nil {
		t.Fatalf("falha ao consultar contagem de decisões: %v", err)
	}
	if decCount != 1 {
		t.Errorf("contagem de decisões no banco = %d, esperado exatamente 1", decCount)
	}
}

func TestModerateEvidenceSource_ValidationErrors(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-val-test"
	esID := "es-val-1"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, esID, claimID, "src-1", "supports", "active")
	esUpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)

	tests := []struct {
		name    string
		params  moderation.ModerateEvidenceSourceParams
		wantErr error
	}{
		{
			name: "evidence_source_id vazio",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  "   ",
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            "Justificativa válida",
				Actor:             "admin",
			},
			wantErr: moderation.ErrEvidenceSourceNotFound,
		},
		{
			name: "evidence_source inexistente",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  "es-inexistente",
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            "Justificativa válida",
				Actor:             "admin",
			},
			wantErr: moderation.ErrEvidenceSourceNotFound,
		},
		{
			name: "motivo vazio",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            "   ",
				Actor:             "admin",
			},
			wantErr: moderation.ErrInvalidReason,
		},
		{
			name: "motivo com tags html",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            "<b>Rejeição com HTML</b>",
				Actor:             "admin",
			},
			wantErr: moderation.ErrInvalidReason,
		},
		{
			name: "motivo acima do limite de 1000 caracteres",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            string(make([]byte, 1001)),
				Actor:             "admin",
			},
			wantErr: moderation.ErrInvalidReason,
		},
		{
			name: "operador vazio",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionReject,
				Reason:            "Justificativa válida",
				Actor:             "   ",
			},
			wantErr: moderation.ErrInvalidActor,
		},
		{
			name: "ação inválida",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationAction("acao_invalida"),
				Reason:            "Justificativa válida",
				Actor:             "admin",
			},
			wantErr: moderation.ErrInvalidAction,
		},
		{
			name: "transição inválida (restore em active)",
			params: moderation.ModerateEvidenceSourceParams{
				EvidenceSourceID:  esID,
				ExpectedUpdatedAt: esUpdatedAt,
				Action:            domain.ModerationActionRestore,
				Reason:            "Justificativa válida",
				Actor:             "admin",
			},
			wantErr: moderation.ErrInvalidTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ModerateEvidenceSource(ctx, tt.params)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ModerateEvidenceSource() erro = %v, esperado %v", err, tt.wantErr)
			}
		})
	}
}

func TestModerateEvidenceSource_SourceTableIntact(t *testing.T) {
	db, ctx := setupTestDB(t)
	_, relID := seedTestData(t, db, ctx)
	svc := moderation.NewService(db)

	claimID := "claim-src-intact"
	esID := "es-src-intact"
	srcID := "src-1"
	insertTestClaim(t, db, ctx, claimID, relID, "published")
	insertTestEvidenceSource(t, db, ctx, esID, claimID, srcID, "supports", "active")
	esUpdatedAt := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)

	// Capturar estado da source antes
	var srcTitleBefore, srcAccessStatusBefore, srcUrlBefore string
	err := db.QueryRowContext(ctx, "SELECT title, source_access_status, original_url FROM sources WHERE id = ?", srcID).Scan(&srcTitleBefore, &srcAccessStatusBefore, &srcUrlBefore)
	if err != nil {
		t.Fatalf("falha ao consultar source: %v", err)
	}

	// Executar moderação do evidence_source
	_, err = svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
		EvidenceSourceID:  esID,
		ExpectedUpdatedAt: esUpdatedAt,
		Action:            domain.ModerationActionReject,
		Reason:            "Rejeição pontual do uso de evidência.",
		Actor:             "admin_editor",
	})
	if err != nil {
		t.Fatalf("ModerateEvidenceSource falhou: %v", err)
	}

	// Capturar estado da source depois
	var srcTitleAfter, srcAccessStatusAfter, srcUrlAfter string
	err = db.QueryRowContext(ctx, "SELECT title, source_access_status, original_url FROM sources WHERE id = ?", srcID).Scan(&srcTitleAfter, &srcAccessStatusAfter, &srcUrlAfter)
	if err != nil {
		t.Fatalf("falha ao consultar source após moderação: %v", err)
	}

	if srcTitleAfter != srcTitleBefore || srcAccessStatusAfter != srcAccessStatusBefore || srcUrlAfter != srcUrlBefore {
		t.Errorf("fonte documental global não deve ser alterada: antes=(%q, %q, %q), depois=(%q, %q, %q)",
			srcTitleBefore, srcAccessStatusBefore, srcUrlBefore,
			srcTitleAfter, srcAccessStatusAfter, srcUrlAfter,
		)
	}
}
