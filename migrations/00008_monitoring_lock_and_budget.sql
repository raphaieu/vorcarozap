-- +goose Up
-- Migration VZ-013: Lock com lease SQLite, janela incremental e limites de custo de LLM

CREATE TABLE monitoring_locks (
    name TEXT PRIMARY KEY,
    holder TEXT NOT NULL CHECK (length(trim(holder)) > 0),
    acquired_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_monitoring_locks_expires_at ON monitoring_locks(expires_at);

ALTER TABLE monitoring_runs ADD COLUMN window_start TEXT;
ALTER TABLE monitoring_runs ADD COLUMN window_end TEXT;
ALTER TABLE monitoring_runs ADD COLUMN discovery_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE monitoring_runs ADD COLUMN discovery_cost_microusd INTEGER NOT NULL DEFAULT 0;
ALTER TABLE monitoring_runs ADD COLUMN discovery_cost REAL NOT NULL DEFAULT 0.0;
ALTER TABLE monitoring_runs ADD COLUMN verification_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE monitoring_runs ADD COLUMN verification_cost_microusd INTEGER NOT NULL DEFAULT 0;
ALTER TABLE monitoring_runs ADD COLUMN verification_cost REAL NOT NULL DEFAULT 0.0;
ALTER TABLE monitoring_runs ADD COLUMN total_cost_microusd INTEGER NOT NULL DEFAULT 0;
ALTER TABLE monitoring_runs ADD COLUMN total_cost REAL NOT NULL DEFAULT 0.0;
ALTER TABLE monitoring_runs ADD COLUMN verifications_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE semantic_evaluations ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE semantic_evaluations ADD COLUMN completion_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE semantic_evaluations ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE semantic_evaluations ADD COLUMN cost_microusd INTEGER NOT NULL DEFAULT 0;
ALTER TABLE semantic_evaluations ADD COLUMN cost REAL NOT NULL DEFAULT 0.0;

-- +goose Down
DROP TABLE IF EXISTS monitoring_locks;

ALTER TABLE semantic_evaluations DROP COLUMN cost;
ALTER TABLE semantic_evaluations DROP COLUMN cost_microusd;
ALTER TABLE semantic_evaluations DROP COLUMN total_tokens;
ALTER TABLE semantic_evaluations DROP COLUMN completion_tokens;
ALTER TABLE semantic_evaluations DROP COLUMN prompt_tokens;

ALTER TABLE monitoring_runs DROP COLUMN verifications_count;
ALTER TABLE monitoring_runs DROP COLUMN total_cost;
ALTER TABLE monitoring_runs DROP COLUMN total_cost_microusd;
ALTER TABLE monitoring_runs DROP COLUMN verification_cost;
ALTER TABLE monitoring_runs DROP COLUMN verification_cost_microusd;
ALTER TABLE monitoring_runs DROP COLUMN verification_tokens;
ALTER TABLE monitoring_runs DROP COLUMN discovery_cost;
ALTER TABLE monitoring_runs DROP COLUMN discovery_cost_microusd;
ALTER TABLE monitoring_runs DROP COLUMN discovery_tokens;
ALTER TABLE monitoring_runs DROP COLUMN window_end;
ALTER TABLE monitoring_runs DROP COLUMN window_start;
