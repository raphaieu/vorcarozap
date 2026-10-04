package store_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/raphaieu/vorcarozap/internal/store/sqlc"
)

func createPublicTestFixtures(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Caso raiz
	c, err := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          "case-root",
		Name:        "Caso Vorcaro",
		Slug:        "caso-vorcaro",
		Description: "Recorte",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso: %v", err)
	}

	// 1. Entidade 1: Daniel Vorcaro (publicada, Grau A, Empresas, Relevância 4)
	e1, _ := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-1",
		Type:               "person",
		Name:               "Daniel Vorcaro",
		NormalizedName:     "daniel vorcaro",
		Slug:               "daniel-vorcaro",
		Category:           "Empresas",
		RoleOrContext:      "Controlador do Banco Master",
		Reach:              "Nacional",
		Summary:            "Síntese",
		Relevance:          4,
		RelevanceRationale: "Investigado",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = queries.CreateEntityAlias(ctx, sqlc.CreateEntityAliasParams{
		ID:              "alias-1",
		EntityID:        e1.ID,
		Alias:           "Dani Vorcaro",
		NormalizedAlias: "dani vorcaro",
		CreatedAt:       now,
	})
	r1, _ := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-1",
		SubjectEntityID:  e1.ID,
		CaseID:           sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "Controlador",
		Summary:          "Controla o Banco Master",
		ContextLimits:    "Nenhum",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	cl1, _ := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "cl-1",
		RelationshipID: r1.ID,
		Proposition:    "Controla o Banco Master e instituições coligadas",
		Attribution:    "",
		Origin:         "curated_seed",
		Grade:          "A",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		ContextStatus:  "Investigado",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	ev1, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-1",
		ClaimID:      cl1.ID,
		Summary:      "Registro societário na Junta Comercial",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	src1, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-1",
		Title:              "Reportagem Exclusiva",
		PublisherOrAuthor:  "Jornal Nacional de Negócios",
		OriginalUrl:        "https://noticias.example.com/vorcaro-master",
		CanonicalUrl:       "https://noticias.example.com/vorcaro-master",
		SourceType:         "article",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-1",
		EvidenceID: ev1.ID,
		SourceID:   src1.ID,
		Excerpt:    "Vorcaro é o titular do controle acionário",
		Locator:    "página 3",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// 2. Entidade 2: Nelson Tanure (publicada, Grau B, Finanças, Relevância 5)
	e2, _ := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-2",
		Type:               "person",
		Name:               "Nelson Tanure",
		NormalizedName:     "nelson tanure",
		Slug:               "nelson-tanure",
		Category:           "Finanças",
		RoleOrContext:      "Investidor",
		Reach:              "Internacional",
		Summary:            "",
		Relevance:          5,
		RelevanceRationale: "Investidor de grande porte",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	r2, _ := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-2",
		SubjectEntityID:  e2.ID,
		CaseID:           sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "Investidor associado",
		Summary:          "Operação de reestruturação conjunta",
		ContextLimits:    "Nega irregularidades",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	cl2, _ := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "cl-2",
		RelationshipID: r2.ID,
		Proposition:    "Participou de tratativas de consolidação",
		Attribution:    "",
		Origin:         "curated_seed",
		Grade:          "B",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		ContextStatus:  "Citado",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	ev2, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-2",
		ClaimID:      cl2.ID,
		Summary:      "Evidência de reportagem",
		EvidenceType: "article",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	src2, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-2",
		Title:              "Artigo Financeiro",
		PublisherOrAuthor:  "Valor Setorial",
		OriginalUrl:        "https://noticias.example.com/tanure",
		CanonicalUrl:       "https://noticias.example.com/tanure",
		SourceType:         "article",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	// Fonte de suporte
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-2-sup",
		EvidenceID: ev2.ID,
		SourceID:   src2.ID,
		Excerpt:    "Tanure reuniu-se com sócios",
		Locator:    "parágrafo 5",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	// Fonte de contraditório / contestação
	src2Contr, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-2-contr",
		Title:              "Nota da Assessoria de Tanure",
		PublisherOrAuthor:  "Assessoria Oficial",
		OriginalUrl:        "https://noticias.example.com/tanure-defesa",
		CanonicalUrl:       "https://noticias.example.com/tanure-defesa",
		SourceType:         "press_release",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-2-contr",
		EvidenceID: ev2.ID,
		SourceID:   src2Contr.ID,
		Excerpt:    "", // excerpt vazio
		Locator:    "",
		Role:       "contradicts",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	// 3. Entidade 3: Pessoa em Quarentena (NÃO deve aparecer publicamente)
	e3, _ := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-3",
		Type:               "person",
		Name:               "Pessoa Quarentenada",
		NormalizedName:     "pessoa quarentenada",
		Slug:               "pessoa-quarentenada",
		Category:           "Empresas",
		RoleOrContext:      "Contato",
		Reach:              "Local",
		Summary:            "",
		Relevance:          1,
		RelevanceRationale: "Pista inicial",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	r3, _ := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-3",
		SubjectEntityID:  e3.ID,
		CaseID:           sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "Pista",
		Summary:          "Pista",
		ContextLimits:    "",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	_, _ = queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "cl-3",
		RelationshipID:    r3.ID,
		Proposition:       "Alegação em quarentena",
		Attribution:       "",
		Origin:            "curated_seed",
		Grade:             "E",
		Disposition:       "possible_link",
		MetricEligible:    0,
		Status:            "quarantined",
		ContextStatus:     "Mencionado",
		QuarantineReasons: `["UNKNOWN_LEGACY_GRADE"]`,
		CreatedAt:         now,
		UpdatedAt:         now,
	})

	// 4. Entidade 4: Claim publicado porém SEM suporte ativo (evidence_source rejeitado)
	e4, _ := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-4",
		Type:               "person",
		Name:               "Pessoa Sem Suporte Ativo",
		NormalizedName:     "pessoa sem suporte ativo",
		Slug:               "pessoa-sem-suporte-ativo",
		Category:           "Jurídico",
		RoleOrContext:      "Advogado",
		Reach:              "Local",
		Summary:            "",
		Relevance:          2,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	r4, _ := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-4",
		SubjectEntityID:  e4.ID,
		CaseID:           sql.NullString{String: c.ID, Valid: true},
		RelationshipType: "Atuação",
		Summary:          "Representação",
		ContextLimits:    "",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	cl4, _ := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "cl-4",
		RelationshipID: r4.ID,
		Proposition:    "Alegação com suporte rejeitado",
		Attribution:    "",
		Origin:         "curated_seed",
		Grade:          "C",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		ContextStatus:  "Ativo",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	ev4, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-4",
		ClaimID:      cl4.ID,
		Summary:      "Evidência",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	// evidence_source rejeitado
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-4",
		EvidenceID: ev4.ID,
		SourceID:   src1.ID,
		Excerpt:    "Trecho",
		Locator:    "",
		Role:       "supports",
		Status:     "rejected",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
}

