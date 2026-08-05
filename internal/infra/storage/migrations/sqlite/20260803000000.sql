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
         (row_number() OVER (PARTITION BY q.user_id ORDER BY q.account_id) - 1) AS n
  FROM (
    SELECT a.id AS account_id, a.user_id AS user_id
    FROM accounts a
    WHERE a.is_deleted = 0
    UNION
    SELECT aa.account_id, aa.user_id
    FROM accounts_access aa
    JOIN accounts a2 ON a2.id = aa.account_id
    WHERE aa.is_accepted = 1 AND a2.is_deleted = 0
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
