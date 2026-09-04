-- +goose Up
-- Migration VZ-008: Visão canônica da fronteira pública e elegibilidade de rede

CREATE VIEW public_claims_view AS
SELECT
    c.id AS claim_id,
    c.relationship_id,
    r.subject_entity_id AS entity_id,
    r.target_entity_id,
    r.case_id,
    r.relationship_type,
    r.summary AS relationship_summary,
    r.context_limits,
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
WHERE c.status = 'published'
  AND EXISTS (
      SELECT 1 FROM evidence ev
      JOIN evidence_sources es ON es.evidence_id = ev.id
      WHERE ev.claim_id = c.id
        AND es.status = 'active'
        AND es.role = 'supports'
  );

-- +goose Down
DROP VIEW IF EXISTS public_claims_view;
