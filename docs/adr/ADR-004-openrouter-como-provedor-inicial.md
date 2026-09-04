# ADR-004 — OpenRouter como provedor inicial

- **Status:** aceito
- **Data:** 2026-09-03; escopo confirmado em 2026-09-04

## Decisão

OpenRouter integra o MVP por meio de `ResearchProvider`. Modelos/engine são configuração. Structured outputs e `openrouter:web_search` alimentam candidatos normalizados. Revalidar a API antes de codificar.

O pipeline publica automaticamente candidatos que passam nos gates locais e coloca os demais em quarentena. A confiança do modelo não substitui os gates.

## Consequências

Monitoramento é o motor de atualização, com custo/instabilidade externos. Limites, idempotência, citações e pós-moderação reduzem o risco sem criar workflow pesado.

## Gatilhos

Mudança da API, custo/qualidade inadequados, política de dados incompatível ou provedor superior.

