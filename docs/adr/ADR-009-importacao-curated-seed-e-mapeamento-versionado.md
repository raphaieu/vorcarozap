# ADR-009 — Importação `curated_seed` e mapeamento versionado

- **Status:** aceito
- **Data:** 2026-09-04

## Contexto

A planilha inicial passou por curadoria anterior ao sistema e usa uma escala legada cujos rótulos precisam ser transformados para o modelo A–E atual. Esconder a transformação no código impediria reproduzir importações e distinguir uma decisão editorial de uma regra técnica.

## Decisão

Registrar `claims.origin = curated_seed` para conteúdo criado pela planilha. Manter o mapeamento exato dos rótulos legados em arquivo declarativo versionado (`config/import-mapping-v1.yaml`) e persistir `mapping_version` em cada `import_run` aplicado. A carga cria claims diretamente; não cria um `monitoring_run` artificial.

Uma linha pode iniciar `published` somente quando possui nome identificável, cargo/contexto, explicação da relação, fonte principal, classificação legada reconhecida e texto que não extrapola a fonte. Falta de fonte, ambiguidade, rótulo desconhecido, mapeamento impossível, extrapolação ou dado essencial ausente leva a `quarantined`. O importador nunca adivinha rótulo nem identidade.

## Alternativas consideradas

- Aplicar ao seed o mesmo pipeline do OpenRouter: uniforme, mas ignora sua curadoria prévia e adiciona custo.
- Publicar todas as linhas: simples, sem proteção para ambiguidades ou erros.
- Codificar `switch` no importador: funcional, porém opaco e não reproduzível entre versões.
- Importar tudo em quarentena: conservador, mas inviabiliza a carga inicial já curada sem trabalho manual repetitivo.

## Consequências positivas

Importação reproduzível e auditável, transformação visível em revisão e tratamento conservador de exceções.

## Consequências negativas

O arquivo de mapeamento passa a ser artefato editorial que exige revisão. Uma nova versão pode produzir resultado diferente e não deve reclassificar dados existentes silenciosamente.

## Riscos

Rótulo conhecido não garante que a linha individual esteja correta; mapeamento direto pode esconder nuance da fonte. Validações por linha, relatório, dry-run, preservação do valor original e quarentena mitigam.

## Gatilhos de revisão

Nova planilha/layout, nova taxonomia, rótulo não reconhecido, incidente editorial ou necessidade de reimportar com outra versão exigem novo arquivo de mapeamento e decisão explícita de migração.
