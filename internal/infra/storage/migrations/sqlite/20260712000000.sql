-- Normalize datetime columns written before driver.go's time.Time bind fix:
-- modernc/sqlite previously serialized a bound time.Time with Go's String()
-- form ("2006-01-02 15:04:05 +0000 UTC", sometimes with fractional seconds)
-- instead of the frozen persistence layout every fixture/legacy row uses
-- ("2006-01-02 15:04:05"). Truncating to the first 19 characters recovers the
-- bare layout; rows already in that form don't match the '%UTC' suffix, so
-- this is idempotent and safe to run on every boot.
UPDATE users SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE users SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE currencies SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';

UPDATE accounts SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE accounts SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE accounts_access SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE accounts_access SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE accounts_options SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE accounts_options SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE users_connections_invites SET expired_at = substr(expired_at, 1, 19) WHERE expired_at LIKE '____-__-__ __:__:__%UTC';

UPDATE folders SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE folders SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE payees SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE payees SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE tags SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE tags SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE categories SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE categories SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE transactions SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE transactions SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';
UPDATE transactions SET spent_at = substr(spent_at, 1, 19) WHERE spent_at LIKE '____-__-__ __:__:__%UTC';

UPDATE users_options SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE users_options SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE users_password_requests SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE users_password_requests SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';
UPDATE users_password_requests SET expired_at = substr(expired_at, 1, 19) WHERE expired_at LIKE '____-__-__ __:__:__%UTC';

-- published_at is a DATE column, but the write path binds a time.Time, so it
-- suffered the same long-format serialization; truncating to 19 chars leaves
-- a valid 'Y-m-d H:i:s' string that date()/datetime() parse identically to
-- the bare 'Y-m-d' rows already in the table.
UPDATE currencies_rates SET published_at = substr(published_at, 1, 19) WHERE published_at LIKE '____-__-__ __:__:__%UTC';

UPDATE operation_requests_ids SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE operation_requests_ids SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE budgets SET started_at = substr(started_at, 1, 19) WHERE started_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE budgets_access SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets_access SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE budgets_folders SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets_folders SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE budgets_elements SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets_elements SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE budgets_elements_limits SET period = substr(period, 1, 19) WHERE period LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets_elements_limits SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets_elements_limits SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

UPDATE budgets_envelopes SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE budgets_envelopes SET updated_at = substr(updated_at, 1, 19) WHERE updated_at LIKE '____-__-__ __:__:__%UTC';

-- access_tokens: expires_at/revoked_at are nullable; NULL LIKE ... is NULL
-- (neither true nor false), so the WHERE clause safely skips those rows.
UPDATE access_tokens SET created_at = substr(created_at, 1, 19) WHERE created_at LIKE '____-__-__ __:__:__%UTC';
UPDATE access_tokens SET last_used_at = substr(last_used_at, 1, 19) WHERE last_used_at LIKE '____-__-__ __:__:__%UTC';
UPDATE access_tokens SET expires_at = substr(expires_at, 1, 19) WHERE expires_at LIKE '____-__-__ __:__:__%UTC';
UPDATE access_tokens SET revoked_at = substr(revoked_at, 1, 19) WHERE revoked_at LIKE '____-__-__ __:__:__%UTC';
