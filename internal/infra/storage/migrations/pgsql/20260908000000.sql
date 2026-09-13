DROP TABLE IF EXISTS import_rule_labels;
DROP TABLE IF EXISTS import_rules;

CREATE TABLE import_rules (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL,
    source_id UUID DEFAULT NULL,
    action TEXT NOT NULL,
    match_field TEXT NOT NULL,
    match_type TEXT NOT NULL,
    match_value TEXT NOT NULL,
    is_case_sensitive BOOLEAN DEFAULT false NOT NULL,
    target_category_id UUID DEFAULT NULL,
    target_payee_id UUID DEFAULT NULL,
    target_tag_id UUID DEFAULT NULL,
    priority BIGINT DEFAULT 0 NOT NULL,
    created_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    updated_at TIMESTAMP(0) WITHOUT TIME ZONE NOT NULL,
    CONSTRAINT FK_import_rules_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rules_source FOREIGN KEY (source_id) REFERENCES import_sources (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rules_category FOREIGN KEY (target_category_id) REFERENCES categories (id) ON DELETE SET NULL,
    CONSTRAINT FK_import_rules_payee FOREIGN KEY (target_payee_id) REFERENCES payees (id) ON DELETE SET NULL,
    CONSTRAINT FK_import_rules_tag FOREIGN KEY (target_tag_id) REFERENCES tags (id) ON DELETE SET NULL,
    CONSTRAINT CHK_import_rules_skip_has_no_targets CHECK (action = 'classify' OR (target_category_id IS NULL AND target_payee_id IS NULL AND target_tag_id IS NULL))
);
CREATE INDEX IDX_import_rules_user_id_priority ON import_rules (user_id, priority);
CREATE INDEX IDX_import_rules_source_id ON import_rules (source_id);

CREATE TABLE import_rule_labels (
    rule_id UUID NOT NULL,
    label_id UUID NOT NULL,
    PRIMARY KEY (rule_id, label_id),
    CONSTRAINT FK_import_rule_labels_rule FOREIGN KEY (rule_id) REFERENCES import_rules (id) ON DELETE CASCADE,
    CONSTRAINT FK_import_rule_labels_label FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE CASCADE
);
CREATE INDEX IDX_import_rule_labels_label_id ON import_rule_labels (label_id);
