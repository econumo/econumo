-- Read-model queries for the currency module (CQRS read side). Both currency
-- endpoints are pure reads, so the whole module lives on the read side; there is
-- no write aggregate. Kept separate from currencies.sql (the user-module lookup)
-- to keep the read concern visibly distinct.

-- name: GetUserCurrencyListView :many
-- Per-user visible currencies: all globals, the user's own customs (archived
-- included, the settings page needs them), and foreign customs reachable via
-- accounts or budgets shared to the user (budget currency and element
-- currencies). Codes can repeat across owners, so id breaks ties.
SELECT c.id, c.code, c.symbol, c.name, c.fraction_digits, c.user_id, c.rate, c.created_at, c.is_deleted
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
-- 'Y-m-d H:i:s' string bound, so rows AT/after the boundary stay out (else
-- "< Dec 1" returns a December date, snapping the rate period to the wrong month).
SELECT published_at FROM currencies_rates
WHERE base_currency_id = ? AND datetime(published_at) < datetime(?)
ORDER BY published_at DESC
LIMIT 1;

-- name: GetNearestCurrencyRates :many
-- Per currency with stored rates: its latest rate before a date, else its first.
-- Fills a period's gaps, since rate histories start and stop at different dates.
SELECT c.id AS currency_id, CAST(COALESCE(
    (SELECT r.rate FROM currencies_rates r
     WHERE r.currency_id = c.id AND r.base_currency_id = sqlc.arg(base_currency_id) AND datetime(r.published_at) < datetime(sqlc.arg(before))
     ORDER BY r.published_at DESC LIMIT 1),
    (SELECT r.rate FROM currencies_rates r
     WHERE r.currency_id = c.id AND r.base_currency_id = sqlc.arg(base_currency_id)
     ORDER BY r.published_at ASC LIMIT 1)
) AS REAL) AS rate
FROM currencies c
WHERE EXISTS (SELECT 1 FROM currencies_rates r WHERE r.currency_id = c.id AND r.base_currency_id = sqlc.arg(base_currency_id));

-- name: GetEarliestCurrencyRateDate :one
-- Oldest published_at for a base currency: the rate month used for periods
-- before the first stored rate.
SELECT published_at FROM currencies_rates
WHERE base_currency_id = ?
ORDER BY published_at ASC
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

-- name: GetCurrencyRateRangeView :many
-- Rates published within [from, to] (inclusive days) plus, per (currency,
-- base) pair, the newest rate published before from. published_at is stored
-- as bare 'Y-m-d' text and the bounds are bound as 'Y-m-d' strings, so the raw
-- column is compared (no date() wrap) and the published_at indexes stay usable.
-- The CASTs keep sqlc from typing the bounds as time.Time, which would bind a
-- 'Y-m-d H:i:s' string and push a rate on from out of the window.
SELECT cr.id, cr.currency_id, cr.base_currency_id, cr.published_at, cr.rate
FROM currencies_rates cr
WHERE cr.published_at >= CAST(sqlc.arg(from_day) AS TEXT)
  AND cr.published_at <= CAST(sqlc.arg(to_day) AS TEXT)
UNION ALL
SELECT cr.id, cr.currency_id, cr.base_currency_id, cr.published_at, cr.rate
FROM currencies_rates cr
JOIN (
    SELECT currency_id, base_currency_id, MAX(published_at) AS published_at
    FROM currencies_rates
    WHERE published_at < CAST(sqlc.arg(from_day) AS TEXT)
    GROUP BY currency_id, base_currency_id
) latest ON latest.currency_id = cr.currency_id
    AND latest.base_currency_id = cr.base_currency_id
    AND latest.published_at = cr.published_at
ORDER BY currency_id ASC, base_currency_id ASC, published_at ASC;
