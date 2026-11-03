-- The key replaces lower(email) as the one uniqueness rule for addresses.
CREATE UNIQUE INDEX users_email_key_uniq ON users (email_key);
DROP INDEX users_email_lower_uniq;
