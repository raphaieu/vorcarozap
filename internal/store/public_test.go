package store_test

import (
	"context"
	"database/sql"
	"errors"
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
