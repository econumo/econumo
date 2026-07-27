-- Write-side queries for the currency module: the CLI admin commands
-- (app:add-currency) and the rate loader
-- (app:update-currency-rates). Kept separate from currencies.sql (the user-module
-- lookup) and currency_read.sql (the CQRS read model) so the write concern is
-- visibly distinct. The HTTP API has no currency write path; these run only from
-- the CLI. (Comments kept ASCII-only to match the sqlite sibling; see its note.)

-- name: ListCurrencyCodes :many
-- Every currency's id + code, used to build the rate loader's symbols list and
-- the code->id map. Mirrors CurrencyRepository::getAll() (code projection only).
SELECT id, code FROM currencies;

-- name: GetCurrencyByCode :one
-- One currency by ISO code (full row), for the idempotency check in add-currency.
SELECT id, code, symbol, name, fraction_digits, created_at
FROM currencies
WHERE code = $1;

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
