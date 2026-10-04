# ADR-028: API Pública Somente Leitura e Contrato JSON v1 (VZ-029)

## Status

Aceito

## Contexto

O VorcaroZAP disponibiliza informações documentais e investigativas consolidadas por meio de interface web Server-Side Rendering (SSR) e exportação em planilha XLSX ([ADR-003](ADR-003-renderizacao-server-side.md), [ADR-007](ADR-007-exportacao-de-dados.md), [ADR-011](ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)).

Para viabilizar integrações programáticas por pesquisadores, jornalistas, desenvolvedores e entidades de transparência pública, sem induzir práticas de web scraping sobre páginas HTML, o backlog previa a criação de uma API pública documentada e somente leitura (item da evolução pós-MVP).

A disponibilização desta API impõe requisitos estritos de segurança, privacidade e integridade editorial:
1. **Fronteira Pública Canônica**: A API deve espelhar com exatidão as regras de visibilidade da área pública SSR: apenas alegações com `status = 'published'` e ao menos um suporte documental ativo com papel `supports` ([ADR-011](ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)).
2. **Elegibilidade Métrica vs. Visibilidade Pública**: Alegações públicas de contexto ou correção (`disposition` em `context_only` ou `correction`) possuem `metric_eligible = 0` (restringindo métricas da rede), mas devem permanecer visíveis e consumíveis na API pública.
3. **Isolamento de Dados Internos e Confidenciais**: Quarentenas, motivos de moderação, operadores, credenciais, segredos, hashes, sessões, dados de MFA, fingerprints internos, custos de LLM e respostas brutas de provedores de IA jamais podem ser serializados ou expostos.
4. **Invalidação Imediata**: Qualquer deliberação administrativa de moderação deve refletir instantaneamente na API pública, sem retenção por caches de aplicação ([ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md)).
5. **Comportamento Seguro e Fail-Closed**: Slugs inexistentes ou entidades restritas à quarentena devem retornar HTTP 404 idêntico e controlado, sem revelar qual das duas situações ocorreu.

---

## Decisão

### 1. Endpoints JSON Somente Leitura sob `/api/v1`

Ficam estabelecidos exclusivamente os seguintes endpoints públicos:
- `GET /api/v1/pessoas`: Listagem paginada de entidades públicas com busca textual (`q`), filtros multifacetados (`category`, `grade`, `relevance`, `period`), ordenação segura (`sort`, `dir`) e paginação (`page`, `page_size`).
- `GET /api/v1/pessoas/{slug}`: Detalhe completo de entidade pública, abrangendo atributos biográficos/institucionais, alegações públicas ativas, fontes documentais associadas, manifestações de contraditório aceitas e histórico editorial público redigido.

Não são criados endpoints de escrita, mutação, autenticação paralela ou bypass por Basic Auth.

### 2. DTOs Explícitos e Omissão de Estruturas Internas

A serialização JSON utiliza DTOs dedicados e tipados (`internal/web/api_v1.go`), proibindo a serialização direta de modelos de banco (SQLC), structs de domínio com campos internos ou objetos do painel administrativo:
- **Omissão de URLs brutas (`original_url`)**: Apenas `canonical_url` (normalizada e sem fragmentos de URL) é exposta, evitando a serialização desnecessária da URL original de importação/candidato e mitigando vazamentos de fragmentos que possam conter tokens ou dados de sessão.
- **Omissão de identificadores de destinos não-verificados**: Slugs de entidades-alvo e casos (`target_entity_slug`, `case_slug`) são omitidos dos DTOs de alegação para evitar a enumeração de identificadores de alvos em quarentena; apenas os nomes contextuais (`target_entity_name`, `case_name`) são expostos.
- Campos públicos adotam nomenclatura padronizada em `snake_case`.
- Valores nulos ou ausentes utilizam `omitempty` ou valores padrão estruturados.
- Localizadores documentais são normalizados e sanitizados via `normalize.Locator`.

### 3. Validação de Parâmetros e Respostas Controladas de Erro

- Parâmetros com valores fora da especificação recebem resposta controlada `400 Bad Request` com código `bad_request` e mensagem explicativa em português (ex.: limites de página `1..10000`, limites de itens `1..50`, faixas de relevância `1..5`, graus `A..E`).
- Proteção aritmética contra overflow em offsets de paginação (`store.MaxPage = 10000`).
- Erros internos retornam `500 Internal Server Error` com mensagem genérica, sem expor SQL, stack traces, caminhos locais ou variáveis de ambiente.
- Métodos não permitidos (ex.: `POST`, `DELETE`, `PUT`) respondem `405 Method Not Allowed` em formato JSON padronizado.

### 4. Política Estrita de Cache Dinâmico

Em conformidade com [ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md), todas as respostas da API pública emitem obrigatoriamente:
```http
Content-Type: application/json; charset=utf-8
Cache-Control: no-cache, no-store, must-revalidate
```
As consultas são resolvidas dinamicamente no SQLite WAL contra a `public_claims_view`, assegurando consistência transacional direta e recálculo imediato após aprovações, rejeições ou desativações de suporte.

---

## Consequências

### Positivas
- Disponibiliza acesso programático estruturado, confiável e determinístico para a sociedade civil e pesquisadores.
- Mantém coerência rigorosa entre a página SSR, a exportação XLSX e a API JSON.
- Garante isolamento fail-closed de itens em quarentena e segurança contra vazamento de metadados internos ou custos de LLM.
- Preserva a simplicidade da arquitetura monolítica em Go e SQLite sem novas dependências externas.

### Negativas / Mitigações
- Consultas dinâmicas em SQLite WAL a cada requisição exigem limites conservadores de tamanho de página (`MaxPageSize = 50`), já adequados à escala e topologia de deploy em instância única.
