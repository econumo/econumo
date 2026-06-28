-- no-op on postgresql: uuid columns already map to Go string in sqlc; this
-- version exists only to keep the migration sequence identical to sqlite,
-- where it converts CHAR(n) -> TEXT. SELECT 1 is a harmless valid statement.
SELECT 1;
