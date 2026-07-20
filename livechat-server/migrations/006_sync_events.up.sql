-- 006_sync_events.up.sql
CREATE TABLE IF NOT EXISTS sync_events (
    event_seq       BIGSERIAL,
    user_id         BIGINT NOT NULL,
    conversation_id VARCHAR(64),
    event_type      VARCHAR(50) NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, event_seq)
);

CREATE INDEX IF NOT EXISTS idx_sync_events_user_seq ON sync_events (user_id, event_seq DESC);
