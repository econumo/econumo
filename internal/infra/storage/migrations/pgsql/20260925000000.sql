-- See the sqlite sibling for why the flag lives on the membership row.
ALTER TABLE budgets_accounts ADD COLUMN is_savings BOOLEAN NOT NULL DEFAULT false;
