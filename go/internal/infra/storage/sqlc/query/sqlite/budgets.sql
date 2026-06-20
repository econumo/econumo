-- Budget module queries (SQLite). Spans the eight budget tables. Money columns
-- (limit amount) are NUMERIC(19,8) -> string; booleans (is_accepted,
-- is_archived) -> bool; SMALLINT/SMALLINT UNSIGNED -> int16 via sqlc.yaml.

-- name: GetBudgetByID :one
SELECT id, currency_id, user_id, name, started_at, created_at, updated_at
FROM budgets
WHERE id = ?;

-- name: ListBudgetsForUser :many
-- Budgets the user owns OR has an access row for. Ordered by created_at for a
-- stable list.
SELECT b.id, b.currency_id, b.user_id, b.name, b.started_at, b.created_at, b.updated_at
FROM budgets b
WHERE b.user_id = ?
   OR b.id IN (SELECT ba.budget_id FROM budgets_access ba WHERE ba.user_id = ?)
ORDER BY b.created_at ASC;

-- name: UpsertBudget :exec
INSERT INTO budgets (id, currency_id, user_id, name, started_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    currency_id = excluded.currency_id,
    name        = excluded.name,
    started_at  = excluded.started_at,
    updated_at  = excluded.updated_at;

-- name: DeleteBudget :exec
DELETE FROM budgets WHERE id = ?;

-- name: ListBudgetExcludedAccountIDs :many
SELECT account_id FROM budgets_excluded_accounts WHERE budget_id = ?;

-- name: AddBudgetExcludedAccount :exec
INSERT INTO budgets_excluded_accounts (budget_id, account_id) VALUES (?, ?)
ON CONFLICT (budget_id, account_id) DO NOTHING;

-- name: RemoveBudgetExcludedAccount :exec
DELETE FROM budgets_excluded_accounts WHERE budget_id = ? AND account_id = ?;

-- name: ListBudgetAccess :many
SELECT budget_id, user_id, role, is_accepted, created_at, updated_at
FROM budgets_access WHERE budget_id = ?;

-- name: GetBudgetAccess :one
SELECT budget_id, user_id, role, is_accepted, created_at, updated_at
FROM budgets_access WHERE budget_id = ? AND user_id = ?;

-- name: UpsertBudgetAccess :exec
INSERT INTO budgets_access (budget_id, user_id, role, is_accepted, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (budget_id, user_id) DO UPDATE SET
    role        = excluded.role,
    is_accepted = excluded.is_accepted,
    updated_at  = excluded.updated_at;

-- name: DeleteBudgetAccess :exec
DELETE FROM budgets_access WHERE budget_id = ? AND user_id = ?;

-- name: ListBudgetFolders :many
SELECT id, budget_id, name, position, created_at, updated_at
FROM budgets_folders WHERE budget_id = ? ORDER BY position ASC;

-- name: GetBudgetFolder :one
SELECT id, budget_id, name, position, created_at, updated_at
FROM budgets_folders WHERE id = ?;

-- name: UpsertBudgetFolder :exec
INSERT INTO budgets_folders (id, budget_id, name, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name       = excluded.name,
    position   = excluded.position,
    updated_at = excluded.updated_at;

-- name: DeleteBudgetFolder :exec
DELETE FROM budgets_folders WHERE id = ?;

-- name: ListBudgetEnvelopes :many
SELECT id, budget_id, name, icon, is_archived, created_at, updated_at
FROM budgets_envelopes WHERE budget_id = ?;

-- name: GetBudgetEnvelope :one
SELECT id, budget_id, name, icon, is_archived, created_at, updated_at
FROM budgets_envelopes WHERE id = ?;

-- name: UpsertBudgetEnvelope :exec
INSERT INTO budgets_envelopes (id, budget_id, name, icon, is_archived, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name        = excluded.name,
    icon        = excluded.icon,
    is_archived = excluded.is_archived,
    updated_at  = excluded.updated_at;

-- name: DeleteBudgetEnvelope :exec
DELETE FROM budgets_envelopes WHERE id = ?;

-- name: ListEnvelopeCategoryIDs :many
SELECT category_id FROM budgets_envelopes_categories WHERE budget_envelope_id = ?;

-- name: AddEnvelopeCategory :exec
INSERT INTO budgets_envelopes_categories (budget_envelope_id, category_id) VALUES (?, ?)
ON CONFLICT (budget_envelope_id, category_id) DO NOTHING;

-- name: RemoveEnvelopeCategory :exec
DELETE FROM budgets_envelopes_categories WHERE budget_envelope_id = ? AND category_id = ?;

-- name: ListBudgetElements :many
SELECT id, budget_id, currency_id, folder_id, external_id, type, created_at, updated_at, position
FROM budgets_elements WHERE budget_id = ?;

-- name: GetBudgetElement :one
SELECT id, budget_id, currency_id, folder_id, external_id, type, created_at, updated_at, position
FROM budgets_elements WHERE id = ?;

-- name: GetBudgetElementByExternal :one
SELECT id, budget_id, currency_id, folder_id, external_id, type, created_at, updated_at, position
FROM budgets_elements WHERE budget_id = ? AND external_id = ?;

-- name: UpsertBudgetElement :exec
INSERT INTO budgets_elements (id, budget_id, currency_id, folder_id, external_id, type, created_at, updated_at, position)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    currency_id = excluded.currency_id,
    folder_id   = excluded.folder_id,
    type        = excluded.type,
    position    = excluded.position,
    updated_at  = excluded.updated_at;

-- name: DeleteBudgetElement :exec
DELETE FROM budgets_elements WHERE id = ?;

-- name: ListBudgetLimitsForPeriod :many
SELECT l.id, l.element_id, l.period, l.created_at, l.updated_at, l.amount
FROM budgets_elements_limits l
JOIN budgets_elements e ON e.id = l.element_id
WHERE e.budget_id = ? AND l.period = ?;

-- name: GetBudgetLimit :one
SELECT id, element_id, period, created_at, updated_at, amount
FROM budgets_elements_limits WHERE element_id = ? AND period = ?;

-- name: UpsertBudgetLimit :exec
INSERT INTO budgets_elements_limits (id, element_id, period, created_at, updated_at, amount)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    amount     = excluded.amount,
    updated_at = excluded.updated_at;

-- name: DeleteBudgetLimit :exec
DELETE FROM budgets_elements_limits WHERE id = ?;

-- name: DeleteBudgetLimitsByBudget :exec
DELETE FROM budgets_elements_limits
WHERE element_id IN (SELECT e.id FROM budgets_elements e WHERE e.budget_id = ?);