func TestPublic_VisibilityRules(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)

	res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{})
	if err != nil {
		t.Fatalf("falha ao listar entidades públicas: %v", err)
	}

	// Apenas e1 e e2 devem aparecer (e3 em quarentena e e4 sem suporte ativo são excluídos)
	if res.TotalCount != 2 {
		t.Fatalf("esperava exatamente 2 entidades públicas ativas, obteve %d", res.TotalCount)
	}

	slugs := map[string]bool{}
	for _, ent := range res.Entities {
		slugs[ent.Slug] = true
	}

	if !slugs["daniel-vorcaro"] {
		t.Error("esperava daniel-vorcaro na listagem pública")
	}
	if !slugs["nelson-tanure"] {
		t.Error("esperava nelson-tanure na listagem pública")
	}
	if slugs["pessoa-quarentenada"] {
		t.Error("pessoa-quarentenada NÃO deve aparecer publicamente")
	}
	if slugs["pessoa-sem-suporte-ativo"] {
		t.Error("pessoa-sem-suporte-ativo NÃO deve aparecer publicamente sem evidence_source ativo com papel supports")
	}
}

func TestPublic_Search(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)

	t.Run("Busca por nome", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "Daniel"})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "daniel-vorcaro" {
			t.Errorf("falha ao buscar por nome: %+v, err=%v", res, err)
		}
	})

	t.Run("Busca por alias", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "Dani Vorcaro"})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "daniel-vorcaro" {
			t.Errorf("falha ao buscar por alias: %+v, err=%v", res, err)
		}
	})

	t.Run("Busca por fonte ou autor", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "Valor Setorial"})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "nelson-tanure" {
			t.Errorf("falha ao buscar por fonte: %+v, err=%v", res, err)
		}
	})

	t.Run("Busca sem resultado", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "Termo Inexistente XYZ"})
		if err != nil || res.TotalCount != 0 {
			t.Errorf("esperava 0 resultados para termo inexistente, obteve %d", res.TotalCount)
		}
	})
}

