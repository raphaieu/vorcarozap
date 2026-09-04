# ADR-005 — Revisão humana antes da publicação

- **Status:** aceito
- **Data:** 2026-09-03

## Contexto

O conteúdo envolve pessoas identificáveis, alegações e alto risco reputacional. LLMs e importações podem errar identidade, contexto, fonte e classificação.

## Decisão

Automação e importação terminam no máximo em `pending_review`. Reviewer humano decide com motivo; editor publica uma versão `approved`. Rejeições ficam auditáveis. Alegações graves/ambíguas poderão exigir dupla revisão configurável antes do go-live.

## Alternativas consideradas

- Publicação automática acima de confiança: rápida, mas confiança técnica não mede verdade e o dano é alto.
- Sem IA: reduz risco de automação, perde capacidade de triagem; revisão ainda pode falhar.
- Revisão posterior: reduz atraso, mas permite dano antes da correção.

## Consequências positivas

Responsabilidade clara, contraditório/contexto avaliados e menor chance de publicação indevida.

## Consequências negativas

Fila e custo humano, latência, necessidade de papéis e risco de inconsistência entre revisores.

## Riscos

Rubber-stamping, conflito de interesse, conta comprometida e backlog. Checklist, RBAC/MFA, métricas, versionamento e amostragem mitigam.

## Gatilhos de revisão

Nunca remover revisão humana para alegações novas sem novo ADR e avaliação editorial/jurídica. Rever quantidade de revisores conforme gravidade, volume, incidentes e equipe.

