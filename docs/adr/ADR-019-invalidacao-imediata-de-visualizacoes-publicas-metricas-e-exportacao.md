# ADR-019 — Invalidação imediata de visualizações públicas, métricas e exportação após moderação

- **Status:** aceito
- **Data:** 2026-09-12
- **Fase:** VZ-017 (Fase 5 — Painel simples e moderação humana)

## Contexto

No VorcaroZAP, deliberações administrativas de pós-moderação humana podem ser aplicadas sobre alegações ([`claims`](../05-modelo-de-dados.md#claims), conforme [ADR-017](ADR-017-moderacao-humana-de-claims-transacoes-e-protecao-csrf.md)) ou sobre vínculos individuais de evidência ([`evidence_sources`](../05-modelo-de-dados.md#evidence_sources), conforme [ADR-018](ADR-018-moderacao-granular-de-evidence-sources-e-quarentena-por-perda-de-suporte.md)).

Essas ações produzem impactos imediatos sobre o estado editorial da plataforma:
1. Aprovar um claim (`quarantined -> published`) torna-o público, agrega suas métricas na Home e inclui o registro na exportação XLSX.
2. Desaprovar/rejeitar um claim (`published -> rejected`) retira a alegação da visibilidade pública, decrementa contagens e distribuições métricas e exclui o registro do arquivo XLSX.
3. Rejeitar um `evidence_source` ativo com papel `supports` que seja o último suporte de um claim publicado move atomicamente o claim para `quarantined` e `metric_eligible = 0`, retirando-o da fronteira pública.
4. Restaurar um claim (`rejected -> quarantined`) ou um `evidence_source` (`rejected -> active`) **nunca** republica o claim automaticamente, mantendo-o invisível na área pública até deliberação explícita.

Para assegurar a tempestividade e a confiabilidade editorial do sistema, é fundamental definir como a invalidação e a consistência das leituras públicas são garantidas imediatamente após qualquer decisão de moderação, sem risco de exibir dados obsoletos (*stale data*) ou requerer reinicialização do processo.

## Decisão

### 1. Arquitetura de Consistência Direta sob Demanda (Sem Cache Intermediário de Aplicação)

O VorcaroZAP adota uma **estratégia de consistência direta sob demanda**, dispensando intencionalmente caches em memória intermediários (`sync.Map`, caches LRU em heap de aplicação, Redis ou materializações temporárias):
- **Consultas em Tempo Real:** Todas as requisições públicas (`/`, `/pessoas`, `/pessoas/{slug}`, `/metodologia`) e endpoints de exportação (`/exportar/base.xlsx`) consultam diretamente o banco de dados SQLite a cada ciclo HTTP.
- **Fronteira Canônica Dinâmica:** Todas as queries públicas consomem a view SQLite não materializada `public_claims_view` ([ADR-011](ADR-011-fronteira-publica-canonica-e-metricas-por-estado-ativo.md)), que avalia dinamicamente no momento da execução se `status = 'published'` e se existe ao menos um `evidence_source` ativo com papel `supports`.
- **Motor de Métricas Dinâmico:** A função `metrics.GetPublicMetrics` agrega em tempo real os dados a partir de queries SQL dinâmicas sobre `public_claims_view`, refletindo alterações instantaneamente.
- **Exportação XLSX Dinâmica:** O serviço `exporter.Exporter` extrai do banco o estado canônico exato no momento da requisição (`/exportar/base.xlsx` ou CLI `vorcarozap export`), garantindo que planilhas geradas nunca contenham alegações rejeitadas, quarentenadas ou desprovidas de suporte.

### 2. Garantias Transacionais ACID e Modo SQLite WAL

- Toda ação de moderação (seja em `claims` ou em `evidence_sources`) é executada dentro de uma única transação atômica (`store.WithTx`).
- O SQLite opera em modo **WAL (Write-Ahead Logging)** com `_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`.
- No modo WAL, o commit da transação de escrita torna os novos dados atomicamente e imediatamente visíveis para todas as conexões de leitura concorrentes, sem bloquear leitores e sem criar estados intermediários parciais.
- Em caso de falha de validação ou conflito de concorrência (OCC), a transação sofre rollback total, assegurando que nenhum contador, view ou exportação reflita mutações incompletas.

### 3. Padrão Post-Redirect-Get (PRG) e Ordem de Visibilidade

- Os formulários de moderação submetem requisições `POST /admin/claims/{id}/moderate` e `POST /admin/evidencias/{id}/moderate`.
- O servidor conclui a transação SQLite (commit atômico) **antes** de enviar a resposta HTTP `303 See Other`.
- Consequentemente, qualquer requisição de navegação ou redirecionamento originada pelo cliente após a moderação é atendida consultando o estado comitado mais recente.

### 4. Políticas Estritas de Cabeçalhos HTTP de Cache

Para evitar que navegadores, proxies reversos ou CDN intermediários sirvam dados desatualizados após deliberações de moderação:
- **Consultas Diretas e Ausência de Cache de Aplicação:** A aplicação não memoiza respostas públicas nem mantém objetos em memória, garantindo consistência transacional direta a partir do SQLite WAL.
- **Páginas Públicas Dinâmicas SSR (`/`, `/pessoas`, `/pessoas/{slug}`):** Enviado obrigatoriamente o cabeçalho `Cache-Control: no-cache, no-store, must-revalidate` em todas as respostas (200, 404 e erros), impedindo cache heurístico de navegadores ou proxies transparentes. Essas rotas públicas **não** incluem `Vary: Authorization`, visto que são anônimas e independentes de credenciais.
- **Exportação de Dados (`/exportar/base.xlsx`):** Cabeçalho `Cache-Control: no-cache, no-store, must-revalidate` com `Content-Disposition: attachment`.
- **Área Administrativa (`/admin/*`):** Cabeçalhos obrigatórios `Cache-Control: no-store` e `Vary: Authorization` em todas as rotas da árvore (sucesso, erro, 404, 405 ou negação de CSRF).

### 5. Regras de Invalidação por Ação de Moderação

| Ação de Moderação | Estado Anterior | Estado Posterior | Reflexo Imediato na Fronteira Pública |
| :--- | :--- | :--- | :--- |
| **Claim: `approve`** | `quarantined` | `published` | Incluído imediatamente em `/`, `/pessoas`, `/pessoas/{slug}` e no XLSX. |
| **Claim: `reject`** | `published` | `rejected` | Removido imediatamente de `/`, `/pessoas`, `/pessoas/{slug}` e do XLSX. Se a entidade não tiver outros claims públicos, retorna 404. |
| **Claim: `restore`** | `rejected` | `quarantined` | **Permanece invisível** na área pública, métricas e XLSX até nova aprovação. |
| **Evidence: `reject` (último suporte)** | `active` (`supports`) | `rejected` | Claim transita atomicamente para `quarantined`, desaparecendo imediatamente de `/`, `/pessoas`, `/pessoas/{slug}` e XLSX. A fonte global em `sources` permanece intacta. |
| **Evidence: `reject` (não-último)** | `active` (`supports`) | `rejected` | Claim permanece `published`, mas a evidência específica é omitida de `/pessoas/{slug}` e da aba "Evidências e Fontes" do XLSX. |
| **Evidence: `restore`** | `rejected` | `active` | Vínculo é reativado no admin, mas **não republica** o claim associado (mantendo-o em quarentena até aprovação explícita). |

## Consequências

### Positivas
- **Simplicidade e Confiabilidade:** Elimina riscos de descompasso de cache (*cache invalidation bugs*), *thundering herd* ou inconsistências temporais entre a área administrativa e a visualização pública.
- **Zero Infraestrutura Adicional:** Preserva a arquitetura do MVP (monólito Go + SQLite + SSR em instância única) sem requerer Redis, Memcached ou mensageria externa.
- **Auditabilidade e Determinismo:** Garante que toda leitura é um reflexo exato e determinístico da base de dados comitada.

### Limitações e Evoluções Futuras
- À medida que o tráfego e o volume de dados crescerem no pós-MVP (Fase 7), estratégias de cache em memória com invalidação baseada em versão monotônica (`revision_id`) poderão ser avaliadas, sem alterar as garantias transacionais do domínio.
