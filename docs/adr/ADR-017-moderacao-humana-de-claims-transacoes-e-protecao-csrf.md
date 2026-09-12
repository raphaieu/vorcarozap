# ADR-017 — Moderação humana de claims, transações atômicas e proteção CSRF

- **Status:** aceito
- **Data:** 2026-09-12
- **Fase:** VZ-016 (Fase 5 — Painel simples e moderação humana)

## Contexto

No VorcaroZAP, alegações documentais (`claims`) podem ter origem em carga curada (`curated_seed`), ingestão automatizada via OpenRouter (`openrouter`) ou inserção manual (`admin`). Conforme definido no [ADR-005](ADR-005-publicacao-automatica-controlada-e-pos-moderacao.md), itens com Graus A, B ou C podem ser publicados automaticamente após passarem pelos dois gates (estrutural e semântico), enquanto Graus D, E ou casos com ambiguidades entram em quarentena (`quarantined`).

Para garantir a integridade editorial, correção de equívocos e controle humano tempestivo:
1. O administrador precisa de capacidade de pós-moderação para: aprovar alegações em quarentena (`approve`), desaprovar/rejeitar alegações públicas ou em quarentena (`reject`) e restaurar alegações anteriormente rejeitadas ou arquivadas (`restore`).
2. Toda deliberação humana de moderação deve ser registrada de maneira auditável e imutável, gravando o autor da ação (`actor`), o motivo fundamentado (`reason`), a ação aplicada e o fingerprint do candidato associado, se houver.
3. Conforme o modelo editorial ([ADR-006](ADR-006-fontes-e-rastreabilidade.md)), uma fonte documental (`sources`) representa o documento bruto verificável e nunca sofre rejeição editorial global. O alvo da moderação humana no MVP inicial (VZ-016) é exclusivamente a **alegação (`claims`)**. A moderação granular de usos de evidência (`evidence_sources`) e a regra de quarentena por perda do último suporte ativo são de responsabilidade da etapa **VZ-021**.
4. Ações de moderação não podem permitir concorrência descontrolada ou estados parciais: cada mutação deve ser executada em transação SQLite curta com validação rigorosa de invariantes em Go.
5. Rotas mutantes no `/admin` precisam de proteção robusta contra Cross-Site Request Forgery (CSRF), assegurando que requisições provenham exclusivamente da origem configurada e autorizada.

## Decisão

### 1. Escopo Exclusivo em Claims e Transições Permitidas (VZ-016)

A moderação humana nesta fase atua estritamente sobre a tabela `claims`:
- **`approve` (Aprovar):** `quarantined -> published`. Exige que o claim possua ao menos um `evidence_source` ativo com papel `supports` (`role = 'supports' AND status = 'active'`). Se não houver suporte ativo, a aprovação é rejeitada.
- **`reject` (Rejeitar):** `published -> rejected` ou `quarantined -> rejected`. Retira a alegação da visibilidade pública e das métricas de rede imediatamente. Atualiza o status de candidatos canônicos em `monitoring_candidates` associados por `published_claim_id` para `rejected`, mantendo o bloqueio determinístico por fingerprint contra republicações automáticas semelhantes.
- **`restore` (Restaurar):** `rejected -> quarantined` ou `archived -> quarantined`. **Invariante fundamental:** a restauração nunca republica diretamente (`quarantined` é sempre o destino intermediário obrigatório). A eventual publicação exige uma segunda deliberação humana explícita de aprovação.
- Qualquer outra combinação de transição de estado é rejeitada por regras puras de domínio em Go.

### 2. Tabela de Decisões de Moderação (`moderation_decisions`)

Cria-se a tabela `moderation_decisions` na migration `00009_moderation_claim_actions.sql` com integridade referencial estrita:
- `id TEXT PRIMARY KEY`: identificador UUID v4;
- `claim_id TEXT REFERENCES claims(id) ON DELETE RESTRICT`: chave estrangeira anulável;
- `evidence_source_id TEXT REFERENCES evidence_sources(id) ON DELETE RESTRICT`: chave estrangeira anulável (não utilizada em VZ-016, reservada para a VZ-021);
- `CONSTRAINT chk_moderation_decisions_target_xor CHECK ((claim_id IS NOT NULL AND evidence_source_id IS NULL) OR (claim_id IS NULL AND evidence_source_id IS NOT NULL))`: restrição XOR estrita em banco que garante exatamente um alvo por decisão;
- `action TEXT NOT NULL CHECK (action IN ('approve', 'reject', 'restore'))`: allowlist restrita às três ações executáveis de moderação humana de claims;
- `reason TEXT NOT NULL CHECK (length(trim(reason)) > 0 AND length(reason) <= 1000)`: justificativa obrigatória, limitada a 1000 caracteres e sem tags HTML;
- `actor TEXT NOT NULL CHECK (length(trim(actor)) > 0 AND length(actor) <= 128)`: operador autenticado responsável pela ação (limite de 128 caracteres compatível com a validação de domínio);
- `candidate_fingerprint TEXT NOT NULL DEFAULT ''`: preservação do fingerprint do candidato de monitoramento canônico quando aplicável;
- `created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`: carimbo UTC.

### 3. Transação SQLite Curta, Controle Otimista de Versão e Invariantes Atômicos

