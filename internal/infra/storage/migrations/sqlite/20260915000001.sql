-- Before 20260915, the SQLite driver stored every bound time.Time as
-- time.Time.String(): "2026-09-14 10:00:00.123456789 +0000 UTC". SQLite's date
-- functions read that as NULL, and against legacy 'Y-m-d H:i:s' rows and bounds
-- it sorts by accident: a row at exactly a '<=' bound compared greater, so a
-- first-of-month midnight transaction was left out of a budget's start
-- balance. The driver now writes 'Y-m-d H:i:s' (sqlite.WithFrozenTimeFormat);
-- this rewrites the rows written before that to the same UTC form.
--
-- Only values with a numeric zone offset (the time.Time.String() shape) are
-- touched: datetime() applies the offset, so a non-UTC value lands on its UTC
-- wall-clock, and sub-seconds are dropped, matching PostgreSQL's TIMESTAMP(0).
-- Legacy rows and NULLs do not match the GLOB. schema_migrations is left alone:
-- its first row anchors the instance id. currencies_rates.published_at (a DATE)
-- was rewritten by 20260915000000. No unique index includes a rewritten column,
-- so the truncation cannot collide.

UPDATE access_tokens SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE access_tokens SET expires_at = coalesce(datetime(substr(expires_at, 1, 19) || substr(substr(expires_at, 20), instr(substr(expires_at, 20), ' ') + 1, 3) || ':' || substr(substr(expires_at, 20), instr(substr(expires_at, 20), ' ') + 4, 2)), expires_at)
WHERE expires_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE access_tokens SET last_used_at = coalesce(datetime(substr(last_used_at, 1, 19) || substr(substr(last_used_at, 20), instr(substr(last_used_at, 20), ' ') + 1, 3) || ':' || substr(substr(last_used_at, 20), instr(substr(last_used_at, 20), ' ') + 4, 2)), last_used_at)
WHERE last_used_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE access_tokens SET revoked_at = coalesce(datetime(substr(revoked_at, 1, 19) || substr(substr(revoked_at, 20), instr(substr(revoked_at, 20), ' ') + 1, 3) || ':' || substr(substr(revoked_at, 20), instr(substr(revoked_at, 20), ' ') + 4, 2)), revoked_at)
WHERE revoked_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE accounts SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE accounts SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE accounts_access SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE accounts_access SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE accounts_options SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE accounts_options SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets SET ended_at = coalesce(datetime(substr(ended_at, 1, 19) || substr(substr(ended_at, 20), instr(substr(ended_at, 20), ' ') + 1, 3) || ':' || substr(substr(ended_at, 20), instr(substr(ended_at, 20), ' ') + 4, 2)), ended_at)
WHERE ended_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets SET started_at = coalesce(datetime(substr(started_at, 1, 19) || substr(substr(started_at, 20), instr(substr(started_at, 20), ' ') + 1, 3) || ':' || substr(substr(started_at, 20), instr(substr(started_at, 20), ' ') + 4, 2)), started_at)
WHERE started_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_access SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_access SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_accounts SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_elements SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_elements SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_elements_limits SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_elements_limits SET period = coalesce(datetime(substr(period, 1, 19) || substr(substr(period, 20), instr(substr(period, 20), ' ') + 1, 3) || ':' || substr(substr(period, 20), instr(substr(period, 20), ' ') + 4, 2)), period)
WHERE period GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_elements_limits SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_envelopes SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_envelopes SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_folders SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE budgets_folders SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE categories SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE categories SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE currencies SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE folders SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE folders SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE labels SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE labels SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE messenger_messages SET available_at = coalesce(datetime(substr(available_at, 1, 19) || substr(substr(available_at, 20), instr(substr(available_at, 20), ' ') + 1, 3) || ':' || substr(substr(available_at, 20), instr(substr(available_at, 20), ' ') + 4, 2)), available_at)
WHERE available_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE messenger_messages SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE messenger_messages SET delivered_at = coalesce(datetime(substr(delivered_at, 1, 19) || substr(substr(delivered_at, 20), instr(substr(delivered_at, 20), ' ') + 1, 3) || ':' || substr(substr(delivered_at, 20), instr(substr(delivered_at, 20), ' ') + 4, 2)), delivered_at)
WHERE delivered_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE operation_requests_ids SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE operation_requests_ids SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE payees SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE payees SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE recurring_transactions SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE recurring_transactions SET next_payment_at = coalesce(datetime(substr(next_payment_at, 1, 19) || substr(substr(next_payment_at, 20), instr(substr(next_payment_at, 20), ' ') + 1, 3) || ':' || substr(substr(next_payment_at, 20), instr(substr(next_payment_at, 20), ' ') + 4, 2)), next_payment_at)
WHERE next_payment_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE recurring_transactions SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE tags SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE tags SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE transactions SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE transactions SET spent_at = coalesce(datetime(substr(spent_at, 1, 19) || substr(substr(spent_at, 20), instr(substr(spent_at, 20), ' ') + 1, 3) || ':' || substr(substr(spent_at, 20), instr(substr(spent_at, 20), ' ') + 4, 2)), spent_at)
WHERE spent_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE transactions SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users SET access_until = coalesce(datetime(substr(access_until, 1, 19) || substr(substr(access_until, 20), instr(substr(access_until, 20), ' ') + 1, 3) || ':' || substr(substr(access_until, 20), instr(substr(access_until, 20), ' ') + 4, 2)), access_until)
WHERE access_until GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_connections_invites SET expired_at = coalesce(datetime(substr(expired_at, 1, 19) || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 1, 3) || ':' || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 4, 2)), expired_at)
WHERE expired_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_email_change_requests SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_email_change_requests SET expired_at = coalesce(datetime(substr(expired_at, 1, 19) || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 1, 3) || ':' || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 4, 2)), expired_at)
WHERE expired_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_email_change_requests SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_email_verifications SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_email_verifications SET expired_at = coalesce(datetime(substr(expired_at, 1, 19) || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 1, 3) || ':' || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 4, 2)), expired_at)
WHERE expired_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_email_verifications SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_hidden_currencies SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_options SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_options SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_password_requests SET created_at = coalesce(datetime(substr(created_at, 1, 19) || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 1, 3) || ':' || substr(substr(created_at, 20), instr(substr(created_at, 20), ' ') + 4, 2)), created_at)
WHERE created_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_password_requests SET expired_at = coalesce(datetime(substr(expired_at, 1, 19) || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 1, 3) || ':' || substr(substr(expired_at, 20), instr(substr(expired_at, 20), ' ') + 4, 2)), expired_at)
WHERE expired_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';

UPDATE users_password_requests SET updated_at = coalesce(datetime(substr(updated_at, 1, 19) || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 1, 3) || ':' || substr(substr(updated_at, 20), instr(substr(updated_at, 20), ' ') + 4, 2)), updated_at)
WHERE updated_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *';
