-- Opaque access tokens: login sessions and personal access tokens (PATs).
-- Only the sha256 hash of a token is ever stored; validity/expiry is
-- evaluated in Go (not SQL) to avoid engine date-format differences.
CREATE TABLE access_tokens
(
    id           TEXT NOT NULL
    , user_id      TEXT NOT NULL
    , kind         TEXT NOT NULL
    , token_hash   TEXT NOT NULL
    , name         TEXT DEFAULT NULL
    , user_agent   TEXT DEFAULT NULL
    , created_at   TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , last_used_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at   TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , revoked_at   TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
