-- Each code and grant records the scope its access tokens carry.
ALTER TABLE oauth_authorization_codes ADD COLUMN scope TEXT NOT NULL DEFAULT 'mcp';
ALTER TABLE oauth_grants ADD COLUMN scope TEXT NOT NULL DEFAULT 'mcp';
