-- The billing poll reads affiliate options by program and created_at; a partial index keeps
-- it off the rest of users_options.
CREATE INDEX affiliate_program_created_at_idx_users_options ON users_options (value, created_at, user_id) WHERE name = 'affiliate';
