-- PostgreSQL stores `period` as TIMESTAMP(0) WITHOUT TIME ZONE, so the textual
-- variance the SQLite counterpart fixes cannot occur here, and the unique index
-- on (element_id, period) already forbids the duplicates it collapses. The
-- statement below is therefore a guaranteed no-op.
--
-- The FILE still has to exist: data:import-sqlite compares the two engines'
-- schema_migrations sets and refuses the import on any skew
-- (sqliteimport.ErrSchemaMismatch), so a version present in only one engine
-- would break every import.
DELETE FROM budgets_elements_limits l
USING budgets_elements_limits d
WHERE l.element_id = d.element_id
  AND l.period = d.period
  AND l.id < d.id;
