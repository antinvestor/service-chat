
-- Recreate with 'simple' configuration and handle empty jsonb_to_tsv properly
ALTER TABLE rooms
    ADD COLUMN search_properties tsvector GENERATED ALWAYS AS (
        jsonb_to_tsv(COALESCE(properties, '{}'::jsonb))
        ) STORED;

-- Recreate the GIN index
CREATE INDEX idx_rooms_search_properties ON rooms USING GIN (search_properties);
