-- internal/store/queries/metrics.sql

-- name: GetPublicOverviewMetrics :one
SELECT
    COUNT(DISTINCT pcv.entity_id) AS total_entities,
    COUNT(pcv.claim_id) AS total_claims,
    (
        SELECT COUNT(DISTINCT s.id)
        FROM public_claims_view pcv2
        JOIN evidence ev ON ev.claim_id = pcv2.claim_id
        JOIN evidence_sources es ON es.evidence_id = ev.id
        JOIN sources s ON s.id = es.source_id
        WHERE es.status = 'active'
    ) AS total_sources,
    COALESCE(MAX(pcv.updated_at), '') AS latest_update
FROM public_claims_view pcv;

-- name: GetEligibleNetworkMetrics :one
SELECT
    COUNT(DISTINCT pcv.entity_id) AS eligible_entities,
    COUNT(pcv.claim_id) AS eligible_claims
FROM public_claims_view pcv
WHERE pcv.metric_eligible = 1;

-- name: ListEligibleClaimsByGrade :many
SELECT
    pcv.grade,
    COUNT(*) AS count
FROM public_claims_view pcv
WHERE pcv.metric_eligible = 1
GROUP BY pcv.grade
ORDER BY pcv.grade ASC;

-- name: ListEligibleEntitiesByRelevance :many
SELECT
    e.relevance,
    COUNT(DISTINCT e.id) AS count
FROM entities e
WHERE e.id IN (
    SELECT pcv.entity_id
    FROM public_claims_view pcv
    WHERE pcv.metric_eligible = 1
)
GROUP BY e.relevance
ORDER BY e.relevance ASC;

-- name: ListEligibleEntitiesByCategory :many
SELECT
    e.category,
    COUNT(DISTINCT e.id) AS count
FROM entities e
WHERE e.id IN (
    SELECT pcv.entity_id
    FROM public_claims_view pcv
    WHERE pcv.metric_eligible = 1
) AND TRIM(e.category) != ''
GROUP BY e.category
ORDER BY count DESC, e.category ASC;

-- name: ListRecentEligibleClaims :many
SELECT
    pcv.claim_id,
    pcv.entity_id,
    e.name AS entity_name,
    e.slug AS entity_slug,
    e.category AS entity_category,
    e.relevance AS entity_relevance,
    pcv.relationship_type AS claim_type,
    pcv.grade,
    pcv.relationship_summary AS short_synthesis,
    pcv.updated_at
FROM public_claims_view pcv
JOIN entities e ON e.id = pcv.entity_id
WHERE pcv.metric_eligible = 1
  AND (CAST(@since AS text) = '' OR pcv.updated_at >= CAST(@since AS text))
ORDER BY pcv.updated_at DESC, pcv.claim_id ASC
LIMIT @claim_limit;
