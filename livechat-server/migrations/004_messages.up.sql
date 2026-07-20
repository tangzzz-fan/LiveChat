-- 004_messages.up.sql
CREATE TABLE IF NOT EXISTS messages (
    server_message_id   VARCHAR(64) PRIMARY KEY,
    conversation_id     VARCHAR(64) NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    conversation_seq    BIGINT NOT NULL,
    sender_user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender_device_id    VARCHAR(64) NOT NULL,
    client_message_id   VARCHAR(128) NOT NULL,
    message_type        VARCHAR(20) NOT NULL DEFAULT 'text',
    content             JSONB NOT NULL DEFAULT '{}',
    server_received_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sender_user_id, client_message_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_conv_seq ON messages (conversation_id, conversation_seq);
