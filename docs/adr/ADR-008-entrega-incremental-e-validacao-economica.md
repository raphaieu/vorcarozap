# ADR-008 — Entrega incremental e validação econômica

- **Status:** aceito
- **Data:** 2026-09-04

## Decisão

O MVP inclui o motor que diferencia o produto: página/métricas, OpenRouter, publicação automática controlada e pós-moderação simples. Segurança avançada, workflow multiusuário e suíte abrangente são adiados.

O gate inicial usa formatação, vet, build e smoke manual. Testes automatizados entram primeiro em gates, dedupe e métricas após estabilização/regressão.

## Consequências

Lançamento rápido sem reduzir o produto a uma página estática. Há maior dependência de monitoramento humano posterior e menor proteção contra regressões, aceita até surgirem gatilhos reais.

## Gatilhos

Tráfego, equipe, incidentes, regressões, contestação, custo de LLM ou frequência de releases.

