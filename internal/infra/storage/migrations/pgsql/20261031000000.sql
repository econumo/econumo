-- No-op counterpart to the SQLite access_tokens repair of the same version
-- (see sqlite/20261031000000.sql). PostgreSQL never lost provider / id_token:
-- its 20260901000000 adds scope with ALTER TABLE instead of rebuilding.
--
-- It exists so both engines record the same version set: data:import-sqlite
-- requires the source SQLite's schema_migrations to match the target
-- PostgreSQL's exactly (sqliteimport.sameVersionSet).
SELECT 1;
