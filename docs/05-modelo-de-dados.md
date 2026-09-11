# Modelo de dados

## Núcleo do MVP

- `entities`: identidade, tipo, nome, slug, cargo, resumo, relevância e justificativa.
- `entity_aliases`: aliases normalizados.
- `cases`: recorte editorial.
- `relationships`: sujeito, alvo/caso, tipo, síntese e limites.
- `claims`: relação/caso, proposição, atribuição, `origin`, grau A–E, `disposition`, `metric_eligible`, estado e datas.
- `evidence`: claim, resumo e tipo.
- `sources`: título, veículo/autor, URLs, publicação/acesso, tipo, `source_access_status` e dados da última verificação.
- `evidence_sources`: evidence, source, trecho, localizador, papel e estado; ID próprio permite múltiplos trechos e moderação do uso específico.
- `monitoring_runs`: janela, providers/modelos de descoberta e avaliação, estado operacional, tokens/custo, erro e timestamps.
- `monitoring_candidates`: monitoring run, fingerprint global, payload normalizado, entidade encontrada, confiança técnica, resultados dos gates e estado editorial.
- `moderation_decisions`: `claim_id` anulável, `evidence_source_id` anulável, ação, motivo, fingerprint, ator único e data; exatamente um alvo.
- `import_runs`: arquivo/hash, `origin = curated_seed`, versão do mapeamento, status e relatório.
- `exports`: arquivo, geração e hash opcional.

`evidence_grade` existe somente em `claims`. Grau de relação é derivado. `technical_confidence` existe somente no candidato/run.

O resultado do gate semântico é armazenado de forma auditável com os campos `identity_match`, `claim_supported`, `claim_overstates_source`, `attribution_explicit`, `grade_compatible`, `contains_illicit_inference`, `uncertainties` e `recommended_action`, além de modelo/schema/timestamp. A decisão final e os motivos pertencem à política Go.

```mermaid
erDiagram
  ENTITIES ||--o{ RELATIONSHIPS : subject
  CASES ||--o{ RELATIONSHIPS : context
  RELATIONSHIPS ||--o{ CLAIMS : frames
  CLAIMS ||--o{ EVIDENCE : has
  EVIDENCE ||--o{ EVIDENCE_SOURCES : cites
  SOURCES ||--o{ EVIDENCE_SOURCES : referenced
  MONITORING_RUNS ||--o{ MONITORING_CANDIDATES : produces
  IMPORT_RUNS ||--o{ CLAIMS : creates
  CLAIMS o|--o{ MODERATION_DECISIONS : moderated_by
  EVIDENCE_SOURCES o|--o{ MODERATION_DECISIONS : moderated_by
```

## Estados

Claims possuem estado `quarantined`, `published`, `rejected` ou `archived`. `evidence_sources.status` aceita `active` ou `rejected`. `sources` não têm rejeição editorial global: representam o documento e registram apenas disponibilidade/acessibilidade. A visibilidade pública é uma consulta explícita: claim publicado precisa de ao menos um `evidence_source` ativo com papel `supports`.

`monitoring_runs` não usa estados editoriais. Seu `status` aceita `pending`, `running`, `partial`, `completed` ou `failed`.

## Constraints e índices

- foreign keys e `CHECKs` para enums/faixas;
- `claims.disposition` aceita `supports_link`, `possible_link`, `contradicts_link`, `context_only` ou `correction`; `metric_eligible` é booleano;
- nomes/aliases normalizados indexados;
- índices por estado, grau, relevância, atualização e fingerprint;
- XOR em relacionamentos com alvo entidade ou caso;
- URL/fingerprint sugerem duplicidade, mas pessoas nunca são mescladas automaticamente;
- decisões rejeitadas preservam fingerprint para bloquear republicação automática;
- `moderation_decisions` aplica `CHECK ((claim_id IS NOT NULL) <> (evidence_source_id IS NOT NULL))`;
- `moderation_decisions.action` aceita `approve`, `reject`, `restore` ou `archive` conforme o tipo/estado do alvo;
- rejeição do último `evidence_source` ativo com papel `supports` coloca o claim em quarentena na mesma transação;
- `claims.origin` diferencia ao menos `openrouter`, `curated_seed` e `admin`;
- `monitoring_candidates.monitoring_run_id` é obrigatório; a importação curada cria claims diretamente e não simula um monitoring run;
- `import_runs.mapping_version` é obrigatório em importações aplicadas e referencia configuração versionada;
- decisões automáticas registram aprovação/reprovação de cada gate e a regra da matriz A–E aplicada.

## Representação temporal (VZ-004)

- Todos os campos de timestamp utilizam strings formatadas em UTC no padrão ISO-8601 (`YYYY-MM-DDTHH:MM:SS.fZ`), compatível com RFC-3339Nano e com as funções de data do SQLite.
- Valores default em inserção são gerados no SQLite via `strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`.
- Atualizações de `updated_at` são tratadas explicitamente nas queries/código da aplicação, sem recorrer a triggers desnecessárias.

