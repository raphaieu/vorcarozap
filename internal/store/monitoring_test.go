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

func setupTestDBForMonitoring(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "store_monitoring_test.db")
	ctx := context.Background()

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

func TestMonitoringRunConstraints(t *testing.T) {
	db, ctx := setupTestDBForMonitoring(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Inserção válida
	run, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                "run-1",
		Status:            "pending",
		Query:             "Daniel Vorcaro",
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    "openai/gpt-4.1-mini",
		CreatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar run válido: %v", err)
	}
	if run.Status != "pending" {
		t.Errorf("status esperado 'pending', obtido %q", run.Status)
	}

	// 2. Status inválido (deve violar CHECK)
	_, err = queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                "run-invalid-status",
		Status:            "published", // Não é status de run
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    "openai/gpt-4.1-mini",
		CreatedAt:         now,
	})
	if err == nil {
		t.Fatal("esperava erro de CHECK ao tentar inserir status inválido em monitoring_runs")
	}

	// 3. DiscoveryModel vazio (deve violar CHECK length > 0)
	_, err = queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                "run-empty-model",
		Status:            "pending",
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    "   ",
		CreatedAt:         now,
	})
	if err == nil {
		t.Fatal("esperava erro de CHECK para discovery_model vazio")
	}
}

func TestMonitoringCandidateConstraintsAndFK(t *testing.T) {
	db, ctx := setupTestDBForMonitoring(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Cria run pai
	_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                "run-parent-1",
		Status:            "running",
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    "openai/gpt-4.1-mini",
		CreatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar run pai: %v", err)
	}

	// 1. Inserção de candidato canônico válido
	c1, err := queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-1",
		MonitoringRunID:      "run-parent-1",
		Fingerprint:          "v1:abc123hash",
		FingerprintVersion:   1,
		EntityName:           "Daniel Vorcaro",
		NormalizedEntityName: "daniel vorcaro",
		Proposition:          "Aquisição de controle",
		SuggestedGrade:       "A",
		SourceUrl:            "https://noticias.com/1",
		CanonicalUrl:         "https://noticias.com/1",
		TechnicalConfidence:  0.95,
		EditorialStatus:      "quarantined",
		IsDuplicate:          0,
		DuplicateReason:      "",
		CanonicalCandidateID: sql.NullString{Valid: false},
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		t.Fatalf("falha ao criar candidato válido: %v", err)
	}
	if c1.ID != "cand-1" {
		t.Errorf("ID esperado 'cand-1', obtido %q", c1.ID)
	}

	// 2. Inserção de candidato duplicado apontando para o canônico
	c2, err := queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-2",
		MonitoringRunID:      "run-parent-1",
		Fingerprint:          "v1:abc123hash",
		FingerprintVersion:   1,
		EntityName:           "Daniel Vorcaro",
		NormalizedEntityName: "daniel vorcaro",
		Proposition:          "Aquisição de controle",
		SuggestedGrade:       "A",
		SourceUrl:            "https://noticias.com/1",
		CanonicalUrl:         "https://noticias.com/1",
		TechnicalConfidence:  0.95,
		EditorialStatus:      "quarantined",
		IsDuplicate:          1,
		DuplicateReason:      "same_run_duplicate",
		CanonicalCandidateID: sql.NullString{String: "cand-1", Valid: true},
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err != nil {
		t.Fatalf("falha ao criar candidato duplicado: %v", err)
	}
	if c2.CanonicalCandidateID.String != "cand-1" {
		t.Errorf("canonical_candidate_id esperado 'cand-1', obtido %v", c2.CanonicalCandidateID)
	}

	// 3. Violação de FK para run inexistente
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-orphan",
		MonitoringRunID:      "run-inexistente",
		Fingerprint:          "v1:hash",
		FingerprintVersion:   1,
		EntityName:           "Nome",
		NormalizedEntityName: "nome",
		Proposition:          "Prop",
		SuggestedGrade:       "B",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.8,
		EditorialStatus:      "quarantined",
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava erro de Foreign Key ao associar candidato a run inexistente")
	}

	// 4. Violação de CHECK de confiança técnica fora de [0.0, 1.0]
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-invalid-conf",
		MonitoringRunID:      "run-parent-1",
		Fingerprint:          "v1:hash",
		FingerprintVersion:   1,
		EntityName:           "Nome",
		NormalizedEntityName: "nome",
		Proposition:          "Prop",
		SuggestedGrade:       "B",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  1.5, // Inválido
		EditorialStatus:      "quarantined",
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava erro de CHECK para confiança técnica > 1.0")
	}

	// 5. Violação de CHECK de grau sugerido inválido
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-invalid-grade",
		MonitoringRunID:      "run-parent-1",
		Fingerprint:          "v1:hash",
		FingerprintVersion:   1,
		EntityName:           "Nome",
		NormalizedEntityName: "nome",
		Proposition:          "Prop",
		SuggestedGrade:       "Z", // Inválido
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.8,
		EditorialStatus:      "quarantined",
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava erro de CHECK para grau sugerido inválido")
	}

	// 6. Deleção de run com candidatos associados deve ser bloqueada por ON DELETE RESTRICT
	_, err = db.ExecContext(ctx, "DELETE FROM monitoring_runs WHERE id = 'run-parent-1'")
	if err == nil {
		t.Fatal("esperava erro de Foreign Key (RESTRICT) ao tentar deletar run com candidatos associados")
	}
}

