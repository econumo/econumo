-- The address in the form it is looked up by, computed in Go (model.NormalizeEmail).
-- lower(email) could not be that key: SQLite's lower() folds ASCII only, so two
-- accounts could differ by the case of a non-ASCII letter and a lookup lowered in
-- Go missed a row stored in uppercase. Nullable because the next step fills it, and
-- a NULL stays on the newer account of a pair that already collides.
ALTER TABLE users ADD COLUMN email_key TEXT;
