-- Onboarding is for people who have nothing yet: anyone who already owns an
-- account (deleted ones included) has been using the app and skips it.
UPDATE users_options
SET value = 'completed',
    updated_at = strftime('%Y-%m-%d %H:%M:%S', 'now')
WHERE name = 'onboarding'
  AND value <> 'completed'
  AND user_id IN (SELECT user_id FROM accounts);
