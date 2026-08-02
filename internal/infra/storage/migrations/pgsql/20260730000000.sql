-- Per-user custom currencies, in one step to the final shape (PostgreSQL
-- variant: in-place ALTERs, no rebuild needed). See the sqlite sibling for
-- the semantics; the option-value comparisons cast currencies.id (UUID) to
-- text because option values are VARCHAR.
ALTER TABLE currencies ADD COLUMN user_id UUID DEFAULT NULL;
ALTER TABLE currencies ADD COLUMN rate NUMERIC(19, 8) DEFAULT NULL;
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

UPDATE users_options SET value = COALESCE(
    (SELECT c.id::text FROM currencies c WHERE c.code = users_options.value AND c.user_id = users_options.user_id),
    (SELECT c.id::text FROM currencies c WHERE c.code = users_options.value AND c.user_id IS NULL),
    (SELECT c.id::text FROM currencies c WHERE c.code = 'USD' AND c.user_id IS NULL)
)
WHERE name = 'currency'
  AND NOT EXISTS (SELECT 1 FROM currencies c2 WHERE c2.id::text = users_options.value);
INSERT INTO users_options (id, user_id, name, value, created_at, updated_at)
SELECT gen_random_uuid(), u.id, 'currency',
       (SELECT c.id::text FROM currencies c WHERE c.code = 'USD' AND c.user_id IS NULL),
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM users u
WHERE NOT EXISTS (SELECT 1 FROM users_options o WHERE o.user_id = u.id AND o.name = 'currency');
