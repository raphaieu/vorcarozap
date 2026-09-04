# ADR-007 — Exportação derivada do banco

- **Status:** aceito
- **Data:** 2026-09-03

## Contexto

Leitores/pesquisadores precisam de XLSX completo, mas manter planilha manual criaria duas fontes de verdade e divergência de classificações/correções.

## Decisão

Gerar XLSX com Excelize a partir de um snapshot transacional do banco. Abas: pessoas, organizações, relações, alegações/evidências, fontes, alterações recentes e metodologia. Arquivo temporário é validado, recebe versão/hash e só então é publicado atomicamente; excluir campos administrativos/sensíveis.

## Alternativas consideradas

- Planilha mestre manual: familiar, mas diverge, perde constraints e auditoria.
- CSV único: simples/aberto, não representa relações/abas/metodologia adequadamente.
- API apenas: automatizável, menos acessível ao público-alvo; possível P2.
- Gerar sob demanda: sempre atual, mas consome recursos e torna falhas visíveis.

## Consequências positivas

Uma fonte de verdade, export reprodutível, transparência de versão e formato útil.

## Consequências negativas

XLSX é formato complexo; snapshots podem ficar brevemente defasados; exige conciliação e proteção contra formula injection.

## Riscos

Vazamento de campos, fórmula maliciosa, arquivo parcial e inconsistência entre abas. Allowlist de colunas, escape de prefixos perigosos, transação de leitura, validação e rename mitigam.

## Gatilhos de revisão

Volume tornar XLSX impraticável, demanda por CSV/JSON/API, necessidade de dados abertos/licença formal ou geração afetar produção.

