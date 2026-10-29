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

-- name: GetLatestRateDate :one
-- Newest stored rate date, for the in-process rate updater's freshness check.
-- ORDER BY ... LIMIT 1 (not MAX) so the result types as the published_at column
-- (time.Time) instead of an aggregate interface{}. sql.ErrNoRows = no rates yet.
SELECT published_at FROM currencies_rates ORDER BY published_at DESC LIMIT 1;

-- name: GetLatestRateBase :one
-- Base currency of the newest stored rate, for the boot-time drift WARN when
-- ECONUMO_CURRENCY_BASE no longer matches what the rates were written against.
SELECT base_currency_id FROM currencies_rates ORDER BY published_at DESC LIMIT 1;

-- name: ListUsedGlobalCurrencyCodes :many
-- Global currencies something converts from or to: an account (deleted ones
-- too, their history still counts), a budget, a budget element, or a user's
-- display currency. The rate-gap check looks at these only.
SELECT c.code FROM currencies c
WHERE c.user_id IS NULL AND (
    EXISTS (SELECT 1 FROM accounts a WHERE a.currency_id = c.id)
    OR EXISTS (SELECT 1 FROM budgets b WHERE b.currency_id = c.id)
    OR EXISTS (SELECT 1 FROM budgets_elements e WHERE e.currency_id = c.id)
    OR EXISTS (SELECT 1 FROM users_options o WHERE o.name = 'currency' AND o.value = c.id::text)
)
ORDER BY c.code;

-- name: ListRatedPeriods :many
-- Distinct (period, code) pairs with a rate against the base in [from, to).
-- The period is published_at formatted with the bound to_char layout
-- ('YYYY-MM' per month, 'YYYY-MM-DD' per day).
SELECT DISTINCT to_char(r.published_at, sqlc.arg(layout)::text)::text AS period, c.code
FROM currencies_rates r
JOIN currencies c ON c.id = r.currency_id
WHERE r.base_currency_id = sqlc.arg(base_currency_id)
  AND r.published_at >= sqlc.arg(from_date)
  AND r.published_at < sqlc.arg(to_date);

-- name: GetEarliestTransactionDate :one
-- Where a rate-gap check starts by default. ORDER BY ... LIMIT 1 so the
-- result types as the column. sql.ErrNoRows = no transactions yet.
SELECT spent_at FROM transactions ORDER BY spent_at ASC LIMIT 1;

-- User currency management (per-user custom currencies). Global currencies
-- have user_id NULL; custom currencies carry their owner id.

-- name: GetCurrencyRecord :one
SELECT id, code, symbol, name, fraction_digits, user_id, rate, created_at, is_deleted
FROM currencies WHERE id = $1;

-- name: GlobalCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = $1 AND user_id IS NULL;

-- Deleted customs release their code, so they must not block a re-create.
-- name: OwnerCurrencyCodeExists :one
SELECT COUNT(*) FROM currencies WHERE code = $1 AND user_id = $2 AND is_deleted = false;

-- name: InsertUserCurrency :exec
INSERT INTO currencies (id, code, symbol, name, fraction_digits, user_id, rate, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateCurrencyDetails :exec
UPDATE currencies SET name = $1, symbol = $2, fraction_digits = $3, rate = $4 WHERE id = $5;

-- Fixed custom-currency rates for the convertor overlay: one rate per custom,
-- period-independent. Globals never carry a rate here.
-- name: GetFixedCurrencyRates :many
SELECT id, rate FROM currencies WHERE rate IS NOT NULL;


-- Currencies are never removed: accounts.currency_id and transactions.account_id
-- both cascade, so a DELETE would destroy account and transaction history.
-- name: SoftDeleteCurrency :exec
UPDATE currencies SET is_deleted = true WHERE id = $1;

-- Usage census for delete protection. Only LIVE references count: a soft-deleted
-- account is unreachable and unrestorable, so it must not pin a currency forever.
-- $1 is reused everywhere, so the generated param is a single field.
-- name: CountCurrencyUsage :one
SELECT (SELECT COUNT(*) FROM accounts WHERE accounts.currency_id = $1 AND accounts.is_deleted = false)
     + (SELECT COUNT(*) FROM budgets WHERE budgets.currency_id = $1)
     + (SELECT COUNT(*) FROM budgets_elements WHERE budgets_elements.currency_id = $1)
     + (SELECT COUNT(*) FROM users_options WHERE users_options.name = 'currency' AND users_options.value = $1) AS usage_count;

-- name: InsertHiddenCurrency :exec
INSERT INTO users_hidden_currencies (user_id, currency_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, currency_id) DO NOTHING;

-- name: DeleteHiddenCurrency :exec
DELETE FROM users_hidden_currencies WHERE user_id = $1 AND currency_id = $2;

-- Bulk visibility over the GLOBAL currencies (customs keep their per-row
-- preference). The two exclusion slots carry the base and profile ids.
-- name: HideGlobalCurrencies :exec
INSERT INTO users_hidden_currencies (user_id, currency_id, created_at)
SELECT $1, c.id, $2
FROM currencies c
WHERE c.user_id IS NULL AND c.id != $3 AND c.id != $4
ON CONFLICT (user_id, currency_id) DO NOTHING;

-- name: ShowGlobalCurrencies :exec
DELETE FROM users_hidden_currencies
WHERE users_hidden_currencies.user_id = $1
  AND currency_id IN (SELECT c.id FROM currencies c WHERE c.user_id IS NULL);

-- Profile defaults holding this currency id, for the archive guard (a
-- default must stay pickable).
