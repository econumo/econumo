-- See the sqlite sibling for the semantics.
ALTER TABLE users ADD COLUMN credentials_generation BIGINT NOT NULL DEFAULT 0;
ALTER TABLE oauth_handoffs ADD COLUMN credentials_generation BIGINT NOT NULL DEFAULT 0;
