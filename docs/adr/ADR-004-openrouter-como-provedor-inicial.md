# ADR-004 — OpenRouter como provedor futuro de pesquisa/LLM

- **Status:** aceito para fase futura
- **Data:** 2026-09-03; sequenciamento revisto em 2026-09-04

## Contexto

O produto poderá monitorar novas publicações, mas o primeiro lançamento pode ser alimentado manualmente. Antecipar LLM, scheduler, fila, revisão e observabilidade aumentaria custo antes da validação do site.

## Decisão

OpenRouter continua escolhido como primeiro candidato, isolado por `ResearchProvider`, porém não integra o MVP público. A documentação da API será revalidada quando a fase de monitoramento começar.

Quando implementado, usará modelo configurável, structured outputs e ferramenta vigente de web search. Saídas produzirão candidatos e não publicação direta.

## Consequências

O lançamento fica mais rápido e barato. Atualizações serão manuais no início. A automação posterior exigirá mecanismos próprios de custo, validação, idempotência e revisão.

## Gatilhos

Frequência de atualização manual se tornar alta, audiência justificar conteúdo mais recente ou fluxo editorial já suportar revisão de candidatos.

