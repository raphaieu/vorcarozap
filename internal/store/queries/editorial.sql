-- name: CreateEntity :one
INSERT INTO entities (
    id, type, name, normalized_name, slug, category, role_or_context, reach, summary, relevance, relevance_rationale, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: GetEntityByID :one
SELECT * FROM entities
WHERE id = ? LIMIT 1;

-- name: GetEntityBySlug :one
SELECT * FROM entities
WHERE slug = ? LIMIT 1;

-- name: GetEntityByNormalizedName :one
SELECT * FROM entities
WHERE normalized_name = ? LIMIT 1;

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
ORDER BY created_at DESC
LIMIT 1;

-- name: GetAppliedImportRun :one
SELECT * FROM import_runs
WHERE file_hash = ? AND mapping_version = ? AND is_dry_run = 0 AND status IN ('running', 'completed', 'partial')
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateImportRun :one
UPDATE import_runs
SET status = ?, summary_counts = ?, summary_report = ?, error_message = ?, completed_at = ?
WHERE id = ?
RETURNING *;

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

-- name: UpdateSourceAccessStatus :one
UPDATE sources
SET source_access_status = ?,
    source_access_checked_at = ?,
    http_status = ?,
    normalized_error_code = ?,
    updated_at = ?
WHERE id = ?
RETURNING *;

-- name: ListSourcesByAccessStatus :many
SELECT * FROM sources
WHERE source_access_status = ?
ORDER BY created_at ASC;

-- name: CreateClaim :one
INSERT INTO claims (
    id, relationship_id, proposition, attribution, origin, grade, disposition, metric_eligible, status, context_status, quarantine_reasons, import_run_id, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
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

-- name: ListPublicEntities :many
WITH order_params AS (
    SELECT CAST(@order_by AS text) AS order_by, CAST(@order_dir AS text) AS order_dir
),
entity_public_stats AS (
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
    e.slug,
    e.name,
    e.category,
    e.role_or_context,
    e.reach,
    e.relevance,
    e.relevance_rationale,
    stats.public_claims_count,
    stats.highest_grade,
    stats.last_public_updated_at,
    COALESCE(
        (
            SELECT pcv.relationship_summary
            FROM public_claims_view pcv
            WHERE pcv.entity_id = e.id
              AND length(trim(pcv.relationship_summary)) > 0
            ORDER BY
                pcv.grade ASC,
                pcv.updated_at DESC,
                pcv.claim_id ASC
            LIMIT 1
        ),
        e.role_or_context
    ) AS short_synthesis
FROM entities e
JOIN entity_public_stats stats ON stats.entity_id = e.id
WHERE
    (@filter_category = '' OR e.category = @filter_category)
    AND (@filter_grade = '' OR EXISTS (
        SELECT 1 FROM public_claims_view pcv
        WHERE pcv.entity_id = e.id AND pcv.grade = @filter_grade
    ))
    AND (@filter_relevance = 0 OR e.relevance = @filter_relevance)
    AND (@filter_period_since = '' OR stats.last_public_updated_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, e.name, '\') OR
        like(@search_query, e.normalized_name, '\') OR
        like(@search_query, e.role_or_context, '\') OR
        like(@search_query, e.category, '\') OR
        EXISTS (
            SELECT 1 FROM entity_aliases ea
            WHERE ea.entity_id = e.id AND like(@search_query, ea.normalized_alias, '\')
        ) OR
        EXISTS (
            SELECT 1 FROM public_claims_view pcv
            WHERE pcv.entity_id = e.id AND (
                like(@search_query, pcv.relationship_type, '\') OR
                like(@search_query, pcv.relationship_summary, '\') OR
                like(@search_query, pcv.proposition, '\')
            )
        ) OR
        EXISTS (
            SELECT 1 FROM sources s
            JOIN evidence_sources es ON es.source_id = s.id
            JOIN evidence ev ON ev.id = es.evidence_id
            JOIN public_claims_view pcv ON pcv.claim_id = ev.claim_id
            WHERE pcv.entity_id = e.id
              AND es.status = 'active'
              AND (
                  like(@search_query, s.publisher_or_author, '\') OR
                  like(@search_query, s.original_url, '\') OR
                  like(@search_query, s.canonical_url, '\')
              )
        )
    )
ORDER BY
    CASE WHEN (SELECT order_by FROM order_params) = 'name' AND (SELECT order_dir FROM order_params) = 'asc' THEN e.name END ASC,
    CASE WHEN (SELECT order_by FROM order_params) = 'name' AND (SELECT order_dir FROM order_params) = 'desc' THEN e.name END DESC,
    CASE WHEN (SELECT order_by FROM order_params) = 'relevance' AND (SELECT order_dir FROM order_params) = 'asc' THEN e.relevance END ASC,
    CASE WHEN (SELECT order_by FROM order_params) = 'relevance' AND (SELECT order_dir FROM order_params) = 'desc' THEN e.relevance END DESC,
    CASE WHEN (SELECT order_by FROM order_params) = 'updated' AND (SELECT order_dir FROM order_params) = 'asc' THEN stats.last_public_updated_at END ASC,
    CASE WHEN (SELECT order_by FROM order_params) = 'updated' AND (SELECT order_dir FROM order_params) = 'desc' THEN stats.last_public_updated_at END DESC,
    e.name ASC,
    e.id ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublicEntities :one
WITH entity_public_stats AS (
    SELECT
        entity_id,
        CAST(MAX(updated_at) AS TEXT) AS last_public_updated_at
    FROM public_claims_view
    GROUP BY entity_id
)
SELECT COUNT(*)
FROM entities e
JOIN entity_public_stats stats ON stats.entity_id = e.id
WHERE
    (@filter_category = '' OR e.category = @filter_category)
    AND (@filter_grade = '' OR EXISTS (
        SELECT 1 FROM public_claims_view pcv
        WHERE pcv.entity_id = e.id AND pcv.grade = @filter_grade
    ))
    AND (@filter_relevance = 0 OR e.relevance = @filter_relevance)
    AND (@filter_period_since = '' OR stats.last_public_updated_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        like(@search_query, e.name, '\') OR
        like(@search_query, e.normalized_name, '\') OR
        like(@search_query, e.role_or_context, '\') OR
        like(@search_query, e.category, '\') OR
        EXISTS (
            SELECT 1 FROM entity_aliases ea
            WHERE ea.entity_id = e.id AND like(@search_query, ea.normalized_alias, '\')
        ) OR
        EXISTS (
            SELECT 1 FROM public_claims_view pcv
            WHERE pcv.entity_id = e.id AND (
                like(@search_query, pcv.relationship_type, '\') OR
                like(@search_query, pcv.relationship_summary, '\') OR
                like(@search_query, pcv.proposition, '\')
            )
        ) OR
        EXISTS (
            SELECT 1 FROM sources s
            JOIN evidence_sources es ON es.source_id = s.id
            JOIN evidence ev ON ev.id = es.evidence_id
            JOIN public_claims_view pcv ON pcv.claim_id = ev.claim_id
            WHERE pcv.entity_id = e.id
              AND es.status = 'active'
              AND (
                  like(@search_query, s.publisher_or_author, '\') OR
                  like(@search_query, s.original_url, '\') OR
                  like(@search_query, s.canonical_url, '\')
              )
        )
    );

-- name: ListPublicCategories :many
SELECT DISTINCT e.category
FROM entities e
JOIN (
    SELECT DISTINCT entity_id
    FROM public_claims_view
) active_ents ON active_ents.entity_id = e.id
WHERE length(trim(e.category)) > 0
ORDER BY e.category ASC;

-- name: GetPublicEntityBySlug :one
SELECT
    e.id,
    e.type,
    e.name,
    e.normalized_name,
    e.slug,
    e.category,
    e.role_or_context,
    e.reach,
    e.summary,
    e.relevance,
    e.relevance_rationale,
    e.created_at,
    e.updated_at
FROM entities e
WHERE e.slug = ?
  AND EXISTS (
      SELECT 1 FROM public_claims_view pcv
      WHERE pcv.entity_id = e.id
  )
LIMIT 1;

-- name: ListPublicClaimsByEntityID :many
SELECT
    pcv.claim_id,
    pcv.relationship_id,
    pcv.relationship_type,
    pcv.relationship_summary,
    pcv.context_limits,
    pcv.target_entity_id,
    te.name AS target_entity_name,
    te.slug AS target_entity_slug,
    pcv.case_id,
    cs.name AS case_name,
    cs.slug AS case_slug,
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
LEFT JOIN entities te ON te.id = pcv.target_entity_id
LEFT JOIN cases cs ON cs.id = pcv.case_id
WHERE pcv.entity_id = ?
ORDER BY
    pcv.grade ASC,
    pcv.updated_at DESC;

-- name: ListPublicEvidenceSourcesByClaimID :many
SELECT
    ev.id AS evidence_id,
    ev.claim_id,
    ev.summary AS evidence_summary,
    ev.evidence_type,
    es.id AS evidence_source_id,
    es.excerpt,
    es.locator,
    es.role,
    es.status AS evidence_source_status,
    s.id AS source_id,
    s.title,
    s.publisher_or_author,
    s.original_url,
    s.canonical_url,
    s.published_at,
    s.accessed_at,
    s.source_type,
    s.source_access_status
FROM evidence ev
JOIN evidence_sources es ON es.evidence_id = ev.id
JOIN sources s ON s.id = es.source_id
WHERE ev.claim_id = ?
  AND es.status = 'active'
ORDER BY
    CASE es.role
        WHEN 'supports' THEN 1
        WHEN 'contradicts' THEN 2
        WHEN 'contextualizes' THEN 3
        ELSE 4
    END ASC,
    es.created_at ASC;

-- name: FindEntitiesByNormalizedName :many
SELECT * FROM entities
WHERE normalized_name = ?
ORDER BY created_at ASC;

-- name: FindEntitiesByNormalizedAlias :many
SELECT e.* FROM entities e
JOIN entity_aliases ea ON ea.entity_id = e.id
WHERE ea.normalized_alias = ?
ORDER BY e.created_at ASC;

-- name: FindCasesByNormalizedNameOrSlug :many
SELECT * FROM cases
WHERE slug = ? OR lower(trim(name)) = ?
ORDER BY created_at ASC;

-- name: FindRelationshipByComponents :one
SELECT * FROM relationships
WHERE subject_entity_id = ?
  AND ((target_entity_id = ? AND case_id IS NULL) OR (target_entity_id IS NULL AND case_id = ?))
  AND relationship_type = ?
LIMIT 1;

-- name: GetPublicDocumentSourceByID :one
SELECT s.*
FROM sources s
WHERE s.id = ?
  AND EXISTS (
      SELECT 1 FROM evidence_sources es
      JOIN evidence ev ON ev.id = es.evidence_id
      JOIN public_claims_view pcv ON pcv.claim_id = ev.claim_id
      WHERE es.source_id = s.id AND es.status = 'active'
  )
LIMIT 1;

-- name: ListPublicDocumentSequenceBySourceID :many
SELECT
    es.id AS evidence_source_id,
    es.evidence_id,
    es.excerpt,
    es.locator,
    es.role,
    es.status AS evidence_source_status,
    es.created_at AS evidence_source_created_at,
    es.updated_at AS evidence_source_updated_at,
    ev.summary AS evidence_summary,
    ev.evidence_type,
    pcv.claim_id,
    pcv.proposition AS claim_proposition,
    pcv.grade AS claim_grade,
    pcv.disposition AS claim_disposition,
    pcv.status AS claim_status,
    pcv.metric_eligible AS claim_metric_eligible,
    pcv.relationship_type,
    pcv.relationship_summary,
    pcv.context_limits,
    pcv.entity_id AS subject_entity_id,
    sub.name AS subject_entity_name,
    sub.slug AS subject_entity_slug,
    pcv.target_entity_id,
    te.name AS target_entity_name,
    te.slug AS target_entity_slug,
    pcv.case_id,
    cs.name AS case_name,
    cs.slug AS case_slug
FROM evidence_sources es
JOIN evidence ev ON ev.id = es.evidence_id
JOIN public_claims_view pcv ON pcv.claim_id = ev.claim_id
JOIN entities sub ON sub.id = pcv.entity_id
LEFT JOIN entities te ON te.id = pcv.target_entity_id
LEFT JOIN cases cs ON cs.id = pcv.case_id
WHERE es.source_id = ?
  AND es.status = 'active'
ORDER BY
    es.created_at ASC,
    es.id ASC;

-- name: ListPublicEditorialHistoryByEntityID :many
SELECT
    d.id AS decision_id,
    d.claim_id,
    d.evidence_source_id,
    d.action,
    d.created_at,
    CASE
        WHEN d.claim_id IS NOT NULL THEN 'claim'
        ELSE 'evidence_source'
    END AS target_type,
    COALESCE(c.id, esc.id, '') AS associated_claim_id,
    COALESCE(c.proposition, esc.proposition, '') AS associated_claim_proposition,
    COALESCE(c.grade, esc.grade, '') AS associated_claim_grade,
    COALESCE(c.status, esc.status, '') AS associated_claim_status,
    COALESCE(c.origin, esc.origin, '') AS associated_claim_origin,
    COALESCE(c.quarantine_reasons, esc.quarantine_reasons, '[]') AS associated_claim_quarantine_reasons,
    COALESCE(es.id, '') AS target_evidence_source_id,
    COALESCE(es.locator, '') AS target_evidence_source_locator,
    COALESCE(es.role, '') AS target_evidence_source_role,
    COALESCE(es.status, '') AS target_evidence_source_status,
    COALESCE(s.id, '') AS target_source_id,
    COALESCE(s.title, '') AS target_source_title,
    COALESCE(s.publisher_or_author, '') AS target_source_publisher,
    CAST(EXISTS (
        SELECT 1 FROM public_claims_view pcv
        WHERE pcv.claim_id = COALESCE(c.id, esc.id)
    ) AS INTEGER) AS is_claim_currently_public,
    CAST(EXISTS (
        SELECT 1 FROM moderation_decisions prev_d
        WHERE prev_d.claim_id = COALESCE(c.id, esc.id) AND prev_d.action = 'approve'
    ) AS INTEGER) AS had_prior_approval
FROM moderation_decisions d
LEFT JOIN claims c ON c.id = d.claim_id
LEFT JOIN relationships rc ON rc.id = c.relationship_id
LEFT JOIN evidence_sources es ON es.id = d.evidence_source_id
LEFT JOIN evidence ev ON ev.id = es.evidence_id
LEFT JOIN claims esc ON esc.id = ev.claim_id
LEFT JOIN relationships resc ON resc.id = esc.relationship_id
LEFT JOIN sources s ON s.id = es.source_id
WHERE (
    (d.claim_id IS NOT NULL AND (rc.subject_entity_id = @entity_id OR rc.target_entity_id = @entity_id))
    OR
    (d.evidence_source_id IS NOT NULL AND (resc.subject_entity_id = @entity_id OR resc.target_entity_id = @entity_id))
)
ORDER BY d.created_at DESC, d.id DESC
LIMIT @event_limit;

-- name: ListPublicEditorialHistoryBySourceID :many
SELECT
    d.id AS decision_id,
    d.claim_id,
    d.evidence_source_id,
    d.action,
    d.created_at,
    CASE
        WHEN d.claim_id IS NOT NULL THEN 'claim'
        ELSE 'evidence_source'
    END AS target_type,
    COALESCE(c.id, esc.id, '') AS associated_claim_id,
    COALESCE(c.proposition, esc.proposition, '') AS associated_claim_proposition,
    COALESCE(c.grade, esc.grade, '') AS associated_claim_grade,
    COALESCE(c.status, esc.status, '') AS associated_claim_status,
    COALESCE(c.origin, esc.origin, '') AS associated_claim_origin,
    COALESCE(c.quarantine_reasons, esc.quarantine_reasons, '[]') AS associated_claim_quarantine_reasons,
    COALESCE(es.id, '') AS target_evidence_source_id,
    COALESCE(es.locator, '') AS target_evidence_source_locator,
    COALESCE(es.role, '') AS target_evidence_source_role,
    COALESCE(es.status, '') AS target_evidence_source_status,
    COALESCE(s.id, '') AS target_source_id,
    COALESCE(s.title, '') AS target_source_title,
    COALESCE(s.publisher_or_author, '') AS target_source_publisher,
    COALESCE(sub.name, '') AS subject_entity_name,
    COALESCE(sub.slug, '') AS subject_entity_slug,
    CAST(EXISTS (
        SELECT 1 FROM public_claims_view pcv
        WHERE pcv.claim_id = COALESCE(c.id, esc.id)
    ) AS INTEGER) AS is_claim_currently_public,
    CAST(EXISTS (
        SELECT 1 FROM moderation_decisions prev_d
        WHERE prev_d.claim_id = COALESCE(c.id, esc.id) AND prev_d.action = 'approve'
    ) AS INTEGER) AS had_prior_approval
FROM moderation_decisions d
LEFT JOIN claims c ON c.id = d.claim_id
LEFT JOIN relationships rc ON rc.id = c.relationship_id
LEFT JOIN entities sub_c ON sub_c.id = rc.subject_entity_id
LEFT JOIN evidence_sources es ON es.id = d.evidence_source_id
LEFT JOIN evidence ev ON ev.id = es.evidence_id
LEFT JOIN claims esc ON esc.id = ev.claim_id
LEFT JOIN relationships resc ON resc.id = esc.relationship_id
LEFT JOIN entities sub_esc ON sub_esc.id = resc.subject_entity_id
LEFT JOIN entities sub ON sub.id = COALESCE(sub_c.id, sub_esc.id)
LEFT JOIN sources s ON s.id = COALESCE(es.source_id, (
    SELECT es2.source_id FROM evidence ev2
    JOIN evidence_sources es2 ON es2.evidence_id = ev2.id
    WHERE ev2.claim_id = d.claim_id AND es2.source_id = @source_id
    LIMIT 1
))
WHERE (
    (d.evidence_source_id IS NOT NULL AND es.source_id = @source_id)
    OR
    (d.claim_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM evidence ev3
        JOIN evidence_sources es3 ON es3.evidence_id = ev3.id
        WHERE ev3.claim_id = d.claim_id AND es3.source_id = @source_id
    ))
)
ORDER BY d.created_at DESC, d.id DESC
LIMIT @event_limit;
