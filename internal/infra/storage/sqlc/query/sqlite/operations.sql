-- Idempotency queries over operation_requests_ids, shared by every module whose
-- create endpoint takes a client-supplied operation id (category, tag, ...). The
-- shared OperationGuard (internal/infra/operation) is built on these.

-- name: InsertOperationId :exec
-- Claim a request id. The PK conflict is detected by the caller via a pre-check
-- (GetOperationId) so a duplicate create is rejected.
INSERT INTO operation_requests_ids (id, is_handled, created_at, updated_at)
VALUES (?, ?, ?, ?)
;

-- name: GetOperationId :one
SELECT id, is_handled, created_at, updated_at
FROM operation_requests_ids
WHERE id = ?
;

-- name: MarkOperationHandled :exec
UPDATE operation_requests_ids SET is_handled = ?, updated_at = ? WHERE id = ?
;
