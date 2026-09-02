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
INSERT INTO import_runs (id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
;

-- name: GetImportRunByID :one
SELECT id, user_id, source_id, provider, params, status, imported_count, matched_count, skipped_count, failed_count, started_at, finished_at
FROM import_runs
WHERE id = $1
;

-- name: UpdateImportRun :exec
UPDATE import_runs
SET status = $1, imported_count = $2, matched_count = $3, skipped_count = $4, failed_count = $5, finished_at = $6
WHERE id = $7
;

-- name: InsertImportTransactionLink :exec
INSERT INTO import_transaction_links (id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
;

-- name: GetImportTransactionLinkByExternalKey :one
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE source_id = $1 AND external_account_id = $2 AND external_transaction_id = $3
;

-- name: ListImportTransactionLinksByTransaction :many
SELECT id, source_id, run_id, event_id, external_account_id, external_transaction_id, transaction_id, status, external_payee, external_description, external_amount, external_currency, external_posted_at, applied_category_id, applied_payee_id, applied_tag_id, applied_rule_id, imported_at
FROM import_transaction_links
WHERE transaction_id = $1
ORDER BY imported_at, id
;
