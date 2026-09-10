-- name: ListPublicEntitiesForExport :many
WITH entity_public_stats AS (
    SELECT
        entity_id,
        COUNT(DISTINCT claim_id) AS public_claims_count,
        CAST(MIN(grade) AS TEXT) AS highest_grade,
        CAST(MAX(updated_at) AS TEXT) AS last_public_updated_at
    FROM public_claims_view
    GROUP BY entity_id
)
SELECT
    e.id,
    e.type,
    e.name,
    e.slug,
    e.category,
    e.role_or_context,
    e.reach,
    e.summary,
    e.relevance,
    e.relevance_rationale,
    stats.public_claims_count,
    stats.highest_grade,
    stats.last_public_updated_at
FROM entities e
JOIN entity_public_stats stats ON stats.entity_id = e.id
ORDER BY e.name ASC, e.id ASC;

-- name: ListPublicClaimsForExport :many
SELECT
    pcv.claim_id,
    pcv.relationship_id,
    se.name AS subject_entity_name,
    se.slug AS subject_entity_slug,
    pcv.relationship_type,
    pcv.relationship_summary,
    pcv.context_limits,
    pcv.target_entity_id,
    COALESCE(te.name, '') AS target_entity_name,
    COALESCE(te.slug, '') AS target_entity_slug,
    pcv.case_id,
    COALESCE(cs.name, '') AS case_name,
    COALESCE(cs.slug, '') AS case_slug,
    pcv.proposition,
    pcv.attribution,
    pcv.origin,
    pcv.grade,
    pcv.disposition,
    pcv.metric_eligible,
    pcv.status,
    pcv.context_status,
    pcv.created_at,
    pcv.updated_at
FROM public_claims_view pcv
JOIN entities se ON se.id = pcv.entity_id
LEFT JOIN entities te ON te.id = pcv.target_entity_id
LEFT JOIN cases cs ON cs.id = pcv.case_id
ORDER BY se.name ASC, pcv.grade ASC, pcv.created_at ASC, pcv.claim_id ASC;

-- name: ListPublicEvidenceSourcesForExport :many
SELECT
    ev.id AS evidence_id,
    ev.claim_id,
    se.name AS subject_entity_name,
    se.slug AS subject_entity_slug,
    ev.summary AS evidence_summary,
    ev.evidence_type,
    es.id AS evidence_source_id,
    es.excerpt,
    es.locator,
    es.role,
    es.status AS evidence_source_status,
    s.id AS source_id,
    s.title AS source_title,
    s.publisher_or_author AS source_publisher,
    s.original_url,
    s.canonical_url,
    s.published_at,
    s.accessed_at,
    s.source_type,
    s.source_access_status
FROM public_claims_view pcv
JOIN entities se ON se.id = pcv.entity_id
JOIN evidence ev ON ev.claim_id = pcv.claim_id
JOIN evidence_sources es ON es.evidence_id = ev.id
JOIN sources s ON s.id = es.source_id
WHERE es.status = 'active'
ORDER BY se.name ASC, pcv.claim_id ASC,
    CASE es.role
        WHEN 'supports' THEN 1
        WHEN 'contradicts' THEN 2
        WHEN 'contextualizes' THEN 3
        ELSE 4
    END ASC,
    es.created_at ASC,
    es.id ASC;
