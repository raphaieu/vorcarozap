# VorcaroZAP

Aplicação pública, investigativa e documental para organizar informações publicadas sobre Daniel Vorcaro, Banco Master e pessoas, organizações e acontecimentos relacionados. A plataforma combina monitoramento automatizado, fontes rastreáveis, classificação editorial, métricas públicas e pós-moderação humana.

> Projeto independente, sem vínculo com WhatsApp, Meta, Banco Master ou pessoas e instituições citadas. “WhatsApp” é marca de seu titular. A identidade usa referência cromática, sem reproduzir logotipo ou interface oficial.

## Estado

- **Fase 1 — Fundação Executável concluída.** Módulo Go, CLI, configuração com defaults seguros, SQLite em modo WAL com validação efetiva de pragmas e driver Pure Go `modernc.org/sqlite` ([ADR-010](docs/adr/ADR-010-escolha-do-driver-sqlite.md)), migrations com Goose, servidor HTTP Chi, renderização SSR com Templ, página base mobile-first, Dockerfile enxuto não-root e Compose com proxy Caddy opcional. Toolchain de desenvolvimento 100% via Docker e Makefile.
- **Fase 2 — Núcleo editorial e importação:**
  - **VZ-004 — Núcleo editorial e schema relacional concluído.** Pacote `internal/domain` com enums e tipos puros sem dependências de infraestrutura; migration `00002_editorial_core.sql` com tabelas `entities`, `entity_aliases`, `cases`, `relationships`, `import_runs`, `claims`, `evidence`, `sources` e `evidence_sources`; integridade relacional estrita (XOR, autorrelação, foreign keys com `ON DELETE` explícito); suporte a queries e transações seguras via SQLC (`internal/store/sqlc`).
  - **VZ-005 — Importador XLSX `curated_seed` concluído.** Pacote `internal/importer` com leitor Excelize, validação estrita de mapping versionado (`config/import-mapping-v1.yaml`), normalização conservadora de URLs e Unicode, idempotência comprovada no SQLite (`uq_import_runs_applied`), suporte a `--dry-run` e execução transacional segura sem dados inventados.
- **Fase 3 — Página pública e métricas:**
  - **VZ-006 — Listagem pública, busca, filtros e detalhe de entidades e alegações concluído.** Listagem paginada em `/pessoas` com ordenação segura por allowlist (`name`, `relevance`, `updated`), filtros multifacetados por Grau (A–E), categoria e nível de relevância (1 a 5), busca textual (`name`, `role_or_context`, `summary`, `relevance_rationale`), preservação de query parameters na navegação paginada e detalhe em `/pessoas/{slug}` com segregação visual de fontes (`supports`, `contradicts`, `contextualizes`), seção de manifestação e defesa, omissão limpa de trechos vazios, e garantia de invisibilidade de entidades e alegações em quarentena ou sem suporte ativo.
  - **VZ-007 — Metodologia pública, fontes rastreáveis e linguagem humana A–E concluído.** Página informativa em `/metodologia` com fundamentação da independência editorial, ausência de juízo de culpa, definições humanas completas para Graus A a E, níveis de relevância institucional 1 a 5, disposições editoriais, regras estritas de elegibilidade métrica da rede (`metric_eligible = true`), transparência sobre acessibilidade de fontes (`not_checked`, `reachable`, `unreachable`), distinção entre resumo editorial e citação literal, e política ágil de contraditório e retificações. Interface 100% navegável via Server-Side Rendering sem dependência de JavaScript, acessível (WCAG AA) e responsiva mobile-first.
  - **VZ-008 — Engine de métricas públicas por estado ativo e integração na Home concluído.** Visão canônica da fronteira pública (`public_claims_view`, [ADR-011](docs/adr/ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)); motor de métricas em `internal/metrics` calculando totais do acervo público e da rede elegível (`metric_eligible = true`), distribuições documentais por Grau (A–E), relevância (1–5) e categorias sem duplicatas por joins; data de corte configurável (`PUBLIC_DATA_CUTOFF=2026-09-03`); página inicial em `/` totalmente integrada com Server-Side Rendering, barras visuais de proporção em CSS puro, links diretos para filtros de pessoas e aviso neutro informando que o monitoramento autônomo contínuo ainda não foi ativado.
