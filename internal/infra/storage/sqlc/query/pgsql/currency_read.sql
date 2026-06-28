-- Read-model queries for the currency module (PostgreSQL variant). No $N
-- placeholders are needed (neither query is parameterised). See the sqlite
-- variant for documentation.

-- name: GetCurrencyListView :many
SELECT id, code, symbol, name, fraction_digits
FROM currencies
ORDER BY code ASC;

-- name: GetCurrencyByIDView :one
SELECT id, code, symbol, name, fraction_digits
FROM currencies
WHERE id = $1;

-- name: GetAverageCurrencyRates :many
-- Period-averaged rate per currency for a base currency over [start, end).
SELECT currency_id, CAST(AVG(rate) AS TEXT) AS rate
FROM currencies_rates
WHERE published_at >= $1 AND published_at < $2 AND base_currency_id = $3
GROUP BY currency_id, base_currency_id;

-- name: GetLatestCurrencyRateDate :one
SELECT published_at FROM currencies_rates
WHERE base_currency_id = $1 AND published_at < $2
ORDER BY published_at DESC
LIMIT 1;

-- name: GetLatestCurrencyRateListView :many
SELECT id, currency_id, base_currency_id, published_at, rate
FROM currencies_rates
WHERE published_at = (SELECT MAX(published_at) FROM currencies_rates);
