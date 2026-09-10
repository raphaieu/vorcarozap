# Changelog

Todas as alterações notáveis deste projeto são registradas neste documento. O formato baseia-se em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/).

## [Não lançado] — Ciclo de Saneamento da Apresentação e Consolidação da Visão Documental

### Modificado
- **Apresentação pública da Home (`web/pages/home.templ`):**
  - Rótulo “Fontes Auditadas” alterado para “Fontes Citadas”, refletindo com exatidão a contagem de fontes ativas distintas.
  - Rótulo “Pessoas na Rede” alterado para “Entidades na Rede”, incorporando organizações ao lado de pessoas físicas.
  - Rótulo “Vínculos Qualificados” alterado para “Alegações Elegíveis”, alinhando a nomenclatura à unidade de contagem (`claim`).
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

### Adicionado
- **ADR-012 (`docs/adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md`):**
  - Proposta arquitetural e editorial para navegação documental profunda inspirada em acervos e relatórios periciais (como a extração do IPJ-A nº 3298613/2026 pelo MasterWhats).
  - Fixação de diretrizes de rastreabilidade pontual via `evidence_sources.locator` e `evidence_sources.excerpt` no schema existente do MVP.
  - Fixação de limites éticos e jurídicos: ausência de licença no repositório consultado veda reuso de código/dados; menção não implica interlocução nem culpa; transcrição é representação secundária; proibição de fabricação de dados ou elevação automática de grau.
- **Documento de Continuidade e Baseline (`docs/11-continuidade-e-baseline.md`):**
  - Síntese técnica do baseline comprovado, testes executados, decisões editoriais e direcionamento para o próximo passo funcional (VZ-009).
- **Consolidação de Documentação:**
  - Atualização de `docs/00-visao-do-produto.md`, `docs/08-roadmap.md`, `docs/09-backlog-inicial.md` e `docs/10-criterios-de-aceite.md` delimitando o estado implementado, o planejado no MVP (com VZ-020 e VZ-021 explicitados como dependências) e as propostas de evolução pós-MVP (VZ-022 e VZ-023).
