-- Balance-correction transaction insert (PostgreSQL: $N placeholders). See the
-- sqlite variant for documentation.

-- name: InsertCorrectionTransaction :exec
INSERT INTO transactions (
    id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
    description, created_at, updated_at, spent_at, type, amount, amount_recipient
) VALUES (
    $1, $2, $3, NULL, NULL, NULL, NULL,
    $4, $5, $6, $7, $8, $9, NULL
);
