# Roadmap

O roadmap do VorcaroZAP organiza a evolução do produto em horizontes objetivos: o que já foi **comprovadamente implementado**, o que está **planejado no escopo do MVP** e o que constitui **evolução proposta** (pós-MVP).

## Fase 0 — especificação (Concluída)

Visão, modelo editorial, arquitetura, schema, ADRs, backlog e escopo econômico alinhados.

## Fase 1 — fundação Go (Concluída)

CLI, config com defaults seguros, SSR com Templ, SQLite WAL (`modernc.org/sqlite`), migrations com Goose, Compose/Caddy, layout mobile-first e página base. (VZ-001, VZ-002, VZ-003).

## Fase 2 — núcleo editorial e importação (Concluída)

- **Concluído:** Entidades, relações, claims com disposição/elegibilidade métrica, evidence_sources moderáveis, sources com acessibilidade, estados, importador XLSX com `origin = curated_seed`, mapeamento versionado em YAML (`config/import-mapping-v1.yaml`) e idempotência comprovada no SQLite. (VZ-004, VZ-005).

## Fase 3 — página pública, métricas e exportação (Concluída)

- **Concluído:** Listagem com busca e filtros multifacetados (`/pessoas`), detalhes de perfis com segregação de fontes e defesas (`/pessoas/{slug}`), página canônica de metodologia e critérios (`/metodologia`), motor de métricas por estado ativo (`public_claims_view`), Home pública com indicadores e distribuições integrados (`/`), e exportação XLSX derivada do banco SQLite com subcomando CLI e download HTTP (`/exportar/base.xlsx`). (VZ-006, VZ-007, VZ-008, VZ-009).

## Fase 4 — monitoramento OpenRouter (Concluída)

- **Concluído:** Verificador GET seguro e limitado de integridade e acessibilidade de fontes externas (`internal/sourcecheck`), proteção rigorosa contra SSRF e DNS rebinding, conexão restrita a IP validado, limites conservadores de bytes, tempo e redirecionamentos, tabela de motivos técnicos, persistência isolada no SQLite e política editorial completa por procedência (`SOURCE_NOT_CHECKED_POLICY_*` no config) conforme ADR-006. (VZ-020).
- **Concluído:** `ResearchProvider`/OpenRouter e fundação da descoberta via `openrouter:web_search` (`internal/research` e `internal/research/openrouter`), interface limpa desacoplada do domínio, cliente HTTP via `net/http` padrão, instruções defensivas no prompt contra prompt injection, parâmetros conservadores de busca/custos e extração de citações e uso de tokens. (VZ-010).
- **Concluído:** Schema estruturado de candidatos (`monitoring_runs` e `monitoring_candidates`), Structured Outputs com JSON Schema estrito, normalização pura reutilizável (`internal/normalize`), fingerprint SHA-256 versionado v1, deduplicação auditável intra e cross-run e serviço de ingestão (`internal/monitoring`), conforme [ADR-013](adr/ADR-013-monitoring-runs-and-candidates-deduplication.md). (VZ-011).
- **Concluído:** Gate estrutural puro em Go (`internal/domain`), gate semântico via OpenRouter (`Verify`) com JSON Schema estrito sem ferramentas externas, política determinística Go por Grau A–E, histórico imutável em `semantic_evaluations`, materialização transacional atômica curta em SQLite (`sources`, `relationships`, `claims`, `evidence`, `evidence_sources`) e integração comprovada à visualização pública (`public_claims_view`) e métricas, conforme [ADR-014](adr/ADR-014-gate-estrutural-gate-semantico-e-politica-de-publicacao-automatica.md). (VZ-012).
- **Concluído:** Lock distribuído com lease SQLite (`monitoring_locks`), renovação periódica com cancelamento reativo, cálculo determinístico de janela incremental UTC, contabilidade e gestão de orçamento baseada no custo real faturado pelo OpenRouter (`usage.cost`) em micro-USD, parada graciosa em `partial` e comando CLI `vorcarozap monitor --query "..."`, conforme [ADR-015](adr/ADR-015-lock-de-execucao-janela-incremental-e-limites-de-custo-llm.md). (VZ-013).

## Fase 5 — painel simples e moderação humana (Em andamento no MVP)

- **Concluído:** Proteção simples do `/admin` por HTTP Basic Authentication nativa no monólito Go (`ADMIN_USER` e `ADMIN_PASSWORD_HASH`), validação rigorosa de hash bcrypt sem suporte a senha em texto puro, mitigação de timing attacks via constante temporal e execução incondicional de bcrypt, cabeçalhos restritivos de cache (`Cache-Control: no-store`, `Vary: Authorization`), comportamento fail-closed (desabilitado responde 404 sem desafio) e página SSR mínima de confirmação, conforme [ADR-016](adr/ADR-016-protecao-simples-do-admin-por-basic-auth-e-bcrypt.md). (VZ-014).
- **Próximo item:** Painel simples para listagem e inspeção de fontes, evidências e candidatos (VZ-015 — Fase 5).
- **Planejado:** Ações de moderação com decisão atômica por XOR (claim ou evidence_source), quarentena automática ao perder último suporte ativo (VZ-021), restauração/aprovação de quarentena, invalidação imediata de visualizações e métricas, e acionamento manual do monitor. (VZ-015, VZ-016, VZ-017, VZ-021).

## Fase 6 — deploy MVP e validação econômica (Planejado no MVP)

Deploy em Oracle VPS, HTTPS via Caddy, agendamento de cron do monitor, volume persistente SQLite com backup operacional, canal de correção e checklist de smoke test completo com suite table-driven. (VZ-018, VZ-019).

## Fase 7 — maturidade e evolução proposta (Pós-MVP)

- **Evolução Documental ([ADR-012](adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md)):**
  - VZ-022: Referenciação aprofundada de laudos e peças (locators cirúrgicos de página e figura sem alterar o schema do MVP);
  - VZ-023: Viewer SSR de documentos e sequências contextuais (avaliação técnica de viabilidade, auditoria de integridade e conformidade de direitos/licenças).
- **Maturidade de Plataforma:** Histórico público de alterações, snapshots integrais, múltiplos usuários com autenticação reforçada (MFA/RBAC), contraditório estruturado, observabilidade aprofundada, backup externo e eventual migração para PostgreSQL conforme gatilhos de volume.

O go-live não depende de cobertura percentual, E2E, workflow multiusuário ou parecer formal completo; depende do baseline técnico/editorial e do smoke test.
