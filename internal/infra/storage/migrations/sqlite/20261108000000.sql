-- Backfill: each existing client is owned by the user of its earliest grant
-- (revoked ones count), so nobody already connected is cut off; that user is
-- the one whose client registered it.
ALTER TABLE oauth_clients ADD COLUMN owner_user_id TEXT DEFAULT NULL REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE oauth_clients ADD COLUMN is_global BOOLEAN DEFAULT '0' NOT NULL;
CREATE INDEX IDX_oauth_clients_owner_user_id ON oauth_clients (owner_user_id);
DROP INDEX IDX_oauth_clients_unused;
CREATE INDEX IDX_oauth_clients_unused ON oauth_clients (created_at) WHERE last_used_at IS NULL AND is_global = false;
UPDATE oauth_clients SET owner_user_id = (
    SELECT g.user_id FROM oauth_grants g WHERE g.client_id = oauth_clients.id
    ORDER BY g.created_at, g.id LIMIT 1);
