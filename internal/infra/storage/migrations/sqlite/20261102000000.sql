-- Budget debts. A debt is one loan or credit line the owner pays down through
-- a category. The five term columns (as_of .. last_payment_date) copy the
-- latest debts_snapshots row and are written together with every new snapshot.
-- The category, payee and tag keys keep the default NO ACTION: the coded
-- in-use check refuses an ordinary delete, and a user deletion cascades to
-- categories and debts in one statement.
CREATE TABLE debts
(
    id                  TEXT          NOT NULL
    , user_id           TEXT          NOT NULL
    , name              TEXT          NOT NULL
    , currency_id       TEXT          NOT NULL
    , category_id       TEXT          NOT NULL
    , payee_id          TEXT
    , tag_id            TEXT
    , start_date        DATE
    , as_of             DATE          NOT NULL
    , balance           NUMERIC(19,8) NOT NULL
    , payment           NUMERIC(19,8) NOT NULL
    , schedule          TEXT          NOT NULL
    , last_payment_date DATE          NOT NULL
    , created_at        DATETIME      NOT NULL
    , updated_at        DATETIME      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id)
    , FOREIGN KEY (category_id) REFERENCES categories (id)
    , FOREIGN KEY (payee_id) REFERENCES payees (id)
    , FOREIGN KEY (tag_id) REFERENCES tags (id)
);
CREATE INDEX debts_user_idx ON debts (user_id);
CREATE INDEX debts_category_idx ON debts (category_id);
CREATE INDEX debts_payee_idx ON debts (payee_id);
CREATE INDEX debts_tag_idx ON debts (tag_id);

CREATE TABLE debts_snapshots
(
    id                  TEXT          NOT NULL
    , debt_id           TEXT          NOT NULL
    , as_of             DATE          NOT NULL
    , balance           NUMERIC(19,8) NOT NULL
    , payment           NUMERIC(19,8) NOT NULL
    , schedule          TEXT          NOT NULL
    , last_payment_date DATE          NOT NULL
    , created_at        DATETIME      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
);
CREATE INDEX debts_snapshots_debt_as_of_idx ON debts_snapshots (debt_id, as_of);

CREATE TABLE debts_payments
(
    debt_id           TEXT          NOT NULL
    , transaction_id  TEXT          NOT NULL
    , interest        NUMERIC(19,8) NOT NULL
    , PRIMARY KEY (transaction_id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
);
CREATE INDEX debts_payments_debt_idx ON debts_payments (debt_id);
