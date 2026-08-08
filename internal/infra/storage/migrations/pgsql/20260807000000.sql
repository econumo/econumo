-- See the sqlite sibling for the semantics.
--
-- PostgreSQL differs in two ways: NOT EXISTS replaces the correlated NOT IN
-- (NOT IN is NULL-poisoned, and the intent here is plainly "no owner-side row
-- matches"), and char_length replaces length.

UPDATE transactions t
SET description = CASE WHEN t.description = '' THEN '' ELSE t.description || ' ' END
    || '[category: ' || c.name || ']'
FROM categories c, accounts a
WHERE c.id = t.category_id
  AND a.id = t.account_id
  AND c.user_id <> a.user_id
  AND char_length(
      CASE WHEN t.description = '' THEN '' ELSE t.description || ' ' END
      || '[category: ' || c.name || ']'
  ) <= 255;

UPDATE transactions t
SET category_id = NULL
WHERE t.category_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM categories c
      JOIN accounts a ON a.id = t.account_id
      WHERE c.id = t.category_id AND c.user_id = a.user_id
  );

UPDATE transactions t
SET description = CASE WHEN t.description = '' THEN '' ELSE t.description || ' ' END
    || '[payee: ' || p.name || ']'
FROM payees p, accounts a
WHERE p.id = t.payee_id
  AND a.id = t.account_id
  AND p.user_id <> a.user_id
  AND char_length(
      CASE WHEN t.description = '' THEN '' ELSE t.description || ' ' END
      || '[payee: ' || p.name || ']'
  ) <= 255;

UPDATE transactions t
SET payee_id = NULL
WHERE t.payee_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM payees p
      JOIN accounts a ON a.id = t.account_id
      WHERE p.id = t.payee_id AND p.user_id = a.user_id
  );

UPDATE transactions t
SET description = CASE WHEN t.description = '' THEN '' ELSE t.description || ' ' END
    || '[tag: ' || g.name || ']'
FROM tags g, accounts a
WHERE g.id = t.tag_id
  AND a.id = t.account_id
  AND g.user_id <> a.user_id
  AND char_length(
      CASE WHEN t.description = '' THEN '' ELSE t.description || ' ' END
      || '[tag: ' || g.name || ']'
  ) <= 255;

UPDATE transactions t
SET tag_id = NULL
WHERE t.tag_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM tags g
      JOIN accounts a ON a.id = t.account_id
      WHERE g.id = t.tag_id AND g.user_id = a.user_id
  );
