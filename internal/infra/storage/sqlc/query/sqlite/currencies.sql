-- name: GetCurrencyIDByCode :one
SELECT id FROM currencies WHERE code = ?;
