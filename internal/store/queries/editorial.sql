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
WITH public_claims AS (
    SELECT
        r.subject_entity_id AS entity_id,
        c.id AS claim_id,
        c.grade AS grade,
        c.updated_at AS updated_at
    FROM relationships r
    JOIN claims c ON c.relationship_id = r.id
    WHERE c.status = 'published'
      AND EXISTS (
          SELECT 1 FROM evidence ev
          JOIN evidence_sources es ON es.evidence_id = ev.id
          WHERE ev.claim_id = c.id
            AND es.status = 'active'
            AND es.role = 'supports'
      )
),
entity_public_stats AS (
    SELECT
        entity_id,
        COUNT(DISTINCT claim_id) AS public_claims_count,
        CAST(MIN(grade) AS TEXT) AS highest_grade,
        CAST(MAX(updated_at) AS TEXT) AS last_public_updated_at
    FROM public_claims
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
            SELECT r.summary
            FROM relationships r
            JOIN claims c ON c.relationship_id = r.id
            WHERE r.subject_entity_id = e.id
              AND c.status = 'published'
              AND length(trim(r.summary)) > 0
            LIMIT 1
        ),
        e.role_or_context
    ) AS short_synthesis
FROM entities e
JOIN entity_public_stats stats ON stats.entity_id = e.id
WHERE
    (@order_by = '' OR @order_by != '')
    AND (@order_dir = '' OR @order_dir != '')
    AND (@filter_category = '' OR e.category = @filter_category)
    AND (@filter_grade = '' OR EXISTS (
        SELECT 1 FROM public_claims pc
        WHERE pc.entity_id = e.id AND pc.grade = @filter_grade
    ))
    AND (@filter_relevance = 0 OR e.relevance = @filter_relevance)
    AND (@filter_period_since = '' OR stats.last_public_updated_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        e.name LIKE @search_query OR
        e.normalized_name LIKE @search_query OR
        e.role_or_context LIKE @search_query OR
        e.category LIKE @search_query OR
        EXISTS (
            SELECT 1 FROM entity_aliases ea
            WHERE ea.entity_id = e.id AND ea.normalized_alias LIKE @search_query
        ) OR
        EXISTS (
            SELECT 1 FROM relationships r
            WHERE r.subject_entity_id = e.id AND (
                r.relationship_type LIKE @search_query OR
                r.summary LIKE @search_query
            )
        ) OR
        EXISTS (
            SELECT 1 FROM relationships r
            JOIN claims c ON c.relationship_id = r.id
            WHERE r.subject_entity_id = e.id
              AND c.status = 'published'
              AND c.proposition LIKE @search_query
        ) OR
        EXISTS (
            SELECT 1 FROM relationships r
            JOIN claims c ON c.relationship_id = r.id
            JOIN evidence ev ON ev.claim_id = c.id
            JOIN evidence_sources es ON es.evidence_id = ev.id
            JOIN sources s ON s.id = es.source_id
            WHERE r.subject_entity_id = e.id
              AND c.status = 'published'
              AND es.status = 'active'
              AND (
                  s.publisher_or_author LIKE @search_query OR
                  s.original_url LIKE @search_query OR
                  s.canonical_url LIKE @search_query
              )
        )
    )
ORDER BY
    CASE WHEN ?1 = 'name' AND ?2 = 'asc' THEN e.name END ASC,
    CASE WHEN ?1 = 'name' AND ?2 = 'desc' THEN e.name END DESC,
    CASE WHEN ?1 = 'relevance' AND ?2 = 'asc' THEN e.relevance END ASC,
    CASE WHEN ?1 = 'relevance' AND ?2 = 'desc' THEN e.relevance END DESC,
    CASE WHEN ?1 = 'updated' AND ?2 = 'asc' THEN stats.last_public_updated_at END ASC,
    CASE WHEN ?1 = 'updated' AND ?2 = 'desc' THEN stats.last_public_updated_at END DESC,
    e.name ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublicEntities :one
