-- Soft delete for currencies. Deletion never removes the row: accounts.currency_id
-- cascades and transactions.account_id cascades from there, so a DELETE would take
-- account and transaction history with it. ADD COLUMN with a constant default needs
-- no table rebuild, so no foreign_keys pragma juggling here.
ALTER TABLE currencies ADD COLUMN is_deleted BOOLEAN DEFAULT '0' NOT NULL;

-- The per-user code index excludes deleted rows so an owner can re-create a code
-- they deleted. The global index is deliberately left alone: re-adding a retired
-- global clears its flag instead of inserting a second row.
DROP INDEX UNIQ_currencies_user_code;
CREATE UNIQUE INDEX UNIQ_currencies_user_code
    ON currencies (user_id, code) WHERE user_id IS NOT NULL AND is_deleted = 0;
