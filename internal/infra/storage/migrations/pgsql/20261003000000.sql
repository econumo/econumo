-- OAuth authorization server for MCP clients: registered clients, one-shot
-- authorization codes, and grants (one per approval, holding the rotating
-- refresh token). Access tokens stay in access_tokens (kind oauth), linked
-- to their grant so revoking the grant revokes them in one statement.
CREATE TABLE oauth_clients
(
    id              UUID NOT NULL
    , name          TEXT NOT NULL
    , redirect_uris TEXT NOT NULL
    , secret_hash   TEXT DEFAULT NULL
    , created_at    TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , last_used_at  TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , PRIMARY KEY (id)
);
CREATE INDEX IDX_oauth_clients_unused ON oauth_clients (created_at) WHERE last_used_at IS NULL;

CREATE TABLE oauth_authorization_codes
(
    code_hash                TEXT NOT NULL
    , client_id              UUID NOT NULL
    , user_id                UUID NOT NULL
    , redirect_uri           TEXT NOT NULL
    , code_challenge         TEXT NOT NULL
    , resource               TEXT NOT NULL
    , credentials_generation BIGINT NOT NULL
    , created_at             TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at             TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (code_hash)
    , FOREIGN KEY (client_id) REFERENCES oauth_clients (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_oauth_codes_expires_at ON oauth_authorization_codes (expires_at);

CREATE TABLE oauth_grants
(
    id                        UUID NOT NULL
    , user_id                 UUID NOT NULL
    , client_id               UUID NOT NULL
    , refresh_token_hash      TEXT NOT NULL
    , prev_refresh_token_hash TEXT DEFAULT NULL
    , rotated_at              TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , created_at              TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , last_used_at            TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at              TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , revoked_at              TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (client_id) REFERENCES oauth_clients (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX UNIQ_oauth_grants_refresh ON oauth_grants (refresh_token_hash);
CREATE INDEX IDX_oauth_grants_user_id ON oauth_grants (user_id);

-- Every refresh-token hash a grant has rotated away, kept for as long as the
-- grant row lives. Replaying any of them, however many rotations ago, is
-- detected as theft; oauth_grants alone remembers only the latest one.
CREATE TABLE oauth_refresh_tokens_spent
(
    token_hash   TEXT NOT NULL
    , grant_id   UUID NOT NULL
    , spent_at   TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (token_hash)
    , FOREIGN KEY (grant_id) REFERENCES oauth_grants (id) ON DELETE CASCADE
);
CREATE INDEX IDX_oauth_refresh_spent_grant_id ON oauth_refresh_tokens_spent (grant_id);

ALTER TABLE access_tokens ADD COLUMN grant_id UUID DEFAULT NULL;
CREATE INDEX IDX_access_tokens_grant_id ON access_tokens (grant_id);
