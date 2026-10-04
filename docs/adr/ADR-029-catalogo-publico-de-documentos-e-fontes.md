# ADR-029: Catálogo Público de Documentos e Fontes (VZ-030)

## Status

Aceito

## Contexto

O VorcaroZAP disponibiliza visualização individual de fontes documentais e laudos periciais em `/documentos/{id}` ([ADR-012](ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md), [ADR-022](ADR-022-avaliacao-tecnica-e-viewer-ssr-de-sequencias-contextuais.md)). No entanto, a descoberta de documentos dependia da navegação individual a partir das fichas de pessoas citadas (`/pessoas/{slug}`), sem um catálogo centralizado para pesquisa, filtragem e exploração do acervo documental.

A criação do catálogo público em `GET /documentos` requer observância estrita dos princípios de integridade editorial, segurança e fronteira pública do projeto:

1. **Fronteira Pública Canônica**: Um documento/fonte (`sources`) só pode ser exibido no catálogo público se possuir ao menos uma referência documental ativa (`evidence_sources.status = 'active'`) associada a uma alegação pública ativa (`public_claims_view`, [ADR-011](ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)).
2. **Elegibilidade Métrica vs. Visibilidade Pública**: Documentos vinculados a alegações públicas com `disposition IN ('context_only', 'correction')` (que possuem `metric_eligible = 0` para não inflar a rede) devem figurar normalmente no catálogo público.
3. **Isolamento Estrito de Quarentena e Rejeição**: Fontes documentais associadas exclusivamente a alegações em quarentena ou rejeitadas, bem como fontes cujos vínculos de evidência foram todos rejeitados (`evidence_sources.status = 'rejected'`), devem ser estritamente omitidas da listagem pública.
4. **Sem Crawler e Sem Proxying**: O catálogo lista fontes já registradas no acervo local, sem executar requisições remotas, web scraping ou proxy de conteúdo externo em tempo de requisição.
5. **Invalidação Imediata**: Qualquer moderação humana que desative o último vínculo público de uma fonte deve removê-la imediatamente do catálogo público, sem latência ou retenção por cache de aplicação ([ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md)).

---

## Decisão

### 1. Rota Pública SSR `GET /documentos`

Implementa-se a rota pública `GET /documentos` servida via Server-Side Rendering (SSR em Go e Templ), integrada ao cabeçalho de navegação principal (`web/components/header.templ`) e com breadcrumbs bidirecionais no visualizador de documento (`/documentos/{id}`).

### 2. Consulta Canônica de Documentos Públicos

A listagem pública agrega fontes documentais com contagem precisa de alegações públicas ativas vinculadas:
- Cada fonte documental (`sources s`) é incluída se, e somente se, estiver presente em `source_public_stats` (derivada de `evidence_sources es JOIN evidence ev ON ev.id = es.evidence_id JOIN public_claims_view pcv ON pcv.claim_id = ev.claim_id WHERE es.status = 'active'`).
- A contagem de citações públicas (`citations_count`) reflete com precisão os vínculos públicos ativos.
- O campo `entities_count` consolida a contagem de entidades públicas distintas (sujeitos e alvos) vinculadas às alegações documentadas (exibido no cartão com o rótulo preciso "Entidades vinculadas").
- O campo `highest_grade` identifica o grau mais severo entre as alegações públicas associadas ao documento (exibido como "Grau da alegação").
- O campo `first_public_excerpt` e seu localizador `first_public_locator` extraem deterministicamente o primeiro trecho contextualizado com suporte ativo para exibição direta no cartão documental.

### 3. Filtros, Busca Textual, Ordenação e Paginação Segura

- **Busca Textual (`q`)**: Filtra simultaneamente por título (`title`) e autor/publicador (`publisher_or_author`) com sanitização contra injeção e truncamento estrito a 200 caracteres (`store.MaxSearchQueryLength = 200`).
- **Filtro por Tipo (`source_type`)**: Permite filtrar por tipos padronizados presentes no acervo público (`court_document`, `police_report`, `official_statement`, `article`, `interview`, `social_media`).
- **Filtro por Status de Acesso (`access_status`)**: Permite filtrar por acessibilidade observada (`reachable`, `unreachable`, `not_checked`).
- **Ordenação Determinística (`sort`, `dir`)**: Suporta ordenação segura por allowlist:
  - `citations` (número de alegações públicas vinculadas, padrão `DESC`);
  - `updated` (data de atualização, padrão `DESC`);
  - `title` (título alfabético, padrão `ASC`);
  - `publisher` (publicador/autor alfabético, padrão `ASC`).
  - Desempate determinístico incondicional por `s.id DESC` / `s.id ASC`.
- **Paginação Bounded**: Parâmetros `page` (1 a 10.000) e `page_size` (1 a 50, padrão 15), com proteção contra integer overflow em offsets de paginação (`store.MaxPage = 10000`).

### 4. Política Estrita de Cache Dinâmico

Em conformidade com [ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md), as respostas de `GET /documentos` emitem obrigatoriamente:
```http
Content-Type: text/html; charset=utf-8
Cache-Control: no-cache, no-store, must-revalidate
```
As consultas são executadas diretamente contra o SQLite WAL, garantindo recálculo e invalidade imediata após qualquer deliberação de moderação.

---

## Consequências

### Positivas
- Permite a jornalistas, pesquisadores e cidadãos explorarem e pesquisarem todo o acervo documental de forma direta e estruturada.
- Mantém rigorosa consistência de fronteira pública e integridade relacional.
- Segrega documentos oficiais primários com chips visuais destacados.
- Garante total interoperabilidade com o visualizador detalhado de documentos existente.

### Negativas / Mitigações
- Consultas dinâmicas a cada requisição em SQLite WAL utilizam índices existentes sobre chaves estrangeiras e a view canônica (`public_claims_view`), mantendo tempos de resposta adequados para o volume do monólito sem necessidade de cache de aplicação.
