-- +goose Up
-- Migration VZ-025: Módulo de contraditório estruturado e submissão pública de manifestações de defesa

CREATE TABLE defense_statements (
    id TEXT PRIMARY KEY,
    claim_id TEXT NOT NULL REFERENCES claims(id) ON DELETE RESTRICT,
    statement_type TEXT NOT NULL CHECK (statement_type IN ('correction', 'rebuttal', 'clarification', 'additional_context')),
    title TEXT NOT NULL CHECK (length(trim(title)) > 0 AND length(title) <= 200),
    content TEXT NOT NULL CHECK (length(trim(content)) >= 10 AND length(content) <= 5000),
    source_url TEXT NOT NULL DEFAULT '' CHECK (length(source_url) <= 1000),
    contact_info TEXT NOT NULL DEFAULT '' CHECK (length(contact_info) <= 255),
    status TEXT NOT NULL DEFAULT 'quarantined' CHECK (status IN ('quarantined', 'under_review', 'accepted', 'rejected', 'archived')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_defense_statements_claim ON defense_statements(claim_id);
CREATE INDEX idx_defense_statements_status ON defense_statements(status);
CREATE INDEX idx_defense_statements_type ON defense_statements(statement_type);
CREATE INDEX idx_defense_statements_created_at ON defense_statements(created_at);
CREATE INDEX idx_defense_statements_claim_status ON defense_statements(claim_id, status);

CREATE TABLE defense_statement_decisions (
    id TEXT PRIMARY KEY,
    statement_id TEXT NOT NULL REFERENCES defense_statements(id) ON DELETE RESTRICT,
    action TEXT NOT NULL CHECK (action IN ('accept', 'reject', 'review', 'archive')),
    reason TEXT NOT NULL CHECK (length(trim(reason)) > 0 AND length(reason) <= 1000),
    actor TEXT NOT NULL CHECK (length(trim(actor)) > 0 AND length(actor) <= 128),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_defense_statement_decisions_statement ON defense_statement_decisions(statement_id);
CREATE INDEX idx_defense_statement_decisions_created_at ON defense_statement_decisions(created_at);

-- +goose Down
DROP TABLE IF EXISTS defense_statement_decisions;
DROP TABLE IF EXISTS defense_statements;
