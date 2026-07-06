ALTER TABLE budgets_elements_limits ALTER COLUMN amount TYPE NUMERIC(19, 8);
ALTER TABLE currencies_rates ALTER COLUMN rate TYPE NUMERIC(19, 8);
ALTER TABLE transactions ALTER COLUMN amount TYPE NUMERIC(19, 8);
ALTER TABLE transactions ALTER COLUMN amount_recipient TYPE NUMERIC(19, 8);
