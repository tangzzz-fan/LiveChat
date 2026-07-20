-- 008_conversation_summaries.up.sql
CREATE TABLE IF NOT EXISTS conversation_summaries (
    user_id              BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id      VARCHAR(64) NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    last_message_preview TEXT NOT NULL DEFAULT '',
    last_message_at      TIMESTAMPTZ,
    unread_count         INT NOT NULL DEFAULT 0,
    is_pinned            BOOLEAN NOT NULL DEFAULT FALSE,
    is_hidden            BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, conversation_id)
);

CREATE INDEX IF NOT EXISTS idx_conv_summaries_user_time
    ON conversation_summaries (user_id, is_pinned DESC, last_message_at DESC);
