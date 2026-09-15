-- The rate updater bound published_at as a time.Time, which modernc stores as
-- "2026-09-14 00:00:00 +0000 UTC". date()/datetime() read that as NULL, so the
-- convertor ignored every such rate and budgets converted foreign-currency
-- amounts 1:1 (issue #257). Rewrite them to the 'Y-m-d' form legacy rows use;
-- the updater writes midnight UTC, so the first ten characters are the date.
--
-- Dedupe BEFORE rewriting: the unique index on (published_at, currency_id,
-- base_currency_id) is TEXTUAL, so a legacy row and a rewritten row for the same
-- day would collide. The updater's row is the later write for that day, so the
-- legacy twin goes.
DELETE FROM currencies_rates
WHERE length(published_at) = 10
  AND EXISTS (
    SELECT 1 FROM currencies_rates g
    WHERE g.currency_id = currencies_rates.currency_id
      AND g.base_currency_id = currencies_rates.base_currency_id
      AND length(g.published_at) > 10
      AND substr(g.published_at, 1, 10) = currencies_rates.published_at
  );

UPDATE currencies_rates
SET published_at = substr(published_at, 1, 10)
WHERE length(published_at) > 10;
