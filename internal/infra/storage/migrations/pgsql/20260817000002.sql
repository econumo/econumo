-- See the sqlite sibling for the semantics.
INSERT INTO budgets_accounts (budget_id, account_id, created_at)
SELECT DISTINCT b.id, a.id, CURRENT_TIMESTAMP
FROM budgets b
JOIN accounts a
  ON a.user_id = b.user_id
  OR a.user_id IN (SELECT ba.user_id FROM budgets_access ba WHERE ba.budget_id = b.id AND ba.is_accepted = TRUE AND ba.role <> 2)
WHERE (a.is_deleted = FALSE OR (a.updated_at >= b.started_at AND a.updated_at > b.created_at))
  AND NOT EXISTS (SELECT 1 FROM budgets_excluded_accounts e WHERE e.budget_id = b.id AND e.account_id = a.id);

DROP TABLE budgets_excluded_accounts;
