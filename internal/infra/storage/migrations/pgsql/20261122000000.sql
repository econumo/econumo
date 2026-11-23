CREATE TABLE import_transaction_suggestions (
    link_id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    category_id UUID DEFAULT NULL,
    payee_id UUID DEFAULT NULL,
    transaction_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    CONSTRAINT FK_import_transaction_suggestions_link FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_transaction_suggestions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_transaction_suggestions_user_status ON import_transaction_suggestions (user_id, status);
