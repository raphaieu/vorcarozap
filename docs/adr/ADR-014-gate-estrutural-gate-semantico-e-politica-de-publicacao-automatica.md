# ADR-014 — Gate estrutural em Go, gate semântico via OpenRouter e política de publicação automática

- **Status:** aceito
- **Data:** 2026-09-11
- **Fase:** VZ-012 (Fase 4 — Monitoramento OpenRouter)

## Contexto

A esteira de monitoramento automatizado do VorcaroZAP utiliza o `ResearchProvider` baseado em OpenRouter para descobrir publicações e fatos documentados na web. Na fase VZ-011, estruturou-se a persistência e deduplicação determinística dos candidatos em quarentena.

Contudo, para que um candidato de monitoramento possa transitar com segurança da quarentena para o estado público (`published`), é indispensável resolver a lacuna relacional e editorial:
1. Um `claim` público no VorcaroZAP não existe no vácuo: ele requer obrigatoriamente um `relationship_id` que ancora o sujeito a uma entidade-alvo ou a um caso de investigação (com restrição XOR e sem autorrelação).
2. O schema anterior de `monitoring_candidates` contemplava apenas `entity_name` (sujeito), sendo insuficiente para materializar um vínculo relacional sem inferências perigosas.
3. A LLM pode recomendar ações, mas não possui autoridade editorial: a decisão de publicação, disposição (`supports_link`, `possible_link`) e elegibilidade a métricas de rede (`metric_eligible`) pertence exclusivamente a regras puras e auditáveis em Go.
4. Falhas de acessibilidade de rede, homônimos, ambiguidades ou extrapolações factuais exigem contenção imediata em quarentena (*fail closed*), sem descarte de histórico e sem poluição da fronteira pública.

## Decisão

### 1. Extensão Relacional Estruturada do Candidato

Evolui-se o schema de `monitoring_candidates` e o contrato de extração do `ResearchProvider` para carregar explicitamente o contexto relacional do fato:
- `target_entity_name` / `normalized_target_entity_name`: nome textual da entidade-alvo mencionada;
- `case_name` / `normalized_case_name`: nome textual do caso/operação contextual;
- `relationship_type`: natureza do vínculo factual documentado (ex.: `"societário"`, `"investigado"`, `"institucional"`);
- Constraints estritas em banco:
  - XOR de alvo textual: `(target_entity_name != '' AND case_name = '') OR (target_entity_name = '' AND case_name != '')` para candidatos com alvo definido;
  - Bloqueio de autorrelação nominal: `normalized_target_entity_name = '' OR normalized_entity_name = '' OR normalized_target_entity_name != normalized_entity_name`.

### 2. Resolução Determinística Exclusiva contra Entidades e Casos Existentes

A resolução de identidades opera de forma determinística e não ambígua contra a base existente:
- Sujeito e alvo são confrontados contra `entities.normalized_name` e `entity_aliases.normalized_alias`.
- Caso é confrontado contra `cases.slug` e `cases.name`.
- **Proibição estrita de criação automática:** o pipeline **nunca** cria novas entidades, aliases ou casos automaticamente a partir de extrações de LLM.
- **Fail closed:** ausência de registro correspondente, múltiplos registros encontrados (ambiguidade/homônimo) ou autorreferência impedem o avanço para a verificação semântica e mantêm o candidato em `quarantined`.
- Os IDs resolvidos (`resolved_subject_entity_id`, `resolved_target_entity_id`, `resolved_case_id`) são persistidos no candidato para auditoria e vinculação.

### 3. Isolamento Transacional e Verificação Fora de Transação

- **Verificação HTTP (`sourcecheck`):** a validação GET segura e limitada da URL da fonte é realizada fora de transações de banco. Candidatos da origem `openrouter` exigem status comprovado `reachable` para prosseguir; `unreachable`, `not_checked`, `cited_by_provider`, erro transitório ou bloqueio SSRF levam à quarentena.
- **Chamada ao Gate Semântico (`ResearchProvider.Verify`):** a requisição ao OpenRouter ocorre fora de transações SQLite, utilizando modelo configurável (`OPENROUTER_VERIFICATION_MODEL`), prompt defensivo anti-injection e JSON Schema estrito sem ferramentas de busca web.

### 4. Gate Estrutural Puro em Go

