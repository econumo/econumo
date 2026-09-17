-- no-op on postgresql: currencies_rates.published_at is a native DATE, so the
-- textual form the SQLite counterpart rewrites cannot occur here. The file must
-- still exist: data:import-sqlite refuses to import on any schema_migrations
-- skew between the engines. SELECT 1 is a harmless valid statement.
SELECT 1;
