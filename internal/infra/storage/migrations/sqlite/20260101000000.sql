-- Convert every CHAR(n) column to TEXT so sqlc's sqlite engine maps them to Go
-- `string` natively (sqlc v1.30 has no CHAR(n) mapping and falls back to
-- interface{}). In SQLite this is storage-identical: TEXT and CHAR(n) share the
-- same TEXT affinity; only the declared type token changes. NUMERIC(19,8),
-- BOOLEAN, DATETIME, DATE, SMALLINT, VARCHAR, CLOB columns are left untouched.
--
-- SQLite cannot ALTER COLUMN type, so each table is rebuilt. The runner wraps
-- this whole file in ONE transaction with PRAGMA foreign_keys = ON (set at the
-- connection level in the sqlite backend), and that pragma is a no-op inside a
-- transaction, so it cannot be turned off here. To rebuild safely WITHOUT
-- cascade-deleting child rows the work is split into four ordered phases:
--
--   A. RENAME every table to <t>__old. With legacy_alter_table OFF (the modern
--      default) RENAME automatically rewrites child FOREIGN KEY targets to the
--      __old name, so no cascade fires and child data is preserved.
--   B. CREATE every table afresh with TEXT columns; FKs reference the FINAL
--      (non-__old) names, so the new tables form a self-consistent graph.
--   C. INSERT ... SELECT * copies data from each __old table.
--   D. DROP every __old table in child-before-parent order. Because the live
--      (new) tables no longer reference any __old table, and __old tables are
--      dropped before the __old tables they point to, no DELETE CASCADE fires.
--
-- Finally all explicit (named) indexes are recreated. Indexes backed by inline
-- PRIMARY KEY / UNIQUE constraints are recreated automatically with the tables
-- and are NOT listed here.

-- ---------------------------------------------------------------------------
-- Phase A: rename existing tables out of the way (rewrites child FK targets).
-- ---------------------------------------------------------------------------
ALTER TABLE users RENAME TO users__old;
ALTER TABLE currencies RENAME TO currencies__old;
ALTER TABLE accounts RENAME TO accounts__old;
ALTER TABLE accounts_access RENAME TO accounts_access__old;
ALTER TABLE accounts_options RENAME TO accounts_options__old;
ALTER TABLE users_connections RENAME TO users_connections__old;
ALTER TABLE users_connections_invites RENAME TO users_connections_invites__old;
ALTER TABLE folders RENAME TO folders__old;
ALTER TABLE accounts_folders RENAME TO accounts_folders__old;
ALTER TABLE payees RENAME TO payees__old;
ALTER TABLE tags RENAME TO tags__old;
ALTER TABLE categories RENAME TO categories__old;
ALTER TABLE transactions RENAME TO transactions__old;
ALTER TABLE users_options RENAME TO users_options__old;
ALTER TABLE users_password_requests RENAME TO users_password_requests__old;
ALTER TABLE currencies_rates RENAME TO currencies_rates__old;
ALTER TABLE operation_requests_ids RENAME TO operation_requests_ids__old;
ALTER TABLE budgets RENAME TO budgets__old;
ALTER TABLE budgets_access RENAME TO budgets_access__old;
ALTER TABLE budgets_folders RENAME TO budgets_folders__old;
ALTER TABLE budgets_elements RENAME TO budgets_elements__old;
ALTER TABLE budgets_elements_limits RENAME TO budgets_elements_limits__old;
ALTER TABLE budgets_envelopes RENAME TO budgets_envelopes__old;
ALTER TABLE budgets_envelopes_categories RENAME TO budgets_envelopes_categories__old;
ALTER TABLE budgets_excluded_accounts RENAME TO budgets_excluded_accounts__old;

