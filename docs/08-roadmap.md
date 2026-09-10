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
- **Próximo item:** Verificador GET seguro/limitado e política de `source_access_status` (VZ-020 — dependência técnica dos gates de publicação automática da Fase 4).

## Fase 4 — monitoramento OpenRouter (Planejado no MVP)

ResearchProvider, web search, schema estruturado, normalização, deduplicação, verificador GET seguro de links com política por origem (VZ-020 como dependência técnica), gate estrutural em Go, segunda avaliação semântica via LLM, política Go A/B/C versus D/E, publicação/quarentena, estados operacionais, idempotência e limites de custo. (VZ-010, VZ-011, VZ-012, VZ-013, VZ-020).

## Fase 5 — painel simples e moderação humana (Planejado no MVP)

Proteção básica do `/admin`, listagem com filtros de moderação, detalhes de candidatos/evidências, moderação com decisão atômica por XOR (claim ou evidence_source), quarentena automática ao perder último suporte ativo (VZ-021), restauração/aprovação de quarentena, invalidação imediata de visualizações e métricas, e acionamento manual do monitor. (VZ-014, VZ-015, VZ-016, VZ-017, VZ-021).

## Fase 6 — deploy MVP e validação econômica (Planejado no MVP)

Deploy em Oracle VPS, HTTPS via Caddy, agendamento de cron do monitor, volume persistente SQLite com backup operacional, canal de correção e checklist de smoke test completo com suite table-driven. (VZ-018, VZ-019).

## Fase 7 — maturidade e evolução proposta (Pós-MVP)

- **Evolução Documental ([ADR-012](adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md)):**
  - VZ-022: Referenciação aprofundada de laudos e peças (locators cirúrgicos de página e figura sem alterar o schema do MVP);
  - VZ-023: Viewer SSR de documentos e sequências contextuais (avaliação técnica de viabilidade, auditoria de integridade e conformidade de direitos/licenças).
- **Maturidade de Plataforma:** Histórico público de alterações, snapshots integrais, múltiplos usuários com autenticação reforçada (MFA/RBAC), contraditório estruturado, observabilidade aprofundada, backup externo e eventual migração para PostgreSQL conforme gatilhos de volume.

O go-live não depende de cobertura percentual, E2E, workflow multiusuário ou parecer formal completo; depende do baseline técnico/editorial e do smoke test.
