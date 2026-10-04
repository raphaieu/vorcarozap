# ADR-030: API Pública de Documentos e Fontes (VZ-031)

## Status

Aceito

## Contexto

Com a consolidação do catálogo público de documentos em Server-Side Rendering (`GET /documentos`, [ADR-029](ADR-029-catalogo-publico-de-documentos-e-fontes.md)) e da infraestrutura da API pública v1 para pessoas (`GET /api/v1/pessoas` e `GET /api/v1/pessoas/{slug}`, [ADR-028](ADR-028-api-publica-somente-leitura-e-contrato-v1.md)), surgiu a necessidade de disponibilizar acesso programático em JSON aos documentos e fontes documentais da plataforma.

A disponibilização desses endpoints requer estrita adesão aos princípios fundamentais do VorcaroZAP:
1. **Fronteira Pública Canônica**: Apenas documentos vinculados a alegações públicas ativas (`public_claims_view`) com suporte documental ativo (`status = 'active'`) devem ser expostos. Documentos associados exclusivamente a itens em quarentena ou com suportes rejeitados permanecem inacessíveis.
2. **Elegibilidade Métrica vs. Visibilidade Pública**: Documentos que sustentam alegações de contexto ou correção com `metric_eligible = false` devem constar no catálogo e nos detalhes da API, preservando a completude informativa.
3. **Isolamento de Dados Internos e Confidenciais**: URLs originais não sanitizadas (`original_url`), fragmentos com tokens/segredos, nomes de operadores, motivos de moderação, fingerprints, custos de LLM e metadados confidenciais jamais podem ser serializados.
4. **Invalidação Imediata**: Qualquer ação de moderação humana sobre alegações ou suportes documentais deve refletir instantaneamente nas consultas da API ([ADR-019](ADR-019-invalidacao-imediata-de-visualizacoes-publicas-metricas-e-exportacao.md)).
5. **Comportamento Fail-Closed**: Documentos inexistentes, restritos à quarentena ou desaprovados retornam o mesmo `404 Not Found` neutro.

---

## Decisão

### 1. Endpoints JSON Somente Leitura sob `/api/v1`

Ficam adicionados ao contrato da API pública v1 exclusivamente os seguintes endpoints:
- `GET /api/v1/documentos`: Listagem paginada de documentos públicos com busca textual (`q`), filtros (`source_type`, `access_status`), ordenação (`sort`, `dir`) e paginação conservadora (`page`, `page_size`).
- `GET /api/v1/documentos/{id}`: Detalhe completo de documento público, abrangendo metadados observados de acessibilidade técnica, sequência contextual de trechos públicos ordenados deterministicamente por localizadores e histórico editorial redigido.

Não são criados endpoints de mutação, escrita ou endpoints que bypassam a autenticação existente.

### 2. DTOs Dedicados e Sanitização de Dados

A serialização JSON utiliza estruturas tipadas dedicadas (`APIDocumentItemDTO`, `APIDocumentsListResponse`, `APIDocumentSequenceItemDTO`, `APIDocumentDetailDTO`, `APIDocumentDetailResponse` em `internal/web/api_v1.go`), garantindo:
- **Omissão total de `original_url`**: Apenas `canonical_url` é exposta.
- **Sanitização estrita de URLs externas (`normalize.SafeURL`)**: Exige esquemas `http` ou `https`, valida hostname, rejeita credenciais embutidas (`userinfo`), remove fragmentos (`#...`) e retorna string vazia em caso de URL inválida ou insegura. Query strings legítimas são preservadas conforme ADR-006 e ADR-013 para não quebrar links de artigos; parâmetros de credenciais em query (`?token=...`) devem ser evitados na curadoria/ingestão ou objeto de política dedicada futura.
- **Omissão de identificadores de destinos não-públicos**: Slugs de entidades-alvo e casos em quarentena são omitidos; expõem-se apenas nomes contextuais (`target_entity_name`, `case_name`) e identificadores do sujeito público (`subject_entity_name`, `subject_entity_slug`).
- **Normalização e Sanitização de Localizadores (`normalize.SafeLocator`)**: Localizadores documentais na sequência são normalizados (ex.: `Pág. 2`, `Fls. 10–12`); localizadores inválidos ou com caracteres proibidos/scripts são convertidos para string vazia.
- **Campos em `snake_case` e `omitempty`** para propriedades opcionais/vazias.

### 3. Validação de Parâmetros e Formato Canônico de Erros

- Parâmetros fora dos limites permitidos (`page < 1` ou `> 10000`, `page_size < 1` ou `> 50`, `source_type` ou `access_status` inválidos, `sort` ou `dir` fora da allowlist) retornam `400 Bad Request` padronizado.
- Documentos não encontrados ou não públicos respondem `404 Not Found` neutro.
- Métodos não-GET respondem `405 Method Not Allowed`.

### 4. Invalidação Imediata de Cache

Todas as respostas emitem:
```http
Content-Type: application/json; charset=utf-8
Cache-Control: no-cache, no-store, must-revalidate
```
As consultas são executadas diretamente contra o SQLite WAL sobre `public_claims_view` e `sources`, assegurando atualização instantânea após deliberações editoriais.

---

## Consequências

### Positivas
- Fornece acesso programático rico e estruturado a jornalistas e pesquisadores para consulta e auditoria do acervo documental.
- Mantém coerência arquitetural e total paridade com o catálogo SSR (`/documentos`) e o visualizador (`/documentos/{id}`).
- Garante proteção estrita contra enumeração de dados confidenciais ou itens em quarentena.

### Negativas / Mitigações
- Consultas diretas a cada requisição exigem limites conservadores de paginação (`MaxPageSize = 50`), já adequados à capacidade do monólito.
- Preservação de query strings requer disciplina na curadoria/ingestão para evitar persistência de links com tokens privados em `?token=...`.
