-- +migrate Up
ALTER TABLE room_events
    ADD COLUMN search_properties tsvector GENERATED ALWAYS AS ( jsonb_to_tsv(COALESCE(properties, '{}'::jsonb)) ) STORED;

-- +migrate StatementBegin
CREATE INDEX idx_room_events_search_properties ON room_events USING GIN (search_properties);
-- +migrate StatementEnd