-- The read scope (the share app): a first-party client is seeded from config,
-- and each code and grant records the scope its access tokens carry.
ALTER TABLE oauth_clients ADD COLUMN first_party BOOLEAN DEFAULT '0' NOT NULL;
ALTER TABLE oauth_authorization_codes ADD COLUMN scope TEXT NOT NULL DEFAULT 'mcp';
ALTER TABLE oauth_grants ADD COLUMN scope TEXT NOT NULL DEFAULT 'mcp';
