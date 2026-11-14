-- A debt is attached to one budget element (a top-level expense category or
-- an envelope) instead of matching payments through rules and planning on a
-- budget row of its own. The feature is unreleased: every debt is deleted,
-- with its snapshots, splits and rate changes, and so is every debt budget row
-- (type 6) with its plans and comments; the tables are recreated rather than
-- converted (no SQLite rebuilds).
DELETE FROM budgets_elements_limits WHERE element_id IN (SELECT id FROM budgets_elements WHERE type = 6);
DELETE FROM budgets_elements_comments WHERE element_id IN (SELECT id FROM budgets_elements WHERE type = 6);
DELETE FROM budgets_elements WHERE type = 6;
DROP TABLE debts_rate_changes;
DROP TABLE debts_payments;
DROP TABLE debts_snapshots;
DROP TABLE debts_rules;
DROP TABLE debts;

-- element_external_id is the attached element's external id (the category or
-- envelope id, as budgets_elements.external_id), element_type 'category' or
-- 'envelope'; no foreign key, since it names either. status is 'active' or
-- 'paid_off'; ended_at, the last day payments count, is set while paid off.
-- An element carries at most one active debt. currency_id is the element row's
-- currency, copied at creation. The term columns as_of, balance and
-- last_payment_date copy the latest debts_snapshots row; all three, and
-- amount_borrowed, are NULL while the debt is not set up. created_day is the
-- creation date in the creating user's timezone.
CREATE TABLE debts
(
    id                    UUID          NOT NULL
    , budget_id           UUID          NOT NULL
    , element_external_id UUID          NOT NULL
    , element_type        TEXT          NOT NULL
    , status              TEXT          NOT NULL DEFAULT 'active'
    , ended_at            DATE
    , currency_id         UUID          NOT NULL
    , start_date          DATE
    , amount_borrowed     NUMERIC(19,8)
    , as_of               DATE
    , balance             NUMERIC(19,8)
    , payment             NUMERIC(19,8) NOT NULL
    , schedule            TEXT          NOT NULL
    , last_payment_date   DATE
    , created_day         DATE          NOT NULL
    , created_at          TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at          TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id)
);
CREATE INDEX debts_budget_element_idx ON debts (budget_id, element_external_id);
CREATE INDEX debts_element_idx ON debts (element_external_id);
CREATE INDEX debts_currency_idx ON debts (currency_id);
CREATE UNIQUE INDEX debts_active_element_uniq ON debts (budget_id, element_external_id) WHERE status = 'active';

CREATE TABLE debts_snapshots
(
    id                  UUID          NOT NULL
    , debt_id           UUID          NOT NULL
    , as_of             DATE          NOT NULL
    , balance           NUMERIC(19,8) NOT NULL
    , payment           NUMERIC(19,8) NOT NULL
    , schedule          TEXT          NOT NULL
    , last_payment_date DATE          NOT NULL
    , created_at        TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
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

-- A rate change is a lender's new yearly rate (percent, full precision) and
-- regular payment from a date on. One per (debt, date).
CREATE TABLE debts_rate_changes
(
    id                  UUID          NOT NULL
    , debt_id           UUID          NOT NULL
    , effective_date    DATE          NOT NULL
    , annual_rate       NUMERIC(19,8) NOT NULL
    , payment           NUMERIC(19,8) NOT NULL
    , created_at        TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX debts_rate_changes_debt_date_uniq ON debts_rate_changes (debt_id, effective_date);
CREATE INDEX debts_rate_changes_debt_idx ON debts_rate_changes (debt_id);
