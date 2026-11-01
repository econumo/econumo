-- Step 1 of 3 of the access_tokens repair: create the table in its final shape.
-- Step 2 (command migration:copy-access-tokens, 20261031000001) copies the rows,
-- step 3 (20261031000002) swaps the tables.
--
-- provider / id_token go missing on one upgrade path. 20260907000000 (v1.5.0)
-- adds them with ALTER TABLE; 20260901000000 (v1.6.0) carries an EARLIER
-- version yet was released later, and rebuilds access_tokens to add a NOT NULL
-- scope without carrying the two columns over. An instance that ran v1.5.x
-- recorded 20260907000000, so upgrading applies the still-pending,
-- lower-numbered 20260901000000 afterwards and drops them; nothing re-adds
-- them. Login then fails with "no such column: provider", because minting a
-- session writes both.
--
-- The copy is a command because it must read provider / id_token where they
-- still exist (every healthy database) and write NULL where they do not, and
-- SQL cannot name a column that may be missing. PostgreSQL never lost the
-- columns (its 20260901000000 uses ALTER TABLE), so its three steps are no-ops.
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
