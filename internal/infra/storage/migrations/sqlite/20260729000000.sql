-- Links a posted transaction back to the recurring template it came from, so
-- the UI can tell a materialized instance apart from a hand-entered one.
-- Nullable: every pre-existing row, and every manually created transaction,
-- has no template. SET NULL on delete keeps the transaction history intact
-- when a template is removed -- the money moved regardless of the schedule.
ALTER TABLE transactions ADD COLUMN recurring_id TEXT DEFAULT NULL
    REFERENCES recurring_transactions (id) ON DELETE SET NULL;
CREATE INDEX recurring_id_idx_transactions ON transactions (recurring_id);
