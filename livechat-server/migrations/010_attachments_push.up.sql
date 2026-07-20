CREATE TABLE IF NOT EXISTS attachments (
    id              BIGSERIAL PRIMARY KEY,
    message_id      VARCHAR(64) NOT NULL REFERENCES messages(server_message_id) ON DELETE CASCADE,
    object_key      VARCHAR(512) NOT NULL,
    mime_type       VARCHAR(100) NOT NULL,
    size_bytes      BIGINT NOT NULL DEFAULT 0,
    width           INT,
    height          INT,
    duration_sec    FLOAT,
    thumbnail_key   VARCHAR(512),
    upload_status   VARCHAR(30) NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachments_message ON attachments (message_id);
CREATE INDEX IF NOT EXISTS idx_attachments_upload_status ON attachments (upload_status, created_at);

CREATE TABLE IF NOT EXISTS push_events (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id),
    device_id       VARCHAR(64),
    push_type       VARCHAR(20) NOT NULL,
    conversation_id VARCHAR(64),
    message_id      VARCHAR(64),
    apns_response   JSONB,
    apns_status     VARCHAR(20),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_push_events_user_time ON push_events (user_id, created_at DESC);
