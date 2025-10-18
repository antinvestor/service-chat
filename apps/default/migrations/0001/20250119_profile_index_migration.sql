ALTER TABLE profiles
    ADD COLUMN search_properties tsvector GENERATED ALWAYS AS ( jsonb_to_tsv(COALESCE(properties, '{}'::jsonb)) ) STORED;

CREATE INDEX idx_profiles_search_properties ON profiles USING GIN (search_properties);