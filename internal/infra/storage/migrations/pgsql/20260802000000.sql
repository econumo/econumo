-- Soft delete for currencies. See the sqlite sibling for the semantics.
ALTER TABLE currencies ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT false;

DROP INDEX UNIQ_currencies_user_code;
CREATE UNIQUE INDEX UNIQ_currencies_user_code
    ON currencies (user_id, code) WHERE user_id IS NOT NULL AND is_deleted = false;