func TestMonitoringCandidateDuplicateConsistencyChecks(t *testing.T) {
	db, ctx := setupTestDBForMonitoring(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Cria run pai
	_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                "run-p2",
		Status:            "running",
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    "openai/gpt-4.1-mini",
		CreatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar run pai: %v", err)
	}

	// 1. Canônico não pode ter canonical_candidate_id preenchido
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-bad-canon-1",
		MonitoringRunID:      "run-p2",
		Fingerprint:          "v1:fp1",
		FingerprintVersion:   1,
		EntityName:           "Daniel",
		NormalizedEntityName: "daniel",
		Proposition:          "Prop",
		SuggestedGrade:       "A",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.9,
		EditorialStatus:      "quarantined",
		IsDuplicate:          0,
		DuplicateReason:      "",
		CanonicalCandidateID: sql.NullString{String: "cand-other", Valid: true}, // Inválido para is_duplicate=0
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava violação de CHECK: is_duplicate=0 com canonical_candidate_id preenchido")
	}

	// 2. Canônico não pode ter duplicate_reason preenchido
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-bad-canon-2",
		MonitoringRunID:      "run-p2",
		Fingerprint:          "v1:fp2",
		FingerprintVersion:   1,
		EntityName:           "Daniel",
		NormalizedEntityName: "daniel",
		Proposition:          "Prop",
		SuggestedGrade:       "A",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.9,
		EditorialStatus:      "quarantined",
		IsDuplicate:          0,
		DuplicateReason:      "same_run_duplicate", // Inválido para is_duplicate=0
		CanonicalCandidateID: sql.NullString{Valid: false},
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava violação de CHECK: is_duplicate=0 com duplicate_reason preenchido")
	}

	// 3. Duplicado deve ter canonical_candidate_id não nulo
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-bad-dup-1",
		MonitoringRunID:      "run-p2",
		Fingerprint:          "v1:fp3",
		FingerprintVersion:   1,
		EntityName:           "Daniel",
		NormalizedEntityName: "daniel",
		Proposition:          "Prop",
		SuggestedGrade:       "A",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.9,
		EditorialStatus:      "quarantined",
		IsDuplicate:          1,
		DuplicateReason:      "same_run_duplicate",
		CanonicalCandidateID: sql.NullString{Valid: false}, // Inválido para is_duplicate=1
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava violação de CHECK: is_duplicate=1 com canonical_candidate_id nulo")
	}

	// 4. Duplicado deve ter duplicate_reason permitido
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-bad-dup-2",
		MonitoringRunID:      "run-p2",
		Fingerprint:          "v1:fp4",
		FingerprintVersion:   1,
		EntityName:           "Daniel",
		NormalizedEntityName: "daniel",
		Proposition:          "Prop",
		SuggestedGrade:       "A",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.9,
		EditorialStatus:      "quarantined",
		IsDuplicate:          1,
		DuplicateReason:      "motivo_invalido", // Inválido
		CanonicalCandidateID: sql.NullString{String: "cand-1", Valid: true},
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava violação de CHECK: is_duplicate=1 com duplicate_reason não permitido")
	}

	// 5. Candidato não pode apontar para si mesmo (autorreferência)
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-self",
		MonitoringRunID:      "run-p2",
		Fingerprint:          "v1:fp5",
		FingerprintVersion:   1,
		EntityName:           "Daniel",
		NormalizedEntityName: "daniel",
		Proposition:          "Prop",
		SuggestedGrade:       "A",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.9,
		EditorialStatus:      "quarantined",
		IsDuplicate:          1,
		DuplicateReason:      "same_run_duplicate",
		CanonicalCandidateID: sql.NullString{String: "cand-self", Valid: true}, // Autorreferência
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava violação de CHECK: candidato apontando para si mesmo (autorreferência)")
	}

	// 6. Versão do fingerprint deve ser 1
	_, err = queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
		ID:                   "cand-bad-version",
		MonitoringRunID:      "run-p2",
		Fingerprint:          "v2:fp6",
		FingerprintVersion:   2, // Inválido no schema v1
		EntityName:           "Daniel",
		NormalizedEntityName: "daniel",
		Proposition:          "Prop",
		SuggestedGrade:       "A",
		SourceUrl:            "https://ex.com",
		CanonicalUrl:         "https://ex.com",
		TechnicalConfidence:  0.9,
		EditorialStatus:      "quarantined",
		IsDuplicate:          0,
		DuplicateReason:      "",
		CanonicalCandidateID: sql.NullString{Valid: false},
		CreatedAt:            now,
		UpdatedAt:            now,
	})
	if err == nil {
		t.Fatal("esperava violação de CHECK: fingerprint_version != 1")
	}
}

