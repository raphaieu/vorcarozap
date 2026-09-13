package moderation_test

import (
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/moderation"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// TestDeterministic_Moderation_TableDriven valida rigorosamente todos os invariantes de moderação humana
// para claims e evidence_sources, controle de concorrência otimista (OCC), quarentena por perda de suporte,
// auditoria imutável XOR e isolamento de tabelas.
func TestDeterministic_Moderation_TableDriven(t *testing.T) {
	t.Run("Controle Otimista de Versão (OCC) em Claims", func(t *testing.T) {
		db, ctx := setupTestDB(t)
		_, relID := seedTestData(t, db, ctx)
		svc := moderation.NewService(db)

		claimID := "claim-occ-test"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-occ-1", claimID, "src-1", "supports", "active")

		validUpdatedAt := getTestClaimUpdatedAt(t, db, ctx, claimID)

		// 1. Versão ausente / vazia => ErrMissingExpectedVersion sem mutação
		_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: "",
			Action:            domain.ModerationActionApprove,
			Reason:            "Aprovação com versão vazia",
			Actor:             "admin",
		})
		if !errors.Is(err, moderation.ErrMissingExpectedVersion) {
			t.Fatalf("esperava ErrMissingExpectedVersion para versão vazia, obtido %v", err)
		}

		// 2. Versão divergente / obsoleta => ErrConflict (409) sem auditoria
		_, err = svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: "2020-01-01T00:00:00Z",
			Action:            domain.ModerationActionApprove,
			Reason:            "Aprovação com versão stale",
			Actor:             "admin",
		})
		if !errors.Is(err, moderation.ErrConflict) {
			t.Fatalf("esperava ErrConflict para versão stale, obtido %v", err)
		}

		// Garante que o claim permaneceu em quarentena e nenhuma decisão foi criada
		var status string
		_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&status)
		if status != "quarantined" {
			t.Errorf("claim status = %q, esperado 'quarantined'", status)
		}
		var decCount int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = ?", claimID).Scan(&decCount)
		if decCount != 0 {
			t.Errorf("nenhuma decisão deveria ser criada após falha de OCC, obtido %d", decCount)
		}

		// 3. Versão correta => Sucesso
		res, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: validUpdatedAt,
			Action:            domain.ModerationActionApprove,
			Reason:            "Aprovação legítima com versão correta",
			Actor:             "admin",
		})
		if err != nil {
			t.Fatalf("esperava sucesso com versão correta, erro: %v", err)
		}
		if res.NewStatus != domain.ClaimStatusPublished {
			t.Errorf("NewStatus = %q, esperado 'published'", res.NewStatus)
		}
	})

	t.Run("Concorrência Real: Duas ações simultâneas resultam em exatamente 1 sucesso e 1 conflito 409", func(t *testing.T) {
		db, ctx := setupTestDB(t)
		_, relID := seedTestData(t, db, ctx)
		svc := moderation.NewService(db)

		claimID := "claim-conc-occ"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-conc-occ", claimID, "src-1", "supports", "active")

		initialVersion := getTestClaimUpdatedAt(t, db, ctx, claimID)

		var wg sync.WaitGroup
		successCount := 0
		conflictCount := 0
		var mu sync.Mutex

		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				_, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
					ClaimID:           claimID,
					ExpectedUpdatedAt: initialVersion,
					Action:            domain.ModerationActionApprove,
					Reason:            "Aprovação simultânea em goroutine",
					Actor:             "admin_worker",
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

		if successCount != 1 || conflictCount != 1 {
			t.Errorf("esperava exatamente 1 sucesso e 1 conflito: sucessos=%d, conflitos=%d", successCount, conflictCount)
		}

		var totalDecisions int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM moderation_decisions WHERE claim_id = ?", claimID).Scan(&totalDecisions)
		if totalDecisions != 1 {
			t.Errorf("total de decisões gravadas no banco esperado 1, obtido %d", totalDecisions)
		}
	})

	t.Run("Moderação de EvidenceSource: Rejeição do último suporte quarentena o claim e zera elegibilidade métrica na mesma transação", func(t *testing.T) {
		db, ctx := setupTestDB(t)
		_, relID := seedTestData(t, db, ctx)
		svc := moderation.NewService(db)

		claimID := "claim-last-supp-det"
		esID := "es-last-supp-det"
		insertTestClaim(t, db, ctx, claimID, relID, "published")
		insertTestEvidenceSource(t, db, ctx, esID, claimID, "src-1", "supports", "active")

		esVersion := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)

		res, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
			EvidenceSourceID:  esID,
			ExpectedUpdatedAt: esVersion,
			Action:            domain.ModerationActionReject,
			Reason:            "Trecho de evidência desautorizado por retratação.",
			Actor:             "editor",
		})
		if err != nil {
			t.Fatalf("ModerateEvidenceSource falhou: %v", err)
		}

		if !res.ClaimQuarantined {
			t.Errorf("ClaimQuarantined deveria ser true")
		}
		if res.NewClaimStatus != domain.ClaimStatusQuarantined {
			t.Errorf("NewClaimStatus esperado 'quarantined', obtido %q", res.NewClaimStatus)
		}

		// Validação no banco: claim foi para quarentena e metric_eligible = 0
		var claimStatus string
		var metricEligible int
		err = db.QueryRowContext(ctx, "SELECT status, metric_eligible FROM claims WHERE id = ?", claimID).Scan(&claimStatus, &metricEligible)
		if err != nil {
			t.Fatalf("falha ao consultar claim no banco: %v", err)
		}
		if claimStatus != "quarantined" || metricEligible != 0 {
			t.Errorf("claim no banco: status=%q, metric_eligible=%d; esperado ('quarantined', 0)", claimStatus, metricEligible)
		}

		// A fonte documental global em sources permanece 100% intacta
		var srcStatus string
		err = db.QueryRowContext(ctx, "SELECT source_access_status FROM sources WHERE id = 'src-1'").Scan(&srcStatus)
		if err != nil || srcStatus != "reachable" {
			t.Errorf("source global 'src-1' deve permanecer intacta (status=%q)", srcStatus)
		}

		// Validação da decisão de moderação: XOR estrito
		var claimFK sql.NullString
		var esFK sql.NullString
		err = db.QueryRowContext(ctx, "SELECT claim_id, evidence_source_id FROM moderation_decisions WHERE id = ?", res.DecisionID).Scan(&claimFK, &esFK)
		if err != nil {
			t.Fatalf("falha ao consultar decisão de moderação: %v", err)
		}
		if claimFK.Valid {
			t.Errorf("claim_id deve ser nulo na moderação de evidence_source (XOR)")
		}
		if !esFK.Valid || esFK.String != esID {
			t.Errorf("evidence_source_id incorreto na decisão: %v", esFK)
		}
	})

	t.Run("Moderação de EvidenceSource: Rejeição de suporte secundário mantém claim publicado", func(t *testing.T) {
		db, ctx := setupTestDB(t)
		_, relID := seedTestData(t, db, ctx)
		svc := moderation.NewService(db)

		claimID := "claim-multi-supp"
		esID1 := "es-supp-1"
		esID2 := "es-supp-2"
		insertTestClaim(t, db, ctx, claimID, relID, "published")
		insertTestEvidenceSource(t, db, ctx, esID1, claimID, "src-1", "supports", "active")
		insertTestEvidenceSource(t, db, ctx, esID2, claimID, "src-1", "supports", "active")

		esVersion1 := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID1)

		res, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
			EvidenceSourceID:  esID1,
			ExpectedUpdatedAt: esVersion1,
			Action:            domain.ModerationActionReject,
			Reason:            "Rejeitando apenas um dos suportes",
			Actor:             "editor",
		})
		if err != nil {
			t.Fatalf("falha ao moderar esID1: %v", err)
		}

		if res.ClaimQuarantined {
			t.Errorf("claim NÃO deveria entrar em quarentena pois ainda possui outro suporte ativo")
		}
		if res.NewClaimStatus != domain.ClaimStatusPublished {
			t.Errorf("NewClaimStatus esperado 'published', obtido %q", res.NewClaimStatus)
		}

		var claimStatus string
		_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&claimStatus)
		if claimStatus != "published" {
			t.Errorf("claim no banco esperado 'published', obtido %q", claimStatus)
		}
	})

	t.Run("Invariante de Restauração: Restaurar evidence_source ou claim NUNCA republica automaticamente", func(t *testing.T) {
		db, ctx := setupTestDB(t)
		_, relID := seedTestData(t, db, ctx)
		svc := moderation.NewService(db)

		// 1. Claim rejeitado -> restauração vai para quarentena
		claimID := "claim-restore-invar"
		insertTestClaim(t, db, ctx, claimID, relID, "rejected")
		insertTestEvidenceSource(t, db, ctx, "es-rest-invar", claimID, "src-1", "supports", "active")

		claimVer := getTestClaimUpdatedAt(t, db, ctx, claimID)
		resClaim, err := svc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: claimVer,
			Action:            domain.ModerationActionRestore,
			Reason:            "Restauração para reavaliação em quarentena",
			Actor:             "editor",
		})
		if err != nil {
			t.Fatalf("falha ao restaurar claim: %v", err)
		}
		if resClaim.NewStatus != domain.ClaimStatusQuarantined {
			t.Errorf("restauração de claim deve ir estritamente para 'quarantined', obtido %q", resClaim.NewStatus)
		}

		// 2. Evidence source rejeitado associado a claim em quarentena -> restauração do suporte NÃO republica o claim
		esID := "es-rest-supp-invar"
		insertTestEvidenceSource(t, db, ctx, esID, claimID, "src-1", "supports", "rejected")

		esVer := getTestEvidenceSourceUpdatedAt(t, db, ctx, esID)
		resES, err := svc.ModerateEvidenceSource(ctx, moderation.ModerateEvidenceSourceParams{
			EvidenceSourceID:  esID,
			ExpectedUpdatedAt: esVer,
			Action:            domain.ModerationActionRestore,
			Reason:            "Reativação de fonte",
			Actor:             "editor",
		})
		if err != nil {
			t.Fatalf("falha ao restaurar evidence_source: %v", err)
		}
		if resES.NewStatus != domain.EvidenceSourceStatusActive {
			t.Errorf("NewStatus de evidence_source esperado 'active', obtido %q", resES.NewStatus)
		}
		if resES.NewClaimStatus != domain.ClaimStatusQuarantined {
			t.Errorf("NewClaimStatus após restaurar evidência DEVE continuar 'quarantined', obtido %q", resES.NewClaimStatus)
		}

		var claimFinalStatus string
		_ = db.QueryRowContext(ctx, "SELECT status FROM claims WHERE id = ?", claimID).Scan(&claimFinalStatus)
		if claimFinalStatus != "quarantined" {
			t.Errorf("claim status no banco deve permanecer 'quarantined', obtido %q", claimFinalStatus)
		}
	})

	t.Run("Integridade Transacional: Constraint XOR em moderation_decisions impede violações de alvo", func(t *testing.T) {
		db, ctx := setupTestDB(t)
		_, relID := seedTestData(t, db, ctx)
		claimID := "claim-xor-table"
		insertTestClaim(t, db, ctx, claimID, relID, "quarantined")
		insertTestEvidenceSource(t, db, ctx, "es-xor-table", claimID, "src-1", "supports", "active")

		q := sqlc.New(db)

		// Cenário A: ambos nulos deve falhar
		_, err := q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
			ID:                   "dec-a-null",
			ClaimID:              sql.NullString{Valid: false},
			EvidenceSourceID:     sql.NullString{Valid: false},
			Action:               "approve",
			Reason:               "Ambos nulos",
			Actor:                "admin",
			CandidateFingerprint: "",
			CreatedAt:            "2026-09-03T12:00:00Z",
		})
		if err == nil {
			t.Errorf("esperava erro de constraint XOR para ambos nulos")
		}

		// Cenário B: ambos preenchidos deve falhar
		_, err = q.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
			ID:                   "dec-b-both",
			ClaimID:              sql.NullString{String: claimID, Valid: true},
			EvidenceSourceID:     sql.NullString{String: "es-xor-table", Valid: true},
			Action:               "approve",
			Reason:               "Ambos preenchidos",
			Actor:                "admin",
			CandidateFingerprint: "",
			CreatedAt:            "2026-09-03T12:00:00Z",
		})
		if err == nil {
			t.Errorf("esperava erro de constraint XOR para ambos preenchidos simultaneamente")
		}
	})
}
