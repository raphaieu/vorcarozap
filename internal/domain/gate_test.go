package domain_test

import (
	"testing"

	"github.com/raphaieu/vorcarozap/internal/domain"
)

func validStructuralInput() domain.StructuralGateInput {
	return domain.StructuralGateInput{
		Fingerprint:                   "v1:3f2d1e4c5b6a7890123456789abcdef0123456789abcdef0123456789abcdef0",
		FingerprintVersion:            1,
		IsDuplicate:                   false,
		EntityName:                    "Daniel Vorcaro",
		NormalizedEntityName:          "daniel vorcaro",
		TargetEntityName:              "Banco Master",
		NormalizedTargetEntityName:    "banco master",
		RelationshipType:              "societário",
		Proposition:                   "Aquisição de participação societária no Banco Master",
		SuggestedGrade:                domain.EvidenceGradeA,
		SourceURL:                     "https://exemplo.com/materia-1",
		CanonicalURL:                  "https://exemplo.com/materia-1",
		SourceTitle:                   "Reportagem sobre estrutura societária",
		PublisherOrAuthor:             "UOL Notícias",
		PublishedAt:                   "2026-09-03",
		Excerpt:                       "O empresário adquiriu controle societário do banco em 2026.",
		Locator:                       "Página 4",
		ContextLimits:                 "Aprovado pelo regulador",
		SubjectResolvedID:             "ent-daniel-vorcaro",
		SubjectAmbiguous:              false,
		TargetResolvedID:              "ent-banco-master",
		TargetAmbiguous:               false,
		CaseResolvedID:                "",
		CaseAmbiguous:                 false,
		PreviouslyRejectedFingerprint: false,
		SourceAccessStatus:            domain.SourceAccessReachable,
	}
}

