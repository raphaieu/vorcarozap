# ADR-006 — Fontes e rastreabilidade por alegação

- **Status:** aceito
- **Data:** 2026-09-03; modelo ajustado em 2026-09-04

## Decisão

Modelar `claim -> evidence -> evidence_source -> source`. Grau existe somente no claim. `evidence_source` possui ID próprio e aceita múltiplos trechos da mesma fonte. Toda publicação automática requer citação/URL e trecho/localizador suficiente.

Dependência entre fontes, snapshots e cadeia de custódia ficam P1, mas o schema pode evoluir sem alterar a identidade dos registros atuais.

## Consequências

Permite auditoria pública suficiente, moderação granular e evita colocar URLs soltas na pessoa. A preservação integral de páginas não está garantida no MVP.

