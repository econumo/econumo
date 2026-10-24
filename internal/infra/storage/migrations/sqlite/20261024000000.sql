-- Per-account-side position of a transaction within its day, as fractional
-- index keys (internal/shared/sortkey): sort_key orders the row on its source
-- account, recipient_sort_key on its recipient account (transfers only), so
-- reordering one account never moves the other, and a move writes one key
-- without renumbering the rest of the day. A higher key is listed higher. The
-- keys themselves are written by the command step that follows
-- (migration:seed-transaction-sort-keys): '' / NULL mean "not placed yet".

ALTER TABLE transactions ADD COLUMN sort_key TEXT NOT NULL DEFAULT '';
ALTER TABLE transactions ADD COLUMN recipient_sort_key TEXT DEFAULT NULL;
