-- Global currency codes may be 4 letters: Open Exchange Rates publishes some
-- digital currencies under 4 (DASH, DOGE). CHAR(3) would refuse them, so widen
-- the column. (SQLite's dynamic typing stores the longer value without a
-- schema change, so this migration is pgsql-only.)
ALTER TABLE currencies ALTER COLUMN code TYPE VARCHAR(4);