- **Próximo item:** VZ-009 — Exportação XLSX derivada do banco (fechamento de Fase 2 e Fase 3).

O arquivo `_notes/mapa-vorcaro-contatos-2026-09-03.xlsx` é um artefato público de pesquisa e base inicial. Estar no arquivo não equivale a culpa nem dispensa classificação e fonte na aplicação.

A carga inicial usa `origin = curated_seed` e o mapeamento editorial versionado em `config/import-mapping-v1.yaml`. O mapeamento define grau, estado inicial, disposição e elegibilidade métrica; correções/contexto podem ser públicos sem contar como vínculo. Linhas que não satisfazem os requisitos da carga curada entram em quarentena.

## MVP

- Go, SSR com `templ`/HTMX, SQLite e Caddy;
- importação da planilha inicial;
- monitoramento via OpenRouter/web search;
- gate estrutural em Go e gate semântico por segunda avaliação da LLM;
- política Go publicando automaticamente A/B válidos e C com linguagem/limites explícitos;
- quarentena por padrão para D/E e para qualquer conteúdo ambíguo/incompleto;
- painel simples protegido para fontes, evidências e moderação;
- listagem, busca, filtros, detalhes e fontes;
- métricas públicas de rede derivadas apenas de claims ativos e elegíveis;
- download da planilha/base;
- deploy em Oracle VPS.

Não integram o MVP: RBAC multiusuário, MFA, workflow de dupla revisão, snapshots completos, observabilidade sofisticada, alta disponibilidade e cobertura ampla de testes.

## Fluxo

```mermaid
flowchart TB
  OR[Descoberta OpenRouter] --> GE[Gate estrutural em Go]
  GE -->|falha| Q[Quarentena]
  GE -->|passou| GS[Gate semântico por segunda avaliação]
  GS -->|ambíguo ou D/E| Q
  GS -->|A/B válido| P[Publicado]
  GS -->|C limitado| P
  Q --> A[Painel simples]
  A -->|desaprovar| R[Rejeitado]
  A -->|aprovar| P
  P --> OUT[Página + métricas + XLSX]
```

## Ambiente e Toolchain

Docker e Docker Compose são os únicos requisitos no host. Não é necessário instalar Go, Templ ou outras ferramentas no ambiente local/WSL. O toolchain completo, compiladores e versões de dependências ficam fixados e isolados pelo projeto via Docker Compose e Makefile.

### Configuração de portas

O projeto separa explicitamente a porta da aplicação da porta do host:
- `APP_PORT=8080`: porta interna em que a aplicação escuta no container;
- `HOST_PORT=8090`: porta mapeada no host pelo Docker Compose (`8090:8080`).

No proxy Caddy local, o cabeçalho `Strict-Transport-Security` (HSTS) fica desativado por atender apenas `:80` HTTP, devendo retornar na configuração de produção com HTTPS real.

## Comandos de desenvolvimento (Makefile)

Todos os comandos de desenvolvimento executam dentro do container `dev`:

```bash
# Gerar templates Templ
make templ

# Formatar código Go
make fmt

# Sincronizar dependências Go
make tidy

# Executar testes unitários
make test

# Executar testes com race detector
make test-race

# Análise estática com go vet
make vet

# Compilar aplicação
make build

# Validação completa do ciclo
make all
```

## Comandos operacionais (Compose)

```bash
# Iniciar a aplicação (compila e sobe container 'app' na porta 8090)
make run

# Executar migrations pendentes explicitamente no container
make migrate

# Simulação de importação (dry-run, sem persistência)
make import-dry-run

# Importação aplicada da planilha curated_seed (transacional e idempotente)
make import

# Subir com proxy Caddy reverso opcional (porta 8000)
make compose-proxy
```

### Comandos planejados para fases posteriores

```bash
# Fase 2 / Fase 3 (Exportação e download de dados):
vorcarozap export

# Fase 4 (Monitoramento automatizado com OpenRouter):
vorcarozap monitor
```

## Validação econômica

Sem meta de cobertura no MVP. Gate inicial:

```bash
make all
```

Migrations, importação, monitoramento, moderação e jornadas públicas passam por smoke test manual. Três grupos têm testes unitários table-driven desde o MVP: decisão `published`/`quarantined`, fingerprint rejeitado impedindo republicação e métricas excluindo estados não públicos ou claims não elegíveis.

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