WITH public_claims AS (
    SELECT
        r.subject_entity_id AS entity_id,
        c.id AS claim_id,
        c.grade AS grade,
        c.updated_at AS updated_at
    FROM relationships r
    JOIN claims c ON c.relationship_id = r.id
    WHERE c.status = 'published'
      AND EXISTS (
          SELECT 1 FROM evidence ev
          JOIN evidence_sources es ON es.evidence_id = ev.id
          WHERE ev.claim_id = c.id
            AND es.status = 'active'
            AND es.role = 'supports'
      )
),
entity_public_stats AS (
    SELECT
        entity_id,
        COUNT(DISTINCT claim_id) AS public_claims_count,
        CAST(MIN(grade) AS TEXT) AS highest_grade,
        CAST(MAX(updated_at) AS TEXT) AS last_public_updated_at
    FROM public_claims
    GROUP BY entity_id
)
SELECT COUNT(*)
FROM entities e
JOIN entity_public_stats stats ON stats.entity_id = e.id
WHERE
    (@filter_category = '' OR e.category = @filter_category)
    AND (@filter_grade = '' OR EXISTS (
        SELECT 1 FROM public_claims pc
        WHERE pc.entity_id = e.id AND pc.grade = @filter_grade
    ))
    AND (@filter_relevance = 0 OR e.relevance = @filter_relevance)
    AND (@filter_period_since = '' OR stats.last_public_updated_at >= @filter_period_since)
    AND (
        @search_query = '' OR
        e.name LIKE @search_query OR
        e.normalized_name LIKE @search_query OR
        e.role_or_context LIKE @search_query OR
        e.category LIKE @search_query OR
        EXISTS (
            SELECT 1 FROM entity_aliases ea
            WHERE ea.entity_id = e.id AND ea.normalized_alias LIKE @search_query
        ) OR
        EXISTS (
            SELECT 1 FROM relationships r
            WHERE r.subject_entity_id = e.id AND (
                r.relationship_type LIKE @search_query OR
                r.summary LIKE @search_query
            )
        ) OR
        EXISTS (
            SELECT 1 FROM relationships r
            JOIN claims c ON c.relationship_id = r.id
            WHERE r.subject_entity_id = e.id
              AND c.status = 'published'
              AND c.proposition LIKE @search_query
        ) OR
        EXISTS (
            SELECT 1 FROM relationships r
            JOIN claims c ON c.relationship_id = r.id
            JOIN evidence ev ON ev.claim_id = c.id
            JOIN evidence_sources es ON es.evidence_id = ev.id
            JOIN sources s ON s.id = es.source_id
            WHERE r.subject_entity_id = e.id
              AND c.status = 'published'
              AND es.status = 'active'
              AND (
                  s.publisher_or_author LIKE @search_query OR
                  s.original_url LIKE @search_query OR
                  s.canonical_url LIKE @search_query
              )
        )
    );

-- name: ListPublicCategories :many
SELECT DISTINCT e.category
FROM entities e
JOIN (
    SELECT DISTINCT r.subject_entity_id AS entity_id
    FROM relationships r
    JOIN claims c ON c.relationship_id = r.id
    WHERE c.status = 'published'
      AND EXISTS (
          SELECT 1 FROM evidence ev
          JOIN evidence_sources es ON es.evidence_id = ev.id
          WHERE ev.claim_id = c.id
            AND es.status = 'active'
            AND es.role = 'supports'
      )
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
      SELECT 1 FROM relationships r
      JOIN claims c ON c.relationship_id = r.id
      WHERE r.subject_entity_id = e.id
        AND c.status = 'published'
        AND EXISTS (
            SELECT 1 FROM evidence ev
            JOIN evidence_sources es ON es.evidence_id = ev.id
            WHERE ev.claim_id = c.id
              AND es.status = 'active'
              AND es.role = 'supports'
        )
  )
LIMIT 1;

-- name: ListPublicClaimsByEntityID :many
SELECT
    c.id AS claim_id,
    c.relationship_id,
    r.relationship_type,
    r.summary AS relationship_summary,
    r.context_limits,
    r.target_entity_id,
    te.name AS target_entity_name,
    te.slug AS target_entity_slug,
    r.case_id,
    cs.name AS case_name,
    cs.slug AS case_slug,
    c.proposition,
    c.attribution,
    c.origin,
    c.grade,
    c.disposition,
    c.metric_eligible,
    c.status,
    c.context_status,
    c.created_at,
    c.updated_at
FROM claims c
JOIN relationships r ON r.id = c.relationship_id
LEFT JOIN entities te ON te.id = r.target_entity_id
LEFT JOIN cases cs ON cs.id = r.case_id
WHERE r.subject_entity_id = ?
  AND c.status = 'published'
  AND EXISTS (
      SELECT 1 FROM evidence ev
      JOIN evidence_sources es ON es.evidence_id = ev.id
      WHERE ev.claim_id = c.id
        AND es.status = 'active'
        AND es.role = 'supports'
  )
ORDER BY
    c.grade ASC,
    c.updated_at DESC;

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
