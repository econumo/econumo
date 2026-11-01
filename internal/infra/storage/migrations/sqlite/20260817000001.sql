-- No-op. This version used to run migration:zero-deleted-accounts, which writes
-- each correction through the current transaction write path: on a database
-- coming from v1.3.x or older that path names columns later migrations add
-- (recipient_sort_key, ...), so boot failed. The step now runs at
-- 20261101000000, after every schema change it depends on; databases that
-- already ran it here run it again there, which is a no-op (idempotent).
SELECT 1;
