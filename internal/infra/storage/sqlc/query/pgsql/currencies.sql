-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = $1 AND user_id IS NULL;

-- name: GetCurrencyIDByCodeForUser :one
SELECT id FROM currencies
WHERE code = $1 AND (user_id IS NULL OR user_id = $2)
ORDER BY (user_id IS NULL) ASC
LIMIT 1;
