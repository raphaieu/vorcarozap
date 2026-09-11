-- +goose Up
-- Migration VZ-011: Schema estruturado de monitoring_runs e monitoring_candidates para ingestão e deduplicação auditável

CREATE TABLE monitoring_runs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'partial', 'completed', 'failed')),
    query TEXT NOT NULL DEFAULT '',
    discovery_provider TEXT NOT NULL DEFAULT 'openrouter',
    discovery_model TEXT NOT NULL CHECK (length(trim(discovery_model)) > 0),
    verification_provider TEXT NOT NULL DEFAULT '',
    verification_model TEXT NOT NULL DEFAULT '',
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    estimated_cost REAL,
    web_search_calls INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    summary_counts TEXT NOT NULL DEFAULT '{}',
    technical_summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at TEXT
);

CREATE INDEX idx_monitoring_runs_status ON monitoring_runs(status);
CREATE INDEX idx_monitoring_runs_created_at ON monitoring_runs(created_at);

CREATE TABLE monitoring_candidates (
    id TEXT PRIMARY KEY,
    monitoring_run_id TEXT NOT NULL REFERENCES monitoring_runs(id) ON DELETE RESTRICT,
    fingerprint TEXT NOT NULL CHECK (length(trim(fingerprint)) > 0),
    fingerprint_version INTEGER NOT NULL DEFAULT 1 CHECK (fingerprint_version = 1),
    entity_name TEXT NOT NULL CHECK (length(trim(entity_name)) > 0),
    normalized_entity_name TEXT NOT NULL CHECK (length(trim(normalized_entity_name)) > 0),
    proposition TEXT NOT NULL CHECK (length(trim(proposition)) > 0),
    suggested_grade TEXT NOT NULL CHECK (suggested_grade IN ('A', 'B', 'C', 'D', 'E')),
    source_url TEXT NOT NULL CHECK (length(trim(source_url)) > 0),
    canonical_url TEXT NOT NULL CHECK (length(trim(canonical_url)) > 0),
    source_title TEXT NOT NULL DEFAULT '',
    publisher_or_author TEXT NOT NULL DEFAULT '',
    published_at TEXT,
    excerpt TEXT NOT NULL DEFAULT '',
    locator TEXT NOT NULL DEFAULT '',
    context_limits TEXT NOT NULL DEFAULT '',
    technical_confidence REAL NOT NULL CHECK (technical_confidence >= 0.0 AND technical_confidence <= 1.0),
    raw_payload TEXT NOT NULL DEFAULT '{}',
    editorial_status TEXT NOT NULL DEFAULT 'quarantined' CHECK (editorial_status IN ('quarantined', 'published', 'rejected', 'archived')),
    is_duplicate INTEGER NOT NULL DEFAULT 0 CHECK (is_duplicate IN (0, 1)),
    duplicate_reason TEXT NOT NULL DEFAULT '',
    canonical_candidate_id TEXT REFERENCES monitoring_candidates(id) ON DELETE SET NULL,
    structural_gate_passed INTEGER CHECK (structural_gate_passed IN (0, 1)),
    structural_gate_reasons TEXT NOT NULL DEFAULT '[]',
    semantic_gate_passed INTEGER CHECK (semantic_gate_passed IN (0, 1)),
    semantic_gate_reasons TEXT NOT NULL DEFAULT '[]',
    policy_action TEXT NOT NULL DEFAULT '',
    policy_reasons TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CONSTRAINT chk_monitoring_candidates_duplicate_consistency CHECK (
        (is_duplicate = 0 AND duplicate_reason = '' AND canonical_candidate_id IS NULL) OR
        (is_duplicate = 1 AND duplicate_reason IN ('same_run_duplicate', 'existing_fingerprint') AND canonical_candidate_id IS NOT NULL)
    ),
    CONSTRAINT chk_monitoring_candidates_no_self_reference CHECK (
        canonical_candidate_id IS NULL OR canonical_candidate_id != id
    )
);

CREATE INDEX idx_monitoring_candidates_run ON monitoring_candidates(monitoring_run_id);
CREATE INDEX idx_monitoring_candidates_fingerprint ON monitoring_candidates(fingerprint);
CREATE INDEX idx_monitoring_candidates_editorial_status ON monitoring_candidates(editorial_status);
CREATE INDEX idx_monitoring_candidates_is_duplicate ON monitoring_candidates(is_duplicate);
CREATE INDEX idx_monitoring_candidates_canonical ON monitoring_candidates(canonical_candidate_id);
CREATE INDEX idx_monitoring_candidates_canonical_url ON monitoring_candidates(canonical_url);

-- +goose Down
DROP TABLE IF EXISTS monitoring_candidates;
DROP TABLE IF EXISTS monitoring_runs;
