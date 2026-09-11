# ADR-015 — Lock de execução com lease SQLite, janela incremental determinística e limites de custo de LLM

- **Status:** aceito
- **Data:** 2026-09-11
- **Fase:** VZ-013 (Fase 4 — Monitoramento OpenRouter)

## Contexto

O VorcaroZAP opera como um monólito Go com banco de dados SQLite em modo WAL, renderização SSR e execução em instância única. Com a introdução da esteira de monitoramento automatizado via OpenRouter (descoberta web e verificação semântica em dois gates), surgem três requisitos operacionais críticos para viabilizar execuções agendadas (cron) e manuais (CLI) em produção econômica:

1. **Prevenção de Execuções Concorrentes e Travamentos Órfãos:**
   - Múltiplas execuções simultâneas de monitoramento (seja por sobreposição de agendamento cron ou comandos concorrentes) causariam contenção de escrita no SQLite, duplicação de chamadas de LLM e estouro financeiro.
   - Um lock puramente em memória ou via mutex local não protege execuções independentes de CLI iniciadas em processos distintos.
   - Travamentos abruptos (SIGKILL, falha de infraestrutura) poderiam deixar um lock booleano estático permanentemente travado, exigindo intervenção manual.

2. **Janela Temporal Incremental Determinística:**
   - Para evitar lacunas temporais ou reprocessamento desnecessário, a busca por novos fatos precisa calcular o intervalo `[window_start, window_end]` com base na última execução com sucesso operacional para aquela consulta.
   - Execuções com falha (`failed`) ou canceladas não devem avançar a janela temporal, permitindo que a próxima execução retome a cobertura desde o último ponto seguro.

3. **Controle Financeiro Rigoroso e Uso Real do Provedor:**
   - Preços de modelos de LLM variam ao longo do tempo e tabelas estáticas locais de precificação ficam obsoletas rapidamente.
   - O OpenRouter retorna o custo real cobrado na resposta de cada requisição (`usage.cost`).
   - A esteira precisa de travas orçamentárias (por execução e por dia) para interromper o pipeline com segurança (*fail closed* e término limpo em status `partial`), gravando os tokens e custos reais consumidos tanto na descoberta quanto em cada verificação semântica individual.

## Decisão

### 1. Lock Distribuído em SQLite com Lease e Renovação Periódica

Implementa-se a tabela `monitoring_locks` com controle atômico de concessão (*lease*):
- **Aquisição Atômica:** realizada via instrução SQL condicional (`INSERT ... ON CONFLICT DO UPDATE ... WHERE expires_at < now RETURNING holder`), garantindo que apenas um titular ativo adquira o lock sem *race conditions*.
- **Expiração Automática (TTL):** o lock possui tempo de vida delimitado (`MONITOR_LOCK_TTL`, padrão 10 minutos). Se o titular morrer de forma abrupta, outra execução pode assumir o lock assim que `expires_at` for ultrapassado.
- **Renovação Periódica em Segundo Plano:** uma goroutine com `time.Ticker` (intervalo de `TTL / 3`) renova atômica e condicionalmente o lock (`UPDATE monitoring_locks SET expires_at = ?, updated_at = ? WHERE name = ? AND holder = ?`).
- **Cancelamento Reativo:** se a renovação falhar (perda de lease), o contexto da execução é imediatamente cancelado, paralisando a esteira com segurança.
- **Liberação Condicional:** ao término da execução, o lock é liberado apenas se o titular atual corresponder ao titular registrado (`DELETE FROM monitoring_locks WHERE name = ? AND holder = ?`).

### 2. Cálculo Determinístico da Janela Temporal

O cálculo da janela segue regras matemáticas estritas em UTC:
- `window_end`: fixado no instante inicial da execução (`now.UTC()`).
- `last_successful_run`: consulta exclusivamente a última execução com status operacional `completed` para a mesma consulta (ordenada por `window_end DESC, created_at DESC`).
- Se houver execução prévia `completed` dentro do limite máximo de retroação (`MONITOR_WINDOW`, padrão 24h, máximo 720h): `window_start = last_successful_run.window_end`.
- Se não houver execução anterior `completed` ou se o último `window_end` for anterior a `now - MONITOR_WINDOW`: `window_start = max(window_end - MONITOR_WINDOW, last_successful_run.window_end)` ou o limite de retroação.
- **Resiliência a Falhas:** execuções com status `failed` ou `partial` não avançam o watermark temporal, assegurando que o intervalo temporal pendente seja reexaminado na execução seguinte.
- Os limites temporais em UTC são transmitidos explicitamente ao prompt do `ResearchProvider` e persistidos em `monitoring_runs (window_start, window_end)`.

