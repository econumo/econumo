DELETE FROM budgets_elements_limits
WHERE ctid NOT IN (
    SELECT MIN(ctid)
    FROM budgets_elements_limits
    GROUP BY element_id, period
);
DROP INDEX IF EXISTS element_period_idx_budgets_elements_limits;
CREATE UNIQUE INDEX element_period_uniq_budgets_elements_limits ON budgets_elements_limits (element_id, period);
