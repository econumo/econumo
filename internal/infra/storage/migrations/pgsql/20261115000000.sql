CREATE TABLE import_suggestions (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    match_field TEXT NOT NULL,
    match_text TEXT NOT NULL,
    category_id UUID DEFAULT NULL,
    payee_id UUID DEFAULT NULL,
    transaction_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    CONSTRAINT FK_import_suggestions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_suggestions_user_status ON import_suggestions (user_id, status);

CREATE TABLE import_suggestion_links (
    suggestion_id UUID NOT NULL,
    link_id UUID NOT NULL,
    PRIMARY KEY (suggestion_id, link_id),
    CONSTRAINT FK_import_suggestion_links_suggestion FOREIGN KEY (suggestion_id) REFERENCES import_suggestions (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_suggestion_links_link FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_suggestion_links_link_id ON import_suggestion_links (link_id);
