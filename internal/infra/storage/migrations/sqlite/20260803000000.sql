-- Reporting labels: a second, budget-neutral classification. Unlike tags, a
-- transaction may carry many labels, so the link is a join table rather than a
-- column on transactions. Labels never become budgets_elements rows, which is
-- what keeps their (deliberately overlapping) totals out of envelope math.
CREATE TABLE labels
(
    id            TEXT     NOT NULL
    , user_id     TEXT     NOT NULL
    , name        VARCHAR(64) NOT NULL
    , icon        TEXT     NOT NULL DEFAULT 'label'
    , position    SMALLINT NOT NULL DEFAULT 0
    , is_archived BOOLEAN  NOT NULL DEFAULT 0
    , created_at  DATETIME NOT NULL
    , updated_at  DATETIME NOT NULL
    , PRIMARY KEY (id)
    , FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX user_id_idx_labels ON labels (user_id);

CREATE TABLE transactions_labels
(
    transaction_id TEXT NOT NULL
    , label_id     TEXT NOT NULL
    , PRIMARY KEY (transaction_id, label_id)
    , FOREIGN KEY (transaction_id) REFERENCES transactions (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
-- The PK's leading transaction_id serves the budget aggregation (which joins
-- from the transactions it already walks); this reverse index serves
-- delete-label and filter-by-label.
CREATE INDEX label_id_idx_transactions_labels ON transactions_labels (label_id);

-- Tags gain a persisted icon so the render path reads the stored value
-- everywhere. Existing rows backfill to the previously hardcoded budget-view
-- fallback, so no rendering changes for them.
ALTER TABLE tags ADD COLUMN icon TEXT NOT NULL DEFAULT 'tag';
