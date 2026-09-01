-- Seed explicit membership from the current implicit population: every account
-- owned by the owner or an accepted non-guest participant, minus the
-- blacklist. A deleted account is seeded only if it was deleted after the
-- budget both started and existed — its history was part of the budget.
INSERT INTO budgets_accounts (budget_id, account_id, created_at)
SELECT DISTINCT b.id, a.id, CURRENT_TIMESTAMP
FROM budgets b
JOIN accounts a
  ON a.user_id = b.user_id
  OR a.user_id IN (SELECT ba.user_id FROM budgets_access ba WHERE ba.budget_id = b.id AND ba.is_accepted = 1 AND ba.role <> 2)
WHERE (a.is_deleted = 0 OR (a.updated_at >= b.started_at AND a.updated_at > b.created_at))
  AND NOT EXISTS (SELECT 1 FROM budgets_excluded_accounts e WHERE e.budget_id = b.id AND e.account_id = a.id);

DROP TABLE budgets_excluded_accounts;