func TestPublic_Filters(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)

	t.Run("Filtro por categoria", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Category: "Empresas"})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "daniel-vorcaro" {
			t.Errorf("filtro por categoria 'Empresas' falhou: %+v, err=%v", res, err)
		}
	})

	t.Run("Filtro por grau", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Grade: "B"})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "nelson-tanure" {
			t.Errorf("filtro por grau 'B' falhou: %+v, err=%v", res, err)
		}
	})

	t.Run("Filtro por relevância", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Relevance: 5})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "nelson-tanure" {
			t.Errorf("filtro por relevância 5 falhou: %+v, err=%v", res, err)
		}
	})
}

func TestPublic_OrderingAndPagination(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)

	t.Run("Ordenação por relevância desc", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{
			OrderBy:  "relevance",
			OrderDir: "desc",
		})
		if err != nil || len(res.Entities) != 2 {
			t.Fatalf("falha ao ordenar por relevância: %v", err)
		}
		// Tanure (5) deve vir antes de Vorcaro (4)
		if res.Entities[0].Slug != "nelson-tanure" || res.Entities[1].Slug != "daniel-vorcaro" {
			t.Errorf("ordenação por relevância desc incorreta: primeiro=%s, segundo=%s",
				res.Entities[0].Slug, res.Entities[1].Slug)
		}
	})

	t.Run("Ordenação inválida cai no default seguro", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{
			OrderBy:  "invalido; drop table",
			OrderDir: "invalido",
		})
		if err != nil || len(res.Entities) != 2 {
			t.Fatalf("falha com ordenação inválida: %v", err)
		}
		// Default é name asc -> Daniel Vorcaro antes de Nelson Tanure
		if res.Entities[0].Slug != "daniel-vorcaro" {
			t.Errorf("ordenação default deveria ser name asc, primeiro foi %s", res.Entities[0].Slug)
		}
	})

	t.Run("Paginação", func(t *testing.T) {
		resPage1, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{
			Page:     1,
			PageSize: 1,
		})
		if err != nil || len(resPage1.Entities) != 1 || resPage1.TotalPages != 2 {
			t.Fatalf("falha na página 1: %+v", resPage1)
		}

		resPage2, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{
			Page:     2,
			PageSize: 1,
		})
		if err != nil || len(resPage2.Entities) != 1 {
			t.Fatalf("falha na página 2: %+v", resPage2)
		}

		if resPage1.Entities[0].Slug == resPage2.Entities[0].Slug {
			t.Errorf("páginas 1 e 2 retornaram a mesma entidade: %s", resPage1.Entities[0].Slug)
		}
	})
}

func TestPublic_EntityDetail(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)

	t.Run("Slug existente e público", func(t *testing.T) {
		detail, err := store.GetPublicEntityDetail(ctx, db, "nelson-tanure")
		if err != nil {
			t.Fatalf("falha ao carregar detalhe de nelson-tanure: %v", err)
		}

		if detail.Entity.Name != "Nelson Tanure" {
			t.Errorf("nome da entidade: esperado 'Nelson Tanure', obteve %q", detail.Entity.Name)
		}
		if len(detail.Claims) != 1 {
			t.Fatalf("esperava 1 claim público para nelson-tanure, obteve %d", len(detail.Claims))
		}

		cl := detail.Claims[0]
		if len(cl.Sources) != 2 {
			t.Fatalf("esperava 2 fontes (1 supports + 1 contradicts), obteve %d", len(cl.Sources))
		}

		// A primeira fonte deve ser supports (ordenação por papel)
		if cl.Sources[0].Role != "supports" {
			t.Errorf("primeira fonte deveria ter role=supports, obteve %s", cl.Sources[0].Role)
		}
		// A segunda fonte deve ser contradicts
		if cl.Sources[1].Role != "contradicts" {
			t.Errorf("segunda fonte deveria ter role=contradicts, obteve %s", cl.Sources[1].Role)
		}
	})

	t.Run("Slug inexistente retorna ErrNotFound (404)", func(t *testing.T) {
		_, err := store.GetPublicEntityDetail(ctx, db, "slug-que-nao-existe")
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("esperava ErrNotFound, obteve %v", err)
		}
	})

	t.Run("Slug de entidade quarentenada retorna ErrNotFound (404)", func(t *testing.T) {
		_, err := store.GetPublicEntityDetail(ctx, db, "pessoa-quarentenada")
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("entidade quarentenada não deve ser acessível publicamente, esperava ErrNotFound, obteve %v", err)
		}
	})

	t.Run("Slug de entidade sem suporte ativo retorna ErrNotFound (404)", func(t *testing.T) {
		_, err := store.GetPublicEntityDetail(ctx, db, "pessoa-sem-suporte-ativo")
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("entidade sem suporte ativo não deve ser acessível publicamente, esperava ErrNotFound, obteve %v", err)
		}
	})
}

