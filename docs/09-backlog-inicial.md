# Backlog inicial priorizado

| ID | P | Entrega | Status |
|---|---|---|---|
| VZ-001 | P0 | módulo Go, CLI, config e build | Concluído (Fase 1) |
| VZ-002 | P0 | SQLite/WAL, migrations e volume | Concluído (Fase 1) |
| VZ-003 | P0 | Compose/Caddy e página base mobile | Concluído (Fase 1) |
| VZ-004 | P0 | schema de entidades, claims com disposição/métrica, evidence_sources moderáveis e sources com acesso | Concluído |
| VZ-005 | P0 | importador XLSX `curated_seed` com dry-run, mapeamento versionado enriquecido e idempotência | |
| VZ-006 | P0 | listagem, busca, filtros e detalhe |
| VZ-007 | P0 | fontes, metodologia e linguagem A–E |
| VZ-008 | P0 | engine de métricas públicas por estado ativo |
| VZ-009 | P0 | exportação XLSX derivada do banco |
| VZ-010 | P0 | ResearchProvider/OpenRouter e web search |
| VZ-011 | P0 | schema estruturado, normalização e dedupe |
| VZ-012 | P0 | gate estrutural Go, gate semântico LLM e política automática A/B/C versus D/E |
| VZ-013 | P0 | lock, janela incremental e limites de custo |
| VZ-014 | P0 | proteção simples do `/admin` |
| VZ-015 | P0 | painel de fontes/evidências/candidatos |
| VZ-016 | P0 | desaprovar, restaurar e aprovar quarentena |
| VZ-017 | P0 | invalidação imediata de página/métricas/export |
| VZ-018 | P0 | deploy VPS, cron, persistência e smoke checklist |
| VZ-019 | P0 | testes table-driven de publicação/quarentena, fingerprint rejeitado e métricas por estado/elegibilidade |
| VZ-020 | P0 | verificador GET seguro/limitado e política de `source_access_status` (incorporar `SOURCE_NOT_CHECKED_POLICY_*` ao config) |
| VZ-021 | P0 | moderação XOR de claim/evidence_source e quarentena ao perder último suporte |

## P1

Testes automatizados adicionais, histórico público, snapshots, auditoria, usuários/papéis, MFA, contraditório estruturado, métricas históricas e alertas.

## P2

API pública, crawler/fetch próprio, múltiplas instâncias, PostgreSQL, busca avançada/FTS e E2E.