-- ---------------------------------------------------------------------------
-- Phase B: create the tables anew with CHAR(n) -> TEXT; everything else kept.
-- ---------------------------------------------------------------------------
CREATE TABLE users
(
    id         TEXT       NOT NULL
    , identifier TEXT     NOT NULL
    , email      VARCHAR(255) NOT NULL
    , name       VARCHAR(255) NOT NULL
    , avatar_url VARCHAR(255) NOT NULL
    , password   VARCHAR(255) NOT NULL
    , salt       VARCHAR(40)  NOT NULL
    , created_at DATETIME     NOT NULL
    , updated_at DATETIME     NOT NULL
    , is_active BOOLEAN DEFAULT '1' NOT NULL, PRIMARY KEY (id)
    , UNIQUE (identifier)
);

CREATE TABLE currencies
(
    id         TEXT    NOT NULL
    , code       TEXT     NOT NULL
    , symbol     VARCHAR(12) NOT NULL
    , created_at DATETIME    NOT NULL
    , name VARCHAR(36) DEFAULT NULL, fraction_digits SMALLINT DEFAULT '2' NOT NULL, PRIMARY KEY (id)
    , UNIQUE (code)
);

CREATE TABLE accounts
(
    id                      TEXT            NOT NULL
    , currency_id             TEXT            NOT NULL
    , user_id                 TEXT            NOT NULL
    , name                    VARCHAR(64)         NOT NULL
    , type                    SMALLINT            NOT NULL
    , icon                    VARCHAR(64)         NOT NULL
    , is_deleted              BOOLEAN DEFAULT '0' NOT NULL
    , created_at              DATETIME            NOT NULL
    , updated_at              DATETIME            NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE accounts_access
(
    account_id TEXT NOT NULL
    , user_id    TEXT NOT NULL
    , role       SMALLINT NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , PRIMARY KEY (account_id, user_id)
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE accounts_options
(
    account_id TEXT                    NOT NULL
    , user_id    TEXT                    NOT NULL
    , position   SMALLINT UNSIGNED DEFAULT 0 NOT NULL
    , created_at DATETIME                    NOT NULL
    , updated_at DATETIME                    NOT NULL
    , PRIMARY KEY (account_id, user_id)
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE users_connections
(
    user_id           TEXT NOT NULL
    , connected_user_id TEXT NOT NULL
    , PRIMARY KEY (user_id, connected_user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (connected_user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE users_connections_invites
(
    user_id    TEXT NOT NULL
    , code       VARCHAR(255) DEFAULT NULL
    , expired_at DATETIME     DEFAULT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
);

CREATE TABLE folders
(
    id         TEXT                         NOT NULL
    , user_id    TEXT                         NOT NULL
    , name       VARCHAR(64)                      NOT NULL
    , position   SMALLINT UNSIGNED DEFAULT 0      NOT NULL
    , is_visible BOOLEAN           DEFAULT 'true' NOT NULL
    , created_at DATETIME                         NOT NULL
    , updated_at DATETIME                         NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE accounts_folders
(
    folder_id  TEXT NOT NULL
    , account_id TEXT NOT NULL
    , PRIMARY KEY (folder_id, account_id)
    , FOREIGN KEY (folder_id) REFERENCES folders (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);

CREATE TABLE payees
(
    id          TEXT                      NOT NULL
    , user_id     TEXT                      NOT NULL
    , name        VARCHAR(64)                   NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0   NOT NULL
    , is_archived BOOLEAN           DEFAULT '0' NOT NULL
    , created_at  DATETIME                      NOT NULL
    , updated_at  DATETIME                      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE tags
(
    id          TEXT                      NOT NULL
    , user_id     TEXT                      NOT NULL
    , name        VARCHAR(64)                   NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0   NOT NULL
    , is_archived BOOLEAN           DEFAULT '0' NOT NULL
    , created_at  DATETIME                      NOT NULL
    , updated_at  DATETIME                      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE categories
(
    id          TEXT                      NOT NULL
    , user_id     TEXT                      NOT NULL
    , name        VARCHAR(64)                   NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0   NOT NULL
    , type        SMALLINT                      NOT NULL
    , icon        VARCHAR(255)                  NOT NULL
    , is_archived BOOLEAN           DEFAULT '0' NOT NULL
    , created_at  DATETIME                      NOT NULL
    , updated_at  DATETIME                      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE transactions (id TEXT NOT NULL COLLATE BINARY
        , user_id TEXT NOT NULL COLLATE BINARY
        , account_id TEXT NOT NULL COLLATE BINARY
        , account_recipient_id TEXT DEFAULT NULL COLLATE BINARY
        , category_id TEXT DEFAULT NULL COLLATE BINARY
        , payee_id TEXT DEFAULT NULL COLLATE BINARY
        , tag_id TEXT DEFAULT NULL COLLATE BINARY
        , description VARCHAR(255) NOT NULL COLLATE BINARY, created_at DATETIME NOT NULL
        , updated_at DATETIME NOT NULL
        , spent_at DATETIME NOT NULL
        , type SMALLINT NOT NULL
        , amount NUMERIC(19, 8) NOT NULL
        , amount_recipient NUMERIC(19, 8) DEFAULT NULL
        , PRIMARY KEY(id)
        , CONSTRAINT FK_EAA81A4CA76ED395 FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_EAA81A4C9B6B5FBA FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_EAA81A4C70F7993E FOREIGN KEY (account_recipient_id) REFERENCES accounts (id) ON DELETE SET NULL NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_EAA81A4C12469DE2 FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_EAA81A4CCB4B68F FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_EAA81A4CBAD26311 FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL NOT DEFERRABLE INITIALLY IMMEDIATE);

CREATE TABLE users_options
(
    id         TEXT     NOT NULL
    , user_id    TEXT     NOT NULL
    , name       VARCHAR(255) NOT NULL
    , value      VARCHAR(256) DEFAULT NULL
    , created_at DATETIME     NOT NULL
    , updated_at DATETIME     NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (user_id, name)
);

CREATE TABLE users_password_requests
(
    id         TEXT NOT NULL
    , user_id    TEXT NOT NULL
    , code       TEXT NOT NULL
    , created_at DATETIME NOT NULL
    , updated_at DATETIME NOT NULL
    , expired_at DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , UNIQUE (code)
    , UNIQUE (user_id)
);

CREATE TABLE currencies_rates (id TEXT NOT NULL COLLATE BINARY
        , currency_id TEXT NOT NULL COLLATE BINARY
        , base_currency_id TEXT NOT NULL COLLATE BINARY
        , published_at DATE NOT NULL
        , rate NUMERIC(19, 8) NOT NULL
        , PRIMARY KEY(id)
        , CONSTRAINT FK_5AA604E038248176 FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_5AA604E03101778E FOREIGN KEY (base_currency_id) REFERENCES currencies (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE);

CREATE TABLE operation_requests_ids
(
    id         TEXT            NOT NULL
    , is_handled BOOLEAN DEFAULT '0' NOT NULL
    , created_at DATETIME            NOT NULL
    , updated_at DATETIME            NOT NULL
    , PRIMARY KEY (id)
);

CREATE TABLE budgets
(
    id          TEXT    NOT NULL
    , currency_id TEXT    NOT NULL
    , user_id     TEXT    NOT NULL
    , name        VARCHAR(64) NOT NULL
    , started_at  DATETIME    NOT NULL
    , created_at  DATETIME    NOT NULL
    , updated_at  DATETIME    NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE SET NULL
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE budgets_access
(
    budget_id   TEXT            NOT NULL
    , user_id     TEXT            NOT NULL
    , role        SMALLINT            NOT NULL
    , is_accepted BOOLEAN DEFAULT '0' NOT NULL
    , created_at  DATETIME            NOT NULL
    , updated_at  DATETIME            NOT NULL
    , PRIMARY KEY (budget_id, user_id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE budgets_folders
(
    id         TEXT                    NOT NULL
    , budget_id  TEXT                    NOT NULL
    , name       VARCHAR(64)                 NOT NULL
    , position   SMALLINT UNSIGNED DEFAULT 0 NOT NULL
    , created_at DATETIME                    NOT NULL
    , updated_at DATETIME                    NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
);

CREATE TABLE budgets_elements
(
    id          TEXT                    NOT NULL
    , budget_id   TEXT                    NOT NULL
    , currency_id TEXT          DEFAULT NULL
    , folder_id   TEXT          DEFAULT NULL
    , external_id TEXT                    NOT NULL
    , type        SMALLINT                    NOT NULL
    , created_at  DATETIME                    NOT NULL
    , updated_at  DATETIME                    NOT NULL
    , position    SMALLINT UNSIGNED DEFAULT 0 NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE SET NULL
    , FOREIGN KEY (folder_id) REFERENCES budgets_folders (id) ON DELETE SET NULL
    , UNIQUE (budget_id, external_id)
);

CREATE TABLE budgets_elements_limits (
    id TEXT NOT NULL COLLATE BINARY,
    element_id TEXT NOT NULL COLLATE BINARY,
    period DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    amount NUMERIC(19, 8) NOT NULL,
    PRIMARY KEY(id),
    CONSTRAINT FK_406C516F1F1F2A24 FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
);

CREATE TABLE budgets_envelopes
(
    id          TEXT                NOT NULL
    , budget_id   TEXT                NOT NULL
    , name        VARCHAR(64) DEFAULT NULL
    , icon        VARCHAR(64) DEFAULT NULL
    , is_archived BOOLEAN     DEFAULT '0' NOT NULL
    , created_at  DATETIME                NOT NULL
    , updated_at  DATETIME                NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
);

CREATE TABLE budgets_envelopes_categories
(
    budget_envelope_id TEXT NOT NULL
    , category_id        TEXT NOT NULL
    , PRIMARY KEY (budget_envelope_id, category_id)
    , FOREIGN KEY (budget_envelope_id) REFERENCES budgets_envelopes (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE CASCADE
);

CREATE TABLE budgets_excluded_accounts
(
    budget_id  TEXT NOT NULL
    , account_id TEXT NOT NULL
    , PRIMARY KEY (budget_id, account_id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- Phase C: copy data from the renamed originals into the rebuilt tables.
-- ---------------------------------------------------------------------------
INSERT INTO users SELECT * FROM users__old;
INSERT INTO currencies SELECT * FROM currencies__old;
INSERT INTO accounts SELECT * FROM accounts__old;
INSERT INTO accounts_access SELECT * FROM accounts_access__old;
INSERT INTO accounts_options SELECT * FROM accounts_options__old;
INSERT INTO users_connections SELECT * FROM users_connections__old;
INSERT INTO users_connections_invites SELECT * FROM users_connections_invites__old;
INSERT INTO folders SELECT * FROM folders__old;
INSERT INTO accounts_folders SELECT * FROM accounts_folders__old;
INSERT INTO payees SELECT * FROM payees__old;
INSERT INTO tags SELECT * FROM tags__old;
INSERT INTO categories SELECT * FROM categories__old;
INSERT INTO transactions SELECT * FROM transactions__old;
INSERT INTO users_options SELECT * FROM users_options__old;
INSERT INTO users_password_requests SELECT * FROM users_password_requests__old;
INSERT INTO currencies_rates SELECT * FROM currencies_rates__old;
INSERT INTO operation_requests_ids SELECT * FROM operation_requests_ids__old;
INSERT INTO budgets SELECT * FROM budgets__old;
INSERT INTO budgets_access SELECT * FROM budgets_access__old;
INSERT INTO budgets_folders SELECT * FROM budgets_folders__old;
INSERT INTO budgets_elements SELECT * FROM budgets_elements__old;
INSERT INTO budgets_elements_limits SELECT * FROM budgets_elements_limits__old;
INSERT INTO budgets_envelopes SELECT * FROM budgets_envelopes__old;
INSERT INTO budgets_envelopes_categories SELECT * FROM budgets_envelopes_categories__old;
INSERT INTO budgets_excluded_accounts SELECT * FROM budgets_excluded_accounts__old;

-- ---------------------------------------------------------------------------
-- Phase D: drop the renamed originals, child-before-parent so no live (new)
-- table references an __old table and no __old table is referenced when it is
-- dropped -> no DELETE CASCADE fires.
-- ---------------------------------------------------------------------------
DROP TABLE budgets_excluded_accounts__old;
DROP TABLE budgets_envelopes_categories__old;
DROP TABLE budgets_envelopes__old;
DROP TABLE budgets_elements_limits__old;
DROP TABLE budgets_elements__old;
DROP TABLE budgets_folders__old;
DROP TABLE budgets_access__old;
DROP TABLE budgets__old;
DROP TABLE operation_requests_ids__old;
DROP TABLE currencies_rates__old;
DROP TABLE users_password_requests__old;
DROP TABLE users_options__old;
DROP TABLE transactions__old;
DROP TABLE accounts_folders__old;
DROP TABLE categories__old;
DROP TABLE tags__old;
DROP TABLE payees__old;
DROP TABLE folders__old;
DROP TABLE users_connections_invites__old;
DROP TABLE users_connections__old;
DROP TABLE accounts_options__old;
DROP TABLE accounts_access__old;
DROP TABLE accounts__old;
DROP TABLE currencies__old;
DROP TABLE users__old;

-- ---------------------------------------------------------------------------
-- Recreate explicit (named) indexes. Indexes backed by inline PRIMARY KEY /
-- UNIQUE constraints are recreated with their tables above and omitted here.
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX UNIQ_1483A5E9772E836A ON users (identifier);
CREATE UNIQUE INDEX UNIQ_37C4469377153098 ON currencies (code);
CREATE INDEX IDX_CAC89EAC38248176 ON accounts (currency_id);
CREATE INDEX IDX_CAC89EACA76ED395 ON accounts (user_id);
CREATE INDEX user_id_is_deleted_idx_accounts ON accounts (user_id, is_deleted);
CREATE INDEX is_deleted_idx_accounts ON accounts (is_deleted);
CREATE INDEX IDX_98A8AF869B6B5FBA ON accounts_access (account_id);
CREATE INDEX IDX_98A8AF86A76ED395 ON accounts_access (user_id);
CREATE INDEX IDX_B87688FB9B6B5FBA ON accounts_options (account_id);
CREATE INDEX IDX_B87688FBA76ED395 ON accounts_options (user_id);
CREATE INDEX IDX_4843C0E7A76ED395 ON users_connections (user_id);
CREATE INDEX IDX_4843C0E7349E946C ON users_connections (connected_user_id);
CREATE INDEX expired_at_idx_connections_invites ON users_connections_invites (expired_at);
CREATE INDEX user_id_idx_connections_invites ON users_connections_invites (user_id);
CREATE UNIQUE INDEX code_uniq_connections_invites ON users_connections_invites (code);
CREATE INDEX IDX_FE37D30FA76ED395 ON folders (user_id);
CREATE INDEX IDX_9674A173162CB942 ON accounts_folders (folder_id);
CREATE INDEX IDX_9674A1739B6B5FBA ON accounts_folders (account_id);
CREATE INDEX IDX_971FAB26A76ED395 ON payees (user_id);
CREATE INDEX IDX_6FBC9426A76ED395 ON tags (user_id);
CREATE INDEX IDX_3AF34668A76ED395 ON categories (user_id);
CREATE INDEX tag_id_account_id_spent_at_idx_transactions ON transactions (tag_id, account_id, spent_at);
CREATE INDEX category_id_account_id_spent_at_idx_transactions ON transactions (category_id, account_id, spent_at);
CREATE INDEX account_recipient_id_spent_at_idx_transactions ON transactions (account_recipient_id, spent_at);
CREATE INDEX account_id_spent_at_idx_transactions ON transactions (account_id, spent_at);
CREATE INDEX spent_idx_transactions ON transactions (spent_at);
CREATE INDEX type_idx_transactions ON transactions (type);
CREATE INDEX IDX_EAA81A4CBAD26311 ON transactions (tag_id);
CREATE INDEX IDX_EAA81A4CCB4B68F ON transactions (payee_id);
CREATE INDEX IDX_EAA81A4C12469DE2 ON transactions (category_id);
CREATE INDEX IDX_EAA81A4C70F7993E ON transactions (account_recipient_id);
CREATE INDEX IDX_EAA81A4C9B6B5FBA ON transactions (account_id);
CREATE INDEX IDX_EAA81A4CA76ED395 ON transactions (user_id);
CREATE INDEX IDX_20358E4DA76ED395 ON users_options (user_id);
CREATE UNIQUE INDEX identifier_uniq_users_options ON users_options (user_id, name);
CREATE UNIQUE INDEX UNIQ_4DBE72F977153098 ON users_password_requests (code);
CREATE UNIQUE INDEX UNIQ_4DBE72F9A76ED395 ON users_password_requests (user_id);
CREATE UNIQUE INDEX identifier_uniq_currencies_rates ON currencies_rates (published_at, currency_id, base_currency_id);
CREATE INDEX base_currency_id_published_at_idx_currencies_rates ON currencies_rates (base_currency_id, published_at);
CREATE INDEX currency_id_published_at_idx_currencies_rates ON currencies_rates (currency_id, published_at);
CREATE INDEX published_at_idx_currencies_rates ON currencies_rates (published_at);
CREATE INDEX IDX_5AA604E03101778E ON currencies_rates (base_currency_id);
CREATE INDEX IDX_5AA604E038248176 ON currencies_rates (currency_id);
CREATE INDEX IDX_DCAA954838248176 ON budgets (currency_id);
CREATE INDEX IDX_DCAA9548A76ED395 ON budgets (user_id);
CREATE INDEX IDX_9300F12F36ABA6B8 ON budgets_access (budget_id);
CREATE INDEX IDX_9300F12FA76ED395 ON budgets_access (user_id);
CREATE INDEX IDX_3975126136ABA6B8 ON budgets_folders (budget_id);
CREATE INDEX IDX_EE8709C336ABA6B8 ON budgets_elements (budget_id);
CREATE INDEX IDX_EE8709C338248176 ON budgets_elements (currency_id);
CREATE INDEX IDX_EE8709C3162CB942 ON budgets_elements (folder_id);
CREATE INDEX external_id_idx_budgets_elements ON budgets_elements (external_id);
CREATE UNIQUE INDEX identifier_uniq_budgets_elements ON budgets_elements (budget_id, external_id);
CREATE INDEX period_idx_budgets_elements_limits ON budgets_elements_limits (period);
CREATE INDEX IDX_406C516F1F1F2A24 ON budgets_elements_limits (element_id);
CREATE UNIQUE INDEX element_period_uniq_budgets_elements_limits ON budgets_elements_limits (element_id, period);
CREATE INDEX IDX_4FF0C05436ABA6B8 ON budgets_envelopes (budget_id);
CREATE INDEX IDX_8F2B05CC310C8D48 ON budgets_envelopes_categories (budget_envelope_id);
CREATE INDEX IDX_8F2B05CC12469DE2 ON budgets_envelopes_categories (category_id);
CREATE INDEX IDX_622BD95836ABA6B8 ON budgets_excluded_accounts (budget_id);
CREATE INDEX IDX_622BD9589B6B5FBA ON budgets_excluded_accounts (account_id);
