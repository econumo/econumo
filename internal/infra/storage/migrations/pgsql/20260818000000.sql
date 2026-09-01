-- Budget lifecycle: ended_at is the LAST month the budget covers (inclusive,
-- first-of-month, NULL = open-ended); is_archived hides the budget and makes
-- it read-only. Independent flags: archiving does not set an end date.
ALTER TABLE budgets ADD COLUMN ended_at TIMESTAMP;
ALTER TABLE budgets ADD COLUMN is_archived BOOLEAN DEFAULT '0' NOT NULL;
