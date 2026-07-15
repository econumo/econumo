-- Write-side queries for the currency module: the CLI admin commands
-- (app:add-currency) and the rate loader
-- (app:update-currency-rates). Kept separate from currencies.sql (the user-module
-- lookup) and currency_read.sql (the CQRS read model) so the write concern is
-- visibly distinct. The HTTP API has no currency write path; these run only from
-- the CLI. (Comments kept ASCII-only to match the sqlite sibling; see its note.)

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
WHERE code = $1 AND user_id IS NULL;

-- name: InsertCurrency :exec
-- Add a new currency. Mirrors CurrencyUpdateService::updateCurrencies (create).
INSERT INTO currencies (id, code, symbol, name, fraction_digits, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: UpsertCurrencyRate :exec
-- Insert or update a rate for (published_at, currency, base). published_at is a
-- native DATE; the conflict target is the identifier_uniq_currencies_rates UNIQUE
-- index. Mirrors CurrencyRatesUpdateService (get-or-create then updateRate).
INSERT INTO currencies_rates (id, currency_id, base_currency_id, published_at, rate)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (published_at, currency_id, base_currency_id)
DO UPDATE SET rate = excluded.rate;

-- User currency management (per-user custom currencies). Global currencies
-- have user_id NULL; custom currencies carry their owner id.

-- name: GetCurrencyRecord :one
SELECT id, code, symbol, name, fraction_digits, user_id, is_archived, created_at
FROM currencies WHERE id = $1;

-- name: GlobalCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = $1 AND user_id IS NULL;

-- name: OwnerCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = $1 AND user_id = $2;

-- name: InsertUserCurrency :exec
INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, is_archived, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateCurrencyDetails :exec
UPDATE currencies SET name = $1, symbol = $2, fraction_digits = $3 WHERE id = $4;

-- name: SetCurrencyArchived :exec
UPDATE currencies SET is_archived = $1 WHERE id = $2;

-- name: DeleteCurrency :exec
DELETE FROM currencies WHERE id = $1;

-- name: GetGlobalCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = $1 AND user_id IS NULL;

-- Usage census for delete protection: accounts (including soft-deleted ones,
-- they still hold the FK), budgets, budget elements, and any user whose
-- profile currency option stores this code. $1 is reused for all three
-- currency-id positions so the generated param stays two fields.
-- name: CountCurrencyUsage :one
SELECT (SELECT COUNT(*) FROM accounts WHERE accounts.currency_id = $1)
     + (SELECT COUNT(*) FROM budgets WHERE budgets.currency_id = $1)
     + (SELECT COUNT(*) FROM budgets_elements WHERE budgets_elements.currency_id = $1)
     + (SELECT COUNT(*) FROM users_options WHERE users_options.name = 'currency' AND users_options.value = $2) AS usage_count;

-- name: InsertHiddenCurrency :exec
INSERT INTO users_hidden_currencies (user_id, currency_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, currency_id) DO NOTHING;

-- name: DeleteHiddenCurrency :exec
DELETE FROM users_hidden_currencies WHERE user_id = $1 AND currency_id = $2;