func TestPublic_LateralLeakagePrevention(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Entidade 1 (Daniel Vorcaro) possui um claim público legítimo.
	// Adicionamos para a mesma entidade um relationship/claim em quarentena
	// e outro claim publicado mas com evidence_source rejeitado, contendo termos exclusivos.
	rHidden, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-hidden-1",
		SubjectEntityID:  "ent-1",
		CaseID:           sql.NullString{String: "case-root", Valid: true},
		RelationshipType: "VínculoOcultoExclusivo",
		Summary:          "ResumoOcultoExclusivo",
		ContextLimits:    "",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relação oculta: %v", err)
	}

	clQuarantine, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "cl-hidden-quarantine",
		RelationshipID:    rHidden.ID,
		Proposition:       "ProposicaoOcultaQuarentenadaExclusiva",
		Attribution:       "",
		Origin:            "curated_seed",
		Grade:             "E",
		Disposition:       "possible_link",
		MetricEligible:    0,
		Status:            "quarantined",
		ContextStatus:     "",
		QuarantineReasons: `["SUSPICIOUS"]`,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim em quarentena: %v", err)
	}

	evHidden, err := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-hidden-1",
		ClaimID:      clQuarantine.ID,
		Summary:      "Evidência Oculta",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("falha ao criar evidência oculta: %v", err)
	}

	srcHidden, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-hidden-1",
		Title:              "FonteSecretaOculta",
		PublisherOrAuthor:  "VeiculoOcultoExclusivo",
		OriginalUrl:        "https://noticias.example.com/oculto-exclusivo",
		CanonicalUrl:       "https://noticias.example.com/oculto-exclusivo",
		SourceType:         "article",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar fonte oculta: %v", err)
	}

	_, err = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-hidden-1",
		EvidenceID: evHidden.ID,
		SourceID:   srcHidden.ID,
		Excerpt:    "Trecho oculto",
		Locator:    "",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("falha ao vincular fonte oculta: %v", err)
	}

	// Cenários de busca que NÃO devem vazar a entidade ent-1
	testCases := []struct {
		name  string
		query string
	}{
		{"Busca por tipo de vínculo oculto", "VínculoOcultoExclusivo"},
		{"Busca por resumo de vínculo oculto", "ResumoOcultoExclusivo"},
		{"Busca por proposição em quarentena", "ProposicaoOcultaQuarentenadaExclusiva"},
		{"Busca por autor/veículo de fonte ligada a claim oculto", "VeiculoOcultoExclusivo"},
		{"Busca por URL de fonte ligada exclusivamente a claim oculto", "oculto-exclusivo"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: tc.query})
			if err != nil {
				t.Fatalf("erro ao executar busca: %v", err)
			}
			if res.TotalCount != 0 {
				t.Errorf("vazamento lateral detectado para query %q: obteve %d resultados (esperava 0)",
					tc.query, res.TotalCount)
			}
		})
	}

	// Busca pelo termo legítimo continua encontrando Daniel Vorcaro
	t.Run("Busca por termo legítimo continua funcionando", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "Banco Master"})
		if err != nil || res.TotalCount != 1 || res.Entities[0].Slug != "daniel-vorcaro" {
			t.Errorf("esperava encontrar daniel-vorcaro para 'Banco Master', obteve %+v, err=%v", res, err)
		}
	})
}

