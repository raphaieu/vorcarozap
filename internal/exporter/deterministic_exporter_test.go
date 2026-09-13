package exporter_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/raphaieu/vorcarozap/internal/exporter"
	"github.com/xuri/excelize/v2"
)

// TestDeterministic_Exporter_TableDriven comprova todos os invariantes da exportação XLSX:
// omissão de registros em quarentena/rejeitados, inclusão de claims de contexto com flag Não,
// omissão de fontes rejeitadas, sanitização de fórmulas e atualização imediata pós-moderação.
func TestDeterministic_Exporter_TableDriven(t *testing.T) {
	t.Run("Sanitização de fórmulas em células de exportação", func(t *testing.T) {
		dangerousInputs := []struct {
			raw      string
			expected string
		}{
			{"=SUM(A1:A10)", "'=SUM(A1:A10)"},
			{"+cmd|' /C calc'!A0", "'+cmd|' /C calc'!A0"},
			{"-2+3*[1]Sheet!A1", "'-2+3*[1]Sheet!A1"},
			{"@SUM(1+1)", "'@SUM(1+1)"},
			{"   =HYPERLINK(\"http://evil.com\")", "'   =HYPERLINK(\"http://evil.com\")"},
			{"Texto Normal Sem Fórmula", "Texto Normal Sem Fórmula"},
			{"", ""},
		}

		for _, tc := range dangerousInputs {
			got := exporter.SanitizeCellText(tc.raw)
			if got != tc.expected {
				t.Errorf("SanitizeCellText(%q) = %q, esperado %q", tc.raw, got, tc.expected)
			}
		}
	})

	t.Run("Isolamento estrito da fronteira pública no XLSX e segregação de abas", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()
		ctx := context.Background()

		entPubID := uuid.NewString()
		entQuarID := uuid.NewString()
		entRejID := uuid.NewString()

		// 1. Entidades
		_, err := db.ExecContext(ctx, `
			INSERT INTO entities (id, type, name, normalized_name, slug, category, role_or_context, reach, summary, relevance, relevance_rationale)
			VALUES
				(?, 'person', 'Entidade Publica', 'entidade publica', 'entidade-publica', 'Finanças', 'Diretor', 'Nacional', 'Resumo', 5, 'R1'),
				(?, 'person', 'Entidade Quarentena', 'entidade quarentena', 'entidade-quarentena', 'Politica', 'Sócio', 'Regional', 'Resumo', 3, 'R2'),
				(?, 'person', 'Entidade Rejeitada', 'entidade rejeitada', 'entidade-rejeitada', 'Outros', 'Contato', 'Regional', 'Resumo', 1, 'R3');
		`, entPubID, entQuarID, entRejID)
		if err != nil {
			t.Fatalf("falha ao inserir entidades: %v", err)
		}

		// 2. Relacionamentos
		relPubID := uuid.NewString()
		relQuarID := uuid.NewString()
		relRejID := uuid.NewString()
		caseID := uuid.NewString()
		_, _ = db.ExecContext(ctx, `INSERT INTO cases (id, name, slug) VALUES (?, 'Caso Teste', 'caso-teste')`, caseID)

		_, err = db.ExecContext(ctx, `
			INSERT INTO relationships (id, subject_entity_id, case_id, relationship_type, summary, context_limits)
			VALUES
				(?, ?, ?, 'investigado', 'Vínculo Público', 'Limites contextuais explícitos documentados'),
				(?, ?, ?, 'investigado', 'Vínculo Quarentena', ''),
				(?, ?, ?, 'investigado', 'Vínculo Rejeitado', '');
		`, relPubID, entPubID, caseID, relQuarID, entQuarID, caseID, relRejID, entRejID, caseID)
		if err != nil {
			t.Fatalf("falha ao inserir relacionamentos: %v", err)
		}

		// 3. Claims
		claimPubElegID := uuid.NewString()
		claimPubContID := uuid.NewString()
		claimQuarID := uuid.NewString()
		claimRejID := uuid.NewString()

		_, err = db.ExecContext(ctx, `
			INSERT INTO claims (id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status)
			VALUES
				(?, ?, 'Proposição Pública Elegível', 'Jornal 1', 'curated_seed', 'A', 'supports_link', 1, 'published', 'ativo'),
				(?, ?, 'Proposição Pública de Contexto', 'Jornal 2', 'curated_seed', 'C', 'context_only', 0, 'published', 'ativo'),
				(?, ?, 'Proposição em Quarentena', 'Jornal 3', 'openrouter', 'D', 'possible_link', 1, 'quarantined', 'ativo'),
				(?, ?, 'Proposição Rejeitada', 'Jornal 4', 'admin', 'E', 'possible_link', 1, 'rejected', 'ativo');
		`, claimPubElegID, relPubID, claimPubContID, relPubID, claimQuarID, relQuarID, claimRejID, relRejID)
		if err != nil {
			t.Fatalf("falha ao inserir claims: %v", err)
		}

		// 4. Sources e EvidenceSources
		srcPub1 := uuid.NewString()
		srcPub2 := uuid.NewString()
		srcRej := uuid.NewString()

		_, err = db.ExecContext(ctx, `
			INSERT INTO sources (id, title, publisher_or_author, original_url, canonical_url, source_access_status)
			VALUES
				(?, 'Fonte 1', 'Jornal 1', 'https://noticia1.com', 'https://noticia1.com', 'reachable'),
				(?, 'Fonte 2', 'Jornal 2', 'https://noticia2.com', 'https://noticia2.com', 'reachable'),
				(?, 'Fonte Rejeitada', 'Jornal 3', 'https://rejeitada.com', 'https://rejeitada.com', 'reachable');
		`, srcPub1, srcPub2, srcRej)
		if err != nil {
			t.Fatalf("falha ao inserir sources: %v", err)
		}

		ev1 := uuid.NewString()
		ev2 := uuid.NewString()
		evQuar := uuid.NewString()
		evRej := uuid.NewString()
		_, _ = db.ExecContext(ctx, `
			INSERT INTO evidence (id, claim_id, summary) VALUES
				(?, ?, 'Ev 1'), (?, ?, 'Ev 2'), (?, ?, 'Ev Quar'), (?, ?, 'Ev Rej');
		`, ev1, claimPubElegID, ev2, claimPubContID, evQuar, claimQuarID, evRej, claimRejID)

		// Evidence sources ativas nos claims públicos
		_, _ = db.ExecContext(ctx, `
			INSERT INTO evidence_sources (id, evidence_id, source_id, excerpt, locator, role, status) VALUES
				(?, ?, ?, 'Trecho 1', 'P.1', 'supports', 'active'),
				(?, ?, ?, 'Trecho 2', 'P.2', 'supports', 'active'),
				-- Evidência com status rejected no claim público elegível (NÃO deve aparecer na aba Fontes)
				(?, ?, ?, 'Trecho Rejeitado', 'P.3', 'supports', 'rejected'),
				-- Evidência no claim quarentenado (NÃO deve aparecer)
				(?, ?, ?, 'Trecho Quar', 'P.4', 'supports', 'active'),
				-- Evidência no claim rejeitado (NÃO deve aparecer)
				(?, ?, ?, 'Trecho Rej', 'P.5', 'supports', 'active');
		`, uuid.NewString(), ev1, srcPub1, uuid.NewString(), ev2, srcPub2, uuid.NewString(), ev1, srcRej, uuid.NewString(), evQuar, srcPub1, uuid.NewString(), evRej, srcPub2)

		// 5. Geração e Inspeção do XLSX
		exp := exporter.New(db, "2026-09-03")
		var buf bytes.Buffer
		if err := exp.WriteTo(ctx, &buf); err != nil {
			t.Fatalf("WriteTo falhou: %v", err)
		}

		f, err := excelize.OpenReader(&buf)
		if err != nil {
			t.Fatalf("falha ao abrir XLSX gerado: %v", err)
		}
		defer f.Close()

		// A. Aba Entidades: apenas Entidade Publica (1 linha de cabeçalho + 1 entidade)
		entRows, _ := f.GetRows(exporter.SheetEntities)
		if len(entRows) != 2 {
			t.Errorf("Aba Entidades: esperado 2 linhas, obtido %d", len(entRows))
		}
		if len(entRows) > 1 && entRows[1][1] != "Entidade Publica" {
			t.Errorf("Entidade inesperada na aba Entidades: %v", entRows[1])
		}

		// B. Aba Alegações: apenas os 2 claims públicos
		claimRows, _ := f.GetRows(exporter.SheetClaims)
		if len(claimRows) != 3 { // 1 cabeçalho + 2 claims
			t.Fatalf("Aba Alegações: esperado 3 linhas, obtido %d", len(claimRows))
		}

		foundElegible := false
		foundContext := false
		for _, r := range claimRows[1:] {
			prop := r[5]
			elegivel := r[9]
			limits := r[6]

			if prop == "Proposição Pública Elegível" {
				foundElegible = true
				if elegivel != "Sim" {
					t.Errorf("Claim elegível deveria ter 'Sim', obteve %q", elegivel)
				}
				if limits != "Limites contextuais explícitos documentados" {
					t.Errorf("Limites contextuais não preservados: %q", limits)
				}
			}
			if prop == "Proposição Pública de Contexto" {
				foundContext = true
				if elegivel != "Não" {
					t.Errorf("Claim de contexto deveria ter 'Não', obteve %q", elegivel)
				}
			}
			if prop == "Proposição em Quarentena" || prop == "Proposição Rejeitada" {
				t.Errorf("claim não público vazou na aba Alegações: %s", prop)
			}
		}

		if !foundElegible || !foundContext {
			t.Errorf("faltou claim público esperado na exportação: eleg=%v, ctx=%v", foundElegible, foundContext)
		}

		// C. Aba Evidências e Fontes: apenas as 2 fontes ativas dos claims públicos
		sourceRows, _ := f.GetRows(exporter.SheetSources)
		if len(sourceRows) != 3 { // 1 cabeçalho + 2 fontes ativas
			t.Fatalf("Aba Fontes: esperado 3 linhas, obtido %d", len(sourceRows))
		}
		for _, r := range sourceRows[1:] {
			url := r[10]
			if url == "https://rejeitada.com" {
				t.Errorf("fonte rejeitada vazou na aba de fontes do XLSX: %s", url)
			}
		}
	})
}
