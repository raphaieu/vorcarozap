-- +goose Up
-- Migration VZ-005: Saneamento e fidelidade de campos do curated_seed
-- Adiciona categoria e alcance próprios em entities, e contexto/status descritivo e motivos de quarentena em claims.

ALTER TABLE entities ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE entities ADD COLUMN reach TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_entities_category ON entities(category);

ALTER TABLE claims ADD COLUMN context_status TEXT NOT NULL DEFAULT '';
ALTER TABLE claims ADD COLUMN quarantine_reasons TEXT NOT NULL DEFAULT '[]';

-- Migração defensiva para bancos locais existentes:
-- 1. Se reach estiver vazio e summary possuir valor de alcance, copia para reach e limpa summary.
UPDATE entities
SET reach = summary, summary = ''
WHERE reach = '' AND summary != '';

-- 2. Se context_status estiver vazio e attribution possuir valor de situação, move para context_status.
UPDATE claims
SET context_status = attribution, attribution = ''
WHERE origin = 'curated_seed' AND context_status = '' AND attribution != '';

-- +goose Down
DROP INDEX IF EXISTS idx_entities_category;

ALTER TABLE claims DROP COLUMN quarantine_reasons;
ALTER TABLE claims DROP COLUMN context_status;
ALTER TABLE entities DROP COLUMN reach;
ALTER TABLE entities DROP COLUMN category;
