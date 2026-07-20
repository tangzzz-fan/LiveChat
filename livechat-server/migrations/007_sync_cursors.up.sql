-- 007_sync_cursors.up.sql
CREATE TABLE IF NOT EXISTS sync_cursors (
    user_id         BIGINT NOT NULL,
    device_id       VARCHAR(64) NOT NULL,
    last_event_seq  BIGINT NOT NULL DEFAULT 0,
    last_sync_at    TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, device_id)
);
