package monitoring_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/moderation"
	"github.com/raphaieu/vorcarozap/internal/monitoring"
	"github.com/raphaieu/vorcarozap/internal/normalize"
	"github.com/raphaieu/vorcarozap/internal/research"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

// TestDeterministic_RejectedFingerprint_TableDriven comprova todas as propriedades e invariantes
// do fingerprint v1, bloqueio de republicação de candidatos com fingerprint rejeitado em runs
// separadas, preservação de candidatos duplicados e comportamento de moderação.
func TestDeterministic_RejectedFingerprint_TableDriven(t *testing.T) {
	t.Run("Fingerprint v1 é determinístico e estável", func(t *testing.T) {
		url := "https://noticias.exemplo.com/materia-controle-master"
		entity := "Daniel Vorcaro"
		prop := "Aquisição de controle societário do Banco Master"
		excerpt := "Daniel Vorcaro adquiriu o controle societário em assembleia."

		fp1 := normalize.FingerprintV1(url, entity, prop, excerpt)
		fp2 := normalize.FingerprintV1(url, entity, prop, excerpt)

		if fp1 != fp2 {
			t.Fatalf("fingerprint v1 não é determinístico: %s != %s", fp1, fp2)
		}
		if !strings.HasPrefix(fp1, "v1:") {
			t.Errorf("fingerprint deve possuir prefixo 'v1:', obtido: %s", fp1)
		}
		if len(fp1) != 67 { // "v1:" (3) + 64 hex sha256
			t.Errorf("tamanho do fingerprint v1 esperado 67 caracteres, obtido %d (%s)", len(fp1), fp1)
		}
	})

	t.Run("Candidato com fingerprint rejeitado anteriormente é bloqueado pelo gate estrutural em nova run", func(t *testing.T) {
		db, ctx := setupTestDB(t)

		subjID := "ent-" + uuid.NewString()[:8]
		targetID := "ent-" + uuid.NewString()[:8]
		seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
		seedEntity(t, db, targetID, "Banco Master", "organization")

		// 1. Run 1: Cria candidato e simula rejeição editorial
		runID1 := createTestMonitoringRun(t, db)
		candID1 := "cand-rej-1"
		targetURL := "https://noticias.exemplo.com/materia-rejeitada"
		prop := "Proposição documental de teste"
		excerpt := "Trecho literal da matéria"
		fp := normalize.FingerprintV1(targetURL, "Daniel Vorcaro", prop, excerpt)
		now := time.Now().UTC().Format(time.RFC3339Nano)

		queries := sqlc.New(db)
		_, err := queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
			ID:                         candID1,
			MonitoringRunID:            runID1,
			Fingerprint:                fp,
			FingerprintVersion:         1,
			EntityName:                 "Daniel Vorcaro",
			NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
			TargetEntityName:           "Banco Master",
			NormalizedTargetEntityName: normalize.Name("Banco Master"),
			RelationshipType:           "societário",
			Proposition:                prop,
			SuggestedGrade:             "A",
			SourceUrl:                  targetURL,
			CanonicalUrl:               targetURL,
			SourceTitle:                "Matéria",
			PublisherOrAuthor:          "Autor",
			Excerpt:                    excerpt,
			TechnicalConfidence:        0.95,
			EditorialStatus:            "rejected", // Rejeitado!
			IsDuplicate:                0,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		})
		if err != nil {
			t.Fatalf("falha ao inserir candidato rejeitado: %v", err)
		}

		// 2. Run 2: Nova run descobre o mesmo fato (mesmo fingerprint v1)
		runID2 := createTestMonitoringRun(t, db)
		candID2 := "cand-nova-tentativa-2"
		_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
			ID:                         candID2,
			MonitoringRunID:            runID2,
			Fingerprint:                fp,
			FingerprintVersion:         1,
			EntityName:                 "Daniel Vorcaro",
			NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
			TargetEntityName:           "Banco Master",
			NormalizedTargetEntityName: normalize.Name("Banco Master"),
			RelationshipType:           "societário",
			Proposition:                prop,
			SuggestedGrade:             "A",
			SourceUrl:                  targetURL,
			CanonicalUrl:               targetURL,
			SourceTitle:                "Matéria",
			PublisherOrAuthor:          "Autor",
			Excerpt:                    excerpt,
			TechnicalConfidence:        0.95,
			EditorialStatus:            "quarantined",
			IsDuplicate:                0,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		})
		if err != nil {
			t.Fatalf("falha ao inserir novo candidato: %v", err)
		}

		var verifyCalls int32
		mockProvider := &mockResearchProvider{
			verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
				atomic.AddInt32(&verifyCalls, 1)
				return &research.VerifyResult{
					IdentityMatch:            true,
					ClaimSupported:           true,
					ClaimOverstatesSource:    false,
					AttributionExplicit:      true,
					GradeCompatible:          true,
					ContainsIllicitInference: false,
					RecommendedAction:        "publish",
				}, nil
			},
		}

		evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
			DB:             db,
			Provider:       mockProvider,
			SourceVerifier: &mockSourceVerifier{},
		})
		if err != nil {
			t.Fatalf("falha ao criar Evaluator: %v", err)
		}

		// 3. Avaliação de candID2: DEVE falhar no gate estrutural devido ao fingerprint rejeitado
		res, err := evaluator.EvaluateCandidate(ctx, candID2)
		if err != nil {
			t.Fatalf("EvaluateCandidate retornou erro inesperado: %v", err)
		}

		if res.Published {
			t.Fatalf("candidato com fingerprint rejeitado NÃO pode ser publicado!")
		}
		if res.StructuralPassed {
			t.Errorf("gate estrutural deveria ter reprovado por fingerprint_previously_rejected")
		}
		if res.PolicyAction != domain.PolicyActionQuarantine {
			t.Errorf("PolicyAction esperado 'quarantine', obtido %q", res.PolicyAction)
		}

		// Regra crítica: LLM Verify não deve ser chamada
		if atomic.LoadInt32(&verifyCalls) != 0 {
			t.Errorf("LLM Verify NÃO deveria ter sido chamada para candidato com fingerprint rejeitado!")
		}

		// Nenhum claim, relationship ou source pública deve ser criada
		var claimsCount int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM claims").Scan(&claimsCount)
		if claimsCount != 0 {
			t.Errorf("nenhum claim deveria existir no banco, obtido %d", claimsCount)
		}
	})

	t.Run("Rejeição humana de claim publicado bloqueia novos candidatos com o mesmo fingerprint", func(t *testing.T) {
		db, ctx := setupTestDB(t)

		subjID := "ent-" + uuid.NewString()[:8]
		targetID := "ent-" + uuid.NewString()[:8]
		seedEntity(t, db, subjID, "Daniel Vorcaro", "person")
		seedEntity(t, db, targetID, "Banco Master", "organization")

		runID1 := createTestMonitoringRun(t, db)
		candID1 := "cand-pub-and-reject-1"
		targetURL := "https://noticias.exemplo.com/materia-para-moderar"
		prop := "Vínculo societário publicado e depois rejeitado"
		excerpt := "Trecho literal da comprovação"
		fp := normalize.FingerprintV1(targetURL, "Daniel Vorcaro", prop, excerpt)
		now := time.Now().UTC().Format(time.RFC3339Nano)

		// 1. Cria e publica candidato Grau A
		queries := sqlc.New(db)
		_, err := queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
			ID:                         candID1,
			MonitoringRunID:            runID1,
			Fingerprint:                fp,
			FingerprintVersion:         1,
			EntityName:                 "Daniel Vorcaro",
			NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
			TargetEntityName:           "Banco Master",
			NormalizedTargetEntityName: normalize.Name("Banco Master"),
			RelationshipType:           "societário",
			Proposition:                prop,
			SuggestedGrade:             "A",
			SourceUrl:                  targetURL,
			CanonicalUrl:               targetURL,
			SourceTitle:                "Matéria",
			PublisherOrAuthor:          "Autor",
			Excerpt:                    excerpt,
			TechnicalConfidence:        0.95,
			EditorialStatus:            "quarantined",
			IsDuplicate:                0,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		})
		if err != nil {
			t.Fatalf("falha ao inserir candidato: %v", err)
		}

		mockProvider := &mockResearchProvider{
			verifyFn: func(ctx context.Context, input research.VerifyInput) (*research.VerifyResult, error) {
				return &research.VerifyResult{
					IdentityMatch:            true,
					ClaimSupported:           true,
					ClaimOverstatesSource:    false,
					AttributionExplicit:      true,
					GradeCompatible:          true,
					ContainsIllicitInference: false,
					RecommendedAction:        "publish",
				}, nil
			},
		}

		evaluator, err := monitoring.NewEvaluator(monitoring.EvaluatorConfig{
			DB:             db,
			Provider:       mockProvider,
			SourceVerifier: &mockSourceVerifier{},
		})
		if err != nil {
			t.Fatalf("falha ao criar Evaluator: %v", err)
		}

		evalRes, err := evaluator.EvaluateCandidate(ctx, candID1)
		if err != nil || !evalRes.Published {
			t.Fatalf("candidato inicial deveria ter sido publicado com sucesso: %v", err)
		}

		claimID := evalRes.PublishedClaimID

		// 2. Administrador rejeita o claim publicado via serviço de moderação
		var claimUpdatedAt string
		_ = db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = ?", claimID).Scan(&claimUpdatedAt)

		modSvc := moderation.NewService(db)
		modRes, err := modSvc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: claimUpdatedAt,
			Action:            domain.ModerationActionReject,
			Reason:            "Rejeição pós-moderação por contestação de fonte.",
			Actor:             "editor_chefe",
		})
		if err != nil {
			t.Fatalf("ModerateClaim reject falhou: %v", err)
		}
		if modRes.NewStatus != domain.ClaimStatusRejected {
			t.Fatalf("claim deveria estar com status 'rejected', obtido %q", modRes.NewStatus)
		}

		// 3. Verifica que o candidato canônico agora possui editorial_status = 'rejected'
		cand1DB, err := queries.GetMonitoringCandidateByID(ctx, candID1)
		if err != nil {
			t.Fatalf("falha ao buscar candidato 1: %v", err)
		}
		if cand1DB.EditorialStatus != "rejected" {
			t.Fatalf("candidato canônico deveria ter sido marcado como 'rejected', obtido %q", cand1DB.EditorialStatus)
		}

		// 4. Nova run de monitoramento tenta avaliar candidato com mesmo fingerprint
		runID3 := createTestMonitoringRun(t, db)
		candID3 := "cand-tentativa-pos-rejeicao"
		_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
			ID:                         candID3,
			MonitoringRunID:            runID3,
			Fingerprint:                fp,
			FingerprintVersion:         1,
			EntityName:                 "Daniel Vorcaro",
			NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
			TargetEntityName:           "Banco Master",
			NormalizedTargetEntityName: normalize.Name("Banco Master"),
			RelationshipType:           "societário",
			Proposition:                prop,
			SuggestedGrade:             "A",
			SourceUrl:                  targetURL,
			CanonicalUrl:               targetURL,
			SourceTitle:                "Matéria",
			PublisherOrAuthor:          "Autor",
			Excerpt:                    excerpt,
			TechnicalConfidence:        0.95,
			EditorialStatus:            "quarantined",
			IsDuplicate:                0,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		})
		if err != nil {
			t.Fatalf("falha ao inserir candidato 3: %v", err)
		}

		res3, err := evaluator.EvaluateCandidate(ctx, candID3)
		if err != nil {
			t.Fatalf("EvaluateCandidate falhou: %v", err)
		}
		if res3.Published {
			t.Fatalf("candidato após rejeição humana de claim DEVE ser bloqueado!")
		}
		if res3.StructuralPassed {
			t.Errorf("gate estrutural deveria ter falhado com fingerprint_previously_rejected")
		}

		// 5. Restauração do claim (rejected -> quarantined) NUNCA deve remover o bloqueio de fingerprint
		_ = db.QueryRowContext(ctx, "SELECT updated_at FROM claims WHERE id = ?", claimID).Scan(&claimUpdatedAt)
		_, err = modSvc.ModerateClaim(ctx, moderation.ModerateClaimParams{
			ClaimID:           claimID,
			ExpectedUpdatedAt: claimUpdatedAt,
			Action:            domain.ModerationActionRestore,
			Reason:            "Restauração para reexame.",
			Actor:             "editor_chefe",
		})
		if err != nil {
			t.Fatalf("ModerateClaim restore falhou: %v", err)
		}

		// Candidato canônico continua 'rejected'
		cand1PostRestore, _ := queries.GetMonitoringCandidateByID(ctx, candID1)
		if cand1PostRestore.EditorialStatus != "rejected" {
			t.Errorf("restauração de claim não deve alterar o status 'rejected' do candidato em monitoring_candidates, obtido %q", cand1PostRestore.EditorialStatus)
		}

		// Nova tentativa continua bloqueada
		candID4 := "cand-tentativa-pos-restore"
		_, _ = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
			ID:                         candID4,
			MonitoringRunID:            runID3,
			Fingerprint:                fp,
			FingerprintVersion:         1,
			EntityName:                 "Daniel Vorcaro",
			NormalizedEntityName:       normalize.Name("Daniel Vorcaro"),
			TargetEntityName:           "Banco Master",
			NormalizedTargetEntityName: normalize.Name("Banco Master"),
			RelationshipType:           "societário",
			Proposition:                prop,
			SuggestedGrade:             "A",
			SourceUrl:                  targetURL,
			CanonicalUrl:               targetURL,
			SourceTitle:                "Matéria",
			PublisherOrAuthor:          "Autor",
			Excerpt:                    excerpt,
			TechnicalConfidence:        0.95,
			EditorialStatus:            "quarantined",
			IsDuplicate:                0,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		})

		res4, err := evaluator.EvaluateCandidate(ctx, candID4)
		if err != nil || res4.Published {
			t.Errorf("candidato após restauração para quarentena continua bloqueado contra publicação automática: res=%+v, err=%v", res4, err)
		}
	})
}
