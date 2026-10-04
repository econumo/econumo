-- A folder's side (income/expense) used to be derived from its members, so an
-- empty income folder read as expense and fell into the wrong area of the
-- plan view. The side is now stored. Existing folders take the side of their
-- members: element types 3 (income category) and 4 (income envelope) are the
-- income side; a folder with none of them, or with no members, is expense.
ALTER TABLE budgets_folders ADD COLUMN side TEXT DEFAULT 'expense' NOT NULL;

UPDATE budgets_folders SET side = 'income'
WHERE EXISTS (
    SELECT 1 FROM budgets_elements e
    WHERE e.folder_id = budgets_folders.id AND e.type IN (3, 4)
);
