package store_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestGetPublicDocumentDetail_DeterministicOrderingAndIsolation(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Cria caso e entidade
	c, err := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          "case-seq-1",
		Name:        "Operação Conexão",
		Slug:        "operacao-conexao",
		Description: "Recorte",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso: %v", err)
	}

	ent, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-seq-1",
		Type:               "person",
		Name:               "Alvo Investigado",
		NormalizedName:     "alvo investigado",
		Slug:               "alvo-investigado",
		Category:           "Empresas",
		RoleOrContext:      "Empresário",
		Reach:              "Nacional",
		Summary:            "Síntese",
		Relevance:          4,
		RelevanceRationale: "Investigado",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade: %v", err)
	}

	rel, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-seq-1",
		SubjectEntityID:  ent.ID,
		CaseID:           sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "Investigado",
		Summary:          "Citado em laudo pericial",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relação: %v", err)
	}

	// 2. Cria fonte primária oficial
	src, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                    "src-laudo-1",
		Title:                 "Laudo Pericial IPJ-A nº 3298613/2026",
		PublisherOrAuthor:     "Polícia Federal / INC",
		OriginalUrl:           "https://pf.gov.br/laudos/3298613",
		CanonicalUrl:          "https://pf.gov.br/laudos/3298613",
		PublishedAt:           sql.NullString{String: "2026-08-15T10:00:00Z", Valid: true},
		AccessedAt:            sql.NullString{String: now, Valid: true},
		SourceType:            "police_report",
		SourceAccessStatus:    "reachable",
		SourceAccessCheckedAt: sql.NullString{String: now, Valid: true},
		HttpStatus:            sql.NullInt64{Int64: 200, Valid: true},
		NormalizedErrorCode:   sql.NullString{},
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	if err != nil {
		t.Fatalf("falha ao criar fonte: %v", err)
	}

	// 3. Cria claims: 1 publicado e 1 quarentenado
	clPub, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-pub-1",
		RelationshipID:    rel.ID,
		Proposition:       "Constatação pericial na página 42",
		Origin:            "curated_seed",
		Grade:             "A",
		Disposition:       "supports_link",
		MetricEligible:    1,
		Status:            "published",
		ContextStatus:     "Concluído",
		QuarantineReasons: "",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim publicado: %v", err)
	}

	clQuar, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-quar-1",
		RelationshipID:    rel.ID,
		Proposition:       "Alegação sob quarentena na página 12",
		Origin:            "openrouter",
		Grade:             "D",
		Disposition:       "supports_link",
		MetricEligible:    0,
		Status:            "quarantined",
		ContextStatus:     "Em avaliação",
		QuarantineReasons: "grau_d",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim quarentenado: %v", err)
	}

	// 4. Cria evidências
	evPub, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-pub-1",
		ClaimID:      clPub.ID,
		Summary:      "Evidência publicada",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	evQuar, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-quar-1",
		ClaimID:      clQuar.ID,
		Summary:      "Evidência em quarentena",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	// 5. Cria múltiplos evidence_sources no claim publicado em ordem desordenada
	// Criamos com locators: "Pág. 42, Fig. 12", "Pág. 5", "Pág. 12", "Fl. 2"
	t1 := "2026-09-01T10:00:00Z"
	t2 := "2026-09-01T10:01:00Z"
	t3 := "2026-09-01T10:02:00Z"

	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-p42",
		EvidenceID: evPub.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho na página 42",
		Locator:    "pág. 42, fig. 12",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  t1,
		UpdatedAt:  t1,
	})

	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-p5",
		EvidenceID: evPub.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho na página 5",
		Locator:    "p. 5",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  t2,
		UpdatedAt:  t2,
	})

	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-p12",
		EvidenceID: evPub.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho na página 12",
		Locator:    "págs. 12-14",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  t3,
		UpdatedAt:  t3,
	})

	// EvidenceSource rejeitado no claim publicado (não deve aparecer na pública)
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-rejected",
		EvidenceID: evPub.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho desaprovado",
		Locator:    "Pág. 1",
		Role:       "supports",
		Status:     "rejected",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// EvidenceSource ativo no claim quarentenado (não deve aparecer na pública)
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-quar-active",
		EvidenceID: evQuar.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho do claim quarentenado",
		Locator:    "Pág. 2",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// Teste 1: Consulta pública deve retornar apenas trechos ativos de claims publicados, ordenados deterministicamente
	detail, err := store.GetPublicDocumentDetail(ctx, db, src.ID)
	if err != nil {
		t.Fatalf("GetPublicDocumentDetail falhou: %v", err)
	}

	if detail.ID != src.ID {
		t.Errorf("detail.ID = %q, esperado %q", detail.ID, src.ID)
	}
	if detail.SourceType != domain.SourceTypePoliceReport {
		t.Errorf("detail.SourceType = %q, esperado %q", detail.SourceType, domain.SourceTypePoliceReport)
	}
	if !detail.SourceType.IsPrimaryDocument() {
		t.Errorf("IsPrimaryDocument() deveria ser true para police_report")
	}

	// Deve conter exatamente os 3 trechos ativos do claim publicado
	if len(detail.Sequence) != 3 {
		t.Fatalf("len(detail.Sequence) = %d, esperado 3", len(detail.Sequence))
	}

	// Ordenação esperada: Pág. 5 -> Págs. 12–14 -> Pág. 42, Fig. 12
	expectedOrder := []string{"es-p5", "es-p12", "es-p42"}
	for i, expID := range expectedOrder {
		if detail.Sequence[i].ID != expID {
			t.Errorf("Sequence[%d].ID = %q, esperado %q (locator: %q)", i, detail.Sequence[i].ID, expID, detail.Sequence[i].Locator)
		}
	}

	// Teste 2: Fonte inexistente deve retornar ErrNotFound
	_, err = store.GetPublicDocumentDetail(ctx, db, "src-inexistente")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetPublicDocumentDetail(inexistente) retornou %v, esperado ErrNotFound", err)
	}

	// Teste 3: Fonte que só tem alegações em quarentena não é visível publicamente (retorna ErrNotFound)
	srcQuarOnly, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-quar-only",
		Title:              "Fonte Apenas em Quarentena",
		PublisherOrAuthor:  "Veículo",
		OriginalUrl:        "https://example.com/quar",
		CanonicalUrl:       "https://example.com/quar",
		SourceType:         "article",
		SourceAccessStatus: "not_checked",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-quar-only",
		EvidenceID: evQuar.ID,
		SourceID:   srcQuarOnly.ID,
		Excerpt:    "Trecho quarentenado",
		Locator:    "Pág. 10",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	_, err = store.GetPublicDocumentDetail(ctx, db, srcQuarOnly.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetPublicDocumentDetail(srcQuarOnly) retornou %v, esperado ErrNotFound", err)
	}

	// Teste 4: Consulta administrativa deve retornar TODOS os trechos (ativos, rejeitados e de claims em quarentena)
	adminDetail, err := store.GetAdminSourceDetail(ctx, db, src.ID)
	if err != nil {
		t.Fatalf("GetAdminSourceDetail falhou: %v", err)
	}

	if adminDetail.Source.ID != src.ID {
		t.Errorf("adminDetail.Source.ID = %q, esperado %q", adminDetail.Source.ID, src.ID)
	}
	// Total de usos no laudo: 3 ativos pub + 1 rejeitado pub + 1 ativo quar = 5
	if len(adminDetail.Sequence) != 5 {
		t.Fatalf("len(adminDetail.Sequence) = %d, esperado 5", len(adminDetail.Sequence))
	}
}
