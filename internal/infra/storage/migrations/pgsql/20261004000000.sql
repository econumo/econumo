-- See the sqlite sibling for why the side is stored and how it is backfilled.
ALTER TABLE budgets_folders ADD COLUMN side TEXT NOT NULL DEFAULT 'expense';

UPDATE budgets_folders SET side = 'income'
WHERE EXISTS (
    SELECT 1 FROM budgets_elements e
    WHERE e.folder_id = budgets_folders.id AND e.type IN (3, 4)
);
