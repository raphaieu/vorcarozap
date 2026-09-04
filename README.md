# VorcaroZAP

Aplicação pública, investigativa e documental para organizar informações publicadas sobre Daniel Vorcaro, Banco Master e pessoas, organizações e acontecimentos relacionados. A plataforma combina monitoramento automatizado, fontes rastreáveis, classificação editorial, métricas públicas e pós-moderação humana.

> Projeto independente, sem vínculo com WhatsApp, Meta, Banco Master ou pessoas e instituições citadas. “WhatsApp” é marca de seu titular. A identidade usa referência cromática, sem reproduzir logotipo ou interface oficial.

## Estado

**Fase 0 — especificação consolidada.** O MVP prioriza o motor público: coleta via OpenRouter, normalização, validação, publicação automática controlada, métricas, consulta pública e painel simples para pós-moderação.

O arquivo `_notes/mapa-vorcaro-contatos-2026-09-03.xlsx` é um artefato público de pesquisa e base inicial. Estar no arquivo não equivale a culpa nem dispensa classificação e fonte na aplicação.

## MVP

- Go, SSR com `templ`/HTMX, SQLite e Caddy;
- importação da planilha inicial;
- monitoramento via OpenRouter/web search;
- validação determinística e estruturação por schema;
- publicação automática do conteúdo que cumprir os gates mínimos;
- quarentena automática do conteúdo ambíguo/incompleto;
- painel simples protegido para fontes, evidências e moderação;
- listagem, busca, filtros, detalhes e fontes;
- métricas públicas derivadas apenas dos registros ativos;
- download da planilha/base;
- deploy em Oracle VPS.

Não integram o MVP: RBAC multiusuário, MFA, workflow de dupla revisão, snapshots completos, observabilidade sofisticada, alta disponibilidade e cobertura ampla de testes.

## Fluxo

```mermaid
flowchart LR
  OR[OpenRouter + web search] --> V[Normalização e validação]
  V -->|válido| P[(Publicado)]
  V -->|ambíguo| Q[(Quarentena)]
  A[Painel simples] --> P
  A --> Q
  P --> W[Página pública]
  P --> M[Métricas]
  P --> X[XLSX]
```

## Comandos planejados

```bash
vorcarozap serve
vorcarozap migrate
vorcarozap import --file arquivo.xlsx --dry-run
vorcarozap import --file arquivo.xlsx
vorcarozap monitor
vorcarozap export
```

## Validação econômica

Sem meta de cobertura no MVP. Gate inicial:

```bash
go fmt ./...
go vet ./...
go build ./...
```

Migrations, importação, monitoramento, moderação e jornadas públicas passam por smoke test manual. Testes automatizados entram primeiro nas regras críticas que apresentarem regressão.

## Documentação

- [Visão](docs/00-visao-do-produto.md)
- [Requisitos funcionais](docs/01-requisitos-funcionais.md)
- [Requisitos não funcionais](docs/02-requisitos-nao-funcionais.md)
- [Modelo editorial](docs/03-modelo-editorial-e-evidencias.md)
- [Arquitetura](docs/04-arquitetura.md)
- [Modelo de dados](docs/05-modelo-de-dados.md)
- [Segurança e privacidade](docs/06-seguranca-e-privacidade.md)
- [Monitoramento e LLM](docs/07-monitoramento-e-llm.md)
- [Roadmap](docs/08-roadmap.md)
- [Backlog](docs/09-backlog-inicial.md)
- [Critérios de aceite](docs/10-criterios-de-aceite.md)
- [ADRs](docs/adr/)

