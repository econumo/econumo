-- Budget debts belong to a budget and match payments through rules. The
-- feature is unreleased: every debt of the user-owned shape is deleted, with
-- its snapshots, splits and budget rows (type 6) and their plans and comments,
-- and the tables are recreated rather than converted (no SQLite rebuilds).
-- The plans and comments go explicitly, independent of the cascades.
DELETE FROM budgets_elements_limits WHERE element_id IN (SELECT id FROM budgets_elements WHERE type = 6);
DELETE FROM budgets_elements_comments WHERE element_id IN (SELECT id FROM budgets_elements WHERE type = 6);
DELETE FROM budgets_elements WHERE type = 6;
DROP TABLE debts_payments;
DROP TABLE debts_snapshots;
DROP TABLE debts;

-- A debt is one loan or credit line a budget pays down. The term columns
-- as_of, balance and last_payment_date copy the latest debts_snapshots row and
-- are written together with every new snapshot; all three, and
-- amount_borrowed, are NULL while the debt is not set up (payment and schedule
-- are held either way). amount_borrowed is the loan's amount as of start_date,
-- the balance of the first snapshot (the origin). created_day is the creation
-- date in the creating user's timezone, fixed at creation: it can start the
-- debt's plannable months, which must not move later. icon '' means the debt
-- has none of its own and shows its first rule's category icon.
CREATE TABLE debts
(
    id                  UUID          NOT NULL
    , budget_id         UUID          NOT NULL
    , name              TEXT          NOT NULL
    , icon              TEXT          NOT NULL DEFAULT ''
    , currency_id       UUID          NOT NULL
    , start_date        DATE
    , amount_borrowed   NUMERIC(19,8)
    , as_of             DATE
    , balance           NUMERIC(19,8)
    , payment           NUMERIC(19,8) NOT NULL
    , schedule          TEXT          NOT NULL
    , last_payment_date DATE
    , created_day       DATE          NOT NULL
    , created_at        TIMESTAMP(0) WITHOUT TIME ZONE      NOT NULL
    , updated_at        TIMESTAMP(0) WITHOUT TIME ZONE      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id)
);
CREATE INDEX debts_budget_idx ON debts (budget_id);
CREATE INDEX debts_currency_idx ON debts (currency_id);

-- A payment counts for a debt when it matches any of its rules: an expense in
-- the category, narrowed by the payee, tag and label that are set. The
-- classification keys keep the default NO ACTION: a delete releases the rules
-- first, so a forgotten release fails loudly instead of leaving a dangling id.
CREATE TABLE debts_rules
(
    id            UUID      NOT NULL
    , debt_id     UUID      NOT NULL
    , category_id UUID      NOT NULL
    , payee_id    UUID
    , tag_id      UUID
    , label_id    UUID
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE  NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id)
    , FOREIGN KEY (payee_id) REFERENCES payees (id)
    , FOREIGN KEY (tag_id) REFERENCES tags (id)
    , FOREIGN KEY (label_id) REFERENCES labels (id)
);
CREATE INDEX debts_rules_debt_idx ON debts_rules (debt_id);
CREATE INDEX debts_rules_category_idx ON debts_rules (category_id);
CREATE INDEX debts_rules_payee_idx ON debts_rules (payee_id);
CREATE INDEX debts_rules_tag_idx ON debts_rules (tag_id);
CREATE INDEX debts_rules_label_idx ON debts_rules (label_id);

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

-- One row per (debt, transaction): a transaction on an account in two budgets
-- can count for a debt in each.
CREATE TABLE debts_payments
(
    debt_id           UUID          NOT NULL
    , transaction_id  UUID          NOT NULL
    , interest        NUMERIC(19,8) NOT NULL
    , PRIMARY KEY (debt_id, transaction_id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
);
CREATE INDEX debts_payments_transaction_idx ON debts_payments (transaction_id);
