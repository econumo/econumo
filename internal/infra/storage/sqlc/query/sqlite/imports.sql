-- Transaction import: sources, the push-event inbox, runs, and the link
-- ledger. Liveness/tombstone logic lives in Go (model.ImportTransactionLink).

-- name: InsertImportSource :exec
INSERT INTO import_sources (id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportSourceByID :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE id = ?;

-- name: InsertImportEvent :execrows
-- The (source_id, payload_hash) unique index makes a re-fired push a no-op;
-- the caller reads the row count to learn whether this payload was new.
INSERT INTO import_events (id, source_id, run_id, payload, payload_hash, status, parse_error, received_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_id, payload_hash) DO NOTHING;

-- name: GetImportEventByID :one
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE id = ?;

-- name: UpdateImportEventStatus :exec
-- Note: sets run_id too so a processed event records the run that consumed it.
UPDATE import_events SET status = ?, parse_error = ?, run_id = ? WHERE id = ?;

-- name: InsertImportRun :exec
INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportRunByID :one
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at
FROM import_runs
WHERE id = ?;

-- name: UpdateImportRun :exec
UPDATE import_runs
SET status = ?, imported_count = ?, matched_count = ?, skipped_count = ?, failed_count = ?, finished_at = ?
WHERE id = ?;

-- name: InsertImportTransactionLink :exec
INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetImportTransactionLinkByExternalKey :one
-- Card identity is case-insensitive (Apple Wallet may report the same card
-- with different casing between taps), so the account-id half of the key
-- folds case; external_transaction_id stays exact.
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = sqlc.arg(source_id) AND lower(external_account_id) = lower(sqlc.arg(external_account_id)) AND external_transaction_id = sqlc.arg(external_transaction_id);

-- name: ListImportTransactionLinksByTransaction :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE transaction_id = ?
ORDER BY imported_at, id;

-- name: GetImportSourceByUserProvider :one
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = ? AND provider = ?
ORDER BY created_at, id
LIMIT 1;

-- name: ListImportSourcesByUser :many
SELECT id, user_id, provider, name, credential_ciphertext, status, last_synced_at, created_at, updated_at
FROM import_sources
WHERE user_id = ?
ORDER BY created_at, id;

-- name: DeleteImportSource :exec
DELETE FROM import_sources WHERE id = ?;

-- name: InsertImportAccountLink :exec
INSERT INTO import_account_links (id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateImportAccountLink :exec
UPDATE import_account_links SET external_currency = ?, account_id = ?, mode = ?, updated_at = ? WHERE id = ?;

-- name: DeleteImportAccountLink :exec
DELETE FROM import_account_links WHERE id = ?;

-- name: GetImportAccountLinkByID :one
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE id = ?;

-- name: ListImportAccountLinksBySource :many
SELECT id, source_id, external_account_id, external_name, external_currency, account_id, mode, created_at, updated_at
FROM import_account_links
WHERE source_id = ?
ORDER BY created_at, id;

-- name: DeleteImportEvent :exec
DELETE FROM import_events WHERE id = ?;

-- name: ListImportEventsBySourceStatus :many
SELECT id, source_id, run_id, payload, payload_hash, status, parse_error, received_at
FROM import_events
WHERE source_id = ? AND status = ?
ORDER BY received_at DESC, id;

-- name: GetImportTransactionLinkByID :one
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE id = ?;

-- name: UpdateImportTransactionLink :exec
UPDATE import_transaction_links
SET run_id = ?, transaction_id = ?, status = ?, external_amount = ?, external_currency = ?, applied_category_id = ?, applied_payee_id = ?, applied_tag_id = ?, applied_rule_id = ?
WHERE id = ?;

-- name: ListImportTransactionLinksBySource :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = ?
ORDER BY external_posted_at DESC, id;

-- name: DeleteQueuedImportTransactionLinksByExternalAccount :exec
DELETE FROM import_transaction_links WHERE source_id = sqlc.arg(source_id) AND lower(external_account_id) = lower(sqlc.arg(external_account_id)) AND status = 'queued';
