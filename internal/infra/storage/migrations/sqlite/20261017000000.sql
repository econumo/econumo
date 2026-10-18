-- Categories of the other type: an income transaction or recurring template
-- carrying an expense category, or the reverse, written before the server
-- refused them. The category is cleared and the removal noted in English on a
-- new line of the description. The note is kept whole within the
-- 255-character column; the old text is shortened with an ellipsis instead.
-- Curly quotes and the ellipsis are char() codes so the file stays ASCII.

-- The accounts' transactions revision moves first, while the rows still
-- match, so synced clients refetch them.
UPDATE accounts SET transactions_revision = transactions_revision + 1
WHERE id IN (
    SELECT t.account_id FROM transactions t
    JOIN categories c ON c.id = t.category_id
    WHERE t.type IN (0, 1) AND t.type <> c.type
);

UPDATE transactions
SET description = (
        SELECT CASE
            WHEN rtrim(transactions.description, char(32, 9, 10, 13)) = '' THEN m.note
            WHEN length(rtrim(transactions.description, char(32, 9, 10, 13))) + 1 + length(m.note) <= 255
                THEN rtrim(transactions.description, char(32, 9, 10, 13)) || char(10) || m.note
            ELSE substr(rtrim(transactions.description, char(32, 9, 10, 13)), 1, 253 - length(m.note)) || char(8230, 10) || m.note
        END
        FROM (
            SELECT 'Category ' || char(8220) || c.name || char(8221) || CASE WHEN c.type = 1
                THEN ' removed: an income category can''t classify an expense.'
                ELSE ' removed: an expense category can''t classify income.' END AS note
            FROM categories c WHERE c.id = transactions.category_id
        ) m
    ),
    category_id = NULL,
    updated_at = strftime('%Y-%m-%d %H:%M:%S', 'now')
WHERE type IN (0, 1)
  AND category_id IN (SELECT c.id FROM categories c WHERE c.type <> transactions.type);

UPDATE recurring_transactions
SET description = (
        SELECT CASE
            WHEN rtrim(recurring_transactions.description, char(32, 9, 10, 13)) = '' THEN m.note
            WHEN length(rtrim(recurring_transactions.description, char(32, 9, 10, 13))) + 1 + length(m.note) <= 255
                THEN rtrim(recurring_transactions.description, char(32, 9, 10, 13)) || char(10) || m.note
            ELSE substr(rtrim(recurring_transactions.description, char(32, 9, 10, 13)), 1, 253 - length(m.note)) || char(8230, 10) || m.note
        END
        FROM (
            SELECT 'Category ' || char(8220) || c.name || char(8221) || CASE WHEN c.type = 1
                THEN ' removed: an income category can''t classify an expense.'
                ELSE ' removed: an expense category can''t classify income.' END AS note
            FROM categories c WHERE c.id = recurring_transactions.category_id
        ) m
    ),
    category_id = NULL,
    updated_at = strftime('%Y-%m-%d %H:%M:%S', 'now')
WHERE type IN (0, 1)
  AND category_id IN (SELECT c.id FROM categories c WHERE c.type <> recurring_transactions.type);
