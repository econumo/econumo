-- Per-account-side position of a transaction within its day. sort_key orders
-- the row on its source account, recipient_sort_key on its recipient account
-- (transfers only), so reordering one account never moves the other. Higher
-- = higher in the list. The backfill numbers each side's days in the order
-- the list showed before (spent_at DESC, id), so an upgrade changes nothing
-- visible; the revisions move because the downloaded rows gain the keys.

ALTER TABLE transactions ADD COLUMN sort_key BIGINT NOT NULL DEFAULT 0;
ALTER TABLE transactions ADD COLUMN recipient_sort_key BIGINT DEFAULT NULL;

UPDATE transactions SET sort_key = r.n
FROM (
    SELECT id, ROW_NUMBER() OVER (
        PARTITION BY account_id, CAST(spent_at AS date)
        ORDER BY spent_at, id DESC
    ) AS n
    FROM transactions
) r
WHERE transactions.id = r.id;

UPDATE transactions SET recipient_sort_key = r.n
FROM (
    SELECT id, ROW_NUMBER() OVER (
        PARTITION BY account_recipient_id, CAST(spent_at AS date)
        ORDER BY spent_at, id DESC
    ) AS n
    FROM transactions
    WHERE account_recipient_id IS NOT NULL
) r
WHERE transactions.id = r.id;

UPDATE accounts SET transactions_revision = transactions_revision + 1;
