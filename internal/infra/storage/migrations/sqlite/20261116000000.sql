-- The billing poll reads affiliate options by created_at; a partial index keeps
-- it off the rest of users_options.
CREATE INDEX affiliate_created_at_idx_users_options ON users_options (created_at, user_id) WHERE name = 'affiliate';
