# Changelog

Todas as alterações notáveis deste projeto são registradas neste documento. O formato baseia-se em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/).

## [Não lançado] — VZ-009: Exportação XLSX Derivada da Base Pública

### Adicionado
- **Motor de Exportação XLSX (`internal/exporter`):**
  - Implementação da geração determinística de planilhas XLSX via `github.com/xuri/excelize/v2` a partir de consultas exclusivas ao estado público do SQLite (`public_claims_view`).
  - Segregação dos dados em 4 abas canônicas: `Entidades`, `Alegações e Relações`, `Evidências e Fontes`, e `Metodologia e Critérios`.
  - Proteção estrita contra Formula Injection (CSV/XLSX injection) gravando todas as células de texto com `SetCellStr` e neutralizando caracteres disparadores (`=`, `+`, `-`, `@`, tab).
  - Preservação da distinção de elegibilidade métrica: alegações públicas com `metric_eligible = false` (ex.: Grau E corrigido) continuam na exportação com a coluna `Elegível nas Métricas = Não`, enquanto itens de rede figuram com `Sim`.
  - Exclusão estrita de dados em quarentena (21 registros legados), alegações rejeitadas, arquivadas ou metadados de processamento interno.
  - Formatação visual institucional, larguras calibradas e congelamento de painéis de cabeçalho.
- **Subcomando CLI de Exportação (`cmd/vorcarozap`):**
  - Comando `vorcarozap export [--out <caminho.xlsx>]` para geração de arquivo diretamente pelo terminal.
- **Download HTTP e Integração na Interface Web (`internal/web`, `web/pages`, `web/components`):**
  - Endpoint `GET /exportar/base.xlsx` com cabeçalhos apropriados de download de anexo (`Content-Disposition: attachment; filename="vorcarozap-dados-publicos-YYYY-MM-DD.xlsx"`).
  - Redirect amigável `GET /exportar` -> `/exportar/base.xlsx`.
  - Botão de download no cabeçalho de resultados de `/pessoas`, no card de consulta da Home e link no menu de navegação.
- **Suíte de Testes Automatizados:**
  - Testes unitários de sanitização e proteção contra formula injection em `internal/exporter/exporter_test.go`.
  - Testes de integridade de abas, células e cabeçalhos em cenário de base vazia.
  - Testes de exclusão de estados não públicos e validação de elegibilidade métrica.
  - Smoke test end-to-end com importação real da planilha seed e exportação no banco SQLite (`internal/exporter/smoke_export_test.go`).
  - Testes de integração HTTP do endpoint `/exportar/base.xlsx` em `internal/web/server_test.go`.

### Modificado
- **Makefile:** Inclusão da flag `-T` em `DEV_RUN` para desabilitar alocação de pseudo-TTY do Docker Compose em execuções de automação não interativas.
- **Documentação:** Atualização de `docs/08-roadmap.md`, `docs/09-backlog-inicial.md`, `docs/11-continuidade-e-baseline.md` e `README.md`, marcando VZ-009 como concluído e definindo VZ-020 como próxima etapa.

## [0.3.0] — Ciclo de Saneamento da Apresentação e Consolidação da Visão Documental

### Modificado
- **Apresentação pública da Home (`web/pages/home.templ`):**
  - Rótulo “Fontes Auditadas” alterado para “Fontes Citadas”, refletindo com exatidão a contagem de fontes ativas distintas.
  - Rótulo “Pessoas na Rede” alterado para “Entidades na Rede”, incorporando organizações ao lado de pessoas físicas.
  - Rótulo “Vínculos Qualificados” alterado para “Alegações Elegíveis”, alinhando a nomenclatura à unidade de contagem (`claim`), com subtítulo corrigido para “Alegações consideradas nas métricas”.
  - Texto explicativo ajustado para esclarecer com precisão que as métricas consideram alegações publicadas elegíveis, incluindo vínculos possíveis conforme a metodologia.
  - Substituição de jargões internos do banco (`published`, `metric_eligible = true`, `OpenRouter`) por linguagem pública clara e acessível.
  - Remoção de afirmações genéricas não respaldadas (“relações comprovadas”, “comprovação documental”, “auditoria de fontes”).
  - Inclusão de notas explicativas nos cartões de distribuição esclarecendo que os links de filtro direcionam para a consulta ampla do acervo público em `/pessoas`.
  - Atualização da meta tag de descrição para utilizar “fontes rastreáveis”.
- **Detalhes de Entidades (`web/pages/entity_detail.templ`):**
  - Ajuste na meta tag de descrição para utilizar “fontes rastreáveis”.
- **Alinhamento do `README.md`:**
  - Sincronização da numeração de fases com o documento canônico de roadmap (`Fase 2` para núcleo/importação e `Fase 3` para página pública e métricas).
  - Atualização de comandos planejados para fases posteriores.
- **Testes de Renderização e Smoke (`internal/web`):**
  - Atualização de asserções em `server_test.go` e `smoke_test.go` para verificar os novos rótulos de apresentação pública.
- **Visão do Produto, ADR-012 e Critérios de Aceite:**
  - Correção na regra de não independência para focar na origem factual/documental da informação e não apenas no veículo publicador.
  - Ajuste do status de VZ-009 para “próxima etapa funcional após fechamento do review”.

### Adicionado
- **ADR-012 (`docs/adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md`):**
  - Proposta arquitetural e editorial para navegação documental profunda inspirada em acervos e relatórios periciais (como a extração do IPJ-A nº 3298613/2026 pelo MasterWhats).
  - Fixação de diretrizes de rastreabilidade pontual via `evidence_sources.locator` e `evidence_sources.excerpt` no schema existente do MVP.
  - Fixação de limites éticos e jurídicos: ausência de licença no repositório consultado veda reuso de código/dados; menção não implica interlocução nem culpa; transcrição é representação secundária; proibição de fabricação de dados ou elevação automática de grau.
- **Documento de Continuidade e Baseline (`docs/11-continuidade-e-baseline.md`):**
  - Síntese técnica do baseline comprovado com ancoragem concreta no commit `309ee8282203f18f8f66a5217d8a004edfcd0e66` (2026-09-10).
  - Discriminação exata do tratamento dos Graus D (público e elegível) e E (corrigido: público não-elegível; ambíguo: quarentena) no seed curado.
  - Direcionamento para o próximo passo funcional (VZ-009 após fechamento do review).
- **Consolidação de Documentação:**
  - Atualização de `docs/00-visao-do-produto.md`, `docs/08-roadmap.md`, `docs/09-backlog-inicial.md` e `docs/10-criterios-de-aceite.md` delimitando o estado implementado, o planejado no MVP (com VZ-020 e VZ-021 explicitados como dependências) e as propostas de evolução pós-MVP (VZ-022 e VZ-023).
