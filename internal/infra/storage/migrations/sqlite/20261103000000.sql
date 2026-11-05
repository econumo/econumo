-- users.email now holds the address in the one form every lookup uses, computed in Go
-- (model.NormalizeEmail) because lower() disagrees across engines: SQLite's folds ASCII
-- only, so two accounts could differ by the case of a non-ASCII letter. The next step
-- rewrites the rows, and the index comes first so its check for a taken address does
-- not scan the table per user.
CREATE UNIQUE INDEX users_email_uniq ON users (email);
DROP INDEX users_email_lower_uniq;