## Decisão de modelagem: claims e relacionamentos (VZ-004)

- Todo `claim` referencia obrigatoriamente um `relationship_id` (`RELATIONSHIPS ||--o{ CLAIMS : frames`).
- A entidade `relationships` implementa a restrição XOR (`target_entity_id preenchido XOR case_id preenchido`) e bloqueio de autorrelação (`target_entity_id != subject_entity_id`), ancorando a alegação ao sujeito e ao seu contexto (outra entidade ou um caso de investigação).
- Isso elimina duplicidade e ambiguidade entre um eventual `case_id` direto no claim e o alvo definido na relação, preservando a hierarquia relacional e simplificando consultas de rede.
- Todas as chaves estrangeiras contêm cláusulas `ON DELETE` explícitas (`RESTRICT` por padrão para histórico e integridade referencial, `CASCADE` para aliases e `SET NULL` para import_runs).

## Métricas

Views/queries de rede agregam `claims.status = published AND claims.metric_eligible = true`. Contexto/correção público permanece na consulta e XLSX, mas não conta como contato/vínculo. `public_metric_snapshots` fica opcional para histórico P1; o MVP calcula o estado corrente.

## Importação inicial e hardening (VZ-005)

O mapeamento legado para o modelo A–E não fica codificado em condicionais do importador. `config/import-mapping-v1.yaml` enumera rótulos e define `grade`, `initial_state`, `disposition` e `metric_eligible`. Cada execução persiste `mapping_version`; rótulo ausente ou não mapeável leva a quarentena.

A migration `00003_import_foundation_hardening.sql` introduziu refinamentos estruturais para garantir fidelidade editorial e idempotência real:
- **Idempotência estrita:** índice único parcial `uq_import_runs_applied` em `import_runs(file_hash, mapping_version)` para execuções onde `is_dry_run = 0 AND status IN ('running', 'completed', 'partial')`. Isso impede concorrência e duplicidade a nível de banco, permitindo repetição de execuções `failed` e simulações com `--dry-run`.
- **Fontes sem metadados inventados:** a planilha inicial contém apenas URLs, sem fornecer título de artigo, autor ou veículo em colunas segregadas. O schema de `sources` foi ajustado para permitir `title` e `publisher_or_author` como strings vazias/desconhecidas para a origem `curated_seed`, com status `not_checked`, evitando invenções artificiais de metadados. A coluna `canonical_url` possui índice único para deduplicação determinística.
- **Resumo editorial não é citação literal:** a coluna "O que está documentado" é uma síntese editorial e alimenta exclusivamente `evidence.summary`. A tabela `evidence_sources` aceita `excerpt` vazio no seed legado, preservando a distinção pública futura entre resumo editorial da equipe e trecho literal extraído da fonte.

## Defesa e contestação

No MVP, defesa/contestação reutiliza `evidence_sources.role = contradicts`. A consulta pública agrupa esses vínculos em seção própria. `defense_statements` continua P1.

## Acessibilidade da fonte

`sources.source_access_status` aceita `cited_by_provider`, `reachable`, `unreachable` ou `not_checked`, acompanhado por `source_access_checked_at`, status HTTP final anulável e código de erro normalizado anulável. Citação do OpenRouter começa em `cited_by_provider`; source do `curated_seed` começa em `not_checked`. GET seguro pode promover ambas a `reachable`. Erro definitivo, como 404/410, vira `unreachable`; timeout, 429, 5xx ou bloqueio inconclusivo vira `not_checked`. `unreachable` põe o candidato/claim novo em quarentena; `not_checked` segue a política diferenciada por origem definida no [ADR-006](adr/ADR-006-fontes-e-rastreabilidade.md) (quarentena para OpenRouter; preserva initial_state para curated_seed).

## Schema de monitoramento e candidatos estruturados (VZ-011)

A migration `00006_monitoring_runs_and_candidates.sql` e a [ADR-013](adr/ADR-013-monitoring-runs-and-candidates-deduplication.md) formalizam a separação estrita entre o ciclo operacional de descoberta e os estados editoriais:
- **`monitoring_runs`**: rastreabilidade operacional de cada execução de descoberta, persistindo janela/query de pesquisa, provider/modelo utilizado, contagem de tokens (`prompt_tokens`, `completion_tokens`, `total_tokens`), chamadas de busca (`web_search_calls`), custo estimado, resumo técnico e estado operacional (`pending`, `running`, `partial`, `completed`, `failed`).
- **`monitoring_candidates`**: persistência de extrações factuais estruturadas via Structured Outputs (JSON Schema estrito), com normalização pura Unicode NFC/espaçamento, URL canônica, fingerprint SHA-256 versionado v1 (`v1:<sha256(canonical_url|normalized_entity_name|normalized_proposition|normalized_excerpt)>`), grau sugerido A–E, confiança técnica em `[0.0, 1.0]`, e colunas para suporte aos gates (`structural_gate_passed`, `semantic_gate_passed`, `policy_action`).
- **Deduplicação auditável:** repetições intra-run e cross-run são identificadas deterministicamente por fingerprint sem descarte de histórico: candidatos duplicados recebem `is_duplicate = 1`, `duplicate_reason` explícito e apontam para `canonical_candidate_id`.
- **Isolamento da fronteira pública:** todo candidato inicia em estado editorial `quarantined`. A ingestão não cria claims, entidades, relacionamentos ou fontes, mantendo a view `public_claims_view` e as métricas públicas 100% blindadas.

