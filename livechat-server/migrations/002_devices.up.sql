-- 002_devices.up.sql
CREATE TABLE IF NOT EXISTS devices (
    id                  VARCHAR(64) PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform            VARCHAR(20) NOT NULL DEFAULT 'unknown',
    push_token          VARCHAR(256) DEFAULT '',
    refresh_token_hash  VARCHAR(128) DEFAULT '',
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices (user_id);
