-- Step 3 of 3 of the access_tokens repair (see 20261031000000): replace the old
-- table with the copy step 2 filled.
--
-- Guard: the swap refuses to run unless the copy holds every row. Step 2 is a
-- command, and a runner that records command steps without running them
-- (migrate.NoCommands) would otherwise reach this point with an empty copy and
-- drop every session and personal token. A mismatch fails the CHECK below and
-- rolls the whole step back.
PRAGMA foreign_keys = OFF;
CREATE TEMP TABLE access_tokens_copy_check (complete INTEGER NOT NULL CHECK (complete = 1));
INSERT INTO access_tokens_copy_check (complete)
SELECT (SELECT COUNT(*) FROM access_tokens) = (SELECT COUNT(*) FROM access_tokens_new);
DROP TABLE access_tokens_copy_check;
DROP TABLE access_tokens;
ALTER TABLE access_tokens_new RENAME TO access_tokens;
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
CREATE INDEX IDX_access_tokens_revoked_at ON access_tokens (revoked_at);
CREATE INDEX IDX_access_tokens_expires_at ON access_tokens (expires_at);
CREATE INDEX IDX_access_tokens_grant_id ON access_tokens (grant_id);
PRAGMA foreign_keys = ON;
