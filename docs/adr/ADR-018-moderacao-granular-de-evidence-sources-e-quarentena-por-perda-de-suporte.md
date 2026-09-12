# ADR-018 — Moderação granular de evidence_sources e quarentena por perda de suporte ativo

- **Status:** aceito
- **Data:** 2026-09-12
- **Fase:** VZ-021 (Fase 5 — Painel simples e pós-moderação humana)

## Contexto

No VorcaroZAP, uma alegação (`claims`) é sustentada ou contextualizada por um ou mais usos de evidência (`evidence_sources`), que por sua vez referenciam fontes documentais (`sources`). Conforme definido no modelo editorial ([ADR-006](ADR-006-fontes-e-rastreabilidade.md)) e na decisão de moderação de claims ([ADR-017](ADR-017-moderacao-humana-de-claims-transacoes-e-protecao-csrf.md)):

1. A fonte documental (`sources`) representa o documento bruto verificável (URL, veículo, dados de acesso) e **nunca recebe rejeição editorial global**. A mesma fonte pode sustentar outros fatos legítimos de forma independente.
2. A moderação humana atua cirurgicamente sobre a alegação (`claims`) ou sobre o uso específico da evidência (`evidence_sources`).
3. Uma alegação publicada (`status = 'published'`) exige obrigatoriamente a existência de ao menos um uso de evidência ativo com papel de suporte (`role = 'supports' AND status = 'active'`).
4. Se um administrador rejeitar o último uso de evidência que dava sustentação a um claim publicado, esse claim não pode permanecer visível na área pública nem computar vínculos em métricas de rede.
5. Quando um uso de evidência for restaurado (`status = 'rejected' -> 'active'`), o claim associado que estiver em quarentena não pode ser republicado automaticamente; ele deve permanecer em quarentena até que ocorra uma deliberação humana explícita posterior.
6. Toda deliberação sobre `evidence_sources` deve ser registrada na tabela imutável `moderation_decisions` respeitando a restrição de integridade XOR em banco (`chk_moderation_decisions_target_xor`), garantindo que cada decisão aponte ou para um `claim_id` ou para um `evidence_source_id`.

## Decisão

### 1. Transições de Estado de `evidence_sources` no Domínio Puro

Os usos de evidência possuem status `active` ou `rejected`. As ações humanas permitidas nesta fase são:
- **`reject` (Desaprovar / Rejeitar Uso):** `active -> rejected`. Desativa aquele uso pontual da evidência.
- **`restore` (Restaurar Uso):** `rejected -> active`. Reativa o uso pontual da evidência.
- Qualquer outra ação ou transição sobre `evidence_sources` é rejeitada por validação pura na camada Go (`domain.ValidateEvidenceSourceTransition`).

### 2. Transação SQLite Curta e Quarentena Automática por Perda de Suporte

A moderação de `evidence_sources` é executada em transação atômica SQLite única (`store.ExecTx`):
1. **Controle Otimista de Versão (OCC):** O formulário submete `expected_updated_at`. A transação consulta o registro atual (`SELECT id, status, role, updated_at, claim_id FROM evidence_sources ...`) e valida se a versão esperada confere. A atualização condicional (`WHERE id = @id AND updated_at = @expected_updated_at`) aborta com erro sentinela `moderation.ErrConflict` (HTTP 409) caso 0 linhas sejam afetadas.
2. **Revalidação de Suportes Ativos do Claim:**
   - Ao executar `reject` em um `evidence_source` cujo papel é `supports`, a transação calcula quantos **outros** suportes ativos o claim possui (`WHERE ev.claim_id = @claim_id AND es.role = 'supports' AND es.status = 'active' AND es.id != @current_es_id`).
   - Se a contagem for zero (ou seja, este era o **último suporte ativo** da alegação):
     - Se o claim estiver `published`, ele é alterado atomicamente na mesma transação para `status = 'quarantined'` e `metric_eligible = 0`;
     - A alegação deixa imediatamente de constar na visão pública `public_claims_view`, nas listagens públicas, nas métricas agregadas de rede e na exportação XLSX.
3. **Invariante de Restauração:**
   - Ao executar `restore` em um `evidence_source`, seu status retorna para `active`.
   - O claim associado **não é republicado automaticamente** e permanece em `quarantined`, preservando a exigência de deliberação editorial explícita para republicação.
4. **Isolamento da Fonte Global:**
   - A tabela `sources` não é modificada sob nenhuma hipótese durante a moderação de um `evidence_source`.
5. **Auditoria Imutável em `moderation_decisions`:**
   - Registra `id`, `evidence_source_id`, `claim_id = NULL`, `action`, `reason` (validado, sem HTML, até 1000 caracteres), `actor` (extraído exclusivamente do contexto HTTP autenticado) e `created_at` (UTC).

### 3. Interface Administrativa SSR, Proteção CSRF e Padrão PRG

- **Rota de Inspeção:** `GET /admin/evidencias/{id}` exibe a ficha completa do uso de evidência, trecho literal, locator, papel probatório, dados do claim associado, dados da fonte documental e histórico de deliberações anteriores.
- **Aviso de Último Suporte:** Quando o uso de evidência for o único suporte ativo de um claim publicado, a interface exibe aviso de advertência editorial explícito alertando que a rejeição colocará o claim em quarentena.
- **Rota de Moderação:** `POST /admin/evidencias/{id}/moderate` com campos `action`, `reason` e `expected_updated_at`.
- **Proteção CSRF e Limites:** Interceptada pelo middleware `AdminCSRFMiddleware`, exigindo validação estrita do cabeçalho `Origin` contra `ADMIN_ALLOWED_ORIGIN`, `Content-Type: application/x-www-form-urlencoded` e limite de 64 KiB no corpo da requisição.
- **Padrão PRG:** Resposta `303 See Other` redirecionando para `/admin/evidencias/{id}?msg=...` após o commit bem-sucedido.

## Consequências

### Positivas
- **Granularidade e Precisão:** Permite desaprovar citações truncadas, equivocadas ou imprecisas sem invalidar o documento original (`sources`) nem exigir exclusão destrutiva.
- **Integridade da Fronteira Pública:** Alegações sem sustentação probatória ativa são removidas instantaneamente das visualizações públicas e métricas de rede no mesmo commit atômico.
- **Auditoria Completa:** Toda deliberação preserva autoria e justificativa imutáveis no banco relacional via restrição XOR.
- **Segurança contra Concorrência:** OCC e transações curtas impedem estados parciais e sobreposições desordenadas.
