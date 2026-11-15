-- Budget debts: a debt attached to one budget element (a top-level expense
-- category or an envelope), its snapshots, payment splits and rate changes.
-- Numbered above every migration on main. Unreleased builds created debt
-- tables in earlier shapes under five lower versions (20261110000500 ..
-- 20261114000000, files since removed), and a database may have stopped at any
-- of them, so whatever debt tables exist are dropped and created fresh, and
-- the debt budget rows (type 6) of the earliest shape go with their plans and
-- comments. Debt data from those builds is not kept.
DELETE FROM budgets_elements_limits WHERE element_id IN (SELECT id FROM budgets_elements WHERE type = 6);
DELETE FROM budgets_elements_comments WHERE element_id IN (SELECT id FROM budgets_elements WHERE type = 6);
DELETE FROM budgets_elements WHERE type = 6;
DROP TABLE IF EXISTS debts_rate_changes;
DROP TABLE IF EXISTS debts_payments;
DROP TABLE IF EXISTS debts_snapshots;
DROP TABLE IF EXISTS debts_rules;
DROP TABLE IF EXISTS debts;
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
