-- +goose Up
-- Migration VZ-004: Núcleo editorial e schema relacional

CREATE TABLE entities (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('person', 'organization')),
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    normalized_name TEXT NOT NULL CHECK (length(trim(normalized_name)) > 0),
    slug TEXT NOT NULL UNIQUE CHECK (length(trim(slug)) > 0),
    role_or_context TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    relevance INTEGER NOT NULL CHECK (relevance BETWEEN 1 AND 5),
    relevance_rationale TEXT NOT NULL CHECK (length(trim(relevance_rationale)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_entities_normalized_name ON entities(normalized_name);
CREATE INDEX idx_entities_type ON entities(type);
CREATE INDEX idx_entities_relevance ON entities(relevance);

CREATE TABLE entity_aliases (
    id TEXT PRIMARY KEY,
    entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    alias TEXT NOT NULL CHECK (length(trim(alias)) > 0),
    normalized_alias TEXT NOT NULL CHECK (length(trim(normalized_alias)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CONSTRAINT uq_entity_alias UNIQUE (entity_id, normalized_alias)
);

CREATE INDEX idx_entity_aliases_normalized ON entity_aliases(normalized_alias);
CREATE INDEX idx_entity_aliases_entity_id ON entity_aliases(entity_id);

CREATE TABLE cases (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug TEXT NOT NULL UNIQUE CHECK (length(trim(slug)) > 0),
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE relationships (
    id TEXT PRIMARY KEY,
    subject_entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE RESTRICT,
    target_entity_id TEXT REFERENCES entities(id) ON DELETE RESTRICT,
    case_id TEXT REFERENCES cases(id) ON DELETE RESTRICT,
    relationship_type TEXT NOT NULL CHECK (length(trim(relationship_type)) > 0),
    summary TEXT NOT NULL DEFAULT '',
    context_limits TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CONSTRAINT chk_relationship_target_xor CHECK (
        (target_entity_id IS NOT NULL AND case_id IS NULL) OR
        (target_entity_id IS NULL AND case_id IS NOT NULL)
    ),
    CONSTRAINT chk_relationship_no_self CHECK (
        target_entity_id IS NULL OR target_entity_id != subject_entity_id
    )
);

CREATE INDEX idx_relationships_subject ON relationships(subject_entity_id);
CREATE INDEX idx_relationships_target ON relationships(target_entity_id);
CREATE INDEX idx_relationships_case ON relationships(case_id);

CREATE TABLE import_runs (
    id TEXT PRIMARY KEY,
    file_path TEXT NOT NULL CHECK (length(trim(file_path)) > 0),
    file_hash TEXT NOT NULL CHECK (length(trim(file_hash)) > 0),
    origin TEXT NOT NULL DEFAULT 'curated_seed' CHECK (origin IN ('curated_seed')),
    mapping_version TEXT NOT NULL CHECK (length(trim(mapping_version)) > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'dry_run', 'completed', 'partial', 'failed')),
    is_dry_run INTEGER NOT NULL DEFAULT 0 CHECK (is_dry_run IN (0, 1)),
    summary_counts TEXT NOT NULL DEFAULT '{}',
    summary_report TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    completed_at TEXT
);

CREATE INDEX idx_import_runs_hash_version ON import_runs(file_hash, mapping_version);
CREATE INDEX idx_import_runs_status ON import_runs(status);

CREATE TABLE claims (
    id TEXT PRIMARY KEY,
    relationship_id TEXT NOT NULL REFERENCES relationships(id) ON DELETE RESTRICT,
    proposition TEXT NOT NULL CHECK (length(trim(proposition)) > 0),
    attribution TEXT NOT NULL DEFAULT '',
    origin TEXT NOT NULL CHECK (origin IN ('openrouter', 'curated_seed', 'admin')),
    grade TEXT NOT NULL CHECK (grade IN ('A', 'B', 'C', 'D', 'E')),
    disposition TEXT NOT NULL CHECK (disposition IN ('supports_link', 'possible_link', 'contradicts_link', 'context_only', 'correction')),
    metric_eligible INTEGER NOT NULL CHECK (metric_eligible IN (0, 1)),
    status TEXT NOT NULL CHECK (status IN ('quarantined', 'published', 'rejected', 'archived')),
    import_run_id TEXT REFERENCES import_runs(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_claims_relationship ON claims(relationship_id);
CREATE INDEX idx_claims_status ON claims(status);
CREATE INDEX idx_claims_grade ON claims(grade);
CREATE INDEX idx_claims_origin ON claims(origin);
CREATE INDEX idx_claims_metric_eligible ON claims(metric_eligible);
CREATE INDEX idx_claims_updated_at ON claims(updated_at);
CREATE INDEX idx_claims_status_metric ON claims(status, metric_eligible);
CREATE INDEX idx_claims_import_run ON claims(import_run_id);

CREATE TABLE evidence (
    id TEXT PRIMARY KEY,
    claim_id TEXT NOT NULL REFERENCES claims(id) ON DELETE RESTRICT,
    summary TEXT NOT NULL CHECK (length(trim(summary)) > 0),
    evidence_type TEXT NOT NULL DEFAULT 'document' CHECK (length(trim(evidence_type)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_evidence_claim_id ON evidence(claim_id);

CREATE TABLE sources (
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

CREATE INDEX idx_sources_canonical_url ON sources(canonical_url);
CREATE INDEX idx_sources_access_status ON sources(source_access_status);

CREATE TABLE evidence_sources (
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

CREATE INDEX idx_evidence_sources_evidence ON evidence_sources(evidence_id);
CREATE INDEX idx_evidence_sources_source ON evidence_sources(source_id);
CREATE INDEX idx_evidence_sources_status_role ON evidence_sources(status, role);
CREATE INDEX idx_evidence_sources_lookup ON evidence_sources(evidence_id, source_id, status, role);

-- +goose Down
DROP TABLE IF EXISTS evidence_sources;
DROP TABLE IF EXISTS sources;
DROP TABLE IF EXISTS evidence;
DROP TABLE IF EXISTS claims;
DROP TABLE IF EXISTS import_runs;
DROP TABLE IF EXISTS relationships;
DROP TABLE IF EXISTS cases;
DROP TABLE IF EXISTS entity_aliases;
DROP TABLE IF EXISTS entities;
