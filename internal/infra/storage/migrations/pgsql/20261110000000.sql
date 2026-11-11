-- Onboarding is for people who have nothing yet: anyone with a transaction
-- has been using the app and skips it. A transaction counts when the user
-- wrote it (on any account, a partner's shared one included) or when it is on
-- an account the user owns, either side, deleted accounts included. Accounts
-- alone do not count: an account created and left empty is no history.
UPDATE users_options
SET value = 'completed',
    updated_at = (now() AT TIME ZONE 'UTC')::timestamp(0)
WHERE name = 'onboarding'
  AND value <> 'completed'
  AND (
    user_id IN (SELECT user_id FROM transactions)
    OR user_id IN (
      SELECT a.user_id FROM accounts a
      WHERE EXISTS (SELECT 1 FROM transactions t WHERE t.account_id = a.id OR t.account_recipient_id = a.id)
    )
  );
