-- Read-model queries for the currency module (PostgreSQL variant). No $N
-- placeholders are needed (neither query is parameterised). See the sqlite
-- variant for documentation.

-- name: GetUserCurrencyListView :many
-- Per-user visible currencies: all globals, the user's own customs (archived
-- included, the settings page needs them), and foreign customs reachable via
-- accounts or budgets shared to the user (budget currency and element
-- currencies). Codes can repeat across owners, so id breaks ties. $1 is
-- reused for all four user-id positions so the generated param stays single.
SELECT c.id, c.code, c.symbol, c.name, c.fraction_digits, c.user_id, c.is_archived
FROM currencies c
WHERE c.user_id IS NULL
   OR c.user_id = $1
   OR c.id IN (
       SELECT a.currency_id FROM accounts a
       JOIN accounts_access aa ON aa.account_id = a.id
       WHERE aa.user_id = $1
   )
   OR c.id IN (
       SELECT b.currency_id FROM budgets b
       JOIN budgets_access ba ON ba.budget_id = b.id
       WHERE ba.user_id = $1
   )
   OR c.id IN (
       SELECT be.currency_id FROM budgets_elements be
       JOIN budgets_access ba ON ba.budget_id = be.budget_id
       WHERE ba.user_id = $1 AND be.currency_id IS NOT NULL
   )
ORDER BY c.code ASC, c.id ASC;

-- name: GetHiddenCurrencyIDs :many
SELECT currency_id FROM users_hidden_currencies WHERE user_id = $1;

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
-- Latest rate row per (currency, base) pair. See the sqlite variant.
SELECT cr.id, cr.currency_id, cr.base_currency_id, cr.published_at, cr.rate
FROM currencies_rates cr
WHERE cr.published_at = (
    SELECT MAX(cr2.published_at) FROM currencies_rates cr2
    WHERE cr2.currency_id = cr.currency_id AND cr2.base_currency_id = cr.base_currency_id
)
ORDER BY cr.currency_id ASC, cr.base_currency_id ASC;
