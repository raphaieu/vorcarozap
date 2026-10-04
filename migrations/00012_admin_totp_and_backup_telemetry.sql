-- +goose Up
-- Migration VZ-028: Proteção contra replay de TOTP e persistência de telemetria de backups

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
