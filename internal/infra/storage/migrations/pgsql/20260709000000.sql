-- Avatars become selectable "<icon>:<color>" values (Material icon name +
-- color slug); the Gravatar URL era ends. Backfill every existing user to the
-- standard brand default.
ALTER TABLE users RENAME COLUMN avatar_url TO avatar;
UPDATE users SET avatar = 'face:fuchsia';
