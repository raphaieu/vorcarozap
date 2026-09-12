# Backlog inicial priorizado

O backlog distingue o que está **concluído**, o que está **planejado no MVP** e o que constitui **evolução proposta pós-MVP**. O próximo item para implementação após a conclusão de VZ-021 é **VZ-017**.

## P0 — Núcleo do MVP

| ID | P | Entrega | Status |
|---|---|---|---|
| VZ-001 | P0 | Estrutura Go, Dockerfile enxuto, Compose e Caddy | Concluído (Fase 1) |
| VZ-002 | P0 | SQLite com WAL, Goose e schema inicial | Concluído (Fase 1) |
| VZ-003 | P0 | SSR com Templ, layout mobile-first e página base | Concluído (Fase 1) |
| VZ-004 | P0 | Domínio, schema relacional e queries tipadas | Concluído (Fase 2) |
| VZ-005 | P0 | Importador da planilha inicial (`origin = curated_seed`) | Concluído (Fase 2) |
| VZ-006 | P0 | Listagem, busca, filtros e detalhe de pessoas/claims | Concluído (Fase 3) |
| VZ-007 | P0 | Metodologia pública, fontes rastreáveis e linguagem humana para Graus A a E | Concluído (Fase 3) |
| VZ-008 | P0 | Engine de métricas públicas por estado ativo e integração na Home | Concluído (Fase 3) |
| VZ-009 | P0 | Exportação XLSX derivada do banco e download público | Concluído (Fase 3) |
| VZ-020 | P0 | Verificador GET seguro/limitado e política de `source_access_status` (incorporar `SOURCE_NOT_CHECKED_POLICY_*` ao config; **dependência técnica dos gates de publicação automática**) | Concluído (Fase 4) |
| VZ-010 | P0 | ResearchProvider/OpenRouter e esteira de web search | Concluído (Fase 4) |
| VZ-011 | P0 | Schema estruturado, normalização de dados e deduplicação | Concluído (Fase 4) |
| VZ-012 | P0 | Gate estrutural Go, gate semântico LLM e política automática A/B/C versus D/E | Concluído (Fase 4) |
| VZ-013 | P0 | Lock de execução, janela incremental e limites de custo de LLM | Concluído (Fase 4) |
| VZ-014 | P0 | Proteção simples do `/admin` por autenticação HTTP | Concluído (Fase 5) |
| VZ-015 | P0 | Painel simples para listagem e inspeção de fontes, evidências e candidatos | Concluído (Fase 5) |
| VZ-016 | P0 | Ações de moderação: desaprovar, restaurar e aprovar quarentena | Concluído (Fase 5) |
| VZ-021 | P0 | Moderação com integridade transacional XOR (claim ou evidence_source) e quarentena imediata ao perder último suporte ativo (**agrupado à moderação**) | Concluído (Fase 5) |
| VZ-017 | P0 | Invalidação imediata de visualizações, métricas e exportação após moderação | Planejado no MVP |
| VZ-018 | P0 | Deploy VPS, cron de monitoramento, persistência SQLite e smoke checklist | Planejado no MVP |
| VZ-019 | P0 | Testes table-driven de publicação/quarentena, bloqueio por fingerprint rejeitado e métricas por estado/elegibilidade | Planejado no MVP |

## P1 — Evolução Proposta (Pós-MVP)

Itens de enriquecimento e maturidade inspirados nas discussões de navegação documental profunda ([ADR-012](adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md)) e escala operacional. **Não fazem parte do MVP imediato.**

| ID | P | Entrega | Status |
|---|---|---|---|
| VZ-022 | P1 | Referenciação aprofundada de laudos periciais e peças (locators cirúrgicos de página e figura em `evidence_sources`, mantendo schema do MVP sem criar novas tabelas) | Proposta (Pós-MVP) |
| VZ-023 | P1 | Viewer SSR de documentos e sequências contextuais (avaliação técnica de viabilidade em Go/Templ, auditoria de integridade e conformidade estrita de licenças/permissões) | Proposta (Pós-MVP) |
| VZ-024 | P1 | Histórico público de alterações editoriais e trilha de auditoria | Proposta (Pós-MVP) |
| VZ-025 | P1 | Módulo de contraditório estruturado e submissão pública de manifestações de defesa | Proposta (Pós-MVP) |
| VZ-026 | P1 | Gestão multiusuário do painel com perfis e autenticação reforçada (MFA/RBAC) | Proposta (Pós-MVP) |

## P2 — Escala e Distribuição Futura

API pública documentada, crawler e verificador distribuído, suporte a réplicas e migração opcional para PostgreSQL conforme gatilhos de volume.
