# Requisitos funcionais

`P0` integra o MVP; `P1` é evolução após validação.

## Página pública e métricas — P0

- **RF-PUB-01:** exibir independência, resumo, data de corte, atualização e metodologia.
- **RF-PUB-02:** listar entidades alfabeticamente com cargo, síntese, relevância, grau e atualização.
- **RF-PUB-03:** buscar por nome/apelido, cargo, organização, relação e fonte.
- **RF-PUB-04:** filtrar por categoria, grau, relevância e período.
- **RF-PUB-05:** ordenar por nome, relevância e atualização.
- **RF-PUB-06:** detalhar relações, alegações, evidências, fontes, limites e contraditório disponível.
- **RF-PUB-07:** servir XLSX atualizado.
- **RF-MET-01:** calcular totais públicos somente sobre entidades/claims/fontes ativas.
- **RF-MET-02:** recalcular ou invalidar cache após ingestão, desaprovação e restauração.
- **RF-MET-03:** exibir distribuição A–E, categorias, relevância, recentes e último monitoramento.

## Importação e monitoramento — P0

- **RF-IMP-01:** importar XLSX via CLI com `--dry-run`, erros por linha e idempotência por hash/mapeamento.
- **RF-MON-01:** executar descoberta via `ResearchProvider`/OpenRouter dentro de janela configurável.
- **RF-MON-02:** extrair JSON estruturado com entidade, alegação, grau sugerido, fonte, trecho/localizador e datas.
- **RF-MON-03:** normalizar, deduplicar e registrar run/modelo/custo.
- **RF-MON-04:** aplicar gates determinísticos de publicação.
- **RF-MON-05:** publicar automaticamente candidatos válidos e colocar os demais em quarentena.
- **RF-MON-06:** nunca promover texto além do que a fonte citada demonstra.

## Painel simples — P0

- **RF-ADM-01:** proteger `/admin` para um administrador configurado.
- **RF-ADM-02:** listar fontes, alegações/evidências e candidatos com busca/filtros por estado, grau, origem e data.
- **RF-ADM-03:** visualizar fonte, trecho, entidade relacionada, origem automática e dados públicos resultantes.
- **RF-ADM-04:** desaprovar item com motivo; ele desaparece da página e métricas.
- **RF-ADM-05:** restaurar item desaprovado ou aprovar item em quarentena.
- **RF-ADM-06:** preservar a decisão mínima de moderação para evitar republicação automática imediata.
- **RF-ADM-07:** disparar monitoramento manual e consultar o último resultado/resumo de custo.

## Operação — P0

- `serve`, `migrate`, `import`, `monitor` e `export`;
- cron/agendamento executa `monitor` sem sobreposição;
- exportação nasce do banco e exclui itens não públicos.

## P1

Usuários/papéis, MFA, edição completa, dupla revisão, snapshots, histórico imutável, métricas históricas, observabilidade/alertas avançados e API pública.