Toda ação de moderação ocorre em uma única transação atômica (`store.ExecTx`) com controle otimista de versão:
1. O formulário GET transporta a versão original do claim (`updated_at`) como campo oculto `expected_updated_at`;
2. Consulta e revalidação do estado atual do claim no banco (`SELECT id, status, updated_at FROM claims WHERE id = ?`);
3. Revalidação da versão esperada dentro da transação (`claim.UpdatedAt == params.ExpectedUpdatedAt`);
4. Validação da transição de estado via regras puras de domínio contra o estado lido no banco (`domain.ValidateClaimTransition`);
5. Se for `approve`, verificação de ao menos um `evidence_source` ativo com papel `supports`;
6. Atualização condicional de `claims` por ID e versão esperada (`UPDATE claims SET status = @new_status, updated_at = @updated_at WHERE id = @id AND updated_at = @expected_updated_at`); caso 0 linhas sejam afetadas, a operação aborta retornando `moderation.ErrConflict` (mapeado para `409 Conflict`);
7. Se for `reject`, marcação de candidatos canônicos associados (`is_duplicate = 0`) em `monitoring_candidates` como `rejected`;
8. Consulta do fingerprint do candidato canônico associado (abortando a transação em qualquer erro de banco não-`sql.ErrNoRows`);
9. Inserção do registro de auditoria em `moderation_decisions`;
10. Commit atômico. Falhas em qualquer etapa provocam rollback integral.

Concorrência: se duas requisições simultâneas tentarem moderar o mesmo claim com a mesma versão inicial, a primeira transação comita com sucesso (303 See Other); a segunda transação detecta versão stale ou 0 linhas afetadas no UPDATE condicional, aborta e responde deterministicamente `409 Conflict`, gravando exatamente uma decisão no banco.

### 4. Contexto Autenticado e Proteção CSRF Estrita

- **Ator confiável:** após autenticação HTTP Basic bem-sucedida, o nome de usuário autenticado é injetado no `context.Context` do request (`web.WithAuthenticatedUser`). O serviço de moderação consome o ator exclusivamente a partir do contexto autenticado. Formulários, headers arbitrários e query strings jamais são aceitos para definir `actor`.
- **Origem permitida (`ADMIN_ALLOWED_ORIGIN`):** introduz-se a variável de ambiente `ADMIN_ALLOWED_ORIGIN` (URL absoluta HTTP/HTTPS). Ao carregar a configuração, a origem é armazenada canonicamente (esquema e host normalizados para minúsculas, preservando porta válida e sem barra final, rejeitando credenciais, query, fragmento ou subcaminhos). Quando a área administrativa estiver habilitada, essa variável é obrigatória no startup (*fail-fast* se ausente ou inválida).
- **Validação de CSRF e limite de payload:** middleware dedicado `AdminCSRFMiddleware` intercepta todas as requisições com métodos mutantes (`POST`, `PUT`, `PATCH`, `DELETE`) sob `/admin/*`:
  - Valida que o cabeçalho `Origin` corresponde exatamente à origem canônica `ADMIN_ALLOWED_ORIGIN`;
  - Ausência, valor inválido ou divergência retorna `403 Forbidden` genérico sem executar nenhuma mutação;
  - Não há fallback para `Referer` nem confiança em cabeçalhos de proxy `X-Forwarded-*`;
  - Exige `Content-Type: application/x-www-form-urlencoded`;
  - Limita o corpo da requisição a 64 KiB via `http.MaxBytesReader`; requisições excedentes são rejeitadas com `413 Request Entity Too Large` sem persistir qualquer modificação.

### 5. Interface SSR e Padrão Post/Redirect/Get (PRG)

- `GET /admin/claims/{id}` exibe a inspeção completa do claim, detalhes relacionais, fontes/evidências associadas e o histórico cronológico de decisões de moderação (`moderation_decisions`).
- Botões e formulários de ação são renderizados **exclusivamente** para as transições válidas a partir do estado atual da alegação.
- Cada ação submete um `POST /admin/claims/{id}/moderate` com campos `action` e `reason`.
- Em caso de sucesso, o servidor responde com `303 See Other` redirecionando para `GET /admin/claims/{id}?msg=...`, impedindo reenvio acidental de formulário.

## Consequências

### Positivas
- **Controle editorial auditável:** cada ação de aprovação, rejeição ou restauração é rastreável com autor, motivo e data UTC.
- **Isolamento de segurança:** proteção contra CSRF via validação estrita de `Origin` e ator derivado exclusivamente do contexto HTTP autenticado.
- **Integridade da fronteira pública:** alegações rejeitadas desaparecem imediatamente de páginas públicas, buscas, métricas e exportações XLSX; alegações restauradas permanecem contidas em quarentena até aprovação explícita.
- **Proteção contra republicação:** rejeição de claim marca o candidato de monitoramento como rejeitado, preservando o bloqueio por fingerprint no gate estrutural.

### Limitações e Delimitação com VZ-021
- A moderação de `evidence_sources` individuais e a regra de quarentena atômica por perda do último suporte ativo permanecem reservadas para a etapa **VZ-021**.
- No MVP, as decisões são geridas por um único operador autenticado configurado no monólito Go.
