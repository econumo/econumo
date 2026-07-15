-- Per-user currencies: currencies gains user_id (NULL = global) and
-- is_archived; UNIQUE(code) is replaced by two partial unique indexes
-- (global codes instance-unique, custom codes unique per owner); new table
-- users_hidden_currencies stores which global currencies a user hid.
--
-- SQLite cannot drop a table-level UNIQUE, so currencies is rebuilt. Under
-- PRAGMA foreign_keys = ON, renaming currencies rewrites the FK clauses of
-- every table that references it, so the ENTIRE FK closure of currencies is
-- rebuilt in one pass (the same pattern as 20260101000000.sql). Rename all,
-- recreate all, copy parents before children, drop old children before old
-- parents.

ALTER TABLE currencies RENAME TO currencies__old;
ALTER TABLE accounts RENAME TO accounts__old;
ALTER TABLE accounts_access RENAME TO accounts_access__old;
ALTER TABLE accounts_folders RENAME TO accounts_folders__old;
ALTER TABLE accounts_options RENAME TO accounts_options__old;
ALTER TABLE transactions RENAME TO transactions__old;
ALTER TABLE currencies_rates RENAME TO currencies_rates__old;
ALTER TABLE budgets RENAME TO budgets__old;
ALTER TABLE budgets_access RENAME TO budgets_access__old;
ALTER TABLE budgets_folders RENAME TO budgets_folders__old;
ALTER TABLE budgets_elements RENAME TO budgets_elements__old;
ALTER TABLE budgets_elements_limits RENAME TO budgets_elements_limits__old;
ALTER TABLE budgets_envelopes RENAME TO budgets_envelopes__old;
ALTER TABLE budgets_envelopes_categories RENAME TO budgets_envelopes_categories__old;
ALTER TABLE budgets_excluded_accounts RENAME TO budgets_excluded_accounts__old;

CREATE TABLE currencies
(
    id         TEXT    NOT NULL
    , code       TEXT     NOT NULL
    , symbol     VARCHAR(12) NOT NULL
    , created_at DATETIME    NOT NULL
    , name VARCHAR(36) DEFAULT NULL
    , fraction_digits SMALLINT DEFAULT '2' NOT NULL
    , user_id TEXT DEFAULT NULL
    , is_archived BOOLEAN DEFAULT '0' NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
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

CREATE TABLE accounts_folders
(
    folder_id  TEXT NOT NULL
    , account_id TEXT NOT NULL
    , PRIMARY KEY (folder_id, account_id)
    , FOREIGN KEY (folder_id) REFERENCES folders (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
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

CREATE TABLE currencies_rates (id TEXT NOT NULL COLLATE BINARY
        , currency_id TEXT NOT NULL COLLATE BINARY
        , base_currency_id TEXT NOT NULL COLLATE BINARY
        , published_at DATE NOT NULL
        , rate NUMERIC(19, 8) NOT NULL
        , PRIMARY KEY(id)
        , CONSTRAINT FK_5AA604E038248176 FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_5AA604E03101778E FOREIGN KEY (base_currency_id) REFERENCES currencies (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE);

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

INSERT INTO currencies (id, code, symbol, created_at, name, fraction_digits)
SELECT id, code, symbol, created_at, name, fraction_digits FROM currencies__old;
INSERT INTO accounts SELECT * FROM accounts__old;
INSERT INTO currencies_rates SELECT * FROM currencies_rates__old;
INSERT INTO accounts_access SELECT * FROM accounts_access__old;
INSERT INTO accounts_folders SELECT * FROM accounts_folders__old;
INSERT INTO accounts_options SELECT * FROM accounts_options__old;
INSERT INTO transactions SELECT * FROM transactions__old;
INSERT INTO budgets SELECT * FROM budgets__old;
INSERT INTO budgets_access SELECT * FROM budgets_access__old;
INSERT INTO budgets_folders SELECT * FROM budgets_folders__old;
INSERT INTO budgets_elements SELECT * FROM budgets_elements__old;
INSERT INTO budgets_elements_limits SELECT * FROM budgets_elements_limits__old;
INSERT INTO budgets_envelopes SELECT * FROM budgets_envelopes__old;
INSERT INTO budgets_envelopes_categories SELECT * FROM budgets_envelopes_categories__old;
INSERT INTO budgets_excluded_accounts SELECT * FROM budgets_excluded_accounts__old;

DROP TABLE budgets_envelopes_categories__old;
DROP TABLE budgets_elements_limits__old;
DROP TABLE budgets_excluded_accounts__old;
DROP TABLE budgets_elements__old;
DROP TABLE budgets_envelopes__old;
DROP TABLE budgets_folders__old;
DROP TABLE budgets_access__old;
DROP TABLE transactions__old;
DROP TABLE accounts_access__old;
DROP TABLE accounts_folders__old;
DROP TABLE accounts_options__old;
DROP TABLE currencies_rates__old;
DROP TABLE accounts__old;
DROP TABLE budgets__old;
DROP TABLE currencies__old;

-- Recreate every explicit index of the rebuilt tables, verbatim from
-- 20260101000000.sql, EXCEPT UNIQ_37C4469377153098 (the old code-unique
-- index, replaced by the two partial unique indexes below).
CREATE INDEX IDX_CAC89EAC38248176 ON accounts (currency_id);
CREATE INDEX IDX_CAC89EACA76ED395 ON accounts (user_id);
CREATE INDEX user_id_is_deleted_idx_accounts ON accounts (user_id, is_deleted);
CREATE INDEX is_deleted_idx_accounts ON accounts (is_deleted);
CREATE INDEX IDX_98A8AF869B6B5FBA ON accounts_access (account_id);
CREATE INDEX IDX_98A8AF86A76ED395 ON accounts_access (user_id);
CREATE INDEX IDX_B87688FB9B6B5FBA ON accounts_options (account_id);
CREATE INDEX IDX_B87688FBA76ED395 ON accounts_options (user_id);
CREATE INDEX IDX_9674A173162CB942 ON accounts_folders (folder_id);
CREATE INDEX IDX_9674A1739B6B5FBA ON accounts_folders (account_id);
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
