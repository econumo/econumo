-- Step 3 of 3 of the access_tokens repair (see 20261031000010): replace the old
-- table with the copy step 2 filled. The indexes are created here, not in step
-- 1, because the old table holds their names until it is dropped.
PRAGMA foreign_keys = OFF;
DROP TABLE access_tokens;
ALTER TABLE access_tokens_new RENAME TO access_tokens;
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
CREATE INDEX IDX_access_tokens_revoked_at ON access_tokens (revoked_at);
CREATE INDEX IDX_access_tokens_expires_at ON access_tokens (expires_at);
CREATE INDEX IDX_access_tokens_grant_id ON access_tokens (grant_id);
PRAGMA foreign_keys = ON;
