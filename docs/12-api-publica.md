# API Pública — VorcaroZAP v1

A API pública do VorcaroZAP oferece acesso programático, somente leitura e padronizado em JSON às entidades, alegações documentadas, fontes e manifestações de defesa disponibilizadas na plataforma.

A API adota a mesma **fronteira pública canônica** da interface web (SSR): apenas alegações aprovadas (`published`) com suporte documental ativo (`status = 'active'` e `role = 'supports'`) são expostas. Itens em quarentena, desaprovados ou sem suporte ativo permanecem estritamente inacessíveis.

---

## Cabeçalhos e Formato

- **Formato**: `application/json; charset=utf-8`
- **Cache**: `Cache-Control: no-cache, no-store, must-revalidate` (dados sempre refletem em tempo real o estado comitado no banco de dados SQLite WAL)
- **Métodos**: Exclusivamente `GET`. Requisições com outros métodos HTTP retornam `405 Method Not Allowed`.
- **Sanitização e Omissão de URLs Externas (`canonical_url`, `source_url`)**: URLs externas apresentadas na API passam por sanitização estrita (`normalize.SafeURL`). Se a URL contiver parâmetros de consulta com credenciais, senhas, tokens de sessão, chaves de API, assinaturas ou URLs assinadas de nuvem (ex.: `?token=...`, `?auth_token=...`, `?api_key=...`, `?X-Amz-Signature=...`, `?sig=...`), o campo é retornado vazio (`""` ou omitido via `omitempty`). Parâmetros legítimos de busca, paginação e identificação de notícias (`?id=123`, `?q=...`, `?page=2`) são preservados ([ADR-031](adr/ADR-031-prevencao-de-exposicao-de-credenciais-em-urls-externas.md)).

---

## Formato Canônico de Erros

Em caso de falha de validação, recurso não encontrado ou erro interno, as respostas de erro seguem o seguinte formato estruturado:

```json
{
  "error": {
    "code": "bad_request",
    "message": "Parâmetro 'page_size' inválido. O valor deve ser um número inteiro entre 1 e 50."
  }
}
```

### Códigos de Erro Comuns

| HTTP Status | Código (`code`) | Descrição |
| :--- | :--- | :--- |
| `400 Bad Request` | `bad_request` | Parâmetro de consulta inválido, fora dos limites permitidos ou não-numérico. |
| `404 Not Found` | `not_found` | Recurso ou entidade pública não encontrada (inclui entidades em quarentena ou sem suporte). |
| `405 Method Not Allowed` | `method_not_allowed` | Método HTTP não suportado. A API é estritamente somente leitura. |
| `500 Internal Server Error` | `internal_error` | Falha interna no servidor ao processar a consulta. |

---

## Endpoints

### 1. Listagem de Entidades Públicas

```http
GET /api/v1/pessoas
```

Retorna a lista paginada de entidades públicas ativas na rede documental com seus respectivos metadados compilados e síntese mais relevante.

#### Parâmetros de Consulta (Query Parameters)

| Parâmetro | Tipo | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `q` | `string` | `""` | Busca textual por nome, apelido, cargo/contexto, categoria, relações, proposições ou fontes citadas. |
| `category` | `string` | `""` | Filtro por categoria (ex.: `Empresas`, `Finanças`, `Politica`, `Advocacia`). |
| `grade` | `string` | `""` | Filtro por Grau probatório da alegação (`A`, `B`, `C`, `D`, `E`). |
| `relevance` | `integer` | `0` (todas) | Filtro por nível de relevância institucional (`1` a `5`). |
| `period` | `string` | `""` (tudo) | Filtro por período de atualização recente (`7d`, `30d`, `90d`, `all`). |
| `sort` | `string` | `name` | Campo de ordenação (`name`, `relevance`, `updated`). |
| `dir` | `string` | `asc` (ou `desc`) | Direção da ordenação (`asc`, `desc`). |
| `page` | `integer` | `1` | Número da página solicitada (mínimo: `1`, máximo: `10000`). |
| `page_size` | `integer` | `15` | Quantidade de itens por página (mínimo: `1`, máximo: `50`). |

#### Exemplo de Resposta (`200 OK`)

