CREATE TABLE budgets
(
    id          CHAR(36)    NOT NULL
    , currency_id CHAR(36)    NOT NULL
    , user_id     CHAR(36)    NOT NULL
    , name        VARCHAR(64) NOT NULL
    , started_at  DATETIME    NOT NULL
    , created_at  DATETIME    NOT NULL
    , updated_at  DATETIME    NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE SET NULL
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_DCAA954838248176 ON budgets (currency_id);
CREATE INDEX IDX_DCAA9548A76ED395 ON budgets (user_id);
CREATE TABLE budgets_access
(
    budget_id   CHAR(36)            NOT NULL
    , user_id     CHAR(36)            NOT NULL
    , role        SMALLINT            NOT NULL
    , is_accepted BOOLEAN DEFAULT '0' NOT NULL
    , created_at  DATETIME            NOT NULL
    , updated_at  DATETIME            NOT NULL
    , PRIMARY KEY (budget_id, user_id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_9300F12F36ABA6B8 ON budgets_access (budget_id);
CREATE INDEX IDX_9300F12FA76ED395 ON budgets_access (user_id);
CREATE TABLE budgets_folders
(
    id         CHAR(36)                    NOT NULL
    , budget_id  CHAR(36)                    NOT NULL
    , name       VARCHAR(64)                 NOT NULL
    , position   SMALLINT UNSIGNED DEFAULT 0 NOT NULL
    , created_at DATETIME                    NOT NULL
    , updated_at DATETIME                    NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
);
CREATE INDEX IDX_3975126136ABA6B8 ON budgets_folders (budget_id);
CREATE TABLE budgets_elements
(
    id          CHAR(36)                    NOT NULL
    , budget_id   CHAR(36)                    NOT NULL
    , currency_id CHAR(36)          DEFAULT NULL
    , folder_id   CHAR(36)          DEFAULT NULL
    , external_id CHAR(36)                    NOT NULL
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
CREATE INDEX IDX_EE8709C336ABA6B8 ON budgets_elements (budget_id);
CREATE INDEX IDX_EE8709C338248176 ON budgets_elements (currency_id);
CREATE INDEX IDX_EE8709C3162CB942 ON budgets_elements (folder_id);
CREATE INDEX external_id_idx_budgets_elements ON budgets_elements (external_id);
CREATE UNIQUE INDEX identifier_uniq_budgets_elements ON budgets_elements (budget_id, external_id);
CREATE TABLE budgets_elements_limits
(
    id         CHAR(36)       NOT NULL
    , element_id CHAR(36)       NOT NULL
    , period     DATETIME       NOT NULL
    , amount     NUMERIC(19, 2) NOT NULL
    , created_at DATETIME       NOT NULL
    , updated_at DATETIME       NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE
);
CREATE INDEX IDX_406C516F1F1F2A24 ON budgets_elements_limits (element_id);
CREATE INDEX period_idx_budgets_elements_limits ON budgets_elements_limits (period);
CREATE INDEX element_period_idx_budgets_elements_limits ON budgets_elements_limits (element_id, period);
CREATE TABLE budgets_envelopes
(
    id          CHAR(36)                NOT NULL
    , budget_id   CHAR(36)                NOT NULL
    , name        VARCHAR(64) DEFAULT NULL
    , icon        VARCHAR(64) DEFAULT NULL
    , is_archived BOOLEAN     DEFAULT '0' NOT NULL
    , created_at  DATETIME                NOT NULL
    , updated_at  DATETIME                NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
);
CREATE INDEX IDX_4FF0C05436ABA6B8 ON budgets_envelopes (budget_id);
CREATE TABLE budgets_envelopes_categories
(
    budget_envelope_id CHAR(36) NOT NULL
    , category_id        CHAR(36) NOT NULL
    , PRIMARY KEY (budget_envelope_id, category_id)
    , FOREIGN KEY (budget_envelope_id) REFERENCES budgets_envelopes (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE CASCADE
);
CREATE INDEX IDX_8F2B05CC310C8D48 ON budgets_envelopes_categories (budget_envelope_id);
CREATE INDEX IDX_8F2B05CC12469DE2 ON budgets_envelopes_categories (category_id);
CREATE TABLE budgets_excluded_accounts
(
    budget_id  CHAR(36) NOT NULL
    , account_id CHAR(36) NOT NULL
    , PRIMARY KEY (budget_id, account_id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX IDX_622BD95836ABA6B8 ON budgets_excluded_accounts (budget_id);
CREATE INDEX IDX_622BD9589B6B5FBA ON budgets_excluded_accounts (account_id);
