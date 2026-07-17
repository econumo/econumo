-- Write-side queries for the currency module: the CLI admin commands
-- (app:add-currency) and the rate loader
-- (app:update-currency-rates). Kept separate from currencies.sql (the user-module
-- lookup) and currency_read.sql (the CQRS read model) so the write concern is
-- visibly distinct. The HTTP API has no currency write path; these run only from
-- the CLI.
--
-- NOTE: keep these comments ASCII-only. sqlc's sqlite engine miscomputes the
-- emitted query length when a query's leading comment contains multi-byte UTF-8
-- (it truncates the SQL by the byte-vs-rune delta), which silently corrupts the
-- generated statement.

-- name: ListCurrencyCodes :many
-- Every GLOBAL currency's id + code, used to build the rate loader's symbols
-- list and the code->id map. Custom (per-user) currencies must never reach the
-- CLI/OXR path. Mirrors CurrencyRepository::getAll() (code projection only).
SELECT id, code FROM currencies WHERE user_id IS NULL;

-- name: GetCurrencyByCode :one
-- One GLOBAL currency by ISO code (full row), for the idempotency check in
-- add-currency.
SELECT id, code, symbol, name, fraction_digits, created_at
FROM currencies
WHERE code = ? AND user_id IS NULL;

-- name: InsertCurrency :exec
-- Add a new currency. Mirrors CurrencyUpdateService::updateCurrencies (create).
INSERT INTO currencies (id, code, symbol, name, fraction_digits, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpsertCurrencyRate :exec
-- Insert or update a rate for (published_at, currency, base). published_at is a
-- DATE; the repo passes a time.Time truncated to midnight UTC. modernc stores
-- date/datetime columns in ISO8601 (like every other date the Go repos write);
-- the read path is format-agnostic because it compares via date()/MAX, and the
-- midnight truncation keeps the value stable so the ON CONFLICT
-- (identifier_uniq_currencies_rates) upsert dedupes per day.
INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (published_at, currency_id, base_currency_id)
DO UPDATE SET rate = excluded.rate;

-- User currency management (per-user custom currencies). Global currencies
-- have user_id NULL; custom currencies carry their owner id.

-- name: GetCurrencyRecord :one
SELECT id, code, symbol, name, fraction_digits, user_id, is_archived, created_at
FROM currencies WHERE id = ?;

-- name: GlobalCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = ? AND user_id IS NULL;

-- name: OwnerCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = ? AND user_id = ?;

-- name: InsertUserCurrency :exec
INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, is_archived, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateCurrencyDetails :exec
UPDATE currencies SET name = ?, symbol = ?, fraction_digits = ? WHERE id = ?;

-- name: SetCurrencyArchived :exec
UPDATE currencies SET is_archived = ? WHERE id = ?;

-- name: DeleteCurrency :exec
DELETE FROM currencies WHERE id = ?;

-- name: GetGlobalCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ? AND user_id IS NULL;

-- Usage census for delete protection: accounts (including soft-deleted ones,
-- they still hold the FK), budgets, budget elements, and any user whose
-- profile currency option stores this code.
-- name: CountCurrencyUsage :one
SELECT (SELECT COUNT(*) FROM accounts WHERE accounts.currency_id = ?)
     + (SELECT COUNT(*) FROM budgets WHERE budgets.currency_id = ?)
     + (SELECT COUNT(*) FROM budgets_elements WHERE budgets_elements.currency_id = ?)
     + (SELECT COUNT(*) FROM users_options WHERE users_options.name = 'currency' AND users_options.value = ?) AS usage_count;

-- name: InsertHiddenCurrency :exec
INSERT INTO users_hidden_currencies (user_id, currency_id, created_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id, currency_id) DO NOTHING;

-- name: DeleteHiddenCurrency :exec
DELETE FROM users_hidden_currencies WHERE user_id = ? AND currency_id = ?;
