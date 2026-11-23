CREATE TABLE import_transaction_suggestions (
    link_id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL,
    category_id TEXT DEFAULT NULL,
    payee_id TEXT DEFAULT NULL,
    transaction_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT FK_import_transaction_suggestions_link FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_transaction_suggestions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_transaction_suggestions_user_status ON import_transaction_suggestions (user_id, status);
