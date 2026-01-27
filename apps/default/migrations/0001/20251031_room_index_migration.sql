-- +migrate Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_room_name_trgm ON rooms USING GIN (name gin_trgm_ops);

-- Add generated tsvector column for full-text search across properties and description.
-- Uses tsvector || tsvector concatenation (not || text ||).
ALTER TABLE rooms
    ADD COLUMN IF NOT EXISTS searchable tsvector GENERATED ALWAYS AS (
        jsonb_to_tsv(COALESCE(properties, '{}'::jsonb)) ||
        to_tsvector('english', COALESCE(description, ''))
        ) STORED;

-- GIN index for efficient full-text search
CREATE INDEX IF NOT EXISTS idx_room_searchable ON rooms USING GIN (searchable);