O gate estrutural avalia deterministicamente:
- Candidato canônico (`is_duplicate = 0`) com fingerprint versionado v1;
- Schema completo e campos obrigatórios preenchidos (URL `http/https`, título, autor/veículo, trecho literal, proposição);
- Data no padrão ISO `YYYY-MM-DD` quando informada;
- Grau A–E válido;
- Alvo textual e IDs resolvidos respeitando XOR e bloqueio de autorrelação;
- Limites de tamanho de caracteres em todos os campos de texto;
- Ausência de PII desnecessária (CPF, RG, telefones, endereços privados);
- Ausência de candidato rejeitado anteriormente com o mesmo fingerprint v1;
- Acessibilidade da fonte com status `reachable`.

Se o gate estrutural for reprovado, o pipeline não chama o modelo de verificação semântica e grava os motivos em `structural_gate_reasons`, definindo `policy_action = 'quarantine'`.

### 5. Gate Semântico e Auditoria Histórica

O gate semântico consome o resultado estruturado da LLM (`VerifyResult`):
- Exige simultaneamente: `identity_match = true`, `claim_supported = true`, `claim_overstates_source = false`, `attribution_explicit = true`, `grade_compatible = true`, `contains_illicit_inference = false` e `uncertainties = []`.
- Toda avaliação semântica executada é persistida na tabela `semantic_evaluations` para histórico e auditoria técnica.
- Qualquer valor inconsistente, divergência semântica ou incerteza declarada reprova o gate semântico e mantém o candidato em quarentena.

### 6. Política Go de Publicação e Disposição

A política de publicação em Go delibera com base nos dois gates:
- **Graus A e B:** publicação automática se ambos os gates forem aprovados. Disposição: `supports_link`; `metric_eligible = true`.
- **Grau C:** publicação automática se ambos os gates forem aprovados, `context_limits` estiver explicitamente documentado e a proposição utilizar linguagem conservadora de associação factual. Disposição: `possible_link`; `metric_eligible = true`.
- **Graus D e E:** quarentena automática por padrão (`policy_action = 'quarantine'`), reservando deliberação para o painel administrativo.
- **Qualquer Grau:** quarentena diante de falhas de gate, homônimos, divergências ou rejeição anterior de fingerprint.

### 7. Materialização Editorial Atômica Curta

Somente quando a política Go determinar `publish`:
- Uma transação SQLite curta e atômica (`store.ExecTx`) é iniciada;
- Revalida-se que o candidato não possui `published_claim_id` e não teve fingerprint rejeitado concorrentemente;
- Reutiliza-se ou cria-se o registro em `sources` pela `canonical_url`, atualizando seu status de acesso;
- Reutiliza-se ou cria-se o relacionamento em `relationships`;
- Cria-se o `claim` (`origin = 'openrouter'`, `status = 'published'`, `grade`, `disposition`, `metric_eligible`);
- Cria-se a `evidence` e o `evidence_source` ativo (`role = 'supports'`);
- Atualiza-se o candidato com `published_claim_id = claim.id` e `editorial_status = 'published'`;
- Qualquer falha provoca rollback total, preservando o candidato em quarentena com o motivo do erro.

A tabela `sources` continua representando documentos técnicos reutilizáveis e não sofre rejeição editorial global.

## Consequências

### Positivas
- Garante integridade referencial estrita e elimina riscos de alegações órfãs ou com sujeitos e alvos inventados;
- Implementa blindagem em profundidade (*defense in depth*) com dois gates independentes e política Go determinística;
- Assegura rastreabilidade total entre candidatos ingeridos, avaliações semânticas e alegações públicas publicadas;
- Preserva a fronteira pública (`public_claims_view`) e o motor de métricas de rede, mantendo `metric_eligible = true` restrito a vínculos legítimos publicados.

### Negativas / Limitações
- Exige que entidades e casos já estejam cadastrados na base para que a publicação automática ocorra (novas entidades encontradas na web permanecerão em quarentena até cadastramento administrativo prévio);
- O ciclo de agendamento em segundo plano, lock concorrente e limites orçamentários cumulativos permanecem escopo da fase VZ-013.

## Gatilhos de Revisão
- Necessidade de enriquecer o vocabulário de relacionamentos ou introduzir resolução probabilística assistida no painel administrativo;
- Alterações na API de Structured Outputs do OpenRouter.
