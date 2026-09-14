package store_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/domain"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func TestGetPublicEditorialHistory_EntityAndSource(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Cria caso e entidade
	c, err := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          "case-hist-1",
		Name:        "Caso Transparência",
		Slug:        "caso-transparencia",
		Description: "Recorte",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso: %v", err)
	}

	ent, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-hist-1",
		Type:               "person",
		Name:               "Pessoa Histórico",
		NormalizedName:     "pessoa historico",
		Slug:               "pessoa-historico",
		Category:           "Política",
		RoleOrContext:      "Agente Público",
		Reach:              "Nacional",
		Summary:            "Síntese pública",
		Relevance:          3,
		RelevanceRationale: "Investigado em auditoria",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade: %v", err)
	}

	rel, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-hist-1",
		SubjectEntityID:  ent.ID,
		CaseID:           sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "Investigado",
		Summary:          "Relação auditada",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relação: %v", err)
	}

	// 2. Cria fonte documental
	src, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                    "src-hist-1",
		Title:                 "Diário Oficial nº 100",
		PublisherOrAuthor:     "Imprensa Oficial",
		OriginalUrl:           "https://example.gov.br/do/100",
		CanonicalUrl:          "https://example.gov.br/do/100",
		SourceType:            "official_statement",
		SourceAccessStatus:    "reachable",
		SourceAccessCheckedAt: sql.NullString{String: now, Valid: true},
		HttpStatus:            sql.NullInt64{Int64: 200, Valid: true},
		CreatedAt:             "2026-09-01T10:00:00Z",
		UpdatedAt:             "2026-09-01T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar fonte: %v", err)
	}

	// 3. Cria claims:
	// Claim 1: Publicado desde o seed (curated_seed)
	cl1, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-seed-1",
		RelationshipID:    rel.ID,
		Proposition:       "Alegação do seed publicada",
		Origin:            "curated_seed",
		Grade:             "A",
		Disposition:       "supports_link",
		MetricEligible:    1,
		Status:            "published",
		ContextStatus:     "Ativo",
		QuarantineReasons: "",
		CreatedAt:         "2026-09-01T10:05:00Z",
		UpdatedAt:         "2026-09-01T10:05:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar claim 1: %v", err)
	}

	// Claim 2: Aprovado via pós-moderação humana (quarantined -> published)
	cl2, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-mod-2",
		RelationshipID:    rel.ID,
		Proposition:       "Alegação aprovada por moderação",
		Origin:            "openrouter",
		Grade:             "B",
		Disposition:       "supports_link",
		MetricEligible:    1,
		Status:            "published",
		ContextStatus:     "Ativo",
		QuarantineReasons: "",
		CreatedAt:         "2026-09-02T11:00:00Z",
		UpdatedAt:         "2026-09-03T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar claim 2: %v", err)
	}

	// Claim 3: Item em quarentena pura rejeitado sem nunca ter sido público
	cl3, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-secret-quar-3",
		RelationshipID:    rel.ID,
		Proposition:       "Alegação sigilosa que falhou nos gates",
		Origin:            "openrouter",
		Grade:             "E",
		Disposition:       "supports_link",
		MetricEligible:    0,
		Status:            "rejected",
		ContextStatus:     "Rejeitado",
		QuarantineReasons: "grau_e_invalido",
		CreatedAt:         "2026-09-02T11:10:00Z",
		UpdatedAt:         "2026-09-02T11:20:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar claim 3: %v", err)
	}

	// Evidências e EvidenceSources
	ev1, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-1",
		ClaimID:      cl1.ID,
		Summary:      "Evidência 1",
		EvidenceType: "document",
		CreatedAt:    "2026-09-01T10:05:00Z",
		UpdatedAt:    "2026-09-01T10:05:00Z",
	})
	es1, _ := queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-1",
		EvidenceID: ev1.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho do diário oficial",
		Locator:    "Pág. 10",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  "2026-09-01T10:05:00Z",
		UpdatedAt:  "2026-09-01T10:05:00Z",
	})

	ev2, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-2",
		ClaimID:      cl2.ID,
		Summary:      "Evidência 2",
		EvidenceType: "document",
		CreatedAt:    "2026-09-02T11:00:00Z",
		UpdatedAt:    "2026-09-02T11:00:00Z",
	})
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-2",
		EvidenceID: ev2.ID,
		SourceID:   src.ID,
		Excerpt:    "Outro trecho do diário",
		Locator:    "Pág. 15",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  "2026-09-02T11:00:00Z",
		UpdatedAt:  "2026-09-02T11:00:00Z",
	})

	// 4. Inserir decisões de moderação em moderation_decisions:
	// Decisão A: Aprovação humana de cl2
	_, err = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-approve-cl2",
		ClaimID:              sql.NullString{String: cl2.ID, Valid: true},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "approve",
		Reason:               "Aprovado após checagem probatória com dados internos secretos e admin_username",
		Actor:                "operador_admin_secreto",
		CandidateFingerprint: "fingerprint_confidencial_sha256_123456",
		CreatedAt:            "2026-09-03T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar decisão de aprovação de claim: %v", err)
	}

	// Decisão B: Rejeição interna do cl3 (que nunca foi público)
	_, err = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-reject-cl3-private",
		ClaimID:              sql.NullString{String: cl3.ID, Valid: true},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "reject",
		Reason:               "Rejeição interna de candidato quarentenado",
		Actor:                "operador_admin_secreto",
		CandidateFingerprint: "fingerprint_confidencial_sha256_789012",
		CreatedAt:            "2026-09-02T11:20:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar decisão de rejeição de claim privado: %v", err)
	}

	// Decisão C: Rejeição de es1 (desativação de suporte)
	_, err = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-reject-es1",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: es1.ID, Valid: true},
		Action:               "reject",
		Reason:               "Trecho desaprovado por inadequação contextual",
		Actor:                "operador_admin_secreto",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-04T15:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar decisão de rejeição de evidence source: %v", err)
	}

	// Decisão D: Restauração de es1 (reativação de suporte)
	_, err = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-restore-es1",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: es1.ID, Valid: true},
		Action:               "restore",
		Reason:               "Trecho reativado após confirmação com novo laudo",
		Actor:                "operador_admin_secreto",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-05T09:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar decisão de restauração de evidence source: %v", err)
	}

	// 5. Teste do Histórico Editorial da Entidade
	detail, err := store.GetPublicEntityDetail(ctx, db, ent.Slug)
	if err != nil {
		t.Fatalf("GetPublicEntityDetail falhou: %v", err)
	}

	history := detail.EditorialHistory
	if len(history) == 0 {
		t.Fatalf("Histórico editorial da entidade está vazio")
	}

	// Validar que o claim 3 (privado e rejeitado em quarentena) NÃO aparece no histórico público
	for _, ev := range history {
		if ev.ClaimID == cl3.ID || ev.ID == "dec-reject-cl3-private" {
			t.Errorf("Decisão privada de candidato em quarentena (%s) NÃO deveria aparecer no histórico público", ev.ID)
		}
		// Validar redação estrita: nada de credenciais, motivos brutos ou fingerprints
		if strings.Contains(ev.Summary, "operador_admin_secreto") || strings.Contains(ev.Summary, "dados internos secretos") {
			t.Errorf("Vazamento de dados administrativos no resumo do evento: %s", ev.Summary)
		}
	}

	// Validar ordenação cronológica estrita decrescente (mais recente primeiro)
	for i := 1; i < len(history); i++ {
		if history[i-1].CreatedAt < history[i].CreatedAt {
			t.Errorf("Histórico fora de ordem decrescente: pos[%d]=%s > pos[%d]=%s", i-1, history[i-1].CreatedAt, i, history[i].CreatedAt)
		}
	}

	// 6. Teste do Histórico Editorial do Documento
	docDetail, err := store.GetPublicDocumentDetail(ctx, db, src.ID)
	if err != nil {
		t.Fatalf("GetPublicDocumentDetail falhou: %v", err)
	}

	docHistory := docDetail.EditorialHistory
	if len(docHistory) == 0 {
		t.Fatalf("Histórico editorial do documento está vazio")
	}

	// O histórico do documento deve conter as deliberações sobre es1 (restauração e rejeição)
	foundRestore := false
	foundReject := false
	for _, ev := range docHistory {
		if ev.ID == "dec-restore-es1" {
			foundRestore = true
			if ev.Action != domain.PublicEditorialActionRestore {
				t.Errorf("Ação esperada 'restore', obtida: %s", ev.Action)
			}
			if ev.ActionLabel != "Reativação de Trecho" {
				t.Errorf("ActionLabel esperado 'Reativação de Trecho', obtido: %s", ev.ActionLabel)
			}
		}
		if ev.ID == "dec-reject-es1" {
			foundReject = true
			if ev.Action != domain.PublicEditorialActionReject {
				t.Errorf("Ação esperada 'reject', obtida: %s", ev.Action)
			}
			if ev.ActionLabel != "Desativação de Trecho" {
				t.Errorf("ActionLabel esperado 'Desativação de Trecho', obtido: %s", ev.ActionLabel)
			}
		}
	}

	if !foundRestore {
		t.Errorf("dec-restore-es1 não foi encontrado no histórico do documento")
	}
	if !foundReject {
		t.Errorf("dec-reject-es1 não foi encontrado no histórico do documento")
	}

	// Teste de Limite de Eventos
	limitedHist, err := store.GetPublicEditorialHistoryForEntity(ctx, db, ent.ID, detail.Claims, 2)
	if err != nil {
		t.Fatalf("GetPublicEditorialHistoryForEntity com limit falhou: %v", err)
	}
	if len(limitedHist) > 2 {
		t.Errorf("len(limitedHist) = %d, esperado no máximo 2", len(limitedHist))
	}
}

