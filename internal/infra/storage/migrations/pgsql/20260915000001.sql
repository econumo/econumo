-- no-op on postgresql: datetime columns are native TIMESTAMP(0), so the
-- time.Time.String() text the SQLite counterpart rewrites cannot occur here.
-- The file must still exist: data:import-sqlite refuses to import on any
-- schema_migrations skew between the engines. SELECT 1 is a harmless valid
-- statement.
SELECT 1;
