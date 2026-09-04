# ADR-005 — Pós-moderação humana

- **Status:** substitui revisão prévia obrigatória no MVP
- **Data:** 2026-09-04

## Contexto

Aprovar manualmente cada item elimina a velocidade do monitoramento e exige painel complexo. Publicar tudo sem gate aumenta ruído.

## Decisão

Adotar publicação automática controlada: gates válidos levam a `published`; falhas/ambiguidade levam a `quarantined`. Um administrador acompanha o painel e pode rejeitar, restaurar ou aprovar quarentena.

Fingerprint rejeitado bloqueia republicação automática imediata. Moderação altera consulta, métricas e exportação.

## Consequências

Atualização rápida e baixo custo operacional, aceitando janela entre publicação e eventual desaprovação. Linguagem neutra, fonte visível, gates e quarentena são obrigatórios.

## Gatilhos

Contestação relevante, equipe maior, alegações mais sensíveis ou incidentes podem exigir aprovação prévia/dupla revisão.

