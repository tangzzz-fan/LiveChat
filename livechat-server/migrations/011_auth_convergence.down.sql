-- 011_auth_convergence.down.sql
DROP INDEX IF EXISTS idx_login_audit_event_type;
DROP INDEX IF EXISTS idx_login_audit_user_time;
DROP TABLE IF EXISTS login_audit_events;
ALTER TABLE devices DROP COLUMN IF EXISTS session_version;
