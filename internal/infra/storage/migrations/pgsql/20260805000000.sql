-- See the sqlite sibling for the semantics. sort_key carries COLLATE "C" for
-- the same reason every other sort_key column does: SQLite compares TEXT
-- byte-wise, so the PostgreSQL column must too or the two engines order the
-- list differently.
CREATE TABLE labels
(
    id            UUID     NOT NULL
    , user_id     UUID     NOT NULL
    , name        VARCHAR(64) NOT NULL
    , icon        TEXT     NOT NULL DEFAULT 'label'
    , sort_key    TEXT COLLATE "C" NOT NULL DEFAULT ''
    , is_archived BOOLEAN  NOT NULL DEFAULT false
    , created_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , updated_at  TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX user_id_idx_labels ON labels (user_id);

CREATE TABLE transactions_labels
(
    transaction_id UUID NOT NULL
    , label_id     UUID NOT NULL
    , PRIMARY KEY (transaction_id, label_id)
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX label_id_idx_transactions_labels ON transactions_labels (label_id);

ALTER TABLE tags ADD COLUMN icon TEXT NOT NULL DEFAULT 'tag';
