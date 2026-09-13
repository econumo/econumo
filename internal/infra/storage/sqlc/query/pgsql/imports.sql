-- Transaction import: sources, the push-event inbox, runs, and the link
-- ledger. Liveness/tombstone logic lives in Go (model.ImportTransactionLink).

-- name: InsertImportSource :exec
INSERT INTO import_sources (id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
;

-- name: GetImportSourceByID :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE id = $1
;

-- name: InsertImportEvent :execrows
-- The (source_id, payload_hash) unique index makes a re-fired push a no-op;
-- the caller reads the row count to learn whether this payload was new.
INSERT INTO import_events (id, source_id, run_id, payload, payload_hash, status, parse_error, received_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (source_id, payload_hash) DO NOTHING
;

-- name: GetImportEventByID :one
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE id = $1
;

-- name: UpdateImportEventStatus :exec
-- Note: sets run_id too so a processed event records the run that consumed it.
UPDATE import_events SET status = $1, parse_error = $2, run_id = $3 WHERE id = $4
;

-- name: InsertImportRun :exec
INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
;

-- name: GetImportRunByID :one
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at
FROM import_runs
WHERE id = $1
;

-- name: UpdateImportRun :exec
UPDATE import_runs
SET status = $1, imported_count = $2, matched_count = $3, skipped_count = $4, failed_count = $5, queued_count = $6, amounts_updated_count = $7, errors = $8, finished_at = $9
WHERE id = $10
;

-- name: InsertImportTransactionLink :exec
INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
;

-- name: GetImportTransactionLinkByExternalKey :one
-- Card identity is case-insensitive (Apple Wallet may report the same card
-- with different casing between taps), so the account-id half of the key
-- folds case; external_transaction_id stays exact.
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = sqlc.arg(source_id) AND lower(external_account_id) = lower(sqlc.arg(external_account_id)) AND external_transaction_id = sqlc.arg(external_transaction_id)
;

-- name: ListImportTransactionLinksByTransaction :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE transaction_id = $1
ORDER BY imported_at, id
;

-- name: GetImportSourceByUserProvider :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = $1 AND provider = $2
ORDER BY created_at, id
LIMIT 1
;

-- name: ListImportSourcesByUser :many
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = $1
ORDER BY created_at, id
;

-- name: DeleteImportSource :exec
DELETE FROM import_sources WHERE id = $1
;

-- name: InsertImportAccountLink :exec
INSERT INTO import_account_links (id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
;

-- name: UpdateImportAccountLink :exec
UPDATE import_account_links SET external_currency = $1, account_id = $2, mode = $3, updated_at = $4 WHERE id = $5
;

-- name: DeleteImportAccountLink :exec
DELETE FROM import_account_links WHERE id = $1
;

-- name: GetImportAccountLinkByID :one
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE id = $1
;

-- name: ListImportAccountLinksBySource :many
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE source_id = $1
ORDER BY created_at, id
;

-- name: DeleteImportEvent :exec
DELETE FROM import_events WHERE id = $1
;

-- name: ListImportEventsBySourceStatus :many
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE source_id = $1 AND status = $2
ORDER BY received_at DESC, id
;

-- name: GetImportTransactionLinkByID :one
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE id = $1
;

-- name: UpdateImportTransactionLink :exec
UPDATE import_transaction_links
SET run_id = $1, transaction_id = $2, status = $3, external_amount = $4, external_currency = $5, applied_category_id = $6, applied_payee_id = $7, applied_tag_id = $8, applied_rule_id = $9
WHERE id = $10
;

-- name: ListImportTransactionLinksBySource :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = $1
ORDER BY external_posted_at DESC, id
;

-- name: DeleteQueuedImportTransactionLinksByExternalAccount :exec
DELETE FROM import_transaction_links WHERE source_id = sqlc.arg(source_id) AND lower(external_account_id) = lower(sqlc.arg(external_account_id)) AND status = 'queued'
;

-- name: UpdateImportSource :exec
UPDATE import_sources SET name = $1, credential_ciphertext = $2, status = $3, last_synced_at = $4, updated_at = $5 WHERE id = $6
;

-- name: UpsertImportCredentialKey :exec
INSERT INTO import_credential_keys (user_id, wrapped_data_key, kdf, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE SET wrapped_data_key = excluded.wrapped_data_key, kdf = excluded.kdf, updated_at = excluded.updated_at
;

-- name: GetImportCredentialKey :one
SELECT user_id, wrapped_data_key, kdf, created_at, updated_at
FROM import_credential_keys
WHERE user_id = $1
;

-- name: DeleteImportCredentialKey :exec
DELETE FROM import_credential_keys WHERE user_id = $1
;

-- name: ListImportRunsByUser :many
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at
FROM import_runs
WHERE user_id = $1
ORDER BY started_at DESC, id DESC
LIMIT $2
;

-- name: ListImportRunsBySource :many
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, queued_count, amounts_updated_count, trigger, errors, started_at, finished_at
FROM import_runs
WHERE user_id = $1 AND source_id = $2
ORDER BY started_at DESC, id DESC
LIMIT $3
;

-- name: ListImportTransactionLinksByRun :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE run_id = $1
ORDER BY imported_at, id
;

-- name: InsertImportRule :exec
INSERT INTO import_rules (id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: UpdateImportRule :exec
UPDATE import_rules
SET source_id = $1, action = $2, match_field = $3, match_type = $4, match_value = $5, is_case_sensitive = $6, target_category_id = $7, target_payee_id = $8, target_tag_id = $9, priority = $10, updated_at = $11
WHERE id = $12;

-- name: DeleteImportRule :exec
DELETE FROM import_rules WHERE id = $1;

-- name: GetImportRuleByID :one
SELECT id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at
FROM import_rules
WHERE id = $1;

-- name: ListImportRulesByUser :many
SELECT id, user_id, source_id, action, match_field, match_type, match_value, is_case_sensitive, target_category_id, target_payee_id, target_tag_id, priority, created_at, updated_at
FROM import_rules
WHERE user_id = $1
ORDER BY priority, created_at, id;

-- name: DeleteImportRuleLabels :exec
DELETE FROM import_rule_labels WHERE rule_id = $1;

-- name: InsertImportRuleLabel :exec
INSERT INTO import_rule_labels (rule_id, label_id) VALUES ($1, $2);

-- name: ListImportRuleLabels :many
SELECT rule_id, label_id FROM import_rule_labels WHERE rule_id = $1 ORDER BY label_id;

-- name: ListImportRuleLabelsByUser :many
SELECT rl.rule_id, rl.label_id
FROM import_rule_labels rl
JOIN import_rules r ON r.id = rl.rule_id
WHERE r.user_id = $1
ORDER BY rl.rule_id, rl.label_id;

-- name: DeleteImportLinkAppliedLabels :exec
DELETE FROM import_link_applied_labels WHERE link_id = $1;

-- name: InsertImportLinkAppliedLabel :exec
INSERT INTO import_link_applied_labels (link_id, label_id) VALUES ($1, $2);

-- name: ListImportLinkAppliedLabels :many
SELECT link_id, label_id FROM import_link_applied_labels WHERE link_id = $1 ORDER BY label_id;

-- name: ListImportTransactionLinksByUser :many
SELECT l.id, l.source_id, l.run_id, l.event_id, l.external_account_id, l.external_transaction_id, l.transaction_id, l.status, l.external_payee, l.external_description, l.external_amount, l.external_currency, l.external_posted_at, l.applied_category_id, l.applied_payee_id, l.applied_tag_id, l.applied_rule_id, l.imported_at
FROM import_transaction_links l
JOIN import_sources s ON s.id = l.source_id
WHERE s.user_id = $1
ORDER BY l.imported_at, l.id;
