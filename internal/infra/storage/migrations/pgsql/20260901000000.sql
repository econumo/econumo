-- Transaction import (docs/superpowers/specs/2026-08-15-transaction-import-design.md, Part 2).
-- Nine tables plus a scope on access tokens. The link ledger is the source of
-- truth for "already seen" — its rows survive the deletion of the transaction
-- they point at (transaction_id goes NULL: a tombstone), so a deleted import
-- is never re-created on the next sync.

CREATE TABLE import_sources
(
    id                    UUID NOT NULL
    , user_id               UUID NOT NULL
    , provider              TEXT NOT NULL
    , name                  TEXT NOT NULL
    , credential_ciphertext TEXT DEFAULT NULL
    , status                TEXT NOT NULL
    , last_synced_at        TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , created_at            TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at            TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_sources_user_id ON import_sources (user_id);

CREATE TABLE import_credential_keys
(
    user_id        UUID NOT NULL
    , key_ciphertext TEXT NOT NULL
    , created_at     TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (user_id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);

CREATE TABLE import_runs
(
    id               UUID NOT NULL
    , user_id          UUID NOT NULL
    , source_id        UUID NOT NULL
    , provider         TEXT NOT NULL
    , params           TEXT NOT NULL
    , status           TEXT NOT NULL
    , imported_count   BIGINT DEFAULT 0 NOT NULL
    , matched_count    BIGINT DEFAULT 0 NOT NULL
    , skipped_count    BIGINT DEFAULT 0 NOT NULL
    , failed_count     BIGINT DEFAULT 0 NOT NULL
    , started_at       TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , finished_at      TIMESTAMP(0) WITHOUT TIME ZONE DEFAULT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_runs_source_id ON import_runs (source_id);

-- Raw push payloads (Apple Wallet). (source_id, payload_hash) is unique so a
-- Shortcut that re-fires the same tap is a no-op at the inbox.
CREATE TABLE import_events
(
    id           UUID NOT NULL
    , source_id    UUID NOT NULL
    , run_id       UUID DEFAULT NULL
    , payload      TEXT NOT NULL
    , payload_hash TEXT NOT NULL
    , status       TEXT NOT NULL
    , parse_error  TEXT DEFAULT NULL
    , received_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (run_id) REFERENCES import_runs (id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX UNIQ_import_events_source_payload ON import_events (source_id, payload_hash);
CREATE INDEX IDX_import_events_run_id ON import_events (run_id);

CREATE TABLE import_account_links
(
    id                  UUID NOT NULL
    , source_id           UUID NOT NULL
    , external_account_id TEXT NOT NULL
    , external_name       TEXT NOT NULL
    , external_currency   TEXT DEFAULT NULL
    , account_id          UUID DEFAULT NULL
    , mode                TEXT NOT NULL
    , created_at          TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at          TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE
    , UNIQUE (source_id, external_account_id)
    , CHECK (mode = 'ignore' OR account_id IS NOT NULL)
);
CREATE INDEX IDX_import_account_links_account_id ON import_account_links (account_id);

-- The ledger. transaction_id is SET NULL (not CASCADE) on transaction delete:
-- the row stays as a tombstone so the external id counts as seen forever.
CREATE TABLE import_transaction_links
(
    id                      UUID NOT NULL
    , source_id               UUID NOT NULL
    , run_id                  UUID DEFAULT NULL
    , event_id                UUID DEFAULT NULL
    , external_account_id     TEXT NOT NULL
    , external_transaction_id TEXT NOT NULL
    , transaction_id          UUID DEFAULT NULL
    , status                  TEXT NOT NULL
    , external_payee          TEXT NOT NULL
    , external_description    TEXT NOT NULL
    , external_amount         NUMERIC(19, 8) NOT NULL
    , external_currency       TEXT DEFAULT NULL
    , external_posted_at      TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , applied_category_id     UUID DEFAULT NULL
    , applied_payee_id        UUID DEFAULT NULL
    , applied_tag_id          UUID DEFAULT NULL
    , applied_rule_id         UUID DEFAULT NULL
    , imported_at             TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (run_id) REFERENCES import_runs (id) ON DELETE SET NULL
    , FOREIGN KEY (event_id) REFERENCES import_events (id) ON DELETE SET NULL
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE SET NULL
    , UNIQUE (source_id, external_account_id, external_transaction_id)
);
CREATE INDEX IDX_import_transaction_links_transaction_id ON import_transaction_links (transaction_id);
CREATE INDEX IDX_import_transaction_links_run_id ON import_transaction_links (run_id);

CREATE TABLE import_rules
(
    id                UUID NOT NULL
    , user_id           UUID NOT NULL
    , source_id         UUID DEFAULT NULL
    , position          BIGINT NOT NULL
    , match_payee       TEXT DEFAULT NULL
    , match_description TEXT DEFAULT NULL
    , match_amount_min  NUMERIC(19, 8) DEFAULT NULL
    , match_amount_max  NUMERIC(19, 8) DEFAULT NULL
    , action            TEXT NOT NULL
    , category_id       UUID DEFAULT NULL
    , payee_id          UUID DEFAULT NULL
    , tag_id            UUID DEFAULT NULL
    , created_at        TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at        TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
    , FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE
    , FOREIGN KEY (category_id) REFERENCES categories (id) ON DELETE SET NULL
    , FOREIGN KEY (payee_id) REFERENCES payees (id) ON DELETE SET NULL
    , FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE SET NULL
    , CHECK (action <> 'skip' OR (category_id IS NULL AND payee_id IS NULL AND tag_id IS NULL))
);
CREATE INDEX IDX_import_rules_user_id ON import_rules (user_id);

CREATE TABLE import_rule_labels
(
    rule_id    UUID NOT NULL
    , label_id   UUID NOT NULL
    , PRIMARY KEY (rule_id, label_id)
    , FOREIGN KEY (rule_id) REFERENCES import_rules (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_rule_labels_label_id ON import_rule_labels (label_id);

CREATE TABLE import_link_applied_labels
(
    link_id    UUID NOT NULL
    , label_id   UUID NOT NULL
    , PRIMARY KEY (link_id, label_id)
    , FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_link_applied_labels_label_id ON import_link_applied_labels (label_id);

-- access_tokens.scope: 'full' (sessions, ordinary PATs) or 'ingest' (a PAT
-- that may only call /api/v1/import/ingest-*). NOT NULL with no default so
-- every writer states the scope explicitly; the default is only a backfill
-- convenience for existing rows and is dropped immediately after.
ALTER TABLE access_tokens ADD COLUMN scope TEXT NOT NULL DEFAULT 'full';
ALTER TABLE access_tokens ALTER COLUMN scope DROP DEFAULT;