func TestEvaluateStructuralGate_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		mutate         func(in *domain.StructuralGateInput)
		expectedPassed bool
		expectedReason string
	}{
		{
			name:           "Candidato válido com entidade-alvo",
			mutate:         func(in *domain.StructuralGateInput) {},
			expectedPassed: true,
		},
		{
			name: "Candidato válido com caso contextual",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetEntityName = ""
				in.NormalizedTargetEntityName = ""
				in.TargetResolvedID = ""
				in.CaseName = "Operação Desdobramento"
				in.NormalizedCaseName = "operacao desdobramento"
				in.CaseResolvedID = "case-desdobramento"
			},
			expectedPassed: true,
		},
		{
			name: "Duplicata reprova",
			mutate: func(in *domain.StructuralGateInput) {
				in.IsDuplicate = true
			},
			expectedPassed: false,
			expectedReason: "duplicate_candidate",
		},
		{
			name: "Versão de fingerprint inválida",
			mutate: func(in *domain.StructuralGateInput) {
				in.FingerprintVersion = 2
			},
			expectedPassed: false,
			expectedReason: "invalid_fingerprint_version",
		},
		{
			name: "Fingerprint vazio",
			mutate: func(in *domain.StructuralGateInput) {
				in.Fingerprint = "   "
			},
			expectedPassed: false,
			expectedReason: "empty_fingerprint",
		},
		{
			name: "URL inválida (esquema ftp)",
			mutate: func(in *domain.StructuralGateInput) {
				in.CanonicalURL = "ftp://exemplo.com/doc.pdf"
			},
			expectedPassed: false,
			expectedReason: "invalid_canonical_url",
		},
		{
			name: "Título vazio",
			mutate: func(in *domain.StructuralGateInput) {
				in.SourceTitle = ""
			},
			expectedPassed: false,
			expectedReason: "empty_source_title",
		},
		{
			name: "Publicador vazio",
			mutate: func(in *domain.StructuralGateInput) {
				in.PublisherOrAuthor = ""
			},
			expectedPassed: false,
			expectedReason: "empty_publisher_or_author",
		},
		{
			name: "Trecho vazio",
			mutate: func(in *domain.StructuralGateInput) {
				in.Excerpt = ""
			},
			expectedPassed: false,
			expectedReason: "empty_excerpt",
		},
		{
			name: "Proposição vazia",
			mutate: func(in *domain.StructuralGateInput) {
				in.Proposition = ""
			},
			expectedPassed: false,
			expectedReason: "empty_proposition",
		},
		{
			name: "Tipo de relacionamento vazio",
			mutate: func(in *domain.StructuralGateInput) {
				in.RelationshipType = ""
			},
			expectedPassed: false,
			expectedReason: "empty_relationship_type",
		},
		{
			name: "Data de publicação em formato inválido",
			mutate: func(in *domain.StructuralGateInput) {
				in.PublishedAt = "03/09/2026"
			},
			expectedPassed: false,
			expectedReason: "invalid_published_at_format",
		},
		{
			name: "Grau inválido",
			mutate: func(in *domain.StructuralGateInput) {
				in.SuggestedGrade = domain.EvidenceGrade("Z")
			},
			expectedPassed: false,
			expectedReason: "invalid_grade",
		},
		{
			name: "Violação de XOR (ambos preenchidos)",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetEntityName = "Banco Master"
				in.CaseName = "Operação Desdobramento"
			},
			expectedPassed: false,
			expectedReason: "target_case_xor_violated",
		},
		{
			name: "Violação de XOR (nenhum preenchido)",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetEntityName = ""
				in.CaseName = ""
			},
			expectedPassed: false,
			expectedReason: "target_case_xor_violated",
		},
		{
			name: "Sujeito ambíguo",
			mutate: func(in *domain.StructuralGateInput) {
				in.SubjectAmbiguous = true
			},
			expectedPassed: false,
			expectedReason: "ambiguous_subject_entity",
		},
		{
			name: "Sujeito não resolvido",
			mutate: func(in *domain.StructuralGateInput) {
				in.SubjectResolvedID = ""
			},
			expectedPassed: false,
			expectedReason: "unresolved_subject_entity",
		},
		{
			name: "Alvo ambíguo",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetAmbiguous = true
			},
			expectedPassed: false,
			expectedReason: "ambiguous_target_entity",
		},
		{
			name: "Alvo não resolvido",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetResolvedID = ""
			},
			expectedPassed: false,
			expectedReason: "unresolved_target_entity",
		},
		{
			name: "Caso não resolvido",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetEntityName = ""
				in.NormalizedTargetEntityName = ""
				in.TargetResolvedID = ""
				in.CaseName = "Caso X"
				in.CaseResolvedID = ""
			},
			expectedPassed: false,
			expectedReason: "unresolved_case",
		},
		{
			name: "Autorrelação por ID",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetResolvedID = in.SubjectResolvedID
			},
			expectedPassed: false,
			expectedReason: "self_relation_prohibited",
		},
		{
			name: "Autorrelação por Nome Normalizado",
			mutate: func(in *domain.StructuralGateInput) {
				in.NormalizedTargetEntityName = in.NormalizedEntityName
			},
			expectedPassed: false,
			expectedReason: "self_relation_prohibited",
		},
		{
			name: "Detecção de PII (CPF)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Proposition = "A pessoa portadora do CPF 123.456.789-09 realizou a operação"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (Telefone)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Excerpt = "Entre em contato pelo telefone (11) 98765-4321 para detalhes"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (E-mail pessoal gmail)",
			mutate: func(in *domain.StructuralGateInput) {
				in.ContextLimits = "Contato enviado para usuario.teste@gmail.com"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (E-mail de domínio arbitrário/corporativo)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Proposition = "Notificação enviada para diretor@empresa.com.br"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII em EntityName (E-mail no nome)",
			mutate: func(in *domain.StructuralGateInput) {
				in.EntityName = "Fulano (fulano@exemplo.org)"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII em TargetEntityName (Telefone no alvo)",
			mutate: func(in *domain.StructuralGateInput) {
				in.TargetEntityName = "Empresa X (11 99999-8888)"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII em SourceTitle (CPF no título)",
			mutate: func(in *domain.StructuralGateInput) {
				in.SourceTitle = "Processo do CPF 123.456.789-09"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII em PublisherOrAuthor (E-mail no autor)",
			mutate: func(in *domain.StructuralGateInput) {
				in.PublisherOrAuthor = "Autor autor@jornal.com"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII em RelationshipType (Telefone no tipo de relação)",
			mutate: func(in *domain.StructuralGateInput) {
				in.RelationshipType = "contato via 31 98888-7777"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII em Locator (Apartamento)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Locator = "Apto 101"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (CEP brasileiro)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Proposition = "Residência localizada no CEP 30140-071 em Belo Horizonte"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (Endereço residencial com Rua e número)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Excerpt = "Mandado cumprido na Rua Oscar Freire, 1234"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (Endereço residencial com Avenida e nº)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Proposition = "Averiguação realizada na Avenida Paulista, nº 1000"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Detecção de PII (Endereço com complemento Apartamento)",
			mutate: func(in *domain.StructuralGateInput) {
				in.Proposition = "Domicílio no Apartamento 402 do edifício"
			},
			expectedPassed: false,
			expectedReason: "contains_unnecessary_pii",
		},
		{
			name: "Fingerprint anteriormente rejeitado",
			mutate: func(in *domain.StructuralGateInput) {
				in.PreviouslyRejectedFingerprint = true
			},
			expectedPassed: false,
			expectedReason: "fingerprint_previously_rejected",
		},
		{
			name: "Fonte unreachable",
			mutate: func(in *domain.StructuralGateInput) {
				in.SourceAccessStatus = domain.SourceAccessUnreachable
			},
			expectedPassed: false,
			expectedReason: "source_unreachable",
		},
		{
			name: "Fonte not_checked",
			mutate: func(in *domain.StructuralGateInput) {
				in.SourceAccessStatus = domain.SourceAccessNotChecked
			},
			expectedPassed: false,
			expectedReason: "source_not_checked",
		},
		{
			name: "Fonte cited_by_provider sem verificação",
			mutate: func(in *domain.StructuralGateInput) {
				in.SourceAccessStatus = domain.SourceAccessCitedByProvider
			},
			expectedPassed: false,
			expectedReason: "source_cited_by_provider_not_verified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validStructuralInput()
			tt.mutate(&input)

			res := domain.EvaluateStructuralGate(input)
			if res.Passed != tt.expectedPassed {
				t.Errorf("esperava Passed=%v, obtido Passed=%v (reasons: %v)", tt.expectedPassed, res.Passed, res.Reasons)
			}

			if tt.expectedReason != "" {
				found := false
				for _, r := range res.Reasons {
					if r == tt.expectedReason {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("esperava motivo %q na lista de razões, obtido %v", tt.expectedReason, res.Reasons)
				}
			}
		})
	}
}
