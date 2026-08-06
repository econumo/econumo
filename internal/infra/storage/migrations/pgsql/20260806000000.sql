-- See the sqlite sibling for the semantics.
CREATE TABLE recurring_transactions_labels
(
    recurring_transaction_id UUID NOT NULL
    , label_id               UUID NOT NULL
    , PRIMARY KEY (recurring_transaction_id, label_id)
    , FOREIGN KEY (recurring_transaction_id) REFERENCES recurring_transactions (id) ON DELETE CASCADE
    , FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX label_id_idx_recurring_transactions_labels ON recurring_transactions_labels (label_id);
