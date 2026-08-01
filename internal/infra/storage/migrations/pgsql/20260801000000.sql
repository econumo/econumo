-- Custom currencies get ONE fixed rate (no history): currencies.rate holds
-- "X units per 1 base unit" for customs (NULL for globals). Backfill each
-- custom's LATEST dated rate, then purge custom rows from currencies_rates,
-- which returns to globals-only OXR history. Customs created without a rate
-- stay NULL (no data invented); the first edit forces one in.
ALTER TABLE currencies ADD COLUMN rate NUMERIC(19, 8) DEFAULT NULL;
UPDATE currencies SET rate = (
    SELECT cr.rate FROM currencies_rates cr
    WHERE cr.currency_id = currencies.id
    ORDER BY cr.published_at DESC LIMIT 1
) WHERE user_id IS NOT NULL;
DELETE FROM currencies_rates WHERE currency_id IN (SELECT id FROM currencies WHERE user_id IS NOT NULL);
