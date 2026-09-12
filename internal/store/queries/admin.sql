-- name: GetAdminOverviewCounts :one
SELECT
    (SELECT count(*) FROM sources) AS total_sources,
    (SELECT count(*) FROM sources WHERE source_access_status = 'reachable') AS sources_reachable,
    (SELECT count(*) FROM sources WHERE source_access_status = 'unreachable') AS sources_unreachable,
    (SELECT count(*) FROM sources WHERE source_access_status = 'not_checked') AS sources_not_checked,
    (SELECT count(*) FROM sources WHERE source_access_status = 'cited_by_provider') AS sources_cited_by_provider,

    (SELECT count(*) FROM claims) AS total_claims,
    (SELECT count(*) FROM claims WHERE status = 'published') AS claims_published,
    (SELECT count(*) FROM claims WHERE status = 'quarantined') AS claims_quarantined,
    (SELECT count(*) FROM claims WHERE status = 'rejected') AS claims_rejected,
    (SELECT count(*) FROM claims WHERE status = 'archived') AS claims_archived,

    (SELECT count(*) FROM evidence_sources) AS total_evidence_sources,
    (SELECT count(*) FROM evidence_sources WHERE status = 'active') AS evidence_sources_active,
    (SELECT count(*) FROM evidence_sources WHERE status = 'rejected') AS evidence_sources_rejected,

    (SELECT count(*) FROM monitoring_candidates) AS total_candidates,
    (SELECT count(*) FROM monitoring_candidates WHERE editorial_status = 'quarantined') AS candidates_quarantined,
    (SELECT count(*) FROM monitoring_candidates WHERE editorial_status = 'published') AS candidates_published,
    (SELECT count(*) FROM monitoring_candidates WHERE editorial_status = 'rejected') AS candidates_rejected,
    (SELECT count(*) FROM monitoring_candidates WHERE editorial_status = 'archived') AS candidates_archived,
    (SELECT count(*) FROM monitoring_candidates WHERE is_duplicate = 1) AS candidates_duplicates,

    (SELECT count(*) FROM monitoring_runs) AS total_runs,
    (SELECT count(*) FROM monitoring_runs WHERE status = 'completed') AS runs_completed,
    (SELECT count(*) FROM monitoring_runs WHERE status = 'partial') AS runs_partial,
    (SELECT count(*) FROM monitoring_runs WHERE status = 'running') AS runs_running,
    (SELECT count(*) FROM monitoring_runs WHERE status = 'failed') AS runs_failed,
    (SELECT count(*) FROM monitoring_runs WHERE status = 'pending') AS runs_pending,

    (SELECT count(*) FROM entities) AS total_entities;

