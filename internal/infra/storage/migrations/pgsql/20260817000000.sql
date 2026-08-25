-- See the sqlite sibling for the semantics.
CREATE TABLE budgets_accounts
(
    budget_id  UUID      NOT NULL
    , account_id UUID      NOT NULL
    , created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (budget_id, account_id)
    , FOREIGN KEY (budget_id)  REFERENCES budgets (id)  ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
);
CREATE INDEX budgets_accounts_account_id_idx ON budgets_accounts (account_id);
