-- 009_groups.up.sql
CREATE TABLE IF NOT EXISTS groups (
    id              VARCHAR(64) PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    avatar_key      VARCHAR(255) NOT NULL DEFAULT '',
    creator_user_id BIGINT NOT NULL REFERENCES users(id),
    max_members     INT NOT NULL DEFAULT 200,
    current_members INT NOT NULL DEFAULT 0,
    is_archived     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version         INT NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id  VARCHAR(64) NOT NULL REFERENCES groups(id),
    user_id   BIGINT NOT NULL REFERENCES users(id),
    role      VARCHAR(20) NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    added_by  BIGINT,
    is_muted  BOOLEAN NOT NULL DEFAULT FALSE,
    left_at   TIMESTAMPTZ,
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS group_events (
    id              BIGSERIAL PRIMARY KEY,
    group_id        VARCHAR(64) NOT NULL,
    event_type      VARCHAR(30) NOT NULL,
    actor_user_id   BIGINT,
    target_user_id  BIGINT,
    payload         JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members (user_id);
CREATE INDEX IF NOT EXISTS idx_group_events_group ON group_events (group_id, created_at DESC);
