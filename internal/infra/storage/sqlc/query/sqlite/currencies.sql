-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ? AND user_id IS NULL;

-- name: GetCurrencyIDByCodeForUser :one
SELECT id FROM currencies
WHERE code = ? AND (user_id IS NULL OR user_id = ?)
ORDER BY (user_id IS NULL) ASC
LIMIT 1;

-- name: GetCurrencyCodeByID :one
-- Maps a stored profile-currency id back to the wire code (the options list
-- is frozen to show codes).
SELECT code FROM currencies WHERE id = ?;
