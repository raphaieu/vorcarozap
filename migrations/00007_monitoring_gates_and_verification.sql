-- +goose Up
-- Migration VZ-012: Schema para gates estrutural e semântico, histórico de avaliações e materialização editorial

CREATE TABLE monitoring_candidates_v2 (
    id TEXT PRIMARY KEY,
    monitoring_run_id TEXT NOT NULL REFERENCES monitoring_runs(id) ON DELETE RESTRICT,
    fingerprint TEXT NOT NULL CHECK (length(trim(fingerprint)) > 0),
    fingerprint_version INTEGER NOT NULL DEFAULT 1 CHECK (fingerprint_version = 1),
    entity_name TEXT NOT NULL CHECK (length(trim(entity_name)) > 0),
    normalized_entity_name TEXT NOT NULL CHECK (length(trim(normalized_entity_name)) > 0),
    target_entity_name TEXT NOT NULL DEFAULT '',
    normalized_target_entity_name TEXT NOT NULL DEFAULT '',
    case_name TEXT NOT NULL DEFAULT '',
    normalized_case_name TEXT NOT NULL DEFAULT '',
    relationship_type TEXT NOT NULL DEFAULT '',
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
    canonical_candidate_id TEXT REFERENCES monitoring_candidates_v2(id) ON DELETE SET NULL,
    resolved_subject_entity_id TEXT REFERENCES entities(id) ON DELETE SET NULL,
    resolved_target_entity_id TEXT REFERENCES entities(id) ON DELETE SET NULL,
    resolved_case_id TEXT REFERENCES cases(id) ON DELETE SET NULL,
    published_claim_id TEXT REFERENCES claims(id) ON DELETE SET NULL,
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
    ),
    CONSTRAINT chk_monitoring_candidates_resolved_xor CHECK (
        (resolved_target_entity_id IS NOT NULL AND resolved_case_id IS NULL) OR
        (resolved_target_entity_id IS NULL AND resolved_case_id IS NOT NULL) OR
        (resolved_target_entity_id IS NULL AND resolved_case_id IS NULL)
    ),
    CONSTRAINT chk_monitoring_candidates_target_text_xor CHECK (
        (length(trim(target_entity_name)) > 0 AND length(trim(case_name)) = 0) OR
        (length(trim(target_entity_name)) = 0 AND length(trim(case_name)) > 0) OR
        (length(trim(target_entity_name)) = 0 AND length(trim(case_name)) = 0)
    ),
    CONSTRAINT chk_monitoring_candidates_no_self_relation CHECK (
        (resolved_target_entity_id IS NULL OR resolved_subject_entity_id IS NULL OR resolved_target_entity_id != resolved_subject_entity_id) AND
        (normalized_target_entity_name = '' OR normalized_entity_name = '' OR normalized_target_entity_name != normalized_entity_name)
    )
);

INSERT INTO monitoring_candidates_v2 (
    id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
    target_entity_name, normalized_target_entity_name, case_name, normalized_case_name, relationship_type,
    proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
    published_at, excerpt, locator, context_limits, technical_confidence, raw_payload,
    editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
    resolved_subject_entity_id, resolved_target_entity_id, resolved_case_id, published_claim_id,
    structural_gate_passed, structural_gate_reasons, semantic_gate_passed, semantic_gate_reasons,
    policy_action, policy_reasons, created_at, updated_at
)
SELECT
    id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
    '', '', '', '', '',
    proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
    published_at, excerpt, locator, context_limits, technical_confidence, raw_payload,
    editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
    NULL, NULL, NULL, NULL,
    structural_gate_passed, structural_gate_reasons, semantic_gate_passed, semantic_gate_reasons,
    policy_action, policy_reasons, created_at, updated_at
FROM monitoring_candidates;

DROP TABLE monitoring_candidates;
ALTER TABLE monitoring_candidates_v2 RENAME TO monitoring_candidates;

