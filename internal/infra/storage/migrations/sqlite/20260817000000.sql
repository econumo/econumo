-- Budget membership becomes an explicit set (replacing the excluded-accounts
-- blacklist in 20260817000002). A soft-deleted member keeps counting, so
-- budgeted and spent totals are always drawn from the same accounts.
CREATE TABLE budgets_accounts
(
    budget_id  TEXT     NOT NULL
    , account_id TEXT     NOT NULL
    , created_at DATETIME NOT NULL
    , PRIMARY KEY (budget_id, account_id)
    , FOREIGN KEY (budget_id)  REFERENCES budgets (id)  ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX budgets_accounts_account_id_idx ON budgets_accounts (account_id);
