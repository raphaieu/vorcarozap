# Continuidade Técnica e Baseline Operacional

Documento sintético de continuidade registrando o estado verificado do VorcaroZAP, validações executadas, pendências e diretrizes da evolução documental.

## 1. Baseline e Estado Comprovado

- **Commit-base de referência:** `72bcedc1535a63368ab7cafa780bbe9e3f9d54b8` (HEAD verificado no início do ciclo VZ-009).
- **Arquitetura comprovada:** A aplicação opera como monólito Go modular, persistido em SQLite modo WAL (`modernc.org/sqlite`, [ADR-010](adr/ADR-010-escolha-do-driver-sqlite.md)), com renderização Server-Side (Templ) sem dependência de JavaScript. O ambiente é 100% conteinerizado via Docker.

### Entregas verificadas no código:
- **Fase 1 — Fundação:** CLI, servidor Chi, migrations (Goose), Dockerfile enxuto não-root e Compose com proxy Caddy opcional ([ADR-001](adr/ADR-001-monolito-modular-em-go.md), [ADR-002](adr/ADR-002-sqlite-com-wal.md), [ADR-003](adr/ADR-003-renderizacao-server-side.md)).
- **Fase 2 — Núcleo Editorial e Carga:** Schema relacional estrito (`00002_editorial_core.sql`), importador transacional XLSX `curated_seed` com dry-run, mapeamento versionado em YAML e idempotência comprovada ([ADR-009](adr/ADR-009-importacao-curated-seed-e-mapeamento-versionado.md)).
- **Fase 3 — Apresentação Pública, Métricas e Exportação:** Listagem e filtros em `/pessoas`, detalhe de entidades e fontes em `/pessoas/{slug}`, metodologia e critérios em `/metodologia`, engine de agregação em tempo real da fronteira ativa (`public_claims_view`, [ADR-011](adr/ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)), Home pública com indicadores e distribuições (`/`), e exportação XLSX derivada exclusivamente da base pública SQLite com motor Excelize em `internal/exporter`, subcomando CLI `export` e download HTTP em `/exportar/base.xlsx` ([ADR-007](adr/ADR-007-exportacao-de-dados.md)).

## 2. Saneamento e Exportação Realizados

No ciclo de saneamento e implementação de VZ-009, consolidaram-se as seguintes entregas:
- **Exportação derivada estritamente da visão pública:** A geração do XLSX decorre exclusivamente da `public_claims_view` e consultas relacionais de suporte ativo, nunca da planilha seed original.
- **Isolamento de visibilidade:** Itens em quarentena técnica (como os 21 registros do seed com Grau E ambíguo), alegações rejeitadas, arquivadas ou metadados de processamento interno não constam na planilha gerada.
- **Preservação de conteúdo e distinção métrica:** Alegações de contexto e correções com `metric_eligible = false` (ex.: Grau E corrigido) continuam presentes na exportação para integridade documental, com a indicação explícita `Elegível nas Métricas = Não`, enquanto itens da rede figuram com `Sim`.
- **Prevenção contra Formula Injection:** Todas as células de texto são gravadas com tipagem estrita de string (`SetCellStr`), e caracteres disparadores (`=`, `+`, `-`, `@`, tab) são neutralizados preventivamente com apóstrofo `'`.
- **4 abas canônicas formatadas:** `Entidades`, `Alegações e Relações`, `Evidências e Fontes`, e `Metodologia e Critérios` (incluindo nota de independência, escala A–E, relevância 1–5 e regras de rede).
- **Rótulos fiéis e linguagem pública:** "Fontes Citadas", "Entidades na Rede" e "Alegações Elegíveis", distinguindo rede ativa do acervo público geral.

## 3. Validações Executadas

Todas as validações foram executadas exclusivamente dentro do container Docker `dev` via Makefile:
- `make templ`: templates recompilados sem pendências;
- `make test`: 100% dos testes unitários, de integração e smoke tests aprovados (incluindo testes de exportação com base vazia, regras de visibilidade e elegibilidade, integridade de células XLSX e rota HTTP de download);
- `make fmt`: código formatado pelo padrão Go;
- `make vet`: análise estática do compilador Go sem avisos;
- `make build`: compilação binária limpa.

*Nota de integridade:* Nenhuma migração ou importação foi aplicada no banco de dados persistente de produção/desenvolvimento durante a validação.

## 4. Proposta de Navegação Documental ([ADR-012](adr/ADR-012-navegacao-documental-e-referencias-a-acervos-externos.md))

Inspirado conceitualmente em projetos de exploração documental (como o [MasterWhats](https://masterwhats.recomendeme.com.br/)), o VorcaroZAP consolidou sua visão de produto para permitir a navegação encadeada:
**Entidades &rarr; Alegações &rarr; Evidências/Trechos &rarr; Fontes Originais e Defesas**.

Limites e diretrizes fixados:
1. **Sem reuso não autorizado:** Nenhum código ou dado do projeto `masterzap` foi incorporado, dada a ausência de licença identificada na raiz consultada.
2. **Schema preservado:** O schema do MVP já atende plenamente à referenciação pontual através de `sources`, `evidence_sources.locator` e `evidence_sources.excerpt`. Não há criação de tabelas de chat/mensagens neste estágio.
3. **Representação secundária:** Transcrições e resumos nunca substituem o documento oficial, cujo laudo, página e figura devem ser informados.
4. **Origem da informação vs. veículo:** A independência probatória considera a origem documental da informação, e não apenas o veículo que a publicou. Múltiplos veículos que reproduzem a mesma peça/laudo pericial não configuram corroboração independente; um mesmo veículo pode publicar apurações factuais de origens distintas. Citações em mensagens ou notas não configuram conluio ou ilícito.
5. **Diferimento de viewer:** Visualizadores específicos de conversas ficam catalogados como evolução proposta pós-MVP (VZ-023).

## 5. Próximo Passo Prioritário

A próxima etapa funcional após fechamento de VZ-009 é:
- **[VZ-020](09-backlog-inicial.md): Verificador GET seguro/limitado e política de `source_access_status`** (incorporação de `SOURCE_NOT_CHECKED_POLICY_*` às configurações e validação de URLs sem atuar como crawler invasivo), servindo como dependência técnica dos gates de publicação automática da Fase 4.

