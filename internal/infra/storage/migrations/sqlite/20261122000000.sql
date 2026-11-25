ALTER TABLE import_transaction_links ADD COLUMN assigned_by TEXT DEFAULT NULL;
UPDATE import_transaction_links SET assigned_by = 'rule' WHERE applied_rule_id IS NOT NULL AND transaction_id IS NOT NULL;
ALTER TABLE import_suggestions ADD COLUMN payee_name TEXT DEFAULT NULL;
