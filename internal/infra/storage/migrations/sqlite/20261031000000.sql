-- Repairs access_tokens.provider / id_token, which go missing on one upgrade
-- path. 20260907000000 (v1.5.0) adds them with ALTER TABLE; 20260901000000
-- (v1.6.0) carries an EARLIER version yet was released later, and rebuilds
-- access_tokens to add a NOT NULL scope without carrying the two columns over.
-- An instance that ran v1.5.x recorded 20260907000000, so upgrading applies the
-- still-pending, lower-numbered 20260901000000 afterwards and drops them;
-- 20260907000000 is already recorded, so nothing re-adds them. Login then fails
-- with "no such column: provider", because minting a session writes both.
--
-- A rebuild rather than ALTER TABLE ADD COLUMN: SQLite has no
-- ADD COLUMN IF NOT EXISTS, and this migration must run on a healthy database
-- (fresh installs, anything upgraded from v1.6.x) as well as a damaged one. A
-- rebuild states the final shape instead of a delta, so it is correct for both
-- without inspecting the live schema.
--
-- provider and id_token are selected as NULL, NOT copied: the damaged table has
-- no such columns to read, so naming them in the SELECT would fail on exactly
-- the databases this repairs. Their values were lost with the original rebuild
-- and cannot be recovered -- a session minted before the upgrade keeps no
-- provider, so logout skips the RP-initiated redirect for it. grant_id IS
-- copied: 20261003000000 adds it to every database, damaged or not, and it
-- links oauth tokens to the grant that revokes them.
--
-- PostgreSQL needs no counterpart: its 20260901000000 adds scope with ALTER
-- TABLE instead of rebuilding the table, so it never dropped the columns.
PRAGMA foreign_keys = OFF;
CREATE TABLE access_tokens_new
(
    id           TEXT NOT NULL
    , user_id      TEXT NOT NULL
    , kind         TEXT NOT NULL
    , token_hash   TEXT NOT NULL
    , scope        TEXT NOT NULL
    , name         TEXT DEFAULT NULL
    , user_agent   TEXT DEFAULT NULL
    , created_at   DATETIME NOT NULL
    , last_used_at DATETIME NOT NULL
    , expires_at   DATETIME DEFAULT NULL
    , revoked_at   DATETIME DEFAULT NULL
    , provider     TEXT DEFAULT NULL
    , id_token     TEXT DEFAULT NULL
    , grant_id     TEXT DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
INSERT INTO access_tokens_new (id, user_id, kind, token_hash, scope, name, user_agent, created_at, last_used_at, expires_at, revoked_at, provider, id_token, grant_id)
SELECT id, user_id, kind, token_hash, scope, name, user_agent, created_at, last_used_at, expires_at, revoked_at, NULL, NULL, grant_id
FROM access_tokens;
DROP TABLE access_tokens;
ALTER TABLE access_tokens_new RENAME TO access_tokens;
CREATE UNIQUE INDEX UNIQ_access_tokens_token_hash ON access_tokens (token_hash);
CREATE INDEX IDX_access_tokens_user_id ON access_tokens (user_id);
CREATE INDEX IDX_access_tokens_revoked_at ON access_tokens (revoked_at);
CREATE INDEX IDX_access_tokens_expires_at ON access_tokens (expires_at);
CREATE INDEX IDX_access_tokens_grant_id ON access_tokens (grant_id);
PRAGMA foreign_keys = ON;
