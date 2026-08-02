-- Per-user custom currencies, in one step to the final shape: currencies
-- gains user_id (NULL = global) and rate (the custom's fixed rate,
-- "X units per 1 base unit"); UNIQUE(code) is replaced by two partial
-- unique indexes (global codes instance-unique, custom codes unique per
-- owner); new table users_hidden_currencies stores per-user hidden
-- currencies (globals AND own customs); the profile currency option becomes
-- a currency ID with a USD-normalized invariant (every user ends up holding
-- a live currency id, so reads never need a fallback).
--
-- SQLite cannot drop a table-level UNIQUE, so currencies is rebuilt -- a
-- SINGLE-table rebuild: the runner hoists the pragmas below outside its
-- transaction (inside one the pragma is a silent no-op and the drop would
-- cascade into every referencing table), and the runner's in-transaction
-- foreign_key_check guards the result.
PRAGMA foreign_keys = OFF;

CREATE TABLE currencies_new
(
    id         TEXT    NOT NULL
    , code       TEXT     NOT NULL
    , symbol     VARCHAR(12) NOT NULL
    , created_at DATETIME    NOT NULL
    , name VARCHAR(64) DEFAULT NULL
    , fraction_digits SMALLINT DEFAULT '2' NOT NULL
    , user_id TEXT DEFAULT NULL
    , rate NUMERIC(19, 8) DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
INSERT INTO currencies_new (id, code, symbol, created_at, name, fraction_digits)
SELECT id, code, symbol, created_at, name, fraction_digits FROM currencies;
DROP TABLE currencies;
ALTER TABLE currencies_new RENAME TO currencies;

CREATE UNIQUE INDEX UNIQ_currencies_code_global ON currencies (code) WHERE user_id IS NULL;
CREATE UNIQUE INDEX UNIQ_currencies_user_code ON currencies (user_id, code) WHERE user_id IS NOT NULL;
CREATE INDEX IDX_currencies_user_id ON currencies (user_id);

CREATE TABLE users_hidden_currencies
(
    user_id     TEXT NOT NULL
    , currency_id TEXT NOT NULL
    , created_at  DATETIME NOT NULL
    , PRIMARY KEY (user_id, currency_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE INDEX IDX_users_hidden_currencies_currency_id ON users_hidden_currencies (currency_id);

-- Profile currency option: code -> id. Values already matching an id stay;
-- resolvable codes map own-first then global; anything else normalizes to
-- the global USD row (baseline-seeded, undeletable). Users without the
-- option row get one -- the same invariant for every user.
UPDATE users_options SET value = COALESCE(
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id = users_options.user_id),
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id IS NULL),
    (SELECT c.id FROM currencies c WHERE c.code = 'USD' AND c.user_id IS NULL)
)
WHERE name = 'currency'
  AND NOT EXISTS (SELECT 1 FROM currencies c2 WHERE c2.id = users_options.value);
INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
SELECT lower(hex(randomblob(16))), u.id, 'currency',
       (SELECT c.id FROM currencies c WHERE c.code = 'USD' AND c.user_id IS NULL),
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM users u
WHERE NOT EXISTS (SELECT 1 FROM users_options o WHERE o.user_id = u.id AND o.name = 'currency');

PRAGMA foreign_keys = ON;
