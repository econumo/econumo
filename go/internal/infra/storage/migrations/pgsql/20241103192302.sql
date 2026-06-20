CREATE TABLE budgets
(
    id          UUID    NOT NULL
    , currency_id UUID    NOT NULL
    , user_id     UUID    NOT NULL
    , name        VARCHAR(64) NOT NULL
    , started_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , CONSTRAINT FK_DCAA954838248176 FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE SET NULL NOT DEFERRABLE
    , CONSTRAINT FK_DCAA9548A76ED395 FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_DCAA954838248176 ON budgets (currency_id);
CREATE INDEX IDX_DCAA9548A76ED395 ON budgets (user_id);
CREATE TABLE budgets_access
(
    budget_id   UUID            NOT NULL
    , user_id     UUID            NOT NULL
    , role        SMALLINT            NOT NULL
    , is_accepted BOOLEAN DEFAULT '0' NOT NULL
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (budget_id, user_id)
    , CONSTRAINT FK_9300F12F36ABA6B8 FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE NOT DEFERRABLE
    , CONSTRAINT FK_9300F12FA76ED395 FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_9300F12F36ABA6B8 ON budgets_access (budget_id);
CREATE INDEX IDX_9300F12FA76ED395 ON budgets_access (user_id);
CREATE TABLE budgets_folders
(
    id         UUID                    NOT NULL
    , budget_id  UUID                    NOT NULL
    , name       VARCHAR(64)                 NOT NULL
    , position   SMALLINT DEFAULT 0 NOT NULL CHECK (position >= 0)
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , CONSTRAINT FK_3975126136ABA6B8 FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_3975126136ABA6B8 ON budgets_folders (budget_id);
CREATE TABLE budgets_elements
(
    id          UUID                    NOT NULL
    , budget_id   UUID                    NOT NULL
    , currency_id UUID          DEFAULT NULL
    , folder_id   UUID          DEFAULT NULL
    , external_id UUID                    NOT NULL
    , type        SMALLINT                    NOT NULL
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , position    SMALLINT DEFAULT 0 NOT NULL
    , PRIMARY KEY (id)
    , CONSTRAINT FK_EE8709C336ABA6B8 FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE NOT DEFERRABLE
    , CONSTRAINT FK_EE8709C338248176 FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE SET NULL NOT DEFERRABLE
    , CONSTRAINT FK_EE8709C3162CB942 FOREIGN KEY (folder_id) REFERENCES budgets_folders (id) ON DELETE SET NULL NOT DEFERRABLE
    , UNIQUE (budget_id, external_id)
);
CREATE INDEX IDX_EE8709C336ABA6B8 ON budgets_elements (budget_id);
CREATE INDEX IDX_EE8709C338248176 ON budgets_elements (currency_id);
CREATE INDEX IDX_EE8709C3162CB942 ON budgets_elements (folder_id);
CREATE INDEX external_id_idx_budgets_elements ON budgets_elements (external_id);
CREATE UNIQUE INDEX identifier_uniq_budgets_elements ON budgets_elements (budget_id, external_id);
CREATE TABLE budgets_elements_limits
(
    id         UUID       NOT NULL
    , element_id UUID       NOT NULL
    , period     TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , amount     NUMERIC(19, 2) NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , CONSTRAINT FK_406C516F1F1F2A24 FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_406C516F1F1F2A24 ON budgets_elements_limits (element_id);
CREATE INDEX period_idx_budgets_elements_limits ON budgets_elements_limits (period);
CREATE INDEX element_period_idx_budgets_elements_limits ON budgets_elements_limits (element_id, period);
CREATE TABLE budgets_envelopes
(
    id          UUID                NOT NULL
    , budget_id   UUID                NOT NULL
    , name        VARCHAR(64) DEFAULT NULL
    , icon        VARCHAR(64) DEFAULT NULL
    , is_archived BOOLEAN     DEFAULT false NOT NULL
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , CONSTRAINT FK_4FF0C05436ABA6B8 FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_4FF0C05436ABA6B8 ON budgets_envelopes (budget_id);
CREATE TABLE budgets_envelopes_categories
(
    budget_envelope_id UUID NOT NULL
    , category_id        UUID NOT NULL
    , PRIMARY KEY (budget_envelope_id, category_id)
    , CONSTRAINT FK_8F2B05CC310C8D48 FOREIGN KEY (budget_envelope_id) REFERENCES budgets_envelopes (id) ON DELETE CASCADE NOT DEFERRABLE
    , CONSTRAINT FK_8F2B05CC12469DE2 FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_8F2B05CC310C8D48 ON budgets_envelopes_categories (budget_envelope_id);
CREATE INDEX IDX_8F2B05CC12469DE2 ON budgets_envelopes_categories (category_id);
CREATE TABLE budgets_excluded_accounts
(
    budget_id  UUID NOT NULL
    , account_id UUID NOT NULL
    , PRIMARY KEY (budget_id, account_id)
    , CONSTRAINT FK_622BD95836ABA6B8 FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE NOT DEFERRABLE
    , CONSTRAINT FK_622BD9589B6B5FBA FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE NOT DEFERRABLE
);
CREATE INDEX IDX_622BD95836ABA6B8 ON budgets_excluded_accounts (budget_id);
CREATE INDEX IDX_622BD9589B6B5FBA ON budgets_excluded_accounts (account_id);
