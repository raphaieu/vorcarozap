-- name: CreateEntity :one
INSERT INTO entities (
    id, type, name, normalized_name, slug, role_or_context, summary, relevance, relevance_rationale, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetEntityByID :one
SELECT * FROM entities
WHERE id = ? LIMIT 1;

-- name: GetEntityBySlug :one
SELECT * FROM entities
WHERE slug = ? LIMIT 1;

-- name: CreateEntityAlias :one
INSERT INTO entity_aliases (
    id, entity_id, alias, normalized_alias, created_at
) VALUES (
    ?, ?, ?, ?, ?
)
RETURNING *;

-- name: ListAliasesByEntityID :many
SELECT * FROM entity_aliases
WHERE entity_id = ?
ORDER BY normalized_alias ASC;

-- name: CreateCase :one
INSERT INTO cases (
    id, name, slug, description, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetCaseBySlug :one
SELECT * FROM cases
WHERE slug = ? LIMIT 1;

-- name: CreateRelationship :one
INSERT INTO relationships (
    id, subject_entity_id, target_entity_id, case_id, relationship_type, summary, context_limits, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetRelationshipByID :one
SELECT * FROM relationships
WHERE id = ? LIMIT 1;

-- name: CreateImportRun :one
INSERT INTO import_runs (
    id, file_path, file_hash, origin, mapping_version, status, is_dry_run, summary_counts, summary_report, error_message, created_at, completed_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetImportRunByHashAndVersion :one
SELECT * FROM import_runs
WHERE file_hash = ? AND mapping_version = ?
LIMIT 1;

-- name: CreateSource :one
INSERT INTO sources (
    id, title, publisher_or_author, original_url, canonical_url, published_at, accessed_at, source_type, source_access_status, source_access_checked_at, http_status, normalized_error_code, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetSourceByCanonicalURL :one
SELECT * FROM sources
WHERE canonical_url = ? LIMIT 1;

-- name: GetSourceByID :one
SELECT * FROM sources
WHERE id = ? LIMIT 1;

-- name: CreateClaim :one
INSERT INTO claims (
    id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, import_run_id, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetClaimByID :one
SELECT * FROM claims
WHERE id = ? LIMIT 1;

-- name: CreateEvidence :one
INSERT INTO evidence (
    id, claim_id, summary, evidence_type, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetEvidenceByID :one
SELECT * FROM evidence
WHERE id = ? LIMIT 1;

-- name: CreateEvidenceSource :one
INSERT INTO evidence_sources (
    id, evidence_id, source_id, excerpt, locator, role, status, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: ListActiveSupportsByClaimID :many
SELECT 
    c.id AS claim_id,
    c.proposition,
    c.grade,
    c.status AS claim_status,
    c.metric_eligible,
    e.id AS evidence_id,
    e.summary AS evidence_summary,
    es.id AS evidence_source_id,
    es.excerpt,
    es.locator,
    es.role,
    es.status AS evidence_source_status,
    s.id AS source_id,
    s.title AS source_title,
    s.publisher_or_author AS source_publisher,
    s.canonical_url AS source_canonical_url,
    s.source_access_status
FROM claims c
JOIN evidence e ON e.claim_id = c.id
JOIN evidence_sources es ON es.evidence_id = e.id
JOIN sources s ON s.id = es.source_id
WHERE c.id = ?
  AND es.role = 'supports'
  AND es.status = 'active'
ORDER BY es.created_at ASC;
