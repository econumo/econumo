-- Read-model queries for the currency module (CQRS read side). Both currency
-- endpoints are pure reads, so the whole module lives on the read side; there is
-- no write aggregate. Kept separate from currencies.sql (the user-module lookup)
-- to keep the read concern visibly distinct.

-- name: GetCurrencyListView :many
-- All currencies ordered by code ASC (matches CurrencyRepository::getAll ->
-- findBy([], ['code' => 'ASC'])). The name column is NULL for every row in
-- practice; the app layer resolves the display name from the Intl table.
SELECT id, code, symbol, name, fraction_digits
FROM currencies
ORDER BY code ASC;

-- name: GetCurrencyByIDView :one
-- One currency by id, for embedding in another resource (e.g. the account
-- result's currency block). name is NULL in practice; the app resolves the
-- display name from the Intl table.
SELECT id, code, symbol, name, fraction_digits
FROM currencies
WHERE id = ?;

-- name: GetAverageCurrencyRates :many
-- Period-averaged rate per currency for a base currency over [start, end).
-- published_at is a DATE (stored TEXT); bounds are 'Y-m-d' strings. Matches
-- CurrencyRateRepository::getAverage. AVG is cast to TEXT and normalized in Go
-- via vo.NewDecimal (matching PHP's new DecimalNumber($rate)).
SELECT currency_id, CAST(AVG(rate) AS REAL) AS rate
FROM currencies_rates
WHERE date(published_at) >= date(?) AND date(published_at) < date(?) AND base_currency_id = ?
GROUP BY currency_id, base_currency_id;

-- name: GetLatestCurrencyRateDate :one
-- Most-recent published_at for a base currency strictly before a date (matches
-- CurrencyRateRepository::getLatestDate).
SELECT published_at FROM currencies_rates
WHERE base_currency_id = ? AND published_at < ?
ORDER BY published_at DESC
LIMIT 1;

-- name: GetLatestCurrencyRateListView :many
-- All rate rows published on the single most-recent published_at date (matches
-- CurrencyRateRepository::getAll(): find MAX(published_at), return every row on
-- it). published_at is a DATE; the wire formats it as "Y-m-d 00:00:00".
SELECT id, currency_id, base_currency_id, published_at, rate
FROM currencies_rates
WHERE published_at = (SELECT MAX(published_at) FROM currencies_rates);
