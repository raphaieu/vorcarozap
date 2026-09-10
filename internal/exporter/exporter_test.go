package exporter_test

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/raphaieu/vorcarozap/internal/store"
	"github.com/xuri/excelize/v2"
)

// setupTestDB cria um banco SQLite em memória descartável e aplica todas as migrations.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("falha ao abrir sqlite em memória: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("falha ao aplicar migrations: %v", err)
	}

	return db
}

func TestSanitizeCellText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"texto normal", "Daniel Vorcaro", "Daniel Vorcaro"},
		{"inicia com igual", "=1+1", "'=1+1"},
		{"inicia com mais", "+551199999999", "'+551199999999"},
		{"inicia com menos", "-100", "'-100"},
		{"inicia com arroba", "@SUM(A1:A10)", "'@SUM(A1:A10)"},
		{"espaços antes de igual", "   =CMD", "'   =CMD"},
		{"string vazia", "", ""},
		{"apenas espaços", "   ", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exporter.SanitizeCellText(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeCellText(%q) = %q, esperado %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExporter_EmptyDatabase(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	exp := exporter.New(db, "2026-09-03")
	ctx := context.Background()

	var buf bytes.Buffer
	if err := exp.WriteTo(ctx, &buf); err != nil {
		t.Fatalf("WriteTo falhou com base vazia: %v", err)
	}

	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX gerado: %v", err)
	}
	defer f.Close()

	// Verifica a existência de todas as 4 abas obrigatórias
	sheets := f.GetSheetList()
	expectedSheets := []string{
		exporter.SheetEntities,
		exporter.SheetClaims,
		exporter.SheetSources,
		exporter.SheetMethodology,
	}

	for _, s := range expectedSheets {
		found := false
		for _, actual := range sheets {
			if actual == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("aba obrigatória %q não encontrada no XLSX gerado: %v", s, sheets)
		}
	}

	// Verifica linha informativa de base vazia
	msg, err := f.GetCellValue(exporter.SheetEntities, "A2")
	if err != nil {
		t.Fatalf("erro ao ler célula A2 da aba Entidades: %v", err)
	}
	if msg == "" {
		t.Errorf("esperava mensagem de estado vazio em A2, obteve vazio")
	}
}

func TestExporter_VisibilityAndMetricEligibility(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// 1. Inserir Entidades
	ent1ID := uuid.NewString() // Pessoa com claim publicado elegível (Grau D)
	ent2ID := uuid.NewString() // Pessoa com claim publicado não-elegível (Grau E corrigido)
	ent3ID := uuid.NewString() // Pessoa com claim em quarentena (Grau E ambíguo) - NÃO deve aparecer
	ent4ID := uuid.NewString() // Pessoa com claim rejeitado - NÃO deve aparecer

	insertEntity := func(id, name, slug, cat string, rel int) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO entities (id, type, name, normalized_name, slug, category, role_or_context, reach, summary, relevance, relevance_rationale)
			VALUES (?, 'person', ?, ?, ?, ?, 'Executivo', 'Nacional', 'Resumo', ?, 'Justificativa')
		`, id, name, name, slug, cat, rel)
		if err != nil {
			t.Fatalf("falha ao inserir entidade %s: %v", name, err)
		}
	}

	insertEntity(ent1ID, "Entidade Publica Elegivel", "entidade-publica-elegivel", "Finanças", 4)
	insertEntity(ent2ID, "Entidade Publica Nao Elegivel", "entidade-publica-nao-elegivel", "Contexto", 2)
	insertEntity(ent3ID, "Entidade Quarentenada", "entidade-quarentenada", "Geral", 1)
	insertEntity(ent4ID, "Entidade Rejeitada", "entidade-rejeitada", "Outros", 1)

	// Inserir Relacionamentos
	insertRel := func(id, subj, limits string) {
		caseID := uuid.NewString()
		_, _ = db.ExecContext(ctx, `INSERT INTO cases (id, name, slug) VALUES (?, 'Caso Teste', ?)`, caseID, "caso-"+id)
		_, err := db.ExecContext(ctx, `
			INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
			VALUES (?, ?, ?, 'investigado', 'Resumo da relação', ?)
		`, id, subj, caseID, limits)
		if err != nil {
			t.Fatalf("falha ao inserir rel: %v", err)
		}
	}

	rel1ID := uuid.NewString()
	rel2ID := uuid.NewString()
	rel3ID := uuid.NewString()
	rel4ID := uuid.NewString()
	insertRel(rel1ID, ent1ID, "Ressalva documental sobre escopo societário")
	insertRel(rel2ID, ent2ID, "Esclarecimento de homônimo arquivado")
	insertRel(rel3ID, ent3ID, "")
	insertRel(rel4ID, ent4ID, "")

	// Inserir Claims
	insertClaim := func(id, relID, prop, grade, disp string, metricEligible int, status string) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES (?, ?, ?, 'Fonte X', 'curated_seed', ?, ?, ?, ?, 'investigado')
		`, id, relID, prop, grade, disp, metricEligible, status)
		if err != nil {
			t.Fatalf("falha ao inserir claim: %v", err)
		}
	}

	claim1ID := uuid.NewString() // Publicado elegível
	claim2ID := uuid.NewString() // Publicado não elegível
	claim3ID := uuid.NewString() // Quarentena
	claim4ID := uuid.NewString() // Rejeitado

	insertClaim(claim1ID, rel1ID, "Claim Publico Elegivel", "D", "possible_link", 1, "published")
	insertClaim(claim2ID, rel2ID, "Claim Publico Contexto", "E", "context_only", 0, "published")
	insertClaim(claim3ID, rel3ID, "Claim Quarentenado", "E", "possible_link", 0, "quarantined")
	insertClaim(claim4ID, rel4ID, "Claim Rejeitado", "B", "supports_link", 1, "rejected")

	// Inserir Sources e Evidence
	insertSource := func(id, url string) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_access_status)
			VALUES (?, 'Titulo Fonte', 'Veiculo X', ?, ?, 'reachable')
		`, id, url, url)
		if err != nil {
			t.Fatalf("falha ao inserir source: %v", err)
		}
	}

	src1ID := uuid.NewString()
	src2ID := uuid.NewString()
	src3ID := uuid.NewString()
	src4ID := uuid.NewString()
	insertSource(src1ID, "https://noticia.com/1")
	insertSource(src2ID, "https://noticia.com/2")
	insertSource(src3ID, "https://noticia.com/3")
	insertSource(src4ID, "https://noticia.com/4")

	insertEvidenceWithSource := func(claimID, srcID, role, esStatus string) {
		evID := uuid.NewString()
		_, err := db.ExecContext(ctx, `INSERT INTO evidence (id, claim_id, summary) VALUES (?, ?, 'Resumo ev')`, evID, claimID)
		if err != nil {
			t.Fatalf("falha ao inserir evidence: %v", err)
		}
		esID := uuid.NewString()
		_, err = db.ExecContext(ctx, `
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status)
			VALUES (?, ?, ?, 'Trecho literal', 'Pag. 10', ?, ?)
		`, esID, evID, srcID, role, esStatus)
		if err != nil {
			t.Fatalf("falha ao inserir evidence_source: %v", err)
		}
	}

	// Vincula fontes com supports ativo
	insertEvidenceWithSource(claim1ID, src1ID, "supports", "active")
	insertEvidenceWithSource(claim2ID, src2ID, "supports", "active")
	insertEvidenceWithSource(claim3ID, src3ID, "supports", "active") // claim está em quarentena
	insertEvidenceWithSource(claim4ID, src4ID, "supports", "active") // claim está rejeitado

	// Adiciona uma evidência extra no claim1 com status rejected - NÃO deve constar na aba de fontes
	insertEvidenceWithSource(claim1ID, src3ID, "supports", "rejected")

	// Gerar exportação
	exp := exporter.New(db, "2026-09-03")
	var buf bytes.Buffer
	if err := exp.WriteTo(ctx, &buf); err != nil {
		t.Fatalf("falha em WriteTo: %v", err)
	}

	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatalf("falha ao abrir XLSX: %v", err)
	}
	defer f.Close()

	// 1. Validar Aba Entidades
	entRows, err := f.GetRows(exporter.SheetEntities)
	if err != nil {
		t.Fatalf("erro ao ler linhas de Entidades: %v", err)
	}
	// Linha 1 = Cabeçalho; Linha 2 e 3 = ent1 e ent2
	if len(entRows) != 3 {
		t.Fatalf("esperava 3 linhas em Entidades (1 cabecalho + 2 entidades publicas), obteve %d", len(entRows))
	}

	foundEnt1 := false
	foundEnt2 := false
	for _, r := range entRows[1:] {
		if len(r) > 1 {
			if r[1] == "Entidade Publica Elegivel" {
				foundEnt1 = true
			}
			if r[1] == "Entidade Publica Nao Elegivel" {
				foundEnt2 = true
			}
			if r[1] == "Entidade Quarentenada" || r[1] == "Entidade Rejeitada" {
				t.Errorf("entidade não pública vazou para a aba Entidades: %s", r[1])
			}
		}
	}
	if !foundEnt1 || !foundEnt2 {
		t.Errorf("faltou entidade esperada na aba Entidades: ent1=%v, ent2=%v", foundEnt1, foundEnt2)
	}

	// 2. Validar Aba Alegações
	claimRows, err := f.GetRows(exporter.SheetClaims)
	if err != nil {
		t.Fatalf("erro ao ler linhas de Alegações: %v", err)
	}
	// Linha 1 = Cabeçalho; Linha 2 e 3 = claim1 e claim2
	if len(claimRows) != 3 {
		t.Fatalf("esperava 3 linhas em Alegações (1 cabecalho + 2 claims publicos), obteve %d", len(claimRows))
	}

	foundClaim1 := false
	foundClaim2 := false
	for _, r := range claimRows[1:] {
		prop := r[5]
		limits := r[6]
		elegivel := r[9]
		if prop == "Claim Publico Elegivel" {
			foundClaim1 = true
			if elegivel != "Sim" {
				t.Errorf("Claim1 deveria ter Elegível nas Métricas = 'Sim', obteve %q", elegivel)
			}
			if limits != "Ressalva documental sobre escopo societário" {
				t.Errorf("Claim1 deveria ter Limites Contextuais preservados, obteve %q", limits)
			}
		}
		if prop == "Claim Publico Contexto" {
			foundClaim2 = true
			if elegivel != "Não" {
				t.Errorf("Claim2 deveria ter Elegível nas Métricas = 'Não', obteve %q", elegivel)
			}
			if limits != "Esclarecimento de homônimo arquivado" {
				t.Errorf("Claim2 deveria ter Limites Contextuais preservados, obteve %q", limits)
			}
		}
		if prop == "Claim Quarentenado" || prop == "Claim Rejeitado" {
			t.Errorf("claim não público vazou para a aba Alegações: %s", prop)
		}
	}
	if !foundClaim1 || !foundClaim2 {
		t.Errorf("faltou claim esperado na aba Alegações: claim1=%v, claim2=%v", foundClaim1, foundClaim2)
	}

	// 3. Validar Aba Evidências e Fontes
	sourceRows, err := f.GetRows(exporter.SheetSources)
	if err != nil {
		t.Fatalf("erro ao ler linhas de Fontes: %v", err)
	}
	// Devem existir apenas as evidências ativas dos claims 1 e 2 (2 fontes ativas)
	if len(sourceRows) != 3 {
		t.Fatalf("esperava 3 linhas em Fontes (1 cabecalho + 2 fontes ativas de claims publicos), obteve %d", len(sourceRows))
	}
	for _, r := range sourceRows[1:] {
		url := r[10]
		if url != "https://noticia.com/1" && url != "https://noticia.com/2" {
			t.Errorf("URL indevida na exportação de fontes: %s", url)
		}
	}
}
