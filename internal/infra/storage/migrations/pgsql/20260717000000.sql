-- Reset codes are now stored as a sha256 hex (64 chars), not the 12-char
-- plaintext. Widen the column so the hash fits. (SQLite's dynamic typing stores
-- the longer value without a schema change, so this migration is pgsql-only.)
ALTER TABLE users_password_requests ALTER COLUMN code TYPE VARCHAR(64);
