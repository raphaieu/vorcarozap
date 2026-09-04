# ADR-006 — Fontes e rastreabilidade por alegação

- **Status:** aceito
- **Data:** 2026-09-03

## Contexto

Uma URL isolada não informa que proposição sustenta, onde está o trecho, se é independente ou se contradiz outra fonte. Links desaparecem e conteúdo pode ter restrição de redistribuição.

## Decisão

Modelar claim → evidence → evidence_source → source. Toda alegação publicada tem evidência, papel da fonte, excerto/localizador, datas e limites. Snapshots possuem hash, acesso/licença e redação separados. Proveniência por campo e histórico são preservados.

## Alternativas consideradas

- URLs em pessoa: simples, sem granularidade e gera culpa por associação.
- Texto livre com notas: flexível, difícil de validar/exportar.
- Copiar toda página: resiliente, mas cria risco autoral, privacidade e armazenamento.

## Consequências positivas

Auditoria, correção granular, transparência e possibilidade de detectar contradições/indisponibilidade.

## Consequências negativas

Mais tabelas e trabalho editorial; snapshots exigem governança; excertos podem envelhecer.

## Riscos

Fonte circular, trecho sem contexto, hash tratado como autenticidade, link malicioso e violação autoral. Revisão de independência/contexto, SSRF e política de snapshot mitigam.

## Gatilhos de revisão

Novos tipos de mídia/prova, exigência de cadeia de custódia, licenciamento, volume de snapshots ou necessidade de preservação externa especializada.

