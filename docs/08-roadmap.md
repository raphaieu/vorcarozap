# Roadmap

## Fase 0 — especificação

Visão, modelo editorial, arquitetura, schema, ADRs, backlog e escopo econômico alinhados. Segurança/testes avançados ficam rastreados sem bloquear desenvolvimento.

## Fase 1 — fundação Go

CLI, config, SSR, SQLite/WAL, migrations, Compose/Caddy, layout e smoke page.

## Fase 2 — núcleo editorial e importação

Entidades, relações, claims, evidências/fontes, estados, importador XLSX com `origin = curated_seed`, mapeamento legado versionado, deduplicação e exportação.

## Fase 3 — página pública e métricas

Home, cards, busca, filtros, detalhes, fontes, metodologia, métricas correntes e download.

## Fase 4 — monitoramento OpenRouter

ResearchProvider, web search, schema, gate estrutural, segunda avaliação semântica, política Go A/B/C versus D/E, publicação/quarentena, estados operacionais, idempotência e custo.

## Fase 5 — painel simples

Proteção de `/admin`, listagem/filtros, detalhes, desaprovação, restauração/aprovação de quarentena e execução manual do monitor.

## Fase 6 — deploy MVP

Oracle VPS, HTTPS, cron do monitor, volume SQLite, backup operacional, domínio, canal de correção e smoke completo.

## Fase 7 — maturidade

Testes direcionados, histórico/snapshots, múltiplos usuários, MFA/RBAC, observabilidade, backup externo e PostgreSQL conforme gatilhos.

O go-live não depende de cobertura, E2E, workflow multiusuário ou parecer formal completo; depende do baseline técnico/editorial e do smoke test.