```json
{
  "data": [
    {
      "id": "ent-1",
      "slug": "daniel-vorcaro",
      "name": "Daniel Vorcaro",
      "category": "Empresas",
      "role_or_context": "Controlador do Banco Master",
      "reach": "Nacional",
      "relevance": 4,
      "relevance_rationale": "Investigado e figura central das apurações societárias.",
      "public_claims_count": 3,
      "highest_grade": "A",
      "last_public_updated_at": "2026-09-03T18:00:00Z",
      "short_synthesis": "Controla o Banco Master e instituições financeiras coligadas."
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 15,
    "total_items": 42,
    "total_pages": 3
  }
}
```

---

### 2. Detalhe de Entidade Pública

```http
GET /api/v1/pessoas/{slug}
```

Retorna o perfil documental completo da entidade pública identificada pelo seu identificador amigável (`slug`), incluindo todas as alegações públicas ativas, fontes documentais associadas (segregadas por papel), manifestações de defesa e histórico editorial redigido.

> **Nota de Privacidade e Segurança:** Slugs que não existem ou que pertencem a pessoas restritas à quarentena retornam rigorosamente `404 Not Found` com a mesma mensagem neutra `{"error": {"code": "not_found", "message": "Entidade pública não encontrada."}}`, sem revelar se o registro existe internamente.

#### Exemplo de Resposta (`200 OK`)

```json
{
  "data": {
    "id": "ent-1",
    "slug": "daniel-vorcaro",
    "name": "Daniel Vorcaro",
    "category": "Empresas",
    "role_or_context": "Controlador do Banco Master",
    "reach": "Nacional",
    "summary": "Empresário e controlador do Banco Master, citado em apurações de mercado.",
    "relevance": 4,
    "relevance_rationale": "Investigado e figura central das apurações societárias.",
    "updated_at": "2026-09-03T18:00:00Z",
    "claims": [
      {
        "id": "clm-1",
        "relationship_type": "Controlador",
        "relationship_summary": "Controla o Banco Master",
        "context_limits": "Sem restrições documentais adicionais",
        "proposition": "Controla o Banco Master e instituições coligadas",
        "attribution": "",
        "grade": "A",
        "disposition": "supports_link",
        "metric_eligible": true,
        "context_status": "Investigado",
        "updated_at": "2026-09-03T18:00:00Z",
        "sources": [
          {
            "id": "src-1",
            "title": "Registro Societário da Junta Comercial",
            "publisher_or_author": "JUCESP",
            "canonical_url": "https://exemplo.gov.br/registro",
            "published_at": "2026-01-15T00:00:00Z",
            "accessed_at": "2026-09-03T12:00:00Z",
            "source_type": "court_document",
            "excerpt": "Daniel Vorcaro é o titular do controle acionário direto e indireto.",
            "locator": "Pág. 3",
            "role": "supports",
            "source_access_status": "reachable"
          }
        ],
        "defense_statements": [
          {
            "id": "stmt-1",
            "statement_type": "official_note",
            "title": "Nota de Esclarecimento da Assessoria",
            "content": "A instituição reitera a conformidade de todas as suas operações regulatórias.",
            "source_url": "https://bancomaster.example.com/nota",
            "created_at": "2026-09-04T10:00:00Z"
          }
        ]
      }
    ],
    "editorial_history": [
      {
        "id": "pub-clm-1",
        "created_at": "2026-09-03T18:00:00Z",
        "action": "initial_publish",
        "target_type": "claim",
        "summary": "Registro documental compilado e publicado na plataforma com suporte de fontes verificadas.",
        "is_target_public": true,
        "claim_id": "clm-1",
        "claim_grade": "A"
      }
    ]
  }
}
```

---

### 3. Listagem de Documentos Públicos

```http
GET /api/v1/documentos
```

Retorna a lista paginada de documentos e fontes documentais públicas ativas no acervo, com métricas consolidadas de citações ativas, primeiro trecho contextualizado e localizador público.

#### Parâmetros de Consulta (Query Parameters)

