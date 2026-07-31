-- Account sharing handshake: new grants start pending (is_accepted = false) and
-- become effective on accept. Rows existing before this migration predate the
-- handshake and are grandfathered as accepted.
ALTER TABLE accounts_access ADD COLUMN is_accepted BOOLEAN DEFAULT '0' NOT NULL;
UPDATE accounts_access SET is_accepted = true;
