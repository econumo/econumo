ALTER TABLE users ADD COLUMN deletion_marked_at DATETIME;
ALTER TABLE users ADD COLUMN deletion_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN last_active_at DATETIME;
UPDATE users SET last_active_at = strftime('%Y-%m-%d %H:%M:%S', 'now');
CREATE INDEX IDX_users_deletion_marked_at ON users (deletion_marked_at);
CREATE TABLE users_audit (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL,
    event TEXT NOT NULL,
    email_hash TEXT,
    ref_id TEXT,
    details TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX UNIQ_users_audit_ref_id ON users_audit (ref_id) WHERE ref_id IS NOT NULL;
CREATE INDEX IDX_users_audit_event_created ON users_audit (event, created_at, id);
CREATE INDEX IDX_users_audit_email_hash ON users_audit (email_hash, created_at);
CREATE TABLE users_deletion_requests (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    code TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    expired_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX UNIQ_users_deletion_requests_user ON users_deletion_requests (user_id);
