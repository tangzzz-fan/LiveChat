-- 001_users.up.sql
CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL PRIMARY KEY,
    phone_e164  VARCHAR(20) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
