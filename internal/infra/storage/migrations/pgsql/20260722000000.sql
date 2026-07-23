-- The email-derived md5 identifier is retired: lower(email) is now the unique
-- key (index below), and the identifier column is kept only to satisfy its
-- pre-existing NOT NULL UNIQUE constraint without a SQLite table rebuild. Each
-- row's own id is a ready-made unique, non-null placeholder, but the column is
-- still the legacy CHAR(32) sized for an md5 hex digest -- too narrow for a
-- 36-char UUID string -- so it is widened first (sqlite's identifier column
-- was already widened to unconstrained TEXT by an earlier baseline squash).
ALTER TABLE users ALTER COLUMN identifier TYPE TEXT;
CREATE UNIQUE INDEX users_email_lower_uniq ON users (lower(email));
UPDATE users SET identifier = id;
