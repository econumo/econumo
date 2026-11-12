CREATE TABLE import_suggestions (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL,
    match_field TEXT NOT NULL,
    match_text TEXT NOT NULL,
    category_id TEXT DEFAULT NULL,
    payee_id TEXT DEFAULT NULL,
    status TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT FK_import_suggestions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_suggestions_user_status ON import_suggestions (user_id, status);

CREATE TABLE import_suggestion_links (
    suggestion_id TEXT NOT NULL,
    link_id TEXT NOT NULL,
    PRIMARY KEY (suggestion_id, link_id),
    CONSTRAINT FK_import_suggestion_links_suggestion FOREIGN KEY (suggestion_id) REFERENCES import_suggestions (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_suggestion_links_link FOREIGN KEY (link_id) REFERENCES import_transaction_links (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_suggestion_links_link_id ON import_suggestion_links (link_id);
