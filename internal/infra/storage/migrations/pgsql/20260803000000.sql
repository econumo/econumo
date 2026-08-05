-- PostgreSQL variant. See the sqlite sibling for the semantics.
--
-- row_number() is bigint here, and substr() has no (text, bigint, int) overload,
-- so the rank is cast to int once in each subquery.
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
  FROM (SELECT id, (row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1)::int AS n
        FROM categories) r
  WHERE r.id = categories.id
);

UPDATE tags SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, (row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1)::int AS n
        FROM tags) r
  WHERE r.id = tags.id
);

UPDATE payees SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, (row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1)::int AS n
        FROM payees) r
  WHERE r.id = payees.id
);

UPDATE folders SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, (row_number() OVER (PARTITION BY user_id ORDER BY position, id) - 1)::int AS n
        FROM folders) r
  WHERE r.id = folders.id
);

UPDATE accounts_options SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT account_id, user_id, (row_number() OVER (PARTITION BY user_id ORDER BY position, account_id) - 1)::int AS n
        FROM accounts_options) r
  WHERE r.account_id = accounts_options.account_id AND r.user_id = accounts_options.user_id
);

-- Synthesize accounts_options rows for (account, user) pairs that have none:
-- the account's owner and accepted sharees. The old read path tolerated the
-- missing row as "position 0", so such pairs exist in very old data and the
-- UPDATE above had nothing to touch. Left keyless they all compare equal, pin
-- to the front of the list, and no anchor can place an account between them.
-- Magnitude-'b' keys (two digits) sort before every backfilled 'c...' key,
-- preserving the old "position 0 -> first" placement; rank is per user by
-- account id. Pending grants and deleted accounts get nothing.
INSERT INTO accounts_options (account_id, user_id, sort_key, created_at, updated_at)
SELECT p.account_id, p.user_id,
       'b' || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (p.n / 62) % 62, 1)
           || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  p.n       % 62, 1),
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM (
  SELECT q.account_id, q.user_id,
         (row_number() OVER (PARTITION BY q.user_id ORDER BY q.account_id) - 1)::int AS n
  FROM (
    SELECT a.id AS account_id, a.user_id AS user_id
    FROM accounts a
    WHERE a.is_deleted = false
    UNION
    SELECT aa.account_id, aa.user_id
    FROM accounts_access aa
    JOIN accounts a2 ON a2.id = aa.account_id
    WHERE aa.is_accepted = true AND a2.is_deleted = false
  ) q
  WHERE NOT EXISTS (
    SELECT 1 FROM accounts_options o
    WHERE o.account_id = q.account_id AND o.user_id = q.user_id
  )
) p;

UPDATE budgets_folders SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, (row_number() OVER (PARTITION BY budget_id ORDER BY position, id) - 1)::int AS n
        FROM budgets_folders) r
  WHERE r.id = budgets_folders.id
);

UPDATE budgets_elements SET sort_key = (
  SELECT 'c'
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n / 3844) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 + (r.n /   62) % 62, 1)
      || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', 1 +  r.n         % 62, 1)
  FROM (SELECT id, (row_number() OVER (PARTITION BY budget_id, folder_id ORDER BY position, id) - 1)::int AS n
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
