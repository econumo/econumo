-- A payment dated on a debt's start date now pays the interest owed for the
-- time before the start first. Stored splits are never recomputed, so drop
-- them all; the next read of each budget splits every payment again.
DELETE FROM debts_payments;
