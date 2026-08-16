-- `period` is compared with datetime() everywhere because legacy rows hold
-- RFC3339 ("2026-08-01T00:00:00Z") while current writes are 'Y-m-d H:i:s'.
-- The unique index on (element_id, period) is TEXTUAL, so the two forms are
-- distinct keys: one month can hold both rows, and ListBudgetLimitsForPeriod
-- returns both. Collapse them, then canonicalize.
--
-- Dedupe BEFORE normalizing: rewriting first would collide on that index.
-- MAX(id) keeps the most recent (ids are UUIDv7, hence time-ordered), matching
-- the precedent in 20251214035500.sql. Amounts are NOT summed -- a duplicate
-- pair is one limit recorded twice, not two limits, so summing would inflate
-- the user's budget.
DELETE FROM budgets_elements_limits
WHERE id NOT IN (
    SELECT MAX(id)
    FROM budgets_elements_limits
    GROUP BY element_id, datetime(period)
);

UPDATE budgets_elements_limits
SET period = datetime(period)
WHERE period <> datetime(period);
