-- Idempotency-Key rows for offline-capable writes: one row per (user, key),
-- storing the id of the resource the first request created. No foreign key:
-- rows are short-lived and purged by age.
CREATE TABLE idempotency_keys
(
    user_id           TEXT     NOT NULL
    , idempotency_key TEXT     NOT NULL
    , resource_id     TEXT
    , created_at      DATETIME NOT NULL
    , PRIMARY KEY (user_id, idempotency_key)
);
CREATE INDEX created_at_idx_idempotency_keys ON idempotency_keys (created_at);
