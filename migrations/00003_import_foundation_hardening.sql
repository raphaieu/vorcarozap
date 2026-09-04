-- +goose NO TRANSACTION
-- +goose Up
-- Migration VZ-005 / Hardening: Idempotência de importação e fontes/trechos curados

PRAGMA foreign_keys = OFF;

-- 1. Idempotência estrita para import_runs aplicados
CREATE UNIQUE INDEX uq_import_runs_applied ON import_runs(file_hash, mapping_version)
WHERE is_dry_run = 0 AND status IN ('running', 'completed', 'partial');

-- 2. Recriação da tabela sources permitindo título e autor desconhecidos/vazios
CREATE TABLE sources_new (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    publisher_or_author TEXT NOT NULL DEFAULT '',
    original_url TEXT NOT NULL CHECK (length(trim(original_url)) > 0),
    canonical_url TEXT NOT NULL CHECK (length(trim(canonical_url)) > 0),
    published_at TEXT,
    accessed_at TEXT,
    source_type TEXT NOT NULL DEFAULT 'article' CHECK (length(trim(source_type)) > 0),
    source_access_status TEXT NOT NULL DEFAULT 'not_checked' CHECK (source_access_status IN ('cited_by_provider', 'reachable', 'unreachable', 'not_checked')),
    source_access_checked_at TEXT,
    http_status INTEGER,
    normalized_error_code TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO sources_new (
    id, title, publisher_or_author, original_url, canonical_url,
    published_at, accessed_at, source_type, source_access_status,
    source_access_checked_at, http_status, normalized_error_code,
    created_at, updated_at
)
SELECT
    id, title, publisher_or_author, original_url, canonical_url,
    published_at, accessed_at, source_type, source_access_status,
    source_access_checked_at, http_status, normalized_error_code,
    created_at, updated_at
FROM sources;

DROP TABLE sources;
ALTER TABLE sources_new RENAME TO sources;

CREATE UNIQUE INDEX uq_sources_canonical_url ON sources(canonical_url);
CREATE INDEX idx_sources_access_status ON sources(source_access_status);

-- 3. Recriação da tabela evidence_sources permitindo excerpt vazio no seed
CREATE TABLE evidence_sources_new (
    id TEXT PRIMARY KEY,
    evidence_id TEXT NOT NULL REFERENCES evidence(id) ON DELETE RESTRICT,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    excerpt TEXT NOT NULL DEFAULT '',
    locator TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL CHECK (role IN ('supports', 'contradicts', 'contextualizes')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'rejected')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO evidence_sources_new (
    id, evidence_id, source_id, excerpt, locator, role, status, created_at, updated_at
)
SELECT
    id, evidence_id, source_id, excerpt, locator, role, status, created_at, updated_at
FROM evidence_sources;

DROP TABLE evidence_sources;
ALTER TABLE evidence_sources_new RENAME TO evidence_sources;

CREATE INDEX idx_evidence_sources_evidence ON evidence_sources(evidence_id);
CREATE INDEX idx_evidence_sources_source ON evidence_sources(source_id);
CREATE INDEX idx_evidence_sources_status_role ON evidence_sources(status, role);
CREATE INDEX idx_evidence_sources_lookup ON evidence_sources(evidence_id, source_id, status, role);

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;

-- +goose Down
-- Reversão falha explicitamente se existirem registros incompatíveis (trecho, título ou autor vazios)
-- para impedir descarte silencioso de dados.
CREATE TEMP TABLE _abort_on_incompatible_data (check_val INTEGER CHECK (check_val = 0));
INSERT INTO _abort_on_incompatible_data SELECT COUNT(*) FROM evidence_sources WHERE length(trim(excerpt)) = 0;
INSERT INTO _abort_on_incompatible_data SELECT COUNT(*) FROM sources WHERE length(trim(title)) = 0 OR length(trim(publisher_or_author)) = 0;
DROP TABLE _abort_on_incompatible_data;

PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS uq_import_runs_applied;

CREATE TABLE evidence_sources_old (
    id TEXT PRIMARY KEY,
    evidence_id TEXT NOT NULL REFERENCES evidence(id) ON DELETE RESTRICT,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    excerpt TEXT NOT NULL CHECK (length(trim(excerpt)) > 0),
    locator TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL CHECK (role IN ('supports', 'contradicts', 'contextualizes')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'rejected')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO evidence_sources_old SELECT * FROM evidence_sources;
DROP TABLE evidence_sources;
ALTER TABLE evidence_sources_old RENAME TO evidence_sources;

CREATE INDEX idx_evidence_sources_evidence ON evidence_sources(evidence_id);
CREATE INDEX idx_evidence_sources_source ON evidence_sources(source_id);
CREATE INDEX idx_evidence_sources_status_role ON evidence_sources(status, role);
CREATE INDEX idx_evidence_sources_lookup ON evidence_sources(evidence_id, source_id, status, role);

CREATE TABLE sources_old (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    publisher_or_author TEXT NOT NULL CHECK (length(trim(publisher_or_author)) > 0),
    original_url TEXT NOT NULL CHECK (length(trim(original_url)) > 0),
    canonical_url TEXT NOT NULL CHECK (length(trim(canonical_url)) > 0),
    published_at TEXT,
    accessed_at TEXT,
    source_type TEXT NOT NULL DEFAULT 'article' CHECK (length(trim(source_type)) > 0),
    source_access_status TEXT NOT NULL DEFAULT 'not_checked' CHECK (source_access_status IN ('cited_by_provider', 'reachable', 'unreachable', 'not_checked')),
    source_access_checked_at TEXT,
    http_status INTEGER,
    normalized_error_code TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO sources_old SELECT * FROM sources;
DROP TABLE sources;
ALTER TABLE sources_old RENAME TO sources;

CREATE INDEX idx_sources_canonical_url ON sources(canonical_url);
CREATE INDEX idx_sources_access_status ON sources(source_access_status);

PRAGMA foreign_key_check;
PRAGMA foreign_keys = ON;
