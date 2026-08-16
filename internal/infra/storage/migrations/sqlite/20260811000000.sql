-- Envelope children were never validated on write, so income-category links
-- could be stored (invisible: reads filter them). Envelope sides become real
-- with the plan view, and every pre-existing envelope is expense-sided, so the
-- never-visible income links are removed. Nothing observable changes.
DELETE FROM budgets_envelopes_categories
WHERE category_id IN (SELECT id FROM categories WHERE type = 1);
