-- Detach classifications that belong to someone other than the account owner,
-- preserving what they were in the notes.
--
-- Every classification describes the ACCOUNT OWNER's books, and the write path
-- now enforces that. Rows written under the older, laxer rule (which accepted
-- the caller's own category/payee/tag on a shared account) fail that check, so
-- the transaction could not be saved at all -- not even to change its amount --
-- until the offending reference was cleared.
--
-- Clearing is the only sound repair: there is no owner-side entity to remap to
-- (a same-named one is a different thing, and guessing would silently
-- misattribute money). So the NAME is appended to the notes first, and only
-- then is the reference dropped -- otherwise the user loses the one clue about
-- what the row used to be filed under.
--
-- description is VARCHAR(255): the append is skipped when it would overflow,
-- since truncating a user's own note to make room for ours would be worse than
-- not annotating at all.
--
-- Labels need no equivalent: transactions_labels postdates the owner-only rule,
-- so no row was ever written under the laxer one.

-- Note first, detach second. Each pair is ordered so the UPDATE that clears the
-- column runs only after the name it names has been captured.

UPDATE transactions
SET description = CASE WHEN description = '' THEN '' ELSE description || ' ' END
    || '[category: '
    || (SELECT c.name FROM categories c WHERE c.id = transactions.category_id)
    || ']'
WHERE category_id IS NOT NULL
  AND category_id NOT IN (
      SELECT c.id FROM categories c
      JOIN accounts a ON a.id = transactions.account_id
      WHERE c.user_id = a.user_id
  )
  AND length(
      CASE WHEN description = '' THEN '' ELSE description || ' ' END
      || '[category: '
      || (SELECT c.name FROM categories c WHERE c.id = transactions.category_id)
      || ']'
  ) <= 255;

UPDATE transactions
SET category_id = NULL
WHERE category_id IS NOT NULL
  AND category_id NOT IN (
      SELECT c.id FROM categories c
      JOIN accounts a ON a.id = transactions.account_id
      WHERE c.user_id = a.user_id
  );

UPDATE transactions
SET description = CASE WHEN description = '' THEN '' ELSE description || ' ' END
    || '[payee: '
    || (SELECT p.name FROM payees p WHERE p.id = transactions.payee_id)
    || ']'
WHERE payee_id IS NOT NULL
  AND payee_id NOT IN (
      SELECT p.id FROM payees p
      JOIN accounts a ON a.id = transactions.account_id
      WHERE p.user_id = a.user_id
  )
  AND length(
      CASE WHEN description = '' THEN '' ELSE description || ' ' END
      || '[payee: '
      || (SELECT p.name FROM payees p WHERE p.id = transactions.payee_id)
      || ']'
  ) <= 255;

UPDATE transactions
SET payee_id = NULL
WHERE payee_id IS NOT NULL
  AND payee_id NOT IN (
      SELECT p.id FROM payees p
      JOIN accounts a ON a.id = transactions.account_id
      WHERE p.user_id = a.user_id
  );

UPDATE transactions
SET description = CASE WHEN description = '' THEN '' ELSE description || ' ' END
    || '[tag: '
    || (SELECT t.name FROM tags t WHERE t.id = transactions.tag_id)
    || ']'
WHERE tag_id IS NOT NULL
  AND tag_id NOT IN (
      SELECT t.id FROM tags t
      JOIN accounts a ON a.id = transactions.account_id
      WHERE t.user_id = a.user_id
  )
  AND length(
      CASE WHEN description = '' THEN '' ELSE description || ' ' END
      || '[tag: '
      || (SELECT t.name FROM tags t WHERE t.id = transactions.tag_id)
      || ']'
  ) <= 255;

UPDATE transactions
SET tag_id = NULL
WHERE tag_id IS NOT NULL
  AND tag_id NOT IN (
      SELECT t.id FROM tags t
      JOIN accounts a ON a.id = transactions.account_id
      WHERE t.user_id = a.user_id
  );
