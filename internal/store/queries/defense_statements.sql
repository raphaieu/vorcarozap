-- name: InsertDefenseStatement :exec
INSERT INTO defense_statements (
    id, claim_id, statement_type, title, content, source_url, contact_info, status, created_at, updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: GetDefenseStatementByID :one
SELECT
    ds.id,
    ds.claim_id,
    ds.statement_type,
    ds.title,
    ds.content,
    ds.source_url,
    ds.contact_info,
    ds.status,
    ds.created_at,
    ds.updated_at,
    c.proposition AS claim_proposition,
    c.grade AS claim_grade,
    c.status AS claim_status,
    e.name AS entity_name,
    e.slug AS entity_slug
FROM defense_statements ds
JOIN claims c ON c.id = ds.claim_id
JOIN relationships r ON r.id = c.relationship_id
JOIN entities e ON e.id = r.subject_entity_id
WHERE ds.id = ?;

-- name: GetDefenseStatementForModeration :one
SELECT id, claim_id, statement_type, title, content, source_url, contact_info, status, created_at, updated_at
FROM defense_statements
WHERE id = ?;

-- name: UpdateDefenseStatementStatusWithVersion :execrows
UPDATE defense_statements
SET status = @new_status,
    updated_at = @updated_at
WHERE id = @id
  AND updated_at = @expected_updated_at;

-- name: GetPublicClaimForManifestation :one
SELECT
    pcv.claim_id,
    pcv.proposition,
    e.name AS entity_name,
    e.slug AS entity_slug
FROM public_claims_view pcv
JOIN entities e ON e.id = pcv.entity_id
WHERE pcv.claim_id = ?
LIMIT 1;

-- name: InsertDefenseStatementDecision :exec
INSERT INTO defense_statement_decisions (
    id, statement_id, action, reason, actor, created_at
) VALUES (
    ?, ?, ?, ?, ?, ?
);

-- name: ListDefenseStatementDecisionsByStatementID :many
SELECT id, statement_id, action, reason, actor, created_at
FROM defense_statement_decisions
WHERE statement_id = ?
ORDER BY created_at DESC, id DESC;

-- name: ListPublicDefenseStatementsByClaimID :many
SELECT
    ds.id,
    ds.claim_id,
    ds.statement_type,
    ds.title,
    ds.content,
    ds.source_url,
    ds.created_at
FROM defense_statements ds
JOIN public_claims_view pcv ON pcv.claim_id = ds.claim_id
WHERE ds.claim_id = ?
  AND ds.status = 'accepted'
ORDER BY ds.created_at ASC, ds.id ASC;

-- name: ListAdminDefenseStatements :many
SELECT
    ds.id,
    ds.claim_id,
    ds.statement_type,
    ds.title,
    ds.content,
    ds.source_url,
    ds.contact_info,
    ds.status,
    ds.created_at,
    ds.updated_at,
    c.proposition AS claim_proposition,
    c.grade AS claim_grade,
    c.status AS claim_status,
    e.name AS entity_name,
    e.slug AS entity_slug
FROM defense_statements ds
JOIN claims c ON c.id = ds.claim_id
JOIN relationships r ON r.id = c.relationship_id
JOIN entities e ON e.id = r.subject_entity_id
ORDER BY ds.created_at DESC, ds.id DESC
LIMIT ? OFFSET ?;

-- name: CountAdminDefenseStatements :one
SELECT COUNT(*)
FROM defense_statements ds;
