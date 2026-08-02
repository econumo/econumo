-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = $1 AND user_id IS NULL;

-- name: GetCurrencyIDByCodeForUser :one
-- Deleted rows are excluded: an owner may hold a live and a deleted currency with
-- the same code, and LIMIT 1 would otherwise pick between them arbitrarily.
SELECT id FROM currencies
WHERE code = $1 AND (user_id IS NULL OR user_id = $2) AND is_deleted = false
ORDER BY (user_id IS NULL) ASC
LIMIT 1;

-- name: GetCurrencyCodeByID :one
-- Maps a stored profile-currency id back to the wire code (the options list
-- is frozen to show codes).
SELECT code FROM currencies WHERE id = $1;
