# Requisitos funcionais

`P0` é necessário para o primeiro lançamento; `P1` é a evolução editorial; `P2` é automação e escala.

## P0 — MVP público

- **RF-PUB-01:** exibir nome do projeto, aviso de independência, resumo, data de atualização e metodologia.
- **RF-PUB-02:** listar pessoas em ordem alfabética com nome, cargo/função, relevância, síntese e grau da evidência.
- **RF-PUB-03:** buscar por nome, apelido, cargo, organização e texto resumido.
- **RF-PUB-04:** filtrar pelo menos por categoria, grau e relevância.
- **RF-PUB-05:** ordenar por nome, relevância e atualização.
- **RF-PUB-06:** exibir detalhe com contexto, alegações/relações, limites, fontes e datas.
- **RF-PUB-07:** abrir fontes externas e indicar veículo, título, URL e data disponível.
- **RF-PUB-08:** exibir grau por texto além da cor.
- **RF-PUB-09:** disponibilizar XLSX para download.
- **RF-IMP-01:** importar o XLSX por CLI com `--dry-run`.
- **RF-IMP-02:** validar cabeçalhos mínimos, reportar erros por linha e preservar valores originais relevantes.
- **RF-IMP-03:** evitar duplicação ao repetir a mesma importação.
- **RF-OPS-01:** fornecer `serve`, `migrate`, `import` e `export`.

No MVP, a curadoria ocorre antes da importação ou por comandos locais. Não existe área pública de escrita.

## P1 — Operação editorial

- painel protegido;
- login e papéis simples;
- cadastro/edição de entidades, alegações e fontes;
- status de rascunho, aprovado, publicado e arquivado;
- histórico de alterações;
- correções e contraditório estruturados;
- geração versionada do resumo e da planilha;
- backup externo e restauração documentada.

## P2 — Monitoramento

- `ResearchProvider` e OpenRouter;
- pesquisa web agendada;
- extração estruturada;
- deduplicação;
- fila de candidatos;
- verificação e proveniência por campo;
- controle de custo;
- revisão humana antes da publicação;
- observabilidade e alertas.

## Regra editorial mínima do MVP

Um item público precisa ter nome identificável, síntese neutra, classificação e pelo menos uma fonte pública. O conteúdo importado não ganha uma conclusão mais forte do que a fonte permite.

