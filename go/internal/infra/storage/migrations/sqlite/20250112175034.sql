DROP INDEX element_period_idx_budgets_elements_limits;
DROP INDEX period_idx_budgets_elements_limits;
DROP INDEX IDX_406C516F1F1F2A24;
CREATE TEMPORARY TABLE __temp__budgets_elements_limits AS SELECT id, element_id, period, amount, created_at, updated_at FROM budgets_elements_limits;
DROP TABLE budgets_elements_limits;
CREATE TABLE budgets_elements_limits (id CHAR(36) NOT NULL COLLATE BINARY
        , element_id CHAR(36) NOT NULL COLLATE BINARY
        , period DATETIME NOT NULL
        , created_at DATETIME NOT NULL
        , updated_at DATETIME NOT NULL
        , amount NUMERIC(19, 8) NOT NULL
        , PRIMARY KEY(id)
        , CONSTRAINT FK_406C516F1F1F2A24 FOREIGN KEY (element_id) REFERENCES budgets_elements (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE);
INSERT INTO budgets_elements_limits (id, element_id, period, amount, created_at, updated_at) SELECT id, element_id, period, amount, created_at, updated_at FROM __temp__budgets_elements_limits;
DROP TABLE __temp__budgets_elements_limits;
CREATE INDEX element_period_idx_budgets_elements_limits ON budgets_elements_limits (element_id, period);
CREATE INDEX period_idx_budgets_elements_limits ON budgets_elements_limits (period);
CREATE INDEX IDX_406C516F1F1F2A24 ON budgets_elements_limits (element_id);
DROP INDEX identifier_uniq_currencies_rates;
DROP INDEX base_currency_id_published_at_idx_currencies_rates;
DROP INDEX currency_id_published_at_idx_currencies_rates;
DROP INDEX published_at_idx_currencies_rates;
DROP INDEX IDX_5AA604E03101778E;
DROP INDEX IDX_5AA604E038248176;
CREATE TEMPORARY TABLE __temp__currencies_rates AS SELECT id, currency_id, base_currency_id, rate, published_at FROM currencies_rates;
DROP TABLE currencies_rates;
CREATE TABLE currencies_rates (id CHAR(36) NOT NULL COLLATE BINARY
        , currency_id CHAR(36) NOT NULL COLLATE BINARY
        , base_currency_id CHAR(36) NOT NULL COLLATE BINARY
        , published_at DATE NOT NULL
        , rate NUMERIC(19, 8) NOT NULL
        , PRIMARY KEY(id)
        , CONSTRAINT FK_5AA604E038248176 FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE
        , CONSTRAINT FK_5AA604E03101778E FOREIGN KEY (base_currency_id) REFERENCES currencies (id) ON DELETE CASCADE NOT DEFERRABLE INITIALLY IMMEDIATE);
INSERT INTO currencies_rates (id, currency_id, base_currency_id, rate, published_at) SELECT id, currency_id, base_currency_id, rate, published_at FROM __temp__currencies_rates;
DROP TABLE __temp__currencies_rates;
CREATE UNIQUE INDEX identifier_uniq_currencies_rates ON currencies_rates (published_at, currency_id, base_currency_id);
CREATE INDEX base_currency_id_published_at_idx_currencies_rates ON currencies_rates (base_currency_id, published_at);
CREATE INDEX currency_id_published_at_idx_currencies_rates ON currencies_rates (currency_id, published_at);
CREATE INDEX published_at_idx_currencies_rates ON currencies_rates (published_at);
CREATE INDEX IDX_5AA604E03101778E ON currencies_rates (base_currency_id);
CREATE INDEX IDX_5AA604E038248176 ON currencies_rates (currency_id);
DROP INDEX tag_id_account_id_spent_at_idx_transactions;
DROP INDEX category_id_account_id_spent_at_idx_transactions;
DROP INDEX account_recipient_id_spent_at_idx_transactions;
DROP INDEX account_id_spent_at_idx_transactions;
DROP INDEX spent_idx_transactions;
DROP INDEX type_idx_transactions;
DROP INDEX IDX_EAA81A4CBAD26311;
DROP INDEX IDX_EAA81A4CCB4B68F;
DROP INDEX IDX_EAA81A4C12469DE2;
DROP INDEX IDX_EAA81A4C70F7993E;
DROP INDEX IDX_EAA81A4C9B6B5FBA;
DROP INDEX IDX_EAA81A4CA76ED395;
CREATE TEMPORARY TABLE __temp__transactions AS SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, type, amount, amount_recipient, description, created_at, updated_at, spent_at FROM transactions;
DROP TABLE transactions;
CREATE TABLE transactions (id CHAR(36) NOT NULL COLLATE BINARY
        , user_id CHAR(36) NOT NULL COLLATE BINARY
        , account_id CHAR(36) NOT NULL COLLATE BINARY
        , account_recipient_id CHAR(36) DEFAULT NULL COLLATE BINARY
        , category_id CHAR(36) DEFAULT NULL COLLATE BINARY
        , payee_id CHAR(36) DEFAULT NULL COLLATE BINARY
        , tag_id CHAR(36) DEFAULT NULL COLLATE BINARY
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
INSERT INTO transactions (id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, type, amount, amount_recipient, description, created_at, updated_at, spent_at) SELECT id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id, type, amount, amount_recipient, description, created_at, updated_at, spent_at FROM __temp__transactions;
DROP TABLE __temp__transactions;
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