CREATE INDEX idx_monitoring_candidates_run ON monitoring_candidates(monitoring_run_id);
CREATE INDEX idx_monitoring_candidates_fingerprint ON monitoring_candidates(fingerprint);
CREATE INDEX idx_monitoring_candidates_editorial_status ON monitoring_candidates(editorial_status);
CREATE INDEX idx_monitoring_candidates_is_duplicate ON monitoring_candidates(is_duplicate);
CREATE INDEX idx_monitoring_candidates_canonical ON monitoring_candidates(canonical_candidate_id);
CREATE INDEX idx_monitoring_candidates_canonical_url ON monitoring_candidates(canonical_url);
CREATE INDEX idx_monitoring_candidates_published_claim ON monitoring_candidates(published_claim_id);
CREATE INDEX idx_monitoring_candidates_resolved_subject ON monitoring_candidates(resolved_subject_entity_id);
CREATE INDEX idx_monitoring_candidates_resolved_target ON monitoring_candidates(resolved_target_entity_id);
CREATE INDEX idx_monitoring_candidates_resolved_case ON monitoring_candidates(resolved_case_id);

CREATE TABLE semantic_evaluations (
    id TEXT PRIMARY KEY,
    monitoring_candidate_id TEXT NOT NULL REFERENCES monitoring_candidates(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (length(trim(provider)) > 0),
    model TEXT NOT NULL CHECK (length(trim(model)) > 0),
    schema_version TEXT NOT NULL DEFAULT 'v1',
    identity_match INTEGER NOT NULL CHECK (identity_match IN (0, 1)),
    claim_supported INTEGER NOT NULL CHECK (claim_supported IN (0, 1)),
    claim_overstates_source INTEGER NOT NULL CHECK (claim_overstates_source IN (0, 1)),
    attribution_explicit INTEGER NOT NULL CHECK (attribution_explicit IN (0, 1)),
    grade_compatible INTEGER NOT NULL CHECK (grade_compatible IN (0, 1)),
    contains_illicit_inference INTEGER NOT NULL CHECK (contains_illicit_inference IN (0, 1)),
    uncertainties TEXT NOT NULL DEFAULT '[]',
    recommended_action TEXT NOT NULL CHECK (recommended_action IN ('publish', 'quarantine', 'reject')),
    raw_response TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_semantic_evaluations_candidate ON semantic_evaluations(monitoring_candidate_id);
CREATE INDEX idx_semantic_evaluations_created_at ON semantic_evaluations(created_at);

-- +goose Down
DROP TABLE IF EXISTS semantic_evaluations;

CREATE TABLE monitoring_candidates_v1 (
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
    canonical_candidate_id TEXT REFERENCES monitoring_candidates_v1(id) ON DELETE SET NULL,
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

INSERT INTO monitoring_candidates_v1 (
    id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
    proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
    published_at, excerpt, locator, context_limits, technical_confidence, raw_payload,
    editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
    structural_gate_passed, structural_gate_reasons, semantic_gate_passed, semantic_gate_reasons,
    policy_action, policy_reasons, created_at, updated_at
)
SELECT
    id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
    proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
    published_at, excerpt, locator, context_limits, technical_confidence, raw_payload,
    editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
    structural_gate_passed, structural_gate_reasons, semantic_gate_passed, semantic_gate_reasons,
    policy_action, policy_reasons, created_at, updated_at
FROM monitoring_candidates;

DROP TABLE monitoring_candidates;
ALTER TABLE monitoring_candidates_v1 RENAME TO monitoring_candidates;

CREATE INDEX idx_monitoring_candidates_run ON monitoring_candidates(monitoring_run_id);
CREATE INDEX idx_monitoring_candidates_fingerprint ON monitoring_candidates(fingerprint);
CREATE INDEX idx_monitoring_candidates_editorial_status ON monitoring_candidates(editorial_status);
CREATE INDEX idx_monitoring_candidates_is_duplicate ON monitoring_candidates(is_duplicate);
CREATE INDEX idx_monitoring_candidates_canonical ON monitoring_candidates(canonical_candidate_id);
CREATE INDEX idx_monitoring_candidates_canonical_url ON monitoring_candidates(canonical_url);
