-- +goose Up
-- Migration VZ-016: Moderação humana de claims e histórico de decisões auditáveis

CREATE TABLE moderation_decisions (
    id TEXT PRIMARY KEY,
    claim_id TEXT REFERENCES claims(id) ON DELETE RESTRICT,
    evidence_source_id TEXT REFERENCES evidence_sources(id) ON DELETE RESTRICT,
    action TEXT NOT NULL CHECK (action IN ('approve', 'reject', 'restore')),
    reason TEXT NOT NULL CHECK (length(trim(reason)) > 0 AND length(reason) <= 1000),
    actor TEXT NOT NULL CHECK (length(trim(actor)) > 0 AND length(actor) <= 128),
    candidate_fingerprint TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CONSTRAINT chk_moderation_decisions_target_xor CHECK (
        (claim_id IS NOT NULL AND evidence_source_id IS NULL) OR
        (claim_id IS NULL AND evidence_source_id IS NOT NULL)
    )
);

CREATE INDEX idx_moderation_decisions_claim ON moderation_decisions(claim_id);
CREATE INDEX idx_moderation_decisions_evidence_source ON moderation_decisions(evidence_source_id);
CREATE INDEX idx_moderation_decisions_created_at ON moderation_decisions(created_at);
CREATE INDEX idx_moderation_decisions_action ON moderation_decisions(action);

-- +goose Down
DROP TABLE IF EXISTS moderation_decisions;
