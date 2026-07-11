-- Indexes for the global dead-token purge (token:purge): the DELETE filters on
-- revoked_at / expires_at cutoffs, which would otherwise scan the whole table.
CREATE INDEX IDX_access_tokens_revoked_at ON access_tokens (revoked_at);
CREATE INDEX IDX_access_tokens_expires_at ON access_tokens (expires_at);