func TestGetPublicEditorialHistory_PrivateQuarantinedExclusion(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Cria caso e entidade
	_, err := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          "case-priv-1",
		Name:        "Caso Privado",
		Slug:        "caso-privado",
		Description: "Recorte",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso: %v", err)
	}

	ent, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-priv-1",
		Type:               "person",
		Name:               "Pessoa Quarentenada",
		NormalizedName:     "pessoa quarentenada",
		Slug:               "pessoa-quarentenada",
		Category:           "Empresas",
		RoleOrContext:      "Investigado",
		Reach:              "Nacional",
		Summary:            "Resumo",
		Relevance:          3,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade: %v", err)
	}

	rel, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-priv-1",
		SubjectEntityID:  ent.ID,
		CaseID:           sql.NullString{String: "case-priv-1", Valid: true},
		RelationshipType: "Investigado",
		Summary:          "Relação",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relação: %v", err)
	}

	src, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                    "src-priv-1",
		Title:                 "Documento Quarentenado",
		PublisherOrAuthor:     "Cartório Privado",
		OriginalUrl:           "https://example.com/priv/doc",
		CanonicalUrl:          "https://example.com/priv/doc",
		SourceType:            "court_document",
		SourceAccessStatus:    "reachable",
		SourceAccessCheckedAt: sql.NullString{String: now, Valid: true},
		HttpStatus:            sql.NullInt64{Int64: 200, Valid: true},
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	if err != nil {
		t.Fatalf("falha ao criar fonte: %v", err)
	}

	// 2. Claim A: curated_seed importado em quarentena (nunca publicado)
	clA, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-seed-quar-a",
		RelationshipID:    rel.ID,
		Proposition:       "Alegação do seed que entrou em quarentena por grau ambíguo",
		Origin:            "curated_seed",
		Grade:             "E",
		Disposition:       "context_only",
		MetricEligible:    0,
		Status:            "quarantined",
		ContextStatus:     "Quarentena",
		QuarantineReasons: `["mapped_initial_state"]`,
		CreatedAt:         "2026-09-01T10:00:00Z",
		UpdatedAt:         "2026-09-01T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar claim A: %v", err)
	}

	evA, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-priv-a",
		ClaimID:      clA.ID,
		Summary:      "Evidência A",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	esA, _ := queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-priv-a",
		EvidenceID: evA.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho privado",
		Locator:    "Pág. 999_SECRETA",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// Decisão 1: Rejeição de claim privado do curated_seed (nunca foi público)
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-priv-claim-reject",
		ClaimID:              sql.NullString{String: clA.ID, Valid: true},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "reject",
		Reason:               "Rejeição em quarentena do seed",
		Actor:                "operador_secreto",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-02T10:00:00Z",
	})

	// Decisão 2: Rejeição de evidence_source privado (pertencente a claim em quarentena)
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-priv-es-reject",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: esA.ID, Valid: true},
		Action:               "reject",
		Reason:               "Rejeição de trecho privado",
		Actor:                "operador_secreto",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-03T10:00:00Z",
	})

	// Decisão 3: Restauração de evidence_source privado (pertencente a claim em quarentena)
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-priv-es-restore",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: esA.ID, Valid: true},
		Action:               "restore",
		Reason:               "Restauração de trecho privado",
		Actor:                "operador_secreto",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-04T10:00:00Z",
	})

	// Decisão 4: Restauração de claim privado (pertencente a claim em quarentena)
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-priv-claim-restore",
		ClaimID:              sql.NullString{String: clA.ID, Valid: true},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "restore",
		Reason:               "Restauração de claim para quarentena",
		Actor:                "operador_secreto",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-05T10:00:00Z",
	})

	// 3. Consulta histórico para a Entidade
	history, err := store.GetPublicEditorialHistoryForEntity(ctx, db, ent.ID, nil, 50)
	if err != nil {
		t.Fatalf("GetPublicEditorialHistoryForEntity falhou: %v", err)
	}

	if len(history) != 0 {
		t.Fatalf("Esperado 0 eventos públicos para alegações/evidências nunca publicadas, obtido: %d (%+v)", len(history), history)
	}

	// 4. Consulta histórico para a Fonte/Documento
	docHistory, err := store.GetPublicEditorialHistoryForSource(ctx, db, src.ID, nil, 50)
	if err != nil {
		t.Fatalf("GetPublicEditorialHistoryForSource falhou: %v", err)
	}

	if len(docHistory) != 0 {
		t.Fatalf("Esperado 0 eventos públicos para documento sem alegações públicas, obtido: %d (%+v)", len(docHistory), docHistory)
	}
}