### 3. Gestão Financeira Baseada em Custo Real do OpenRouter e Limites Operacionais

- **Fonte de Verdade:** o campo `usage.cost` retornado pelas APIs do OpenRouter é a fonte primária e mandatória de custo financeiro em USD.
- **Aritmética e Persistência Segura em Inteiros (Micro-USD):** para eliminar qualquer imprecisão de ponto flutuante em decisões e persistência após reinício do processo, todos os custos são persistidos e somados diretamente como inteiros (`cost_microusd INTEGER`, `discovery_cost_microusd`, `verification_cost_microusd`, `total_cost_microusd`).
- **Conversão Exata e Fail-Closed sem Float:** o campo `usage.cost` é recebido em JSON bruto (`json.RawMessage`) e convertido via aritmética racional exata (`math/big.Rat`) multiplicada por `1_000_000`, aplicando divisão inteira com teto (*ceiling*) em qualquer resto fracionário positivo. Rejeita custo ausente, `null`, string, negativo, NaN, infinito, formato inválido e overflow. Custo numérico explícito `0` ou `0.0` é aceito. Colunas `REAL` legadas (`total_cost`, etc.) são calculadas no SQL exclusivamente a partir do inteiro (`CAST(cost_microusd AS REAL) / 1000000.0`), sem receber decimais brutos da aplicação.
- **Orçamento Diário UTC Fixo:** a checagem orçamentária diária consulta a soma dos inteiros `total_cost_microusd` de todas as runs (`completed`, `partial`, `running`, `failed`) criadas a partir do início do dia corrente em UTC (`00:00:00Z`), e não uma janela móvel de 24 horas.
- **Aplicação de Limite de Candidatos (`MONITOR_MAX_CANDIDATES_PER_RUN`):** a quantidade de candidatos avaliados na esteira é deterministicamente restrita pelo teto configurado. Candidatos além do limite permanecem persistidos e auditáveis em `quarantined`, e a run finaliza como `partial` com justificativa técnica explícita.
- **Atomicidade da Verificação Semântica:** a inserção em `semantic_evaluations`, atualização do candidato (`quarantined`/`published`), materialização de claims/evidências e incremento no ledger da run (`IncrementMonitoringRunUsage`) ocorrem na *mesma* transação curta de banco de dados, sem descarte de erros de conversão ou persistência.
- **Preservação Imediata de Custos e Tokens e Resiliência a Perda de Lease:**
  - Imediatamente após o retorno da etapa de descoberta (`Discover`) ou de cada verificação (`Verify`), o consumo de tokens e micro-USD é persistido no banco de dados com um contexto independente e curto (5s).
  - Em caso de cancelamento por perda de lease, a run encerra como `failed` ou `partial`, mas os tokens e micro-USD já consumidos ficam 100% preservados no banco e na soma orçamentária diária, e nenhuma chamada externa adicional é disparada.

### 4. Separação Estrita de Ciclos de Vida (Operacional vs. Editorial)

- **Ciclo Operacional (`monitoring_runs`):** transita exclusivamente entre `running` -> `completed` / `partial` / `failed`. Registra métricas de desempenho, tokens, custos e diagnósticos técnicos.
- **Ciclo Editorial (`monitoring_candidates` / `claims`):** transita entre `quarantined`, `published`, `rejected`, `deprecated`. Uma interrupção orçamentária (`partial`) não invalida nem altera o estado dos candidatos já processados ou pendentes.

### 5. Subcomando CLI `vorcarozap monitor`

Disponibiliza o comando para execução agendada e manual:
- Exige `--query` não vazia e rejeita consultas em branco.
- Aceita injeção de dependências em testes sem dependência de chaves externas.
- Inicializa provedor OpenRouter, verificador de fontes e runner com as configurações validadas.
- Emite sumário estruturado contendo status operacional, janela UTC, candidatos descobertos/únicos/duplicados, publicações, quarentenas, tokens e custos reais consolidados.

## Consequências

### Positivas
- **Segurança contra concorrência:** impossibilidade de sobreposição de jobs cron ou execuções manuais gerando escritas conflitantes ou custos duplicados.
- **Previsibilidade orçamentária:** garantia de que o consumo de LLM nunca ultrapassará os limites configurados por execução ou por dia.
- **Cobertura temporal contínua:** ausência de lacunas temporais entre execuções bem-sucedidas.
- **Auditoria detalhada:** rastreabilidade completa de tokens e dólares gastos em cada run e em cada verificação semântica.

### Limitações e Próximos Passos
- O lock via banco SQLite é projetado para arquitetura single-instance/processos locais.
- A granularidade da janela incremental é por consulta (`query`). Consultas com variações textuais mínimas são tratadas como chaves de monitoramento distintas.
