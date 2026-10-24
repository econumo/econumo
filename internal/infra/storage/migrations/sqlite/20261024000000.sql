-- Per-account-side position of a transaction within its day. sort_key orders
-- the row on its source account, recipient_sort_key on its recipient account
-- (transfers only), so reordering one account never moves the other. Higher
-- = higher in the list. The backfill numbers each side's days in the order
-- the list showed before (spent_at DESC, id), so an upgrade changes nothing
-- visible; the revisions move because the downloaded rows gain the keys.

ALTER TABLE transactions ADD COLUMN sort_key INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transactions ADD COLUMN recipient_sort_key INTEGER DEFAULT NULL;

UPDATE transactions SET sort_key = (
  SELECT r.n
  FROM (SELECT id, row_number() OVER (PARTITION BY account_id, substr(spent_at, 1, 10) ORDER BY spent_at, id DESC) AS n
        FROM transactions) r
  WHERE r.id = transactions.id
);

UPDATE transactions SET recipient_sort_key = (
  SELECT r.n
  FROM (SELECT id, row_number() OVER (PARTITION BY account_recipient_id, substr(spent_at, 1, 10) ORDER BY spent_at, id DESC) AS n
        FROM transactions
        WHERE account_recipient_id IS NOT NULL) r
  WHERE r.id = transactions.id
)
WHERE account_recipient_id IS NOT NULL;

UPDATE accounts SET transactions_revision = transactions_revision + 1;
