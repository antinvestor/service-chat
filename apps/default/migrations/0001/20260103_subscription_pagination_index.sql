-- Migration: Add composite index for efficient keyset pagination
-- This supports the recursive batching fan-out optimization for rooms with millions of subscribers.

-- Drop the existing partial index in favor of a more complete one
DROP INDEX IF EXISTS idx_roomsubscription_room_id_subscription_state;

-- Create a composite index on (room_id, subscription_state, id)
-- This allows efficient keyset pagination using:
-- WHERE room_id = ? AND subscription_state IN (?) AND id > ? ORDER BY id ASC LIMIT ?
CREATE INDEX idx_roomsubscription_room_state_id ON room_subscriptions (room_id, subscription_state, id);
