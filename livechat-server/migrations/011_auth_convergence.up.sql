-- 011_auth_convergence.up.sql
ALTER TABLE devices ADD COLUMN IF NOT EXISTS session_version INT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS login_audit_events (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id),
    device_id       VARCHAR(64),
    event_type      VARCHAR(30) NOT NULL,
    ip_address      VARCHAR(45) NOT NULL DEFAULT '',
    user_agent      VARCHAR(512) NOT NULL DEFAULT '',
    failure_reason  VARCHAR(255) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_login_audit_user_time ON login_audit_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_audit_event_type ON login_audit_events (event_type, created_at DESC);