func TestPublic_SearchEscaping(t *testing.T) {
	db, ctx := setupTestDB(t)
	createPublicTestFixtures(t, db, ctx)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Inserir entidade com caracteres especiais literais: %, _, \
	eSpecial, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-special",
		Type:               "person",
		Name:               "Pessoa 100%_Livre\\Teste",
		NormalizedName:     "pessoa 100%_livre\\teste",
		Slug:               "pessoa-100-livre-teste",
		Category:           "Especial",
		RoleOrContext:      "Auditor",
		Reach:              "Nacional",
		Summary:            "Síntese especial",
		Relevance:          3,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade com caracteres especiais: %v", err)
	}

	rSpecial, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-special",
		SubjectEntityID:  eSpecial.ID,
		CaseID:           sql.NullString{String: "case-root", Valid: true},
		RelationshipType: "Vínculo 50%_Confirmado\\",
		Summary:          "Resumo com 10%_taxa",
		ContextLimits:    "",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar relação especial: %v", err)
	}

	clSpecial, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "cl-special",
		RelationshipID: rSpecial.ID,
		Proposition:    "Proposição com 100%_certeza\\fato",
		Attribution:    "",
		Origin:         "curated_seed",
		Grade:          "A",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		ContextStatus:  "Ativo",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim especial: %v", err)
	}

	evSpecial, err := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{
		ID:           "ev-special",
		ClaimID:      clSpecial.ID,
		Summary:      "Evidência especial",
		EvidenceType: "document",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("falha ao criar evidência especial: %v", err)
	}

	srcSpecial, err := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-special",
		Title:              "Fonte Especial 100%",
		PublisherOrAuthor:  "Jornal 100%_Fatos",
		OriginalUrl:        "https://noticias.example.com/100%_fatos",
		CanonicalUrl:       "https://noticias.example.com/100%_fatos",
		SourceType:         "article",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar fonte especial: %v", err)
	}

	_, err = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{
		ID:         "es-special",
		EvidenceID: evSpecial.ID,
		SourceID:   srcSpecial.ID,
		Excerpt:    "Trecho 100%_verídico",
		Locator:    "",
		Role:       "supports",
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("falha ao vincular fonte especial: %v", err)
	}

	t.Run("Busca por '%' não retorna toda a base", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "%"})
		if err != nil {
			t.Fatalf("erro na busca por '%%': %v", err)
		}
		// Apenas a entidade especial contém '%' literalmente
		if res.TotalCount != 1 || res.Entities[0].Slug != "pessoa-100-livre-teste" {
			t.Errorf("busca por '%%' deveria retornar apenas a entidade com %% literal, obteve %d resultados", res.TotalCount)
		}
	})

	t.Run("Busca por '_' não atua como curinga de caractere único", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "_"})
		if err != nil {
			t.Fatalf("erro na busca por '_': %v", err)
		}
		// Apenas a entidade especial contém '_' literalmente
		if res.TotalCount != 1 || res.Entities[0].Slug != "pessoa-100-livre-teste" {
			t.Errorf("busca por '_' deveria retornar apenas a entidade com _ literal, obteve %d resultados", res.TotalCount)
		}
	})

	t.Run("Busca por barra invertida funciona previsivelmente", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: `\`})
		if err != nil {
			t.Fatalf("erro na busca por '\\': %v", err)
		}
		if res.TotalCount != 1 || res.Entities[0].Slug != "pessoa-100-livre-teste" {
			t.Errorf("busca por '\\' deveria retornar a entidade especial, obteve %d resultados", res.TotalCount)
		}
	})

	t.Run("Busca por sequência exata '100%_Livre'", func(t *testing.T) {
		res, err := store.ListPublicEntities(ctx, db, store.PublicEntityFilter{Search: "100%_Livre"})
		if err != nil {
			t.Fatalf("erro na busca: %v", err)
		}
		if res.TotalCount != 1 || res.Entities[0].Slug != "pessoa-100-livre-teste" {
			t.Errorf("esperava encontrar entidade para '100%%_Livre', obteve %d", res.TotalCount)
		}
	})
}

func TestPublic_PeriodFilter(t *testing.T) {
	baseTime, _ := time.Parse(time.RFC3339, "2026-09-04T12:00:00Z")

	t.Run("Preset 7d é resolvido corretamente", func(t *testing.T) {
		f := store.SanitizeFilterWithClock(store.PublicEntityFilter{Period: "7d"}, baseTime)
		expected := "2026-08-28T12:00:00Z"
		if f.Period != "7d" || f.PeriodSince != expected {
			t.Errorf("preset 7d esperado (%s, %s), obteve (%s, %s)", "7d", expected, f.Period, f.PeriodSince)
		}
	})

	t.Run("Preset 30d é resolvido corretamente", func(t *testing.T) {
		f := store.SanitizeFilterWithClock(store.PublicEntityFilter{Period: "30d"}, baseTime)
		expected := "2026-08-05T12:00:00Z"
		if f.Period != "30d" || f.PeriodSince != expected {
			t.Errorf("preset 30d esperado (%s, %s), obteve (%s, %s)", "30d", expected, f.Period, f.PeriodSince)
		}
	})

	t.Run("Preset 90d é resolvido corretamente", func(t *testing.T) {
		f := store.SanitizeFilterWithClock(store.PublicEntityFilter{Period: "90d"}, baseTime)
		expected := "2026-06-06T12:00:00Z"
		if f.Period != "90d" || f.PeriodSince != expected {
			t.Errorf("preset 90d esperado (%s, %s), obteve (%s, %s)", "90d", expected, f.Period, f.PeriodSince)
		}
	})

	t.Run("Preset all ou vazio zera PeriodSince", func(t *testing.T) {
		fAll := store.SanitizeFilterWithClock(store.PublicEntityFilter{Period: "all"}, baseTime)
		if fAll.Period != "" || fAll.PeriodSince != "" {
			t.Errorf("preset all deveria zerar Period/PeriodSince, obteve %q / %q", fAll.Period, fAll.PeriodSince)
		}

		fEmpty := store.SanitizeFilterWithClock(store.PublicEntityFilter{Period: ""}, baseTime)
		if fEmpty.Period != "" || fEmpty.PeriodSince != "" {
			t.Errorf("preset vazio deveria zerar Period/PeriodSince, obteve %q / %q", fEmpty.Period, fEmpty.PeriodSince)
		}
	})

	t.Run("Timestamp arbitrário não é aceito e cai no default", func(t *testing.T) {
		fArbitrary := store.SanitizeFilterWithClock(store.PublicEntityFilter{Period: "2026-01-01T00:00:00Z"}, baseTime)
		if fArbitrary.Period != "" || fArbitrary.PeriodSince != "" {
			t.Errorf("timestamp arbitrário não deveria ser aceito, obteve %q / %q", fArbitrary.Period, fArbitrary.PeriodSince)
		}
	})
}

func TestPublicDocuments_ListAndFilter(t *testing.T) {
	db, ctx := setupTestDB(t)
	queries := sqlc.New(db)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Cria caso de teste
	_, err := queries.CreateCase(ctx, sqlc.CreateCaseParams{
		ID:          "case-doc-root",
		Name:        "Caso Documental",
		Slug:        "caso-documental",
		Description: "Caso de teste",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("falha ao criar caso: %v", err)
	}

	// Cria fixtures:
	// 1. Entidade pública A
	eA, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-doc-a",
		Type:               "person",
		Name:               "Pessoa Alpha",
		NormalizedName:     "pessoa alpha",
		Slug:               "pessoa-alpha",
		Category:           "Politica",
		RoleOrContext:      "Deputado",
		Reach:              "Nacional",
		Summary:            "Resumo",
		Relevance:          4,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade: %v", err)
	}

	// 2. Entidade pública B
	eB, err := queries.CreateEntity(ctx, sqlc.CreateEntityParams{
		ID:                 "ent-doc-b",
		Type:               "person",
		Name:               "Pessoa Beta",
		NormalizedName:     "pessoa beta",
		Slug:               "pessoa-beta",
		Category:           "Empresas",
		RoleOrContext:      "Empresário",
		Reach:              "Estadual",
		Summary:            "Resumo",
		Relevance:          3,
		RelevanceRationale: "Justificativa",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("falha ao criar entidade: %v", err)
	}

	// 3. Relações
	rA, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-doc-a",
		SubjectEntityID:  eA.ID,
		CaseID:           sql.NullString{String: "case-doc-root", Valid: true},
		RelationshipType: "Investigado",
		Summary:          "Resumo",
		ContextLimits:    "",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar rel A: %v", err)
	}

	rB, err := queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		ID:               "rel-doc-b",
		SubjectEntityID:  eB.ID,
		CaseID:           sql.NullString{String: "case-doc-root", Valid: true},
		RelationshipType: "Citado",
		Summary:          "Resumo",
		ContextLimits:    "",
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		t.Fatalf("falha ao criar rel B: %v", err)
	}

	// 4. Claims:
	// Claim 1 (Alpha): Publicado, Grau A, supports_link, metric_eligible=1
	cl1, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "clm-doc-1",
		RelationshipID: rA.ID,
		Proposition:    "Alpha participou de reunião",
		Attribution:    "",
		Origin:         "curated_seed",
		Grade:          "A",
		Disposition:    "supports_link",
		MetricEligible: 1,
		Status:         "published",
		ContextStatus:  "Confirmado",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim 1: %v", err)
	}

	// Claim 2 (Beta): Publicado, Grau B, context_only, metric_eligible=0 (Contexto)
	cl2, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:             "clm-doc-2",
		RelationshipID: rB.ID,
		Proposition:    "Beta prestou depoimento técnico",
		Attribution:    "",
		Origin:         "curated_seed",
		Grade:          "B",
		Disposition:    "context_only",
		MetricEligible: 0,
		Status:         "published",
		ContextStatus:  "Testemunha",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim 2: %v", err)
	}

	// Claim 3 (Alpha): Quarentenado
	cl3, err := queries.CreateClaim(ctx, sqlc.CreateClaimParams{
		ID:                "clm-doc-3",
		RelationshipID:    rA.ID,
		Proposition:       "Alpha em alegação não confirmada",
		Attribution:       "",
		Origin:            "openrouter",
		Grade:             "E",
		Disposition:       "possible_link",
		MetricEligible:    0,
		Status:            "quarantined",
		ContextStatus:     "Pendente",
		QuarantineReasons: "[\"GRADE_E\"]",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("falha ao criar claim 3: %v", err)
	}

	// 5. Evidências
	ev1, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{ID: "ev-doc-1", ClaimID: cl1.ID, Summary: "Ev 1", EvidenceType: "document", CreatedAt: now, UpdatedAt: now})
	ev2, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{ID: "ev-doc-2", ClaimID: cl2.ID, Summary: "Ev 2", EvidenceType: "document", CreatedAt: now, UpdatedAt: now})
	ev3, _ := queries.CreateEvidence(ctx, sqlc.CreateEvidenceParams{ID: "ev-doc-3", ClaimID: cl3.ID, Summary: "Ev 3", EvidenceType: "document", CreatedAt: now, UpdatedAt: now})

	// 6. Fontes:
	// Fonte Compartilhada (usada por Alpha no cl1 e Beta no cl2)
	srcShared, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-shared-doc",
		Title:              "Relatório da CPI nº 42",
		PublisherOrAuthor:  "Senado Federal",
		OriginalUrl:        "https://senado.leg.br/cpi42.pdf",
		CanonicalUrl:       "https://senado.leg.br/cpi42.pdf",
		SourceType:         "court_document",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// Fonte de Contexto exclusiva (usada por Beta no cl2 com metric_eligible=0)
	srcContextOnly, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-context-doc",
		Title:              "Ata Notarial de Esclarecimentos",
		PublisherOrAuthor:  "Cartório de Notas",
		OriginalUrl:        "https://cartorio.com.br/ata",
		CanonicalUrl:       "https://cartorio.com.br/ata",
		SourceType:         "official_statement",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// Fonte Exclusiva de Quarentena (usada apenas no cl3)
	srcQuarantine, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-quarantine-only",
		Title:              "Boato em Blog Anônimo",
		PublisherOrAuthor:  "Blog Oculto",
		OriginalUrl:        "https://blogoculto.com/boato",
		CanonicalUrl:       "https://blogoculto.com/boato",
		SourceType:         "article",
		SourceAccessStatus: "unreachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// Fonte com EvidenceSource rejeitado exclusivamente
	srcRejectedOnly, _ := queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:                 "src-rejected-only",
		Title:              "Documento Desaprovado",
		PublisherOrAuthor:  "Jornal Regional",
		OriginalUrl:        "https://jornal.com/desaprovado",
		CanonicalUrl:       "https://jornal.com/desaprovado",
		SourceType:         "article",
		SourceAccessStatus: "reachable",
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// 7. Evidence Sources
	// Suportes ativos
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{ID: "es-sh-1", EvidenceID: ev1.ID, SourceID: srcShared.ID, Role: "supports", Excerpt: "Trecho Alpha", Locator: "Pág. 10", Status: "active", CreatedAt: now, UpdatedAt: now})
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{ID: "es-sh-2", EvidenceID: ev2.ID, SourceID: srcShared.ID, Role: "supports", Excerpt: "Trecho Beta", Locator: "Pág. 20", Status: "active", CreatedAt: now, UpdatedAt: now})
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{ID: "es-ctx-1", EvidenceID: ev2.ID, SourceID: srcContextOnly.ID, Role: "supports", Excerpt: "Trecho Contexto", Locator: "Folha 1", Status: "active", CreatedAt: now, UpdatedAt: now})

	// EvidenceSource na quarentena
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{ID: "es-quar-1", EvidenceID: ev3.ID, SourceID: srcQuarantine.ID, Role: "supports", Excerpt: "Trecho Quarentenado", Locator: "", Status: "active", CreatedAt: now, UpdatedAt: now})

	// EvidenceSource rejeitado em claim publicado
	_, _ = queries.CreateEvidenceSource(ctx, sqlc.CreateEvidenceSourceParams{ID: "es-rej-1", EvidenceID: ev1.ID, SourceID: srcRejectedOnly.ID, Role: "supports", Excerpt: "Trecho Rejeitado", Locator: "", Status: "rejected", CreatedAt: now, UpdatedAt: now})

	t.Run("Fonte pública compartilhada aparece exatamente uma vez com contagens agregadas", func(t *testing.T) {
		res, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{})
		if err != nil {
			t.Fatalf("erro ao listar documentos públicos: %v", err)
		}

		// Esperado: srcShared e srcContextOnly (2 fontes públicas no total)
		if res.TotalCount != 2 {
			t.Fatalf("total de documentos públicos: esperado 2, obtido %d", res.TotalCount)
		}

		var sharedDoc *sqlc.ListPublicDocumentsRow
		for i := range res.Documents {
			if res.Documents[i].ID == srcShared.ID {
				sharedDoc = &res.Documents[i]
				break
			}
		}
		if sharedDoc == nil {
			t.Fatalf("fonte compartilhada %s não encontrada na listagem", srcShared.ID)
		}

		// Valida contagens agregadas: 2 citações ativas, 2 entidades públicas vinculadas
		if sharedDoc.CitationsCount != 2 {
			t.Errorf("citations_count: esperado 2, obtido %d", sharedDoc.CitationsCount)
		}
		if sharedDoc.EntitiesCount != 2 {
			t.Errorf("entities_count: esperado 2, obtido %d", sharedDoc.EntitiesCount)
		}
		if sharedDoc.FirstPublicExcerpt != "Trecho Alpha" {
			t.Errorf("first_public_excerpt: esperado 'Trecho Alpha', obtido '%s'", sharedDoc.FirstPublicExcerpt)
		}
		if sharedDoc.FirstPublicLocator != "Pág. 10" {
			t.Errorf("first_public_locator: esperado 'Pág. 10', obtido '%s'", sharedDoc.FirstPublicLocator)
		}
	})

	t.Run("Fonte exclusiva de quarentena NÃO aparece no catálogo", func(t *testing.T) {
		res, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{Search: "Blog Oculto"})
		if err != nil {
			t.Fatalf("erro ao buscar: %v", err)
		}
		if res.TotalCount != 0 {
			t.Errorf("vazamento de quarentena: fonte de quarentena apareceu com %d resultados", res.TotalCount)
		}
	})

	t.Run("Fonte com uso rejeitado exclusivamente NÃO aparece no catálogo", func(t *testing.T) {
		res, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{Search: "Documento Desaprovado"})
		if err != nil {
			t.Fatalf("erro ao buscar: %v", err)
		}
		if res.TotalCount != 0 {
			t.Errorf("vazamento de fonte rejeitada: obteve %d resultados", res.TotalCount)
		}
	})

	t.Run("Fonte ligada a claim com metric_eligible=false ESTÁ presente no catálogo", func(t *testing.T) {
		res, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{Search: "Ata Notarial"})
		if err != nil {
			t.Fatalf("erro ao buscar fonte de contexto: %v", err)
		}
		if res.TotalCount != 1 || res.Documents[0].ID != srcContextOnly.ID {
			t.Errorf("fonte de contexto não encontrada no catálogo: %+v", res)
		}
		if res.Documents[0].FirstPublicExcerpt != "Trecho Contexto" {
			t.Errorf("first_public_excerpt: esperado 'Trecho Contexto', obtido '%s'", res.Documents[0].FirstPublicExcerpt)
		}
	})

	t.Run("Busca e filtros por source_type e access_status", func(t *testing.T) {
		// Filtro por tipo court_document
		resType, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{
			SourceType: "court_document",
		})
		if err != nil || resType.TotalCount != 1 || resType.Documents[0].ID != srcShared.ID {
			t.Errorf("filtro por source_type falhou: %+v, err=%v", resType, err)
		}

		// Filtro por access_status reachable
		resAccess, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{
			AccessStatus: "reachable",
		})
		if err != nil || resAccess.TotalCount != 2 {
			t.Errorf("filtro por access_status falhou: %+v, err=%v", resAccess, err)
		}
	})

	t.Run("Ordenação e paginação com limites conservadores", func(t *testing.T) {
		// Página 1 com page_size 1
		resP1, err := store.ListPublicDocuments(ctx, db, store.PublicDocumentFilter{
			OrderBy:  "title",
			OrderDir: "asc",
			Page:     1,
			PageSize: 1,
		})
		if err != nil || resP1.TotalCount != 2 || len(resP1.Documents) != 1 {
			t.Fatalf("página 1 falhou: %+v, err=%v", resP1, err)
		}
		if resP1.Documents[0].ID != srcContextOnly.ID { // "Ata Notarial" vem antes de "Relatório"
			t.Errorf("ordenação ascendente por título falhou, obtido: %s", resP1.Documents[0].Title)
		}

		// Sanitização de página extrema e truncamento de busca em MaxSearchQueryLength (200 caracteres)
		longQuery := strings.Repeat("busca-extensa-", 30) // 420 caracteres
		fExtreme := store.SanitizeDocumentFilter(store.PublicDocumentFilter{
			Search:   longQuery,
			Page:     99999999,
			PageSize: 1000,
		})
		if fExtreme.Page != store.MaxPage || fExtreme.PageSize != store.MaxPageSize {
			t.Errorf("sanitização falhou: obtido page=%d, pageSize=%d", fExtreme.Page, fExtreme.PageSize)
		}
		if len([]rune(fExtreme.Search)) != store.MaxSearchQueryLength {
			t.Errorf("truncamento de busca falhou: esperado %d caracteres, obtido %d", store.MaxSearchQueryLength, len([]rune(fExtreme.Search)))
		}
	})
}
