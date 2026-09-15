-- The rate updater bound published_at as a time.Time, which modernc stores as
-- "2026-09-14 00:00:00 +0000 UTC". date()/datetime() read that as NULL, so the
-- convertor ignored every such rate and budgets converted foreign-currency
-- amounts 1:1 (issue #257). Rewrite them to the 'Y-m-d' form legacy rows use;
-- the updater writes midnight UTC, so the first ten characters are the date.
--
-- OR REPLACE: the unique index on (published_at, currency_id, base_currency_id)
-- is TEXTUAL, so a rewritten row can collide with a legacy row for the same
-- day. The updater's row is the later write, so REPLACE drops the legacy twin
-- and keeps the rewritten row. This is one indexed pass over the few long-form
-- rows; a separate correlated dedupe DELETE was quadratic in days-per-currency
-- (about a minute on a three-year daily rate history).
UPDATE OR REPLACE currencies_rates
SET published_at = substr(published_at, 1, 10)
WHERE length(published_at) > 10;
