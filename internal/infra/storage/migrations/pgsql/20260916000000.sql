-- Stage 3 (SimpleFIN): the credential-key table gets its real shape (one
-- wrapped data key + KDF params per user; the stage-1 placeholder column was
-- never written), and runs learn the pull-side counters and per-account errors.
DROP TABLE import_credential_keys
;
CREATE TABLE import_credential_keys
(
    user_id          UUID NOT NULL
    , wrapped_data_key TEXT NOT NULL
    , kdf              TEXT NOT NULL
    , created_at       TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at       TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
)
;
ALTER TABLE import_runs ADD COLUMN queued_count BIGINT DEFAULT 0 NOT NULL
;
ALTER TABLE import_runs ADD COLUMN amounts_updated_count BIGINT DEFAULT 0 NOT NULL
;
ALTER TABLE import_runs ADD COLUMN trigger TEXT DEFAULT 'manual' NOT NULL
;
ALTER TABLE import_runs ADD COLUMN errors TEXT DEFAULT '[]' NOT NULL
;
CREATE INDEX IDX_import_runs_user_started ON import_runs (user_id, started_at)
;
