
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_room_name_trgm ON rooms USING GIN (name gin_trgm_ops);

-- Recreate with 'simple' configuration and handle empty jsonb_to_tsv properly
ALTER TABLE rooms
    ADD COLUMN searchable tsvector GENERATED ALWAYS AS (
        jsonb_to_tsv(COALESCE(properties, '{}'::jsonb)) || ' ' ||
        to_tsvector('english', COALESCE(description, ''))
        ) STORED;

-- Recreate the GIN index
CREATE INDEX idx_room_searchable ON rooms USING GIN (searchable);

