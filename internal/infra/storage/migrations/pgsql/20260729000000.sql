-- See the sqlite variant for rationale.
ALTER TABLE transactions ADD COLUMN recurring_id UUID DEFAULT NULL
    REFERENCES recurring_transactions (id) ON DELETE SET NULL;
CREATE INDEX recurring_id_idx_transactions ON transactions (recurring_id);
