-- Budget debts: a debt attached to one budget element (a top-level expense
-- category or an envelope), its snapshots, payment splits and rate changes.
-- Numbered above every migration on main. Unreleased builds created these
-- tables under five earlier versions (20261110000500 .. 20261114000000, files
-- since removed); a database that ran them already holds the same tables, so
-- every statement here is IF NOT EXISTS and keeps their rows.
CREATE TABLE IF NOT EXISTS debts
(
    id                    TEXT          NOT NULL
    , budget_id           TEXT          NOT NULL
    , element_external_id TEXT          NOT NULL
    , element_type        TEXT          NOT NULL
    , status              TEXT          NOT NULL DEFAULT 'active'
    , ended_at            DATE
    , currency_id         TEXT          NOT NULL
    , start_date          DATE
    , amount_borrowed     NUMERIC(19,8)
    , as_of               DATE
    , balance             NUMERIC(19,8)
    , payment             NUMERIC(19,8) NOT NULL
    , schedule            TEXT          NOT NULL
    , last_payment_date   DATE
    , created_day         DATE          NOT NULL
    , created_at          DATETIME      NOT NULL
    , updated_at          DATETIME      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (budget_id) REFERENCES budgets (id) ON DELETE CASCADE
    , FOREIGN KEY (currency_id) REFERENCES currencies (id)
);
CREATE INDEX IF NOT EXISTS debts_budget_element_idx ON debts (budget_id, element_external_id);
CREATE INDEX IF NOT EXISTS debts_element_idx ON debts (element_external_id);
CREATE INDEX IF NOT EXISTS debts_currency_idx ON debts (currency_id);
CREATE UNIQUE INDEX IF NOT EXISTS debts_active_element_uniq ON debts (budget_id, element_external_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS debts_snapshots
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
CREATE INDEX IF NOT EXISTS debts_snapshots_debt_as_of_idx ON debts_snapshots (debt_id, as_of);

CREATE TABLE IF NOT EXISTS debts_payments
(
    debt_id           TEXT          NOT NULL
    , transaction_id  TEXT          NOT NULL
    , interest        NUMERIC(19,8) NOT NULL
    , PRIMARY KEY (debt_id, transaction_id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS debts_payments_transaction_idx ON debts_payments (transaction_id);

CREATE TABLE IF NOT EXISTS debts_rate_changes
(
    id                  TEXT          NOT NULL
    , debt_id           TEXT          NOT NULL
    , effective_date    DATE          NOT NULL
    , annual_rate       NUMERIC(19,8) NOT NULL
    , payment           NUMERIC(19,8) NOT NULL
    , created_at        DATETIME      NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (debt_id) REFERENCES debts (id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS debts_rate_changes_debt_date_uniq ON debts_rate_changes (debt_id, effective_date);
CREATE INDEX IF NOT EXISTS debts_rate_changes_debt_idx ON debts_rate_changes (debt_id);
