-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ? AND user_id IS NULL;

-- name: GetCurrencyIDByCodeForUser :one
-- Deleted rows are excluded: an owner may hold a live and a deleted currency with
-- the same code, and LIMIT 1 would otherwise pick between them arbitrarily.
SELECT id FROM currencies
WHERE code = ? AND (user_id IS NULL OR user_id = ?) AND is_deleted = 0
ORDER BY (user_id IS NULL) ASC
LIMIT 1;

-- name: GetCurrencyCodeByID :one
-- Maps a stored profile-currency id back to the wire code (the options list
-- is frozen to show codes).
SELECT code FROM currencies WHERE id = ?;
