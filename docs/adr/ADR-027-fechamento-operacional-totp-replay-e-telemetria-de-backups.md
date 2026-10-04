# ADR-027: Fechamento Operacional, Proteção contra Replay de TOTP e Persistência de Telemetria de Backups

## Status

Aceito

## Contexto

Na etapa de fechamento operacional e release de produção (VZ-028), a revisão de segurança e observabilidade identificou a necessidade de endurecer os seguintes aspectos arquiteturais no monólito Go e SQLite WAL ([ADR-001](ADR-001-monolito-modular-em-go.md), [ADR-002](ADR-002-sqlite-com-wal.md), [ADR-020](ADR-020-deploy-economico-em-vps-cron-persistencia-backup-e-smoke-checklist.md), [ADR-025](ADR-025-gestao-multiusuario-rbac-mfa-e-sessoes.md), [ADR-026](ADR-026-observabilidade-aprofundada-e-backup-externo-seguro.md)):

1. **Proteção Criptográfica contra Replay de TOTP (RFC 6238)**:
   - A validação de códigos TOTP de 6 dígitos aceita uma janela de tolerância de ±1 passo temporal (30 segundos). Sem o registro atômico do último passo temporal validado para a conta, um código interceptado ou reutilizado durante a janela de validade poderia autenticar múltiplas sessões concorrentes.
   - É mandatório que o primeiro consumo de um código TOTP invalide imediatamente o seu timestep para autenticações subsequentes no mesmo usuário.

2. **Persistência e Segregação de Telemetria de Backups no Monólito**:
   - A execução de rotinas de backup ocorre tanto em processos de curta duração via CLI (`vorcarozap backup` disparado por cron ou scripts operacionais) quanto no servidor HTTP de longa duração.
   - Para que o painel administrativo de observabilidade (`GET /admin/observabilidade` e `GET /admin/api/metrics`) reflita com precisão o estado das rotinas independentemente do ciclo de vida dos processos, os relatórios de execução de backup devem ser persistidos no SQLite.
   - **Princípio de segregação de estado:** Uma falha no upload para o Object Storage externo jamais pode mascarar o sucesso e integridade do snapshot local previamente gerado e verificado; o modelo de dados e a interface devem apresentar estados independentes para o backup local (`local_status`) e para o envio remoto (`remote_status`).

3. **Atomicidade em Transições Administrativas de Segurança**:
   - Operações de alteração de senha, reset administrativo, desativação de MFA, mudança de papel (RBAC) e bloqueio de usuários devem ocorrer sob transações SQL atômicas (`BEGIN IMMEDIATE`), englobando a alteração de estado com OCC, a invalidação imediata de todas as sessões ativas do usuário e a gravação do registro de auditoria.

---

## Decisão

### 1. Schema Migration 00012: `last_totp_timestep` e Tabela `backup_runs`

Aplica-se a migração `migrations/00012_admin_totp_and_backup_telemetry.sql`:

```sql
-- +goose Up
ALTER TABLE admin_users ADD COLUMN last_totp_timestep INTEGER NOT NULL DEFAULT 0;

CREATE TABLE backup_runs (
    id TEXT PRIMARY KEY,
    snapshot_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    schema_version INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    sha256_hex TEXT NOT NULL,
    local_status TEXT NOT NULL DEFAULT 'ok' CHECK (local_status IN ('ok', 'error')),
    local_error TEXT,
    remote_provider TEXT NOT NULL DEFAULT '',
    remote_bucket TEXT NOT NULL DEFAULT '',
    remote_key TEXT NOT NULL DEFAULT '',
    remote_duration_ms INTEGER NOT NULL DEFAULT 0,
    remote_status TEXT NOT NULL DEFAULT 'disabled' CHECK (remote_status IN ('disabled', 'ok', 'error')),
    remote_error TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_backup_runs_created_at ON backup_runs(created_at);
CREATE INDEX idx_backup_runs_local_status ON backup_runs(local_status);
CREATE INDEX idx_backup_runs_remote_status ON backup_runs(remote_status);

-- +goose Down
DROP TABLE IF EXISTS backup_runs;
ALTER TABLE admin_users DROP COLUMN last_totp_timestep;
```

### 2. Consumo Atômico de Timestep no Desafio MFA

- O método `ConsumeTOTPTimestepAndRecordSuccess` executa a query atômica:
  ```sql
  UPDATE admin_users
  SET
      last_totp_timestep = @timestep,
      failed_login_attempts = 0,
      mfa_failed_attempts = 0,
      locked_until = NULL,
      last_login_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
      updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  WHERE id = @id AND (last_totp_timestep IS NULL OR last_totp_timestep < @timestep);
  ```
- Caso `RowsAffected == 0`, a tentativa é rejeitada com erro `ErrTOTPReplay`, registrando log de auditoria com ação `mfa_replay_blocked`.

### 3. Modelo de Ciclo de Vida e Telemetria de Backups

- Cada execução do `BackupManager.RunBackup` registra um registro na tabela `backup_runs`.
- **Valores normalizados de status:**
  - `local_status`: `'ok'` (snapshot íntegro e validado via `PRAGMA integrity_check`) ou `'error'` (falha local);
  - `remote_status`: `'disabled'` (quando backup remoto está desabilitado), `'ok'` (upload com SHA-256 validado) ou `'error'` (falha no upload ou na verificação remota).
- O coletor de métricas (`MetricsRegistry`) consulta os registros mais recentes de forma segregada: o último evento com `local_status` define a telemetria do backup local, enquanto o último evento com `remote_status != 'disabled'` define a telemetria do backup remoto, evitando que execuções locais apaguem o histórico de envios remotos bem-sucedidos.
- **Sanitização estrita:** Mensagens de erro de provedores S3 são redigidas e sanitizadas antes de serem persistidas ou exportadas em JSON/HTML.

### 4. Plano de Rollback

1. Reversão da migration via `store.Rollback(ctx, db)` ou goose down (`DROP TABLE backup_runs; ALTER TABLE admin_users DROP COLUMN last_totp_timestep;`).
2. O código da aplicação mantém compatibilidade com fallback em memória caso a tabela `backup_runs` não esteja presente.

---

## Consequências

### Positivas
- Eliminação definitiva de vetores de ataque por repetição de códigos TOTP em janelas temporais válidas.
- Visibilidade operacional contínua no painel `/admin/observabilidade` e endpoint `/admin/api/metrics` para execuções agendadas por cron ou CLI.
- Segregação visual e de telemetria entre a garantia do backup local e eventuais oscilações do provedor de Object Storage remoto.
- Transações administrativas de segurança 100% atômicas com revogação imediata de sessões.

### Negativas / Mitigações
- Adição de 1 coluna em `admin_users` e 1 tabela leve (`backup_runs`) no SQLite, gerenciadas via migrations e indexadas por `created_at`.
