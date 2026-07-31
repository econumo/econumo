-- The email-derived md5 identifier is retired: lower(email) is now the unique
-- key (index below), and the identifier column is kept only to satisfy its
-- pre-existing NOT NULL UNIQUE constraint without a SQLite table rebuild. Each
-- row's own id is a ready-made unique, non-null placeholder.
CREATE UNIQUE INDEX users_email_lower_uniq ON users (lower(email));
UPDATE users SET identifier = id;
