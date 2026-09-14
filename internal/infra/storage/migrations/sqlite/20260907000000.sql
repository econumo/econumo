-- OAuth / OIDC login (docs/superpowers/specs/2026-09-07-oauth-login-design.md §4).
-- users_identities maps a provider subject to a user; email is display only.
-- The issuer is part of the key because the custom slot's provider id is
-- always "oidc": without it, repointing ECONUMO_OIDC_ISSUER_URL would let a
-- subject collision at the new issuer sign in as the old issuer's user.
-- oauth_states holds the in-flight authorization requests (server-side because
-- Apple's cross-site form POST carries no SameSite cookie and the app's browser
-- sheet shares no storage with the SPA); oauth_handoffs the one-shot codes the
-- client exchanges for a session (kind 'login') or for the identity write the
-- link callback defers to its initiating client (kind 'link'), so the token
-- never travels in a URL and no unattended callback can link a victim. Both
-- carry flow_hash: the sha256 of a secret handed to the client that started the
-- flow, so a forged callback cannot log a victim into an attacker's account.
CREATE TABLE users_identities
(
    id           TEXT     NOT NULL
    , user_id    TEXT     NOT NULL
    , provider   TEXT     NOT NULL
    , issuer     TEXT     NOT NULL DEFAULT ''
    , subject    TEXT     NOT NULL
    , email      TEXT     NOT NULL DEFAULT ''
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX users_identities_provider_issuer_subject_uniq ON users_identities (provider, issuer, subject);
CREATE UNIQUE INDEX users_identities_user_provider_uniq ON users_identities (user_id, provider);

CREATE TABLE oauth_states
(
    state_hash      TEXT     NOT NULL
    , provider      TEXT     NOT NULL
    , nonce         TEXT     NOT NULL
    , code_verifier TEXT     NOT NULL DEFAULT ''
    , flow_hash     TEXT     NOT NULL DEFAULT ''
    , client        TEXT     NOT NULL
    , intent        TEXT     NOT NULL
    , link_user_id  TEXT
    , created_at    DATETIME NOT NULL
    , expires_at    DATETIME NOT NULL
    , PRIMARY KEY (state_hash)
);
CREATE INDEX oauth_states_expires_at_idx ON oauth_states (expires_at);

CREATE TABLE oauth_handoffs
(
    code_hash    TEXT     NOT NULL
    , kind       TEXT     NOT NULL DEFAULT 'login'
    , user_id    TEXT     NOT NULL
    , provider   TEXT     NOT NULL
    , issuer     TEXT     NOT NULL DEFAULT ''
    , subject    TEXT     NOT NULL DEFAULT ''
    , email      TEXT     NOT NULL DEFAULT ''
    , flow_hash  TEXT     NOT NULL DEFAULT ''
    , id_token   TEXT
    , created_at DATETIME NOT NULL
    , expires_at DATETIME NOT NULL
    , PRIMARY KEY (code_hash)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX oauth_handoffs_expires_at_idx ON oauth_handoffs (expires_at);

-- Sessions minted from a provider remember it (sessions list, logout notice);
-- id_token is kept only for the custom slot's RP-initiated logout.
ALTER TABLE access_tokens ADD COLUMN provider TEXT;
ALTER TABLE access_tokens ADD COLUMN id_token TEXT;
