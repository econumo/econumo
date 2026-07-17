-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ? AND user_id IS NULL;

-- name: GetCurrencyIDByCodeForUser :one
SELECT id FROM currencies
WHERE code = ? AND (user_id IS NULL OR user_id = ?)
ORDER BY (user_id IS NULL) ASC
LIMIT 1;
