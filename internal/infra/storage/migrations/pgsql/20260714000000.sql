CREATE TABLE recurring_transactions
(
    id                   UUID           NOT NULL
    , user_id              UUID           NOT NULL
    , account_id           UUID           NOT NULL
    , account_recipient_id UUID           DEFAULT NULL
    , category_id          UUID           DEFAULT NULL
    , payee_id             UUID           DEFAULT NULL
    , tag_id               UUID           DEFAULT NULL
    , type                 SMALLINT       NOT NULL
    , amount               NUMERIC(19, 8) NOT NULL
    , description          VARCHAR(255)   NOT NULL
    , schedule             VARCHAR(16)    NOT NULL
    , next_payment_at      TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , scheduled_day        SMALLINT       NOT NULL
    , created_at           TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at           TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (account_recipient_id) REFERENCES accounts (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL
    , FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL
    , FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL
);
CREATE INDEX account_id_idx_recurring_transactions ON recurring_transactions (account_id);
CREATE INDEX user_id_idx_recurring_transactions ON recurring_transactions (user_id);