func TestGetPublicEditorialHistory_QuarantineWithEmptyReasons_NeverPublic(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// 1. Caso, entidade e fonte
	_, err := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          "case-empty-reasons-1",
		Name:        "Caso Motivos Vazios",
		Slug:        "caso-motivos-vazios",
		Description: "Recorte",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso: %v", err)
	}

	ent, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-empty-reasons-1",
		Type:               "person",
		Name:               "Pessoa Quarentena Motivos Vazios",
		NormalizedName:     "pessoa quarentena motivos vazios",
		Slug:               "pessoa-quarentena-motivos-vazios",
		Category:           "Política",
		RoleOrContext:      "Investigado",
		Reach:              "Nacional",
		Summary:            "Resumo",
		Relevance:          2,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade: %v", err)
	}

	rel, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-empty-reasons-1",
		SubjectEntityID:  ent.ID,
		CaseID:           sql.NullString{String: "case-empty-reasons-1", Valid: true},
		RelationshipType: "Investigado",
		Summary:          "Relação",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relação: %v", err)
	}

	src, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                    "src-empty-reasons-1",
		Title:                 "Documento de Quarentena Sem Motivos",
		PublisherOrAuthor:     "Cartório",
		OriginalUrl:           "https://example.com/empty/doc",
		CanonicalUrl:          "https://example.com/empty/doc",
		SourceType:            "official_statement",
		SourceAccessStatus:    "reachable",
		SourceAccessCheckedAt: sql.NullString{String: now, Valid: true},
		HttpStatus:            sql.NullInt64{Int64: 200, Valid: true},
		CreatedAt:             now,
		UpdatedAt:             now,
	})
	if err != nil {
		t.Fatalf("falha ao criar fonte: %v", err)
	}

	// 2. Claim criado em quarentena com quarantine_reasons vazio ("" e "[]")
	clEmpty, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "claim-empty-reasons-1",
		RelationshipID:    rel.ID,
		Proposition:       "Alegação em quarentena criada sem motivos preenchidos",
		Origin:            "admin",
		Grade:             "C",
		Disposition:       "possible_link",
		MetricEligible:    0,
		Status:            "quarantined",
		ContextStatus:     "Quarentena",
		QuarantineReasons: "", // Motivos intencionalmente vazios
		CreatedAt:         "2026-09-01T10:00:00Z",
		UpdatedAt:         "2026-09-01T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("falha ao criar claim com motivos vazios: %v", err)
	}

	evEmpty, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-empty-reasons-1",
		ClaimID:      clEmpty.ID,
		Summary:      "Evidência em quarentena",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	esEmpty, _ := queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-empty-reasons-1",
		EvidenceID: evEmpty.ID,
		SourceID:   src.ID,
		Excerpt:    "Trecho em quarentena",
		Locator:    "Pág. 1234_LOCATOR_VAZIO",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// Decisão 1: Rejeição do claim em quarentena
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-empty-claim-rej",
		ClaimID:              sql.NullString{String: clEmpty.ID, Valid: true},
		EvidenceSourceID:     sql.NullString{Valid: false},
		Action:               "reject",
		Reason:               "Rejeição de claim em quarentena com motivos vazios",
		Actor:                "operador_admin",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-02T10:00:00Z",
	})

	// Decisão 2: Rejeição de evidence_source de claim em quarentena
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-empty-es-rej",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: esEmpty.ID, Valid: true},
		Action:               "reject",
		Reason:               "Rejeição de trecho de claim em quarentena com motivos vazios",
		Actor:                "operador_admin",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-03T10:00:00Z",
	})

	// Decisão 3: Restauração de evidence_source de claim em quarentena
	_, _ = queries.CreateModerationDecision(ctx, sqlc.CreateModerationDecisionParams{
		ID:                   "dec-empty-es-rst",
		ClaimID:              sql.NullString{Valid: false},
		EvidenceSourceID:     sql.NullString{String: esEmpty.ID, Valid: true},
		Action:               "restore",
		Reason:               "Restauração de trecho de claim em quarentena com motivos vazios",
		Actor:                "operador_admin",
		CandidateFingerprint: "",
		CreatedAt:            "2026-09-04T10:00:00Z",
	})

	// 3. Consulta histórico para a Entidade: deve ser rigorosamente 0 (fail-closed)
	history, err := store.GetPublicEditorialHistoryForEntity(ctx, db, ent.ID, nil, 50)
	if err != nil {
		t.Fatalf("GetPublicEditorialHistoryForEntity falhou: %v", err)
	}

	if len(history) != 0 {
		t.Fatalf("VIOLAÇÃO FAIL-CLOSED: esperado 0 eventos públicos para claim em quarentena com motivos vazios, obtido: %d (%+v)", len(history), history)
	}

	// 4. Consulta histórico para o Documento: deve ser rigorosamente 0 (fail-closed)
	docHistory, err := store.GetPublicEditorialHistoryForSource(ctx, db, src.ID, nil, 50)
	if err != nil {
		t.Fatalf("GetPublicEditorialHistoryForSource falhou: %v", err)
	}

	if len(docHistory) != 0 {
		t.Fatalf("VIOLAÇÃO FAIL-CLOSED: esperado 0 eventos públicos para documento com claim em quarentena com motivos vazios, obtido: %d (%+v)", len(docHistory), docHistory)
	}
}
