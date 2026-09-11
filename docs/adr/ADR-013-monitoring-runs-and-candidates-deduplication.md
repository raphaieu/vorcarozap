# ADR-013 — Schema de monitoramento, extração estruturada de candidatos e deduplicação auditável

- **Status:** aceito
- **Data:** 2026-09-11
- **Fase:** VZ-011 (Fase 4 — Monitoramento OpenRouter)

## Contexto

A esteira de monitoramento automatizado do VorcaroZAP utiliza o `ResearchProvider` baseado em OpenRouter para descobrir publicações e fatos documentados na web. Para que essa descoberta seja segura, determinística e editorialmente responsável, o sistema precisa:

1. Superar o modelo de prosa editorial livre, exigindo que o modelo de linguagem retorne extrações estritamente estruturadas em conformidade com um JSON Schema rígido (`response_format` com `strict: true`);
2. Desvincular completamente o ciclo operacional das execuções de monitoramento dos estados editoriais das alegações públicas;
3. Persistir com fidelidade histórica e auditabilidade técnica todas as extrações geradas, sem descarte silencioso de duplicatas;
4. Identificar duplicidades técnicas de forma determinística e estável entre execuções distintas e dentro da mesma resposta do modelo, sem usar URL isolada como prova de duplicidade;
5. Garantir que a fase de ingestão estruturada (VZ-011) não publique conteúdo automaticamente nem afete a fronteira pública canônica (`public_claims_view`) ou o motor de métricas de rede.

## Decisão

### 1. Separação entre ciclo operacional do run e estado editorial do candidato

Adota-se uma separação estrita de domínios entre as tabelas `monitoring_runs` e `monitoring_candidates`:

- **`monitoring_runs.status`** gerencia exclusivamente o ciclo de vida operacional da execução técnica:
  - `pending`: execução criada, aguardando início;
  - `running`: execução ativa (busca ou ingestão em andamento);
  - `partial`: execução concluída com advertências técnicas ou candidatos com inconsistências pontuais;
  - `completed`: execução concluída com sucesso total;
  - `failed`: execução abortada por falha técnica de rede, autenticação ou timeout.
- **`monitoring_candidates.editorial_status`** gerencia o estado editorial da extração. Na fase VZ-011:
  - Todo candidato recém-ingerido é gravado obrigatoriamente como `quarantined`.
  - Esta fase **não cria nem muta** registros nas tabelas editoriais públicas (`claims`, `entities`, `relationships`, `sources`, `evidence` ou `evidence_sources`).
  - A publicação automática controlada e a avaliação semântica pertencem exclusivamente às fases subsequentes (VZ-012/VZ-013).

### 2. Extração estruturada via JSON Schema estrito (Structured Outputs)

A comunicação com o modelo no endpoint do OpenRouter utiliza `response_format` com schema JSON estrito (`strict: true`, `additionalProperties: false` e todos os campos no array `required`).

O payload estruturado contempla exclusivamente:
- `entity_name`: entidade/identidade textual encontrada;
- `proposition`: proposição factual atribuível;
- `suggested_grade`: grau sugerido na escala A–E;
- `source_url`: URL da publicação citada;
- `source_title`: título da matéria/documento;
- `publisher_or_author`: veículo ou autor responsável;
- `published_at`: data da publicação informada ou vazia;
- `excerpt`: trecho literal comprobatório ou citação extraída;
- `locator`: localizador pontual (página, seção ou figura);
- `context_limits`: limites contextuais ou ressalvas da fonte;
- `technical_confidence`: valor numérico em ponto flutuante no intervalo `[0.0, 1.0]`.

Todo conteúdo recebido é validado localmente em Go. Falhas de parsing, campos obrigatórios ausentes ou valores fora de domínio impedem a promoção do candidato e resultam em estado operacional `failed` ou `partial` da execução, assegurando o princípio *fail closed*.

### 3. Normalização pura e fingerprint determinístico versionado (v1)

A normalização de dados é implementada em pacote puro Go (`internal/normalize`), sem acoplamento a banco de dados ou bibliotecas externas de importação:
- Normalização Unicode em forma canônica NFC (`norm.NFC`);
- Colapso de sequências de espaços em branco e remoção de espaços nas extremidades;
- Normalização conservadora de URLs canônicas (`CanonicalURL`): esquema restrito a `http/https`, host em minúsculas, remoção de portas padrão (`:80`, `:443`), remoção de fragmentos (`#...`) e barra inicial no caminho vazio.

O fingerprint global do candidato é gerado de forma determinística pela função de hash criptográfico SHA-256 sobre a concatenação dos campos canônicos essenciais delimitados por pipe (`|`):

$$\text{Fingerprint}_{\text{v1}} = \text{"v1:"} + \text{SHA-256}(\text{canonical\_url} \parallel "|" \parallel \text{normalized\_entity\_name} \parallel "|" \parallel \text{normalized\_proposition} \parallel "|" \parallel \text{normalized\_excerpt})$$

Essa composição assegura que:
- A mesma fonte citando diferentes entidades gera fingerprints distintos;
- A mesma matéria contendo múltiplas proposições distintas gera fingerprints distintos;
- Variações superficiais de espaçamento ou maiúsculas/minúsculas convergem para o mesmo hash;
- O prefixo de versão (`v1:`) viabiliza evolução futura do algoritmo de hashing sem perda de integridade dos registros anteriores.

### 4. Deduplicação técnica sem descarte de histórico (Auditabilidade Total)

A duplicidade técnica é tratada como uma propriedade operacional segregada do estado editorial:
- Todo candidato extraído em um run é persistido em `monitoring_candidates`, mantendo o vínculo com seu respectivo `monitoring_run_id`.
- Se um candidato for identificado como duplicado (por repetição intra-run ou por colisão com candidato canônico de run anterior):
  - É marcado com `is_duplicate = 1`;
  - Registra a razão técnica (`duplicate_reason = 'same_run_duplicate'` ou `'existing_fingerprint'`);
  - Aponta `canonical_candidate_id` para a chave primária do candidato canônico original.
- O candidato canônico mantém `is_duplicate = 0` e `canonical_candidate_id = NULL`.
- Isso garante rastreabilidade orçamentária e analítica: sabe-se exatamente quantas vezes um fato foi reencontrado por diferentes runs e quais tokens foram consumidos.

### 5. Isolamento transacional

Chamadas de rede HTTP ao OpenRouter são executadas obrigatoriamente fora de qualquer transação SQLite aberta. As operações de banco no SQLite utilizam transações atômicas de curta duração para registrar o início do run, persistir os candidatos estruturados e atualizar o sumário final da execução.

## Consequências

### Positivas
- Elimina alucinações de formato ao utilizar JSON Schema estrito nativo com validação em Go;
- Cria um pipeline determinístico de deduplicação sem perda de histórico de execuções;
- Mantém a fronteira pública e as métricas de rede blindadas contra poluição por dados não validados;
- Prepara o schema de candidatos com colunas dedicadas aos resultados dos gates estrutural e semântico de VZ-012 (`structural_gate_passed`, `semantic_gate_passed`, `policy_action`).

### Negativas / Limitações
- Duplicatas consom armazenamento em `monitoring_candidates`, embora com baixo impacto no volume esperado para o MVP;
- Não há publicação automática nesta fase: o valor dos candidatos permanece restrito à área operacional/quarentena até a implementação dos gates em VZ-012.

## Gatilhos de revisão
- Necessidade de enriquecer os componentes do fingerprint v1 caso surjam casos reais de colisão indevida (disparando a criação do fingerprint v2);
- Mudança nas especificações da API do OpenRouter para Structured Outputs ou Web Search.
