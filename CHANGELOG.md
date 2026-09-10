# Changelog

Todas as alterações notáveis deste projeto são registradas neste documento. O formato baseia-se em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/).

## [Não lançado] — VZ-020: Verificador GET Seguro e Política de Acessibilidade de Fontes

### Adicionado
- **Módulo Verificador de Fontes (`internal/sourcecheck`):**
  - Implementação de adaptador estreito e seguro para execução de HTTP GET real (nunca HEAD) com limites conservadores (timeout de 5s, connect/header de 3s, até 3 redirecionamentos e leitura de corpo limitada a 64 KB).
  - Proteção robusta contra Server-Side Request Forgery (SSRF) no caminho efetivo da conexão: bloqueio irrestrito de loopback (IPv4/IPv6, incluindo IPv4-mapped IPv6 `::ffff:127.0.0.1`), endereços privados (RFC 1918), link-local, multicast, faixas reservadas e metadados de nuvem (`169.254.169.254`, `metadata.google.internal`).
  - Prevenção contra DNS Rebinding através de conexão `net.Dialer` direta ao primeiro IP público validado, preservando validação estrita de certificados TLS e SNI/ServerName.
  - Revalidação de cada redirecionamento contra SSRF, esquema HTTP/HTTPS, portas e ausência de credenciais embutidas.
  - Isolamento de proxies do ambiente (`Proxy: nil`) e proibição de cookies ou cabeçalhos sensíveis.
  - Sanitização de URLs para log (`SanitizeURLForLogging`) mascarando parâmetros sensíveis na query string (`token`, `key`, `auth`, `secret`).
  - Tabela determinística de causas técnicas (`TechnicalReason`) mapeando códigos HTTP, erros de DNS (NXDOMAIN vs transitório), TLS, timeout e cancelamento.
- **Política Editorial de `source_access_status` conforme ADR-006:**
  - `EvaluateSourceAccessGate`: deliberação do gate de acessibilidade considerando procedência (`openrouter`, `curated_seed`, `admin`), status técnico e variáveis de configuração:
    - `reachable`: atende o requisito técnico de acessibilidade (`GateDecisionAllow`), sem autorizar publicação por si só.
    - `unreachable`: erro definitivo ou bloqueio de segurança leva a quarentena obrigatória (`GateDecisionQuarantine`).
    - `cited_by_provider`: citação pela LLM não equivale a verificação própria -> quarentena (`GateDecisionQuarantine`).
    - `not_checked`: diferenciação por origem: `curated_seed` preserva `initial_state` do mapeamento com política `allow`, enquanto `openrouter` vai preventivamente para quarentena com política `quarantine`.
  - `ResolveStatusUpdate`: transição de estado não destrutiva no banco, impedindo que falhas temporárias (timeout, 429, 5xx) apaguem silenciosamente comprovações prévias de `reachable`.
- **Configuração Operacional (`internal/config`, `.env.example`):**
  - Incorporação das variáveis de política `SOURCE_NOT_CHECKED_POLICY_OPENROUTER` (padrão: `quarantine`) e `SOURCE_NOT_CHECKED_POLICY_CURATED_SEED` (padrão: `allow`), além de `SOURCE_CHECK_TIMEOUT` (padrão: `5s`).
- **Persistência Segura (`internal/store/queries/editorial.sql`, `internal/sourcecheck/updater.go`):**
  - Novas queries SQLC `UpdateSourceAccessStatus` e `ListSourcesByAccessStatus`.
  - Persistência desacoplada da rede (`PersistCheckResult` e `CheckAndPersist`), garantindo que chamadas de rede nunca ocorram dentro de transações do SQLite.
- **Suíte de Testes Automatizados (`internal/sourcecheck/*_test.go`, `internal/config/*_test.go`):**
  - Testes table-driven para validação do método GET, classificação de status HTTP, detecção de erros conclusivos e inconclusivos.
  - Testes de bloqueio SSRF (IPv4, IPv6, loopback, redes privadas, metadados de nuvem, credenciais na URL e portas inválidas).
  - Testes de redirecionamentos (limite de saltos e interceptação de redirecionamento para IP privado).
  - Testes de resiliência a timeouts, cancelamento de contexto e limitação de leitura de corpos extensos.
  - Teste de injeção de resolver para validação de erros de DNS e rebinding.
  - Testes da matriz editorial e regras de transição.
  - Teste de integração com SQLite em memória descartável.

## [0.4.0] — VZ-009: Exportação XLSX Derivada da Base Pública

### Adicionado
- **Motor de Exportação XLSX (`internal/exporter`):**
  - Implementação da geração determinística de planilhas XLSX via `github.com/xuri/excelize/v2` a partir de consultas exclusivas ao estado público do SQLite (`public_claims_view`).
  - Extração transacional atômica e consistente via `BeginTx(ctx, &sql.TxOptions{ReadOnly: true})`, encerrando a transação de leitura antes da montagem do XLSX.
  - Segregação dos dados em 4 abas canônicas: `Entidades`, `Alegações e Relações`, `Evidências e Fontes`, e `Metodologia e Critérios`.
  - Inclusão da coluna **"Limites Contextuais / Ressalvas"** (`context_limits`) na aba de alegações, preservando ressalvas e contrapontos exibidos no site.
  - Aba de metodologia alinhada à metodologia canônica do produto: Relevância 1–5 mensurando alcance institucional e interesse público (sem aferição de suspeição/culpa), Graus A–E canônicos e seção discriminando as regras da base inicial curada (Grau D publicado elegível; Grau E retificado como contexto não elegível; Grau E ambíguo quarentenado).
  - Proteção estrita contra Formula Injection (CSV/XLSX injection) gravando todas as células de texto com `SetCellStr` e neutralizando caracteres disparadores (`=`, `+`, `-`, `@`, tab).
  - Preservação da distinção de elegibilidade métrica: alegações públicas com `metric_eligible = false` (ex.: Grau E corrigido) continuam na exportação com a coluna `Elegível nas Métricas = Não`, enquanto itens de rede figuram com `Sim`.
  - Exclusão estrita de dados em quarentena (21 registros legados), alegações rejeitadas, arquivadas ou metadados de processamento interno.
  - Formatação visual institucional, larguras calibradas e congelamento de painéis de cabeçalho.
- **Subcomando CLI de Exportação (`cmd/vorcarozap`):**
  - Comando `vorcarozap export [--out <caminho.xlsx>]` para geração de arquivo diretamente pelo terminal.
- **Download HTTP e Integração na Interface Web (`internal/web`, `web/pages`, `web/components`):**
  - Endpoint `GET /exportar/base.xlsx` com geração prévia em memória (`bytes.Buffer`), garantindo que em falha retorne HTTP 500 sem cabeçalho `Content-Disposition`.
  - Redirect amigável `GET /exportar` -> `/exportar/base.xlsx`.
  - Botão de download no cabeçalho de resultados de `/pessoas`, no card de consulta da Home e link no menu de navegação.
- **Suíte de Testes Automatizados:**
  - Testes unitários de sanitização e proteção contra formula injection em `internal/exporter/exporter_test.go`.
  - Testes de integridade de abas, células e cabeçalhos em cenário de base vazia.
  - Testes de exclusão de estados não públicos, validação de elegibilidade métrica e preservação de `context_limits`.
  - Teste de tratamento de erro do endpoint de exportação (`TestExportEndpoint_ErrorHandling`), garantindo retorno 500 sem cabeçalho de anexo em falha de extração.
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
