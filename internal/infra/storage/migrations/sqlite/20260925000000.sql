-- Savings is a role an account plays in one budget, not a property of the
-- account: the same account can be savings in one budget and everyday in
-- another, so the flag lives on the membership row. Existing members read as
-- everyday. ADD COLUMN with a constant default needs no table rebuild.
ALTER TABLE budgets_accounts ADD COLUMN is_savings BOOLEAN DEFAULT '0' NOT NULL;
