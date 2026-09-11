-- name: AcquireMonitoringLock :one
INSERT INTO monitoring_locks (name, holder, acquired_at, expires_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (name) DO UPDATE
SET holder = excluded.holder,
    acquired_at = excluded.acquired_at,
    expires_at = excluded.expires_at,
    updated_at = excluded.updated_at
WHERE monitoring_locks.expires_at < excluded.acquired_at
RETURNING *;

-- name: RenewMonitoringLock :one
UPDATE monitoring_locks
SET expires_at = ?, updated_at = ?
WHERE name = ? AND holder = ?
RETURNING *;

-- name: ReleaseMonitoringLock :execrows
DELETE FROM monitoring_locks
WHERE name = ? AND holder = ?;

-- name: GetMonitoringLock :one
SELECT * FROM monitoring_locks
WHERE name = ? LIMIT 1;

-- name: GetLastSuccessfulMonitoringRunByQuery :one
SELECT * FROM monitoring_runs
WHERE query = ?
  AND status = 'completed'
  AND window_end IS NOT NULL
ORDER BY window_end DESC, created_at DESC
LIMIT 1;

-- name: GetDailyMonitoringCostMicroUSD :one
SELECT CAST(coalesce(sum(total_cost_microusd), 0) AS INTEGER) AS daily_cost_microusd
FROM monitoring_runs
WHERE created_at >= ?;

-- name: CreateMonitoringRun :one
INSERT INTO monitoring_runs (
    id, status, query, window_start, window_end,
    discovery_provider, discovery_model, verification_provider, verification_model,
    prompt_tokens, completion_tokens, total_tokens, estimated_cost, web_search_calls,
    discovery_tokens, discovery_cost_microusd, discovery_cost,
    verification_tokens, verification_cost_microusd, verification_cost,
    total_cost_microusd, total_cost, verifications_count,
    error_message, summary_counts, technical_summary, created_at, completed_at
) VALUES (
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    0, 0, 0, 0.0, 0,
    0, 0, 0.0,
    0, 0, 0.0,
    0, 0.0, 0,
    ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetMonitoringRunByID :one
SELECT * FROM monitoring_runs
WHERE id = ? LIMIT 1;

-- name: RecordDiscoveryUsage :one
UPDATE monitoring_runs
SET prompt_tokens = sqlc.arg(prompt_tokens),
    completion_tokens = sqlc.arg(completion_tokens),
    total_tokens = sqlc.arg(total_tokens),
    discovery_tokens = sqlc.arg(discovery_tokens),
    discovery_cost_microusd = sqlc.arg(discovery_cost_microusd),
    discovery_cost = CAST(sqlc.arg(discovery_cost_microusd) AS REAL) / 1000000.0,
    total_cost_microusd = sqlc.arg(total_cost_microusd),
    total_cost = CAST(sqlc.arg(total_cost_microusd) AS REAL) / 1000000.0,
    web_search_calls = sqlc.arg(web_search_calls)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: IncrementMonitoringRunUsage :one
UPDATE monitoring_runs
SET prompt_tokens = prompt_tokens + sqlc.arg(prompt_tokens),
    completion_tokens = completion_tokens + sqlc.arg(completion_tokens),
    total_tokens = total_tokens + sqlc.arg(total_tokens),
    verification_tokens = verification_tokens + sqlc.arg(verification_tokens),
    verification_cost_microusd = verification_cost_microusd + sqlc.arg(cost_microusd),
    verification_cost = CAST((verification_cost_microusd + sqlc.arg(cost_microusd)) AS REAL) / 1000000.0,
    total_cost_microusd = total_cost_microusd + sqlc.arg(cost_microusd),
    total_cost = CAST((total_cost_microusd + sqlc.arg(cost_microusd)) AS REAL) / 1000000.0,
    verifications_count = verifications_count + 1
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CompleteMonitoringRun :one
UPDATE monitoring_runs
SET status = ?,
    error_message = ?,
    summary_counts = ?,
    technical_summary = ?,
    completed_at = ?
WHERE id = ?
RETURNING *;

-- name: FailMonitoringRun :one
UPDATE monitoring_runs
SET status = 'failed',
    error_message = ?,
    technical_summary = ?,
    completed_at = ?
WHERE id = ?
RETURNING *;

-- name: UpdateMonitoringRunUsage :one
UPDATE monitoring_runs
SET prompt_tokens = ?,
    completion_tokens = ?,
    total_tokens = ?,
    estimated_cost = ?,
    web_search_calls = ?,
    discovery_tokens = ?,
    discovery_cost_microusd = ?,
    discovery_cost = ?,
    verification_tokens = ?,
    verification_cost_microusd = ?,
    verification_cost = ?,
    total_cost_microusd = ?,
    total_cost = ?,
    verifications_count = ?
WHERE id = ?
RETURNING *;

-- name: UpdateMonitoringRunStatus :one
UPDATE monitoring_runs
SET status = ?,
    error_message = ?,
    summary_counts = ?,
    technical_summary = ?,
    completed_at = ?
WHERE id = ?
RETURNING *;

-- name: ListMonitoringRuns :many
SELECT * FROM monitoring_runs
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CreateMonitoringCandidate :one
INSERT INTO monitoring_candidates (
    id, monitoring_run_id, fingerprint, fingerprint_version, entity_name, normalized_entity_name,
    target_entity_name, normalized_target_entity_name, case_name, normalized_case_name, relationship_type,
    proposition, suggested_grade, source_url, canonical_url, source_title, publisher_or_author,
    published_at, excerpt, locator, context_limits, technical_confidence, raw_payload,
    editorial_status, is_duplicate, duplicate_reason, canonical_candidate_id,
    resolved_subject_entity_id, resolved_target_entity_id, resolved_case_id, published_claim_id,
    structural_gate_passed, structural_gate_reasons, semantic_gate_passed, semantic_gate_reasons,
    policy_action, policy_reasons, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?
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

-- name: ListQuarantinedCandidatesForEvaluation :many
SELECT * FROM monitoring_candidates
WHERE editorial_status = 'quarantined'
  AND is_duplicate = 0
ORDER BY created_at ASC, rowid ASC
LIMIT ?;

-- name: CountCandidatesByRunID :one
SELECT
    count(*) AS total_count,
    coalesce(sum(CASE WHEN is_duplicate = 0 THEN 1 ELSE 0 END), 0) AS unique_count,
    coalesce(sum(CASE WHEN is_duplicate = 1 THEN 1 ELSE 0 END), 0) AS duplicate_count
FROM monitoring_candidates
WHERE monitoring_run_id = ?;

-- name: HasRejectedCandidateByFingerprint :one
SELECT count(*) > 0 AS is_rejected
FROM monitoring_candidates
WHERE fingerprint = ?
  AND editorial_status = 'rejected';

-- name: UpdateMonitoringCandidateGates :one
UPDATE monitoring_candidates
SET structural_gate_passed = ?,
    structural_gate_reasons = ?,
    semantic_gate_passed = ?,
    semantic_gate_reasons = ?,
    policy_action = ?,
    policy_reasons = ?,
    editorial_status = ?,
    resolved_subject_entity_id = ?,
    resolved_target_entity_id = ?,
    resolved_case_id = ?,
    published_claim_id = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ClaimQuarantinedCandidateForPublication :one
UPDATE monitoring_candidates
SET updated_at = ?
WHERE id = ?
  AND editorial_status = 'quarantined'
  AND is_duplicate = 0
  AND published_claim_id IS NULL
RETURNING *;

-- name: CreateSemanticEvaluation :one
INSERT INTO semantic_evaluations (
    id, monitoring_candidate_id, provider, model, schema_version,
    identity_match, claim_supported, claim_overstates_source, attribution_explicit,
    grade_compatible, contains_illicit_inference, uncertainties, recommended_action,
    raw_response, prompt_tokens, completion_tokens, total_tokens, cost_microusd, cost, created_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(monitoring_candidate_id),
    sqlc.arg(provider),
    sqlc.arg(model),
    sqlc.arg(schema_version),
    sqlc.arg(identity_match),
    sqlc.arg(claim_supported),
    sqlc.arg(claim_overstates_source),
    sqlc.arg(attribution_explicit),
    sqlc.arg(grade_compatible),
    sqlc.arg(contains_illicit_inference),
    sqlc.arg(uncertainties),
    sqlc.arg(recommended_action),
    sqlc.arg(raw_response),
    sqlc.arg(prompt_tokens),
    sqlc.arg(completion_tokens),
    sqlc.arg(total_tokens),
    sqlc.arg(cost_microusd),
    CAST(sqlc.arg(cost_microusd) AS REAL) / 1000000.0,
    sqlc.arg(created_at)
)
RETURNING *;

-- name: ListSemanticEvaluationsByCandidateID :many
SELECT * FROM semantic_evaluations
WHERE monitoring_candidate_id = ?
ORDER BY created_at ASC;