## Gates estrutural e semântico, histórico de avaliações e materialização editorial (VZ-012)

A migration `00007_monitoring_gates_and_verification.sql` e a [ADR-014](adr/ADR-014-gate-estrutural-gate-semantico-e-politica-de-publicacao-automatica.md) estendem o schema de monitoramento para suportar resolução unívoca, histórico auditável de avaliações semânticas e materialização editorial transacional curta:
- **Extensão relacional de `monitoring_candidates`:** campos `target_entity_name`, `normalized_target_entity_name`, `case_name`, `normalized_case_name`, `relationship_type` com restrição XOR textual e de chaves resolvidas (`resolved_subject_entity_id`, `resolved_target_entity_id`, `resolved_case_id`, `published_claim_id`), proibição de autorrelação (`chk_monitoring_candidates_no_self_relation`) e colunas de auditoria de motivos em JSON (`structural_gate_reasons`, `semantic_gate_reasons`, `policy_reasons`).
- **`semantic_evaluations`**: tabela imutável de histórico de validações pela LLM via OpenRouter Structured Outputs (JSON Schema estrito), contendo `monitoring_candidate_id`, `provider`, `model`, `schema_version`, os 6 critérios semânticos booleanos (`identity_match`, `claim_supported`, `claim_overstates_source`, `attribution_explicit`, `grade_compatible`, `contains_illicit_inference`), array JSON de `uncertainties`, `recommended_action` e payload bruto `raw_response`.
- **Materialização editorial atômica:** publicação automática ocorre em transação SQLite curta apenas quando autorizada pela Política Go (Graus A e B aprovados nos 2 gates -> `supports_link`; Grau C aprovado nos 2 gates com limites e cautela -> `possible_link`), criando/reutilizando `sources` (atualizando `source_access_status`), `relationships`, `claims` (`status = 'published'`, `metric_eligible = 1`), `evidence` e `evidence_sources` (`role = 'supports'`), e vinculando o candidato ao `published_claim_id`.

## Lock com lease SQLite, janela incremental e limites de custo de LLM (VZ-013)

A migration `00008_monitoring_lock_and_budget.sql` e a [ADR-015](adr/ADR-015-lock-de-execucao-janela-incremental-e-limites-de-custo-llm.md) formalizam a proteção contra execuções concorrentes, o avanço temporal determinístico e o controle orçamentário granular baseado no custo real retornado pelo OpenRouter:
- **`monitoring_locks`**: tabela de controle de concorrência com lease SQLite (`name PRIMARY KEY`, `holder`, `acquired_at`, `expires_at`, `updated_at` e índice em `expires_at`). Garante exclusividade mútua entre execuções manuais de CLI e jobs cron agendados com renovação periódica em segundo plano e expiração automática (*fail-safe* para processos interrompidos abruptamente).
- **Extensão de `monitoring_runs`**:
  - `window_start` / `window_end`: limites temporais UTC da janela examinada na descoberta, calculados deterministicamente a partir do `window_end` da última run bem-sucedida (`status = 'completed'`); runs `partial` ou `failed` não avançam o watermark;
  - `discovery_tokens` / `discovery_cost_microusd` (`discovery_cost` decimal derivado): tokens e custo real faturado pelo OpenRouter na etapa de busca web e extração de candidatos;
  - `verification_tokens` / `verification_cost_microusd` (`verification_cost` decimal derivado): tokens e custo real acumulados atomicamente nas chamadas individuais do gate semântico;
  - `total_cost_microusd` (`total_cost` decimal derivado): soma consolidada em inteiros do custo da run (`discovery_cost_microusd + verification_cost_microusd`);
  - `verifications_count`: contagem de verificações semânticas executadas.
- **Extensão de `semantic_evaluations`**: colunas `prompt_tokens`, `completion_tokens`, `total_tokens` e `cost_microusd` (`cost` decimal derivado) para auditoria financeira precisa por avaliação documental individual gravada na mesma transação atômica.
- **Persistência Segura em Inteiros:** todos os limites orçamentários (`MONITOR_MAX_COST_PER_RUN_USD`, `MONITOR_MAX_COST_PER_DAY_USD`) e custos reais são administrados e somados no SQLite em micro-USD inteiros (`int64`, arredondamento para cima via `math.Ceil(cost * 1_000_000)`), garantindo precisão absoluta e imunidade a subcontagem após reinício de processo.

## Futuro

`users`, `sessions`, `publication_revisions`, `editorial_summaries`, `source_snapshots`, `source_relationships`, auditoria completa e contraditório estruturado entram quando painel/equipe amadurecerem.
