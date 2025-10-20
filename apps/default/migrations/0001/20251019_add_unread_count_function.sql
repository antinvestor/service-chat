-- Migration to add unread count calculation function and trigger
-- This calculates unread_count based on pending outbox entries

-- Function to calculate unread count for a subscription
CREATE OR REPLACE FUNCTION calculate_unread_count(subscription_id_param VARCHAR(50))
RETURNS INTEGER AS $$
BEGIN
    RETURN (
        SELECT COUNT(*)::INTEGER
        FROM room_outboxes
        WHERE subscription_id = subscription_id_param
          AND status = 'pending'
    );
END;
$$ LANGUAGE plpgsql STABLE;

-- Function to update unread count when outbox changes
CREATE OR REPLACE FUNCTION update_subscription_unread_count()
RETURNS TRIGGER AS $$
BEGIN
    -- Update the subscription's unread count
    UPDATE room_subscriptions
    SET unread_count = calculate_unread_count(
        CASE 
            WHEN TG_OP = 'DELETE' THEN OLD.subscription_id
            ELSE NEW.subscription_id
        END
    )
    WHERE id = CASE 
        WHEN TG_OP = 'DELETE' THEN OLD.subscription_id
        ELSE NEW.subscription_id
    END;
    
    RETURN CASE 
        WHEN TG_OP = 'DELETE' THEN OLD
        ELSE NEW
    END;
END;
$$ LANGUAGE plpgsql;

-- Trigger on room_outboxes INSERT
DROP TRIGGER IF EXISTS trigger_outbox_insert_update_unread ON room_outboxes;
CREATE TRIGGER trigger_outbox_insert_update_unread
    AFTER INSERT ON room_outboxes
    FOR EACH ROW
    EXECUTE FUNCTION update_subscription_unread_count();

-- Trigger on room_outboxes UPDATE (status changes)
DROP TRIGGER IF EXISTS trigger_outbox_update_update_unread ON room_outboxes;
CREATE TRIGGER trigger_outbox_update_update_unread
    AFTER UPDATE OF status ON room_outboxes
    FOR EACH ROW
    WHEN (OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION update_subscription_unread_count();

-- Note: RoomOutbox doesn't use soft deletes, so no delete trigger needed

-- Add unread_count column if it doesn't exist (with default 0)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'room_subscriptions' 
        AND column_name = 'unread_count'
    ) THEN
        ALTER TABLE room_subscriptions ADD COLUMN unread_count INTEGER DEFAULT 0 NOT NULL;
    END IF;
END $$;

-- Initialize unread_count for existing subscriptions
UPDATE room_subscriptions
SET unread_count = calculate_unread_count(id)
WHERE unread_count = 0 OR unread_count IS NULL;
