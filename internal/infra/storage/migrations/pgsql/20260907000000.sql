-- See the sqlite sibling for the semantics.
CREATE TABLE users_identities
(
    id           UUID     NOT NULL
    , user_id    UUID     NOT NULL
    , provider   TEXT     NOT NULL
    , subject    TEXT     NOT NULL
    , email      TEXT     NOT NULL DEFAULT ''
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX users_identities_provider_subject_uniq ON users_identities (provider, subject);
CREATE UNIQUE INDEX users_identities_user_provider_uniq ON users_identities (user_id, provider);

CREATE TABLE oauth_states
(
    state_hash      TEXT     NOT NULL
    , provider      TEXT     NOT NULL
    , nonce         TEXT     NOT NULL
    , code_verifier TEXT     NOT NULL DEFAULT ''
    , client        TEXT     NOT NULL
    , intent        TEXT     NOT NULL
    , link_user_id  UUID
    , created_at    TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at    TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (state_hash)
);
CREATE INDEX oauth_states_expires_at_idx ON oauth_states (expires_at);

CREATE TABLE oauth_handoffs
(
    code_hash    TEXT     NOT NULL
    , user_id    UUID     NOT NULL
    , provider   TEXT     NOT NULL
    , id_token   TEXT
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , expires_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (code_hash)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX oauth_handoffs_expires_at_idx ON oauth_handoffs (expires_at);

ALTER TABLE access_tokens ADD COLUMN provider TEXT;
ALTER TABLE access_tokens ADD COLUMN id_token TEXT;
