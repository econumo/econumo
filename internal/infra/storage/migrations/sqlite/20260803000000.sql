-- Replace the int16 `position` ordering column with `sort_key`, a base-62
-- fractional index key (see internal/shared/sortkey). Keys sort byte-wise and a
-- new key always exists between any two, so moving one row writes one row and
-- never renumbers its siblings.
--
-- Backfill: row_number() gives each row's rank in the CURRENT (position, id)
-- order within its scope, and three substr() calls encode that rank as base-62
-- digits. The literal 'c' is the magnitude head for a 3-digit integer part, so
-- every row in a scope shares a magnitude and ordering is decided by the digits
-- alone. Three digits hold 62^3 = 238328 rows per scope.
--
-- `position` is retained here and dropped once every read path has moved over.

ALTER TABLE categories ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE tags ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE payees ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE folders ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts_options ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE budgets_folders ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE budgets_elements ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';

UPDATE categories SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n
        FROM categories) r
  WHERE r.id = categories.id
);

UPDATE tags SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n
        FROM tags) r
  WHERE r.id = tags.id
);

UPDATE payees SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n
        FROM payees) r
  WHERE r.id = payees.id
);

UPDATE folders SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1 AS n
        FROM folders) r
  WHERE r.id = folders.id
);

UPDATE accounts_options SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT account_id, user_id, row_number() OVER (PARTITION BY user_id ORDER BY position, account_id) - 1 AS n
        FROM accounts_options) r
  WHERE r.account_id = accounts_options.account_id AND r.user_id = accounts_options.user_id
);

UPDATE budgets_folders SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY budget_id ORDER BY position, id) - 1 AS n
        FROM budgets_folders) r
  WHERE r.id = budgets_folders.id
);

UPDATE budgets_elements SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, row_number() OVER (PARTITION BY budget_id, folder_id ORDER BY position, id) - 1 AS n
        FROM budgets_elements) r
  WHERE r.id = budgets_elements.id
);


-- Drop the old ordering column now that every read path uses sort_key. position
-- is not indexed anywhere, so sqlite can drop it in place with no table rebuild.
-- One-way: there is no down migration.
ALTER TABLE categories        DROP COLUMN position;
ALTER TABLE tags              DROP COLUMN position;
ALTER TABLE payees            DROP COLUMN position;
ALTER TABLE folders           DROP COLUMN position;
ALTER TABLE accounts_options  DROP COLUMN position;
ALTER TABLE budgets_folders   DROP COLUMN position;
ALTER TABLE budgets_elements  DROP COLUMN position;
