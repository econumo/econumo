-- Per-account-side position of a transaction within its day. sort_key orders
-- the row on its source account, recipient_sort_key on its recipient account
-- (transfers only), so reordering one account never moves the other. Higher
-- = higher in the list. The backfill numbers each account's days in the order
-- the list showed before (spent_at DESC, id), counting the rows it is the
-- source of and the transfers it receives together, so an upgrade changes
-- nothing visible; the revisions move because the downloaded rows gain the keys.

ALTER TABLE transactions ADD COLUMN sort_key INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transactions ADD COLUMN recipient_sort_key INTEGER DEFAULT NULL;

UPDATE transactions SET sort_key = (
  SELECT r.n
  FROM (SELECT id, side, row_number() OVER (PARTITION BY acct, substr(s.spent_at, 1, 10) ORDER BY s.spent_at, id DESC) AS n
        FROM (SELECT id, 0 AS side, account_id AS acct, spent_at FROM transactions
              UNION ALL
              SELECT id, 1 AS side, account_recipient_id AS acct, spent_at FROM transactions WHERE account_recipient_id IS NOT NULL) s) r
  WHERE r.id = transactions.id AND r.side = 0
);

UPDATE transactions SET recipient_sort_key = (
  SELECT r.n
  FROM (SELECT id, side, row_number() OVER (PARTITION BY acct, substr(s.spent_at, 1, 10) ORDER BY s.spent_at, id DESC) AS n
        FROM (SELECT id, 0 AS side, account_id AS acct, spent_at FROM transactions
              UNION ALL
              SELECT id, 1 AS side, account_recipient_id AS acct, spent_at FROM transactions WHERE account_recipient_id IS NOT NULL) s) r
  WHERE r.id = transactions.id AND r.side = 1
)
WHERE account_recipient_id IS NOT NULL;

UPDATE accounts SET transactions_revision = transactions_revision + 1;
