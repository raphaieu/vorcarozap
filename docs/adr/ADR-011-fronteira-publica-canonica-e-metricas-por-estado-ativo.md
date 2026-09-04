# ADR-011 — Fronteira pública canônica e métricas por estado ativo

- **Status:** aceito
- **Data:** 2026-09-04

## Contexto

A revisão de VZ-006 e VZ-007 identificou que a regra de visibilidade pública — um claim deve ter `status = 'published'` E ao menos um `evidence_source` ativo com papel `supports` — estava dispersa e duplicada de maneira heterogênea entre queries de listagem, contagem, detalhe e busca. Isso provocava vazamento lateral: entidades com alegações ocultas (quarentena ou suporte rejeitado) podiam ser encontradas via busca textual por termos restritos a tais alegações ou fontes inativas, e o resumo do card (`short_synthesis`) selecionava vínculos sem garantir suporte ativo nem ordenação determinística.

Ao mesmo tempo, VZ-008 exige calcular métricas públicas de rede considerando estritamente claims que satisfaçam a visibilidade pública completa e `metric_eligible = true`, sem dupla contagem por múltiplos vínculos, evidências ou fontes.

## Decisão

1. Formalizar a fronteira pública única por meio da view de banco `public_claims_view` (migration `00005_canonical_public_boundary.sql`).
2. A view encapsula os invariantes de publicação:
   - `claims.status = 'published'`;
   - Existência de ao menos um `evidence_sources` com `status = 'active'` e `role = 'supports'`.
3. Todas as consultas públicas (`ListPublicEntities`, `CountPublicEntities`, `ListPublicCategories`, `GetPublicEntityBySlug`, `ListPublicClaimsByEntityID`, resumos e métricas) passam a consumir exclusivamente `public_claims_view`.
4. A busca textual e o resumo do card acessam relacionamentos, proposições e fontes unicamente a partir de `public_claims_view`, eliminando qualquer vazamento lateral de entidades ou dados em quarentena/rejeitados.
5. A busca com operador `LIKE` adota cláusula explícita `ESCAPE '\'` com sanitização dos caracteres especiais `%`, `_` e `\`.
6. A ordenação utiliza parâmetros SQLC nomeados diretamente no `ORDER BY`, com allowlist em Go (`name`, `relevance`, `updated`) e desempate determinístico por `e.name ASC, e.id ASC`.
7. O filtro de período aceita exclusivamente presets (`7d`, `30d`, `90d`, `all`), convertidos em Go para RFC3339 via relógio testável.
8. As métricas públicas de VZ-008 são derivadas diretamente do SQLite sobre o estado ativo em duas seções bem delimitadas:
   - **Base pública:** contagens informativas de entidades públicas, alegações públicas e fontes ativas vinculadas;
   - **Rede elegível:** agregações restritas a `metric_eligible = true` (entidades, alegações, distribuição A–E, relevância 1–5 e categorias).

## Consequências

- **Positivas:** Elimina divergências conceituais de visibilidade pública, impede vazamentos laterais de busca, assegura determinismo na síntese do card e consolida um contrato seguro e sem redundâncias para as métricas da rede.
- **Negativas:** Requer uma migration adicional (`00005`), mas em SQLite a view não materializada tem custo zero de armazenamento e simplifica a manutenção de queries SQLC.
