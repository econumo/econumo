ALTER TABLE users ADD COLUMN deletion_marked_at TIMESTAMP(0) WITHOUT TIME ZONE;
ALTER TABLE users ADD COLUMN deletion_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN last_active_at TIMESTAMP(0) WITHOUT TIME ZONE;
UPDATE users SET last_active_at = date_trunc('second', NOW() AT TIME ZONE 'UTC');
CREATE INDEX IDX_users_deletion_marked_at ON users (deletion_marked_at);
CREATE TABLE users_audit (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    event TEXT NOT NULL,
    email_hash TEXT,
    ref_id UUID,
    details TEXT NOT NULL DEFAULT '{}',
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
);
CREATE UNIQUE INDEX UNIQ_users_audit_ref_id ON users_audit (ref_id) WHERE ref_id IS NOT NULL;
CREATE INDEX IDX_users_audit_event_created ON users_audit (event, created_at, id);
CREATE INDEX IDX_users_audit_email_hash ON users_audit (email_hash, created_at);
CREATE TABLE users_deletion_requests (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    code TEXT NOT NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    expired_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
);
CREATE UNIQUE INDEX UNIQ_users_deletion_requests_user ON users_deletion_requests (user_id);
