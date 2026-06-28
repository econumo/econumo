-- Balance-correction transaction insert (SQLite). The account module's create
-- (initial non-zero balance) and update (balance change) write a correction
-- transaction so the computed balance matches the requested one. This is a
-- minimal write the account module owns; the full transaction module (later)
-- will own the broader transaction surface.
--
-- A correction is a plain income/expense (type 0 or 1) with no category/payee/
-- tag/recipient. The factory chooses the type from the sign of the correction
-- amount; the stored amount is the absolute value.

-- name: InsertCorrectionTransaction :exec
INSERT INTO transactions (
    id, user_id, account_id, account_recipient_id, category_id, payee_id, tag_id,
    description, created_at, updated_at, spent_at, type, amount, amount_recipient
) VALUES (
    ?, ?, ?, NULL, NULL, NULL, NULL,
    ?, ?, ?, ?, ?, ?, NULL
);
