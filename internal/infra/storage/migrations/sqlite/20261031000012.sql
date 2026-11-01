-- Step 3 of 3 of the access_tokens repair (see 20261031000010): replace the old
-- table with the copy step 2 filled.
--
-- First the copy is brought up to date with the old table for every column the
-- two share, so the swap can neither lose a write nor get stuck. Step 2 is
-- recorded once it succeeds and never runs again, so anything that changed the
-- old table afterwards (a token:purge or user:deactivate run from the CLI while
-- serve boots, or a runner that recorded the command without running it, like
-- migrate.NoCommands) would otherwise be undone by the swap: a revoked token
-- revived, a purged one back, every row gone. Only provider / id_token come from
-- step 2 alone; a row it never saw keeps them NULL.
PRAGMA foreign_keys = OFF;
DELETE FROM access_tokens_new WHERE id NOT IN (SELECT id FROM access_tokens);
UPDATE access_tokens_new
SET user_id      = o.user_id
    , kind         = o.kind
    , token_hash   = o.token_hash
    , scope        = o.scope
    , name         = o.name
    , user_agent   = o.user_agent
    , created_at   = o.created_at
    , last_used_at = o.last_used_at
    , expires_at   = o.expires_at
    , revoked_at   = o.revoked_at
    , grant_id     = o.grant_id
FROM access_tokens o
WHERE o.id = access_tokens_new.id;
INSERT INTO access_tokens_new (id, user_id, kind, token_hash, scope, name, user_agent, created_at, last_used_at, expires_at, revoked_at, grant_id)
SELECT id, user_id, kind, token_hash, scope, name, user_agent, created_at, last_used_at, expires_at, revoked_at, grant_id
FROM access_tokens
WHERE id NOT IN (SELECT id FROM access_tokens_new);
DROP TABLE access_tokens;
ALTER TABLE access_tokens_new RENAME TO access_tokens;
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
CREATE INDEX IDX_access_tokens_revoked_at ON access_tokens (revoked_at);
CREATE INDEX IDX_access_tokens_expires_at ON access_tokens (expires_at);
CREATE INDEX IDX_access_tokens_grant_id ON access_tokens (grant_id);
PRAGMA foreign_keys = ON;