-- name: CountAdminCandidates :one
SELECT count(*)
FROM monitoring_candidates c
WHERE
    (@filter_status = '' OR c.editorial_status = @filter_status)
    AND (@filter_grade = '' OR c.suggested_grade = @filter_grade)
    AND (@filter_run_id = '' OR c.monitoring_run_id = @filter_run_id)
    AND (@filter_period_since = '' OR c.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, c.entity_name, '\') OR
        like(@search_query, c.normalized_entity_name, '\') OR
        like(@search_query, c.target_entity_name, '\') OR
        like(@search_query, c.case_name, '\') OR
        like(@search_query, c.proposition, '\') OR
        like(@search_query, c.source_url, '\') OR
        like(@search_query, c.canonical_url, '\') OR
        like(@search_query, c.source_title, '\') OR
        like(@search_query, c.publisher_or_author, '\')
    );

-- name: ListAdminCandidates :many
SELECT
    c.id,
    c.monitoring_run_id,
    c.fingerprint,
    c.entity_name,
    c.target_entity_name,
    c.case_name,
    c.relationship_type,
    c.proposition,
    c.suggested_grade,
    c.source_url,
    c.canonical_url,
    c.source_title,
    c.publisher_or_author,
    COALESCE(c.published_at, '') AS published_at,
    c.excerpt,
    c.locator,
    c.context_limits,
    c.technical_confidence,
    c.editorial_status,
    c.is_duplicate,
    c.duplicate_reason,
    COALESCE(c.canonical_candidate_id, '') AS canonical_candidate_id,
    COALESCE(c.resolved_subject_entity_id, '') AS resolved_subject_entity_id,
    COALESCE(c.resolved_target_entity_id, '') AS resolved_target_entity_id,
    COALESCE(c.resolved_case_id, '') AS resolved_case_id,
    COALESCE(c.published_claim_id, '') AS published_claim_id,
    c.structural_gate_passed,
    c.semantic_gate_passed,
    c.policy_action,
    c.created_at,
    c.updated_at,
    r.query AS run_query,
    r.discovery_model AS run_discovery_model
FROM monitoring_candidates c
JOIN monitoring_runs r ON r.id = c.monitoring_run_id
WHERE
    (@filter_status = '' OR c.editorial_status = @filter_status)
    AND (@filter_grade = '' OR c.suggested_grade = @filter_grade)
    AND (@filter_run_id = '' OR c.monitoring_run_id = @filter_run_id)
    AND (@filter_period_since = '' OR c.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, c.entity_name, '\') OR
        like(@search_query, c.normalized_entity_name, '\') OR
        like(@search_query, c.target_entity_name, '\') OR
        like(@search_query, c.case_name, '\') OR
        like(@search_query, c.proposition, '\') OR
        like(@search_query, c.source_url, '\') OR
        like(@search_query, c.canonical_url, '\') OR
        like(@search_query, c.source_title, '\') OR
        like(@search_query, c.publisher_or_author, '\')
    )
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: GetAdminCandidateByID :one
SELECT
    c.id,
    c.monitoring_run_id,
    c.fingerprint,
    c.fingerprint_version,
    c.entity_name,
    c.normalized_entity_name,
    c.target_entity_name,
    c.normalized_target_entity_name,
    c.case_name,
    c.normalized_case_name,
    c.relationship_type,
    c.proposition,
    c.suggested_grade,
    c.source_url,
    c.canonical_url,
    c.source_title,
    c.publisher_or_author,
    COALESCE(c.published_at, '') AS published_at,
    c.excerpt,
    c.locator,
    c.context_limits,
    c.technical_confidence,
    c.editorial_status,
    c.is_duplicate,
    c.duplicate_reason,
    COALESCE(c.canonical_candidate_id, '') AS canonical_candidate_id,
    COALESCE(c.resolved_subject_entity_id, '') AS resolved_subject_entity_id,
    COALESCE(c.resolved_target_entity_id, '') AS resolved_target_entity_id,
    COALESCE(c.resolved_case_id, '') AS resolved_case_id,
    COALESCE(c.published_claim_id, '') AS published_claim_id,
    c.structural_gate_passed,
    c.structural_gate_reasons,
    c.semantic_gate_passed,
    c.semantic_gate_reasons,
    c.policy_action,
    c.policy_reasons,
    c.created_at,
    c.updated_at,
    r.query AS run_query,
    r.status AS run_status,
    r.discovery_provider AS run_discovery_provider,
    r.discovery_model AS run_discovery_model,
    r.verification_provider AS run_verification_provider,
    r.verification_model AS run_verification_model,
    r.created_at AS run_created_at,
    COALESCE(subj.name, '') AS resolved_subject_name,
    COALESCE(subj.slug, '') AS resolved_subject_slug,
    COALESCE(tgt.name, '') AS resolved_target_name,
    COALESCE(tgt.slug, '') AS resolved_target_slug,
    COALESCE(cs.name, '') AS resolved_case_name,
    COALESCE(cs.slug, '') AS resolved_case_slug,
    COALESCE(clm.status, '') AS published_claim_status,
    COALESCE(clm.grade, '') AS published_claim_grade,
    COALESCE(clm.disposition, '') AS published_claim_disposition
FROM monitoring_candidates c
JOIN monitoring_runs r ON r.id = c.monitoring_run_id
LEFT JOIN entities subj ON subj.id = c.resolved_subject_entity_id
LEFT JOIN entities tgt ON tgt.id = c.resolved_target_entity_id
LEFT JOIN cases cs ON cs.id = c.resolved_case_id
LEFT JOIN claims clm ON clm.id = c.published_claim_id
WHERE c.id = ?
LIMIT 1;

-- name: ListAdminSemanticEvaluationsByCandidateID :many
SELECT
    se.id,
    se.monitoring_candidate_id,
    se.provider,
    se.model,
    se.schema_version,
    se.identity_match,
    se.claim_supported,
    se.claim_overstates_source,
    se.attribution_explicit,
    se.grade_compatible,
    se.contains_illicit_inference,
    se.uncertainties,
    se.recommended_action,
    se.created_at
FROM semantic_evaluations se
WHERE se.monitoring_candidate_id = ?
ORDER BY se.created_at ASC, se.id ASC;

-- name: CountAdminEvidenceSources :one
SELECT count(*)
FROM evidence_sources es
JOIN evidence ev ON ev.id = es.evidence_id
JOIN claims c ON c.id = ev.claim_id
JOIN relationships rel ON rel.id = c.relationship_id
JOIN entities subj ON subj.id = rel.subject_entity_id
LEFT JOIN entities tgt ON tgt.id = rel.target_entity_id
LEFT JOIN cases cs ON cs.id = rel.case_id
JOIN sources s ON s.id = es.source_id
WHERE
    (@filter_claim_status = '' OR c.status = @filter_claim_status)
    AND (@filter_evidence_source_status = '' OR es.status = @filter_evidence_source_status)
    AND (@filter_origin = '' OR c.origin = @filter_origin)
    AND (@filter_grade = '' OR c.grade = @filter_grade)
    AND (@filter_role = '' OR es.role = @filter_role)
    AND (@filter_period_since = '' OR es.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, subj.name, '\') OR
        like(@search_query, COALESCE(tgt.name, ''), '\') OR
        like(@search_query, COALESCE(cs.name, ''), '\') OR
        like(@search_query, c.proposition, '\') OR
        like(@search_query, s.title, '\') OR
        like(@search_query, s.publisher_or_author, '\') OR
        like(@search_query, s.canonical_url, '\') OR
        like(@search_query, s.original_url, '\') OR
        like(@search_query, es.excerpt, '\')
    );

-- name: ListAdminEvidenceSources :many
SELECT
    es.id AS evidence_source_id,
    es.evidence_id,
    es.source_id,
    es.excerpt,
    es.locator,
    es.role,
    es.status AS evidence_source_status,
    es.created_at AS evidence_source_created_at,
    es.updated_at AS evidence_source_updated_at,
    ev.summary AS evidence_summary,
    ev.evidence_type,
    c.id AS claim_id,
    c.proposition AS claim_proposition,
    c.attribution AS claim_attribution,
    c.origin AS claim_origin,
    c.grade AS claim_grade,
    c.disposition AS claim_disposition,
    c.metric_eligible AS claim_metric_eligible,
    c.status AS claim_status,
    c.context_status AS claim_context_status,
    c.created_at AS claim_created_at,
    c.updated_at AS claim_updated_at,
    rel.id AS relationship_id,
    rel.relationship_type,
    rel.summary AS relationship_summary,
    rel.context_limits AS relationship_context_limits,
    subj.id AS subject_entity_id,
    subj.name AS subject_entity_name,
    subj.slug AS subject_entity_slug,
    COALESCE(tgt.id, '') AS target_entity_id,
    COALESCE(tgt.name, '') AS target_entity_name,
    COALESCE(tgt.slug, '') AS target_entity_slug,
    COALESCE(cs.id, '') AS case_id,
    COALESCE(cs.name, '') AS case_name,
    COALESCE(cs.slug, '') AS case_slug,
    s.title AS source_title,
    s.publisher_or_author AS source_publisher_or_author,
    s.original_url AS source_original_url,
    s.canonical_url AS source_canonical_url,
    s.source_type,
    s.source_access_status,
    COALESCE(s.source_access_checked_at, '') AS source_access_checked_at,
    s.http_status AS source_http_status,
    COALESCE(s.normalized_error_code, '') AS source_normalized_error_code
FROM evidence_sources es
JOIN evidence ev ON ev.id = es.evidence_id
JOIN claims c ON c.id = ev.claim_id
JOIN relationships rel ON rel.id = c.relationship_id
JOIN entities subj ON subj.id = rel.subject_entity_id
LEFT JOIN entities tgt ON tgt.id = rel.target_entity_id
LEFT JOIN cases cs ON cs.id = rel.case_id
JOIN sources s ON s.id = es.source_id
WHERE
    (@filter_claim_status = '' OR c.status = @filter_claim_status)
    AND (@filter_evidence_source_status = '' OR es.status = @filter_evidence_source_status)
    AND (@filter_origin = '' OR c.origin = @filter_origin)
    AND (@filter_grade = '' OR c.grade = @filter_grade)
    AND (@filter_role = '' OR es.role = @filter_role)
    AND (@filter_period_since = '' OR es.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, subj.name, '\') OR
        like(@search_query, COALESCE(tgt.name, ''), '\') OR
        like(@search_query, COALESCE(cs.name, ''), '\') OR
        like(@search_query, c.proposition, '\') OR
        like(@search_query, s.title, '\') OR
        like(@search_query, s.publisher_or_author, '\') OR
        like(@search_query, s.canonical_url, '\') OR
        like(@search_query, s.original_url, '\') OR
        like(@search_query, es.excerpt, '\')
    )
ORDER BY es.created_at DESC, es.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAdminSources :one
SELECT count(*)
FROM sources s
WHERE
    (@filter_access_status = '' OR s.source_access_status = @filter_access_status)
    AND (@filter_source_type = '' OR s.source_type = @filter_source_type)
    AND (@filter_period_since = '' OR s.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, s.title, '\') OR
        like(@search_query, s.publisher_or_author, '\') OR
        like(@search_query, s.canonical_url, '\') OR
        like(@search_query, s.original_url, '\')
    );

-- name: ListAdminSources :many
SELECT
    s.id,
    s.title,
    s.publisher_or_author,
    s.original_url,
    s.canonical_url,
    COALESCE(s.published_at, '') AS published_at,
    COALESCE(s.accessed_at, '') AS accessed_at,
    s.source_type,
    s.source_access_status,
    COALESCE(s.source_access_checked_at, '') AS source_access_checked_at,
    s.http_status,
    COALESCE(s.normalized_error_code, '') AS normalized_error_code,
    s.created_at,
    s.updated_at,
    (SELECT count(*) FROM evidence_sources es WHERE es.source_id = s.id) AS total_uses,
    (SELECT count(*) FROM evidence_sources es WHERE es.source_id = s.id AND es.status = 'active') AS active_uses
FROM sources s
WHERE
    (@filter_access_status = '' OR s.source_access_status = @filter_access_status)
    AND (@filter_source_type = '' OR s.source_type = @filter_source_type)
    AND (@filter_period_since = '' OR s.created_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, s.title, '\') OR
        like(@search_query, s.publisher_or_author, '\') OR
        like(@search_query, s.canonical_url, '\') OR
        like(@search_query, s.original_url, '\')
    )
ORDER BY s.created_at DESC, s.id DESC
LIMIT @page_limit OFFSET @page_offset;
