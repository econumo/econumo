-- Profile default currency becomes a currency ID (was a code). Resolve each
-- stored code the way reads did: the owner's own custom first, then global.
-- Values already matching a currencies.id are left as-is (double-applied
-- pre-release DBs). Unresolvable codes lose the row: the absent-option USD
-- fallback is the same behavior the dangling code already produced.
UPDATE users_options SET value = COALESCE(
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id = users_options.user_id),
    (SELECT c.id FROM currencies c WHERE c.code = users_options.value AND c.user_id IS NULL)
)
WHERE name = 'currency'
  AND NOT EXISTS (SELECT 1 FROM currencies c2 WHERE c2.id = users_options.value);
DELETE FROM users_options WHERE name = 'currency' AND value IS NULL;
