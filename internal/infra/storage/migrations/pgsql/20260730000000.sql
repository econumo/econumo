-- Per-user currencies (PostgreSQL variant): in-place ALTERs. The inline
-- UNIQUE (code) constraint is auto-named currencies_code_key; the baseline
-- also created a redundant UNIQ_37C4469377153098 index on code. Both go.
ALTER TABLE currencies ADD COLUMN user_id UUID DEFAULT NULL;
ALTER TABLE currencies ADD COLUMN is_archived BOOLEAN DEFAULT FALSE NOT NULL;
ALTER TABLE currencies ADD CONSTRAINT fk_currencies_user_id FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE currencies ALTER COLUMN name TYPE VARCHAR(64);
ALTER TABLE currencies DROP CONSTRAINT currencies_code_key;
DROP INDEX UNIQ_37C4469377153098;
CREATE UNIQUE INDEX UNIQ_currencies_code_global ON currencies (code) WHERE user_id IS NULL;
CREATE UNIQUE INDEX UNIQ_currencies_user_code ON currencies (user_id, code) WHERE user_id IS NOT NULL;
CREATE INDEX IDX_currencies_user_id ON currencies (user_id);

CREATE TABLE users_hidden_currencies
(
    user_id     UUID NOT NULL
    , currency_id UUID NOT NULL
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (user_id, currency_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id) ON DELETE CASCADE
);
CREATE INDEX IDX_users_hidden_currencies_currency_id ON users_hidden_currencies (currency_id);
