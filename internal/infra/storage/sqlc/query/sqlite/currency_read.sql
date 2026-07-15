-- Read-model queries for the currency module (CQRS read side). Both currency
-- endpoints are pure reads, so the whole module lives on the read side; there is
-- no write aggregate. Kept separate from currencies.sql (the user-module lookup)
-- to keep the read concern visibly distinct.

-- name: GetUserCurrencyListView :many
-- Per-user visible currencies: all globals, the user's own customs (archived
-- included, the settings page needs them), and foreign customs reachable via
-- accounts or budgets shared to the user (budget currency and element
-- currencies). Codes can repeat across owners, so id breaks ties.
SELECT c.id, c.code, c.symbol, c.name, c.fraction_digits, c.user_id, c.is_archived
FROM currencies c
WHERE c.user_id IS NULL
   OR c.user_id = ?
   OR c.id IN (
       SELECT a.currency_id FROM accounts a
       JOIN accounts_access aa ON aa.account_id = a.id
       WHERE aa.user_id = ?
   )
   OR c.id IN (
       SELECT b.currency_id FROM budgets b
       JOIN budgets_access ba ON ba.budget_id = b.id
       WHERE ba.user_id = ?
   )
   OR c.id IN (
       SELECT be.currency_id FROM budgets_elements be
       JOIN budgets_access ba ON ba.budget_id = be.budget_id
       WHERE ba.user_id = ? AND be.currency_id IS NOT NULL
   )
ORDER BY c.code ASC, c.id ASC;

-- name: GetHiddenCurrencyIDs :many
SELECT currency_id FROM users_hidden_currencies WHERE user_id = ?;

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
-- CurrencyRateRepository::getLatestDate). Compare via datetime() with a
-- 'Y-m-d H:i:s' string bound: a time.Time bound mis-compares against the stored
-- datetime TEXT, letting rows AT/after the boundary leak in (so "< Dec 1" wrongly
-- returned a December date, snapping the rate period to the wrong month).
SELECT published_at FROM currencies_rates
WHERE base_currency_id = ? AND datetime(published_at) < datetime(?)
ORDER BY published_at DESC
LIMIT 1;

-- name: GetLatestCurrencyRateListView :many
-- Latest rate row per (currency, base) pair. The previous single-latest-date
-- form dropped any currency whose newest rate predates the newest OXR batch,
-- which breaks backdated custom rates.
SELECT cr.id, cr.currency_id, cr.base_currency_id, cr.published_at, cr.rate
FROM currencies_rates cr
WHERE cr.published_at = (
    SELECT MAX(cr2.published_at) FROM currencies_rates cr2
    WHERE cr2.currency_id = cr.currency_id AND cr2.base_currency_id = cr.base_currency_id
)
ORDER BY cr.currency_id ASC, cr.base_currency_id ASC;
