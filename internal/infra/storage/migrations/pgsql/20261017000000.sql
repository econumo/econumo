-- Categories of the other type: an income transaction or recurring template
-- carrying an expense category, or the reverse, written before the server
-- refused them. The category is cleared and the removal noted in English on a
-- new line of the description. The note is kept whole within the
-- 255-character column; the old text is shortened with an ellipsis instead.
-- Curly quotes and the ellipsis are chr() codes so the file stays ASCII.

-- The accounts' transactions revision moves first, while the rows still
-- match, so synced clients refetch them.
UPDATE accounts SET transactions_revision = transactions_revision + 1
WHERE id IN (
    SELECT t.account_id FROM transactions t
    JOIN categories c ON c.id = t.category_id
    WHERE t.type IN (0, 1) AND t.type <> c.type
);

UPDATE transactions t
SET description = CASE
        WHEN rtrim(t.description, E' \t\n\r') = '' THEN m.note
        WHEN char_length(rtrim(t.description, E' \t\n\r')) + 1 + char_length(m.note) <= 255
            THEN rtrim(t.description, E' \t\n\r') || chr(10) || m.note
        ELSE substr(rtrim(t.description, E' \t\n\r'), 1, 253 - char_length(m.note)) || chr(8230) || chr(10) || m.note
    END,
    category_id = NULL,
    updated_at = (now() AT TIME ZONE 'UTC')::timestamp(0)
FROM (
    SELECT c.id, c.type, 'Category ' || chr(8220) || c.name || chr(8221) || CASE WHEN c.type = 1
        THEN ' removed: an income category can''t classify an expense.'
        ELSE ' removed: an expense category can''t classify income.' END AS note
    FROM categories c
) m
WHERE m.id = t.category_id AND t.type IN (0, 1) AND t.type <> m.type;

UPDATE recurring_transactions r
SET description = CASE
        WHEN rtrim(r.description, E' \t\n\r') = '' THEN m.note
        WHEN char_length(rtrim(r.description, E' \t\n\r')) + 1 + char_length(m.note) <= 255
            THEN rtrim(r.description, E' \t\n\r') || chr(10) || m.note
        ELSE substr(rtrim(r.description, E' \t\n\r'), 1, 253 - char_length(m.note)) || chr(8230) || chr(10) || m.note
    END,
    category_id = NULL,
    updated_at = (now() AT TIME ZONE 'UTC')::timestamp(0)
FROM (
    SELECT c.id, c.type, 'Category ' || chr(8220) || c.name || chr(8221) || CASE WHEN c.type = 1
        THEN ' removed: an income category can''t classify an expense.'
        ELSE ' removed: an expense category can''t classify income.' END AS note
    FROM categories c
) m
WHERE m.id = r.category_id AND r.type IN (0, 1) AND r.type <> m.type;