func TestCandidatesZeroImpactOnPublicBoundaryAndMetrics(t *testing.T) {
	db, ctx := setupTestDBForMonitoring(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Cria um run e múltiplos candidatos
	_, err := queries.CreateMonitoringRun(ctx, sqlc.CreateMonitoringRunParams{
		ID:                "run-boundary-test",
		Status:            "completed",
		DiscoveryProvider: "openrouter",
		DiscoveryModel:    "openai/gpt-4.1-mini",
		CreatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar run: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_, err := queries.CreateMonitoringCandidate(ctx, sqlc.CreateMonitoringCandidateParams{
			ID:                   "cand-boundary-" + string(rune('0'+i)),
			MonitoringRunID:      "run-boundary-test",
			Fingerprint:          "v1:fingerprint-" + string(rune('0'+i)),
			FingerprintVersion:   1,
			EntityName:           "Daniel Vorcaro",
			NormalizedEntityName: "daniel vorcaro",
			Proposition:          "Alegação em quarentena",
			SuggestedGrade:       "A",
			SourceUrl:            "https://noticias.com",
			CanonicalUrl:         "https://noticias.com",
			TechnicalConfidence:  0.99,
			EditorialStatus:      "quarantined",
			CreatedAt:            now,
			UpdatedAt:            now,
		})
		if err != nil {
			t.Fatalf("falha ao criar candidato: %v", err)
		}
	}

	// 1. Consulta public_claims_view
	var publicCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM public_claims_view").Scan(&publicCount); err != nil {
		t.Fatalf("falha ao consultar public_claims_view: %v", err)
	}
	if publicCount != 0 {
		t.Errorf("public_claims_view deveria ter 0 registros, obtido %d", publicCount)
	}

	// 2. Consulta queries públicas do store
	metrics, err := queries.GetPublicOverviewMetrics(ctx)
	if err != nil {
		t.Fatalf("falha ao consultar métricas públicas: %v", err)
	}
	if metrics.TotalEntities != 0 || metrics.TotalClaims != 0 || metrics.TotalSources != 0 {
		t.Errorf("métricas públicas devem ser 0: %+v", metrics)
	}
}
