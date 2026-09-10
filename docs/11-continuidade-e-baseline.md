# Continuidade Técnica e Baseline Operacional

Documento sintético de continuidade registrando o estado verificado do VorcaroZAP, validações executadas, pendências e diretrizes da evolução documental.

## 1. Baseline e Estado Comprovado

A aplicação opera como monólito Go modular, persistido em SQLite modo WAL (`modernc.org/sqlite`, [ADR-010](adr/ADR-010-escolha-do-driver-sqlite.md)), com renderização Server-Side (Templ) sem dependência de JavaScript. O ambiente é 100% conteinerizado via Docker.

### Entregas verificadas no código:
- **Fase 1 — Fundação:** CLI, servidor Chi, migrations (Goose), Dockerfile enxuto não-root e Compose com proxy Caddy opcional ([ADR-001](adr/ADR-001-monolito-modular-em-go.md), [ADR-002](adr/ADR-002-sqlite-com-wal.md), [ADR-003](adr/ADR-003-renderizacao-server-side.md)).
- **Fase 2 — Núcleo Editorial e Carga:** Schema relacional estrito (`00002_editorial_core.sql`), importador transacional XLSX `curated_seed` com dry-run, mapeamento versionado em YAML e idempotência comprovada ([ADR-009](adr/ADR-009-importacao-curated-seed-e-mapeamento-versionado.md)).
- **Fase 3 — Apresentação Pública e Métricas:** Listagem e filtros em `/pessoas`, detalhe de entidades e fontes em `/pessoas/{slug}`, metodologia e critérios em `/metodologia`, engine de agregação em tempo real da fronteira ativa (`public_claims_view`, [ADR-011](adr/ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)) e Home pública com indicadores e distribuições (`/`).

## 2. Saneamento de Apresentação Realizado

No ciclo recente de saneamento, corrigiram-se achados textuais na Home e no README:
- **Rótulos fiéis:** “Fontes Auditadas” &rarr; “Fontes Citadas” (fiel à contagem de fontes ativas distintas); “Pessoas na Rede” &rarr; “Entidades na Rede” (abrangendo pessoas físicas e organizações); “Vínculos Qualificados” &rarr; “Alegações Elegíveis” (unidade canônica `claim`).
- **Linguagem pública transparente:** Remoção de termos internos de banco de dados (`published`, `metric_eligible`, `OpenRouter`) na interface de usuário.
- **Rigor factual:** Eliminação de afirmações genéricas de “comprovação documental” ou “relações comprovadas” não sustentadas pela política (Graus D/E do seed podem ser públicos/elegíveis; `not_checked` indica ausência de verificação, não auditoria).
- **Distinção entre rede e acervo:** Esclarecimento textual de que as distribuições estatísticas refletem a rede elegível, enquanto os links de filtro abrem o acervo público geral em `/pessoas`.
- **Alinhamento de fases:** Sincronização da numeração de fases no `README.md` com o [Roadmap](08-roadmap.md).

## 3. Validações Executadas

Todas as validações foram executadas exclusivamente dentro do container Docker `dev` via Makefile:
- `make templ`: templates recompilados sem pendências;
- `make test`: 100% dos testes unitários e de integração aprovados (incluindo testes de renderização da Home e smoke test da planilha com banco descartável);
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
4. **Menção não é culpa:** Citações em mensagens ou notas não configuram conluio ou ilícito. Múltiplos trechos do mesmo laudo não configuram confirmação independente.
5. **Diferimento de viewer:** Visualizadores específicos de conversas ficam catalogados como evolução proposta pós-MVP (VZ-023).

## 5. Próximo Passo Prioritário

A próxima etapa funcional aprovada para execução é:
- **[VZ-009](09-backlog-inicial.md): Exportação XLSX derivada diretamente do banco de dados**, fechando o ciclo de entrega de dados do MVP antes do início do monitoramento automatizado.
