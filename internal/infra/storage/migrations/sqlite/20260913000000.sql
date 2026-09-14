-- A fencing token for account reclaim (completing a password reset). Sweeping
-- the credentials a reclaim invalidates is not enough on its own: a request
-- that read its evidence BEFORE the sweep can still write after it — redeeming
-- a sign-in handoff it consumed a moment earlier, or landing an identity from a
-- callback already in flight. Both writes are therefore conditional on the
-- user's generation still being the one the flow started under, which the
-- database evaluates at write time. The reclaim bumps it, so anything in
-- flight lands on 0 rows affected and fails closed.
ALTER TABLE users ADD COLUMN credentials_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE oauth_handoffs ADD COLUMN credentials_generation INTEGER NOT NULL DEFAULT 0;
