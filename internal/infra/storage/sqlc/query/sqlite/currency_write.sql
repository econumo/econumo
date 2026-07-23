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
-- Every currency's id + code, used to build the rate loader's symbols list and
-- the code->id map. Mirrors CurrencyRepository::getAll() (code projection only).
SELECT id, code FROM currencies;

-- name: GetCurrencyByCode :one
-- One currency by ISO code (full row), for the idempotency check in add-currency.
SELECT id, code, symbol, name, fraction_digits, created_at
FROM currencies
WHERE code = ?;

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

-- name: GetLatestRateDate :one
-- Newest stored rate date, for the in-process rate updater's freshness check.
-- ORDER BY ... LIMIT 1 (not MAX) so the result types as the published_at column
-- (time.Time) instead of an aggregate interface{}. sql.ErrNoRows = no rates yet.
SELECT published_at FROM currencies_rates ORDER BY published_at DESC LIMIT 1;
