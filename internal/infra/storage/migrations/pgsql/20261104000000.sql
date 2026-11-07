-- Budget period start day: the day of the month (1-28) a budget month starts
-- on. 1 is the calendar month, so every existing budget keeps its figures.
-- Limit/comment periods, started_at and ended_at stay first-of-month keys.
ALTER TABLE budgets ADD COLUMN period_start_day SMALLINT NOT NULL DEFAULT 1;
