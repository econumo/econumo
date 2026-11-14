-- A rate change is a lender's new yearly rate (percent, full precision) and
-- regular payment from a date on; the loan's math reads the latest one on or
-- before a day. One per (debt, date): saving on a taken date replaces it.
CREATE TABLE debts_rate_changes
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
CREATE UNIQUE INDEX debts_rate_changes_debt_date_uniq ON debts_rate_changes (debt_id, effective_date);
CREATE INDEX debts_rate_changes_debt_idx ON debts_rate_changes (debt_id);