| Parâmetro | Tipo | Padrão | Descrição |
| :--- | :--- | :--- | :--- |
| `q` | `string` | `""` | Busca textual por título do documento ou veículo/autor. |
| `source_type` | `string` | `""` | Filtro por tipo de fonte (`article`, `official_statement`, `court_document`, `police_report`, `interview`, `social_media`). |
| `access_status` | `string` | `""` | Filtro por status de acessibilidade técnica (`reachable`, `unreachable`, `cited_by_provider`, `not_checked`). |
| `sort` | `string` | `title` | Campo de ordenação (`title`, `publisher`, `updated`, `citations`). |
| `dir` | `string` | `asc` (ou `desc`) | Direção da ordenação (`asc`, `desc`). |
| `page` | `integer` | `1` | Número da página solicitada (mínimo: `1`, máximo: `10000`). |
| `page_size` | `integer` | `15` | Quantidade de itens por página (mínimo: `1`, máximo: `50`). |

#### Exemplo de Resposta (`200 OK`)

```json
{
  "data": [
    {
      "id": "src-1",
      "title": "Registro Societário da Junta Comercial",
      "publisher_or_author": "JUCESP",
      "canonical_url": "https://exemplo.gov.br/registro",
      "published_at": "2026-01-15T00:00:00Z",
      "accessed_at": "2026-09-03T12:00:00Z",
      "source_type": "court_document",
      "source_access_status": "reachable",
      "source_access_checked_at": "2026-09-03T12:05:00Z",
      "citations_count": 2,
      "entities_count": 2,
      "highest_grade": "A",
      "last_public_updated_at": "2026-09-03T18:00:00Z",
      "first_public_excerpt": "Daniel Vorcaro é o titular do controle acionário direto e indireto.",
      "first_public_locator": "Pág. 3"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 15,
    "total_items": 18,
    "total_pages": 2
  }
}
```

---

### 4. Detalhe de Documento Público

```http
GET /api/v1/documentos/{id}
```

Retorna o perfil detalhado de um documento público, incluindo integridade técnica observada, sequência contextual completa de trechos vinculados a alegações públicas ativas (ordenada deterministicamente) e histórico editorial público redigido.

> **Nota de Privacidade e Segurança:** Identificadores que não existem, que pertencem exclusivamente a itens restritos à quarentena ou cujos vínculos foram rejeitados retornam rigorosamente `404 Not Found` com a mesma mensagem neutra `{"error": {"code": "not_found", "message": "Documento público não encontrado."}}`, sem revelar se a fonte existe internamente no banco.

#### Exemplo de Resposta (`200 OK`)

```json
{
  "data": {
    "id": "src-1",
    "title": "Registro Societário da Junta Comercial",
    "publisher_or_author": "JUCESP",
    "canonical_url": "https://exemplo.gov.br/registro",
    "published_at": "2026-01-15T00:00:00Z",
    "accessed_at": "2026-09-03T12:00:00Z",
    "source_type": "court_document",
    "source_access_status": "reachable",
    "source_access_checked_at": "2026-09-03T12:05:00Z",
    "http_status": 200,
    "updated_at": "2026-09-03T18:00:00Z",
    "sequence": [
      {
        "id": "es-1",
        "excerpt": "Daniel Vorcaro é o titular do controle acionário direto e indireto.",
        "locator": "Pág. 3",
        "role": "supports",
        "claim_id": "clm-1",
        "claim_proposition": "Controla o Banco Master e instituições coligadas",
        "claim_grade": "A",
        "claim_disposition": "supports_link",
        "metric_eligible": true,
        "relationship_type": "Controlador",
        "relationship_summary": "Controla o Banco Master",
        "context_limits": "Sem restrições documentais adicionais",
        "subject_entity_name": "Daniel Vorcaro",
        "subject_entity_slug": "daniel-vorcaro"
      }
    ],
    "editorial_history": [
      {
        "id": "pub-doc-es-1",
        "created_at": "2026-09-03T18:00:00Z",
        "action": "initial_publish",
        "target_type": "evidence_source",
        "summary": "Trecho documental (Pág. 3) incluído e referenciado em alegação pública.",
        "is_target_public": true,
        "claim_id": "clm-1",
        "claim_grade": "A",
        "source_id": "src-1",
        "locator": "Pág. 3"
      }
    ]
  }
}
```
