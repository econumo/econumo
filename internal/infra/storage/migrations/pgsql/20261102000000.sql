-- Budget debts. A debt is one loan or credit line the owner pays down through
-- a category. The term columns as_of, balance and last_payment_date copy the
-- latest debts_snapshots row and are written together with every new snapshot;
-- all three are NULL while the debt is not set up (payment and schedule are
-- held either way).
-- The category, payee, tag and label keys keep the default NO ACTION: the coded
-- in-use check refuses an ordinary delete, and a user deletion cascades to
-- categories and debts in one statement.
CREATE TABLE debts
(
    id                  UUID          NOT NULL
    , user_id           UUID          NOT NULL
    , name              TEXT          NOT NULL
    , currency_id       UUID          NOT NULL
    , category_id       UUID          NOT NULL
    , payee_id          UUID
    , tag_id            UUID
    , label_id          UUID
    , start_date        DATE
    , as_of             DATE
    , balance           NUMERIC(19,8)
    , payment           NUMERIC(19,8) NOT NULL
    , schedule          TEXT          NOT NULL
    , last_payment_date DATE
    , created_at        TIMESTAMP(0) WITHOUT TIME ZONE      NOT NULL
    , updated_at        TIMESTAMP(0) WITHOUT TIME ZONE      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id)
    , FOREIGN KEY (category_id) REFERENCES categories (id)
    , FOREIGN KEY (payee_id) REFERENCES payees (id)
    , FOREIGN KEY (tag_id) REFERENCES tags (id)
    , FOREIGN KEY (label_id) REFERENCES labels (id)
);
CREATE INDEX debts_user_idx ON debts (user_id);
CREATE INDEX debts_category_idx ON debts (category_id);
CREATE INDEX debts_payee_idx ON debts (payee_id);
CREATE INDEX debts_tag_idx ON debts (tag_id);
CREATE INDEX debts_label_idx ON debts (label_id);
CREATE INDEX debts_currency_idx ON debts (currency_id);

CREATE TABLE debts_snapshots
(
    id                  UUID          NOT NULL
    , debt_id           UUID          NOT NULL
    , as_of             DATE          NOT NULL
    , balance           NUMERIC(19,8) NOT NULL
    , payment           NUMERIC(19,8) NOT NULL
    , schedule          TEXT          NOT NULL
    , last_payment_date DATE          NOT NULL
    , created_at        TIMESTAMP(0) WITHOUT TIME ZONE      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
);
CREATE INDEX debts_snapshots_debt_as_of_idx ON debts_snapshots (debt_id, as_of);

CREATE TABLE debts_payments
(
    debt_id           UUID          NOT NULL
    , transaction_id  UUID          NOT NULL
    , interest        NUMERIC(19,8) NOT NULL
    , PRIMARY KEY (transaction_id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
);
CREATE INDEX debts_payments_debt_idx ON debts_payments (debt_id);
