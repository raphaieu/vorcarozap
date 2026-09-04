# ADR-005 — Revisão humana antes da publicação automatizada

- **Status:** aceito com implementação incremental
- **Data:** 2026-09-03; sequenciamento revisto em 2026-09-04

## Contexto

O conteúdo envolve pessoas identificáveis. Ao mesmo tempo, o primeiro lançamento será operado por uma pessoa, sem painel ou ingestão automática.

## Decisão

No MVP, a revisão ocorre manualmente antes da importação/publicação e não exige workflow, RBAC ou dupla aprovação no software. Quando painel ou monitoramento forem introduzidos, automação terminará em `pending_review` e publicação continuará sendo uma ação humana.

Dupla revisão, MFA, papéis separados e trilha avançada permanecem futuras, acionadas por equipe maior, alegações sensíveis, tráfego ou risco real.

## Consequências

O MVP evita construir um sistema editorial multiusuário antes de precisar dele. Em contrapartida, a disciplina inicial depende do responsável e de checkpoints no Git/arquivo de importação.

## Gatilhos

Segundo colaborador, volume recorrente de atualizações, ingestão automática, contestação relevante ou necessidade de demonstrar autoria detalhada.

