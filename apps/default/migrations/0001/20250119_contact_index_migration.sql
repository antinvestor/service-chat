
-- Recreate with 'simple' configuration and handle empty jsonb_to_tsv properly
ALTER TABLE contacts
    ADD COLUMN search_properties tsvector GENERATED ALWAYS AS (
        jsonb_to_tsv(COALESCE(properties, '{}'::jsonb))
        ) STORED;

-- Recreate the GIN index
CREATE INDEX idx_contacts_search_properties ON contacts USING GIN (search_properties);
