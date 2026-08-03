-- PostgreSQL variant. See the sqlite sibling for the semantics.
--
-- COLLATE "C" is load-bearing: SQLite compares TEXT byte-wise (BINARY) while
-- PostgreSQL compares by database collation, which folds case and orders digits
-- against letters differently. The enginecompare suite requires byte-identical
-- responses from both engines, so this column must sort by raw bytes.

ALTER TABLE categories ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE tags ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE payees ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE folders ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE accounts_options ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE budgets_folders ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';
ALTER TABLE budgets_elements ADD COLUMN sort_key TEXT COLLATE "C" NOT NULL DEFAULT '';

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


-- Drop the old ordering column now that every read path uses sort_key. This also
-- removes budgets_folders' CHECK (position >= 0), which sqlite never had.
-- One-way: there is no down migration.
ALTER TABLE categories        DROP COLUMN position;
ALTER TABLE tags              DROP COLUMN position;
ALTER TABLE payees            DROP COLUMN position;
ALTER TABLE folders           DROP COLUMN position;
ALTER TABLE accounts_options  DROP COLUMN position;
ALTER TABLE budgets_folders   DROP COLUMN position;
ALTER TABLE budgets_elements  DROP COLUMN position;
