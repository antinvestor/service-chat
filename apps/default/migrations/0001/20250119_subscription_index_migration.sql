-- +migrate Up
ALTER TABLE room_subscriptions
    ADD COLUMN search_properties tsvector GENERATED ALWAYS AS ( jsonb_to_tsv(COALESCE(properties, '{}'::jsonb)) ) STORED;

-- +migrate StatementBegin
CREATE INDEX idx_room_subscriptions_search_properties ON room_subscriptions USING GIN (search_properties);
-- +migrate StatementEnd
