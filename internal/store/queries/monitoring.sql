-- name: CreateMonitoringRun :one
INSERT INTO monitoring_runs (
    id, status, query, discovery_provider, discovery_model, verification_provider, verification_model,
    prompt_tokens, completion_tokens, total_tokens, estimated_cost, web_search_calls, error_message,
    summary_counts, technical_summary, created_at, completed_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetMonitoringRunByID :one
SELECT * FROM monitoring_runs
WHERE id = ? LIMIT 1;

-- name: UpdateMonitoringRunStatus :one
UPDATE monitoring_runs
SET status = ?, error_message = ?, summary_counts = ?, technical_summary = ?,
    prompt_tokens = ?, completion_tokens = ?, total_tokens = ?, estimated_cost = ?,
    web_search_calls = ?, completed_at = ?
WHERE id = ?
RETURNING *;

-- name: ListMonitoringRuns :many
SELECT * FROM monitoring_runs
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CreateMonitoringCandidate :one
INSERT INTO monitoring_candidates (
    id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
    proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
    published_at, excerpt, locator, context_limits, technical_confidence, raw_payload,
    editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
    structural_gate_passed, structural_gate_reasons, semantic_gate_passed, semantic_gate_reasons,
    policy_action, policy_reasons, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetMonitoringCandidateByID :one
SELECT * FROM monitoring_candidates
WHERE id = ? LIMIT 1;

-- name: GetCanonicalCandidateByFingerprint :one
SELECT * FROM monitoring_candidates
WHERE fingerprint = ? AND is_duplicate = 0
ORDER BY created_at ASC, rowid ASC
LIMIT 1;

-- name: ListCandidatesByRunID :many
SELECT * FROM monitoring_candidates
WHERE monitoring_run_id = ?
ORDER BY created_at ASC, rowid ASC;

-- name: CountCandidatesByRunID :one
SELECT
    count(*) AS total_count,
    coalesce(sum(CASE WHEN is_duplicate = 0 THEN 1 ELSE 0 END), 0) AS unique_count,
    coalesce(sum(CASE WHEN is_duplicate = 1 THEN 1 ELSE 0 END), 0) AS duplicate_count
FROM monitoring_candidates
WHERE monitoring_run_id = ?;
