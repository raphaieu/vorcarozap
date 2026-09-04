# ADR-007 — Exportação derivada da visão pública

- **Status:** aceito
- **Data:** 2026-09-03; escopo ajustado em 2026-09-04

## Decisão

Gerar XLSX por CLI a partir do SQLite e da mesma regra de visibilidade usada pela página/métricas. Itens rejeitados, em quarentena ou administrativos não entram. Arquivo pronto é servido pela aplicação.

O MVP inclui pessoas, relações/alegações, fontes e metodologia. Histórico, hashes públicos e outros formatos ficam para demanda futura.

## Consequências

Evita duas verdades e mantém custo baixo. A exportação precisa ser regenerada/invalida após ingestão ou moderação.

