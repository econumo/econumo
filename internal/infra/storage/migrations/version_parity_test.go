package migrations_test

import (
	"testing"

	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

// TestEngineVersionSetsMatch pins that both engines carry the same migration
// versions, even where one engine's file is a no-op.
//
// data:import-sqlite compares the source SQLite's schema_migrations against the
// target PostgreSQL's for EXACT set equality (sqliteimport.sameVersionSet), so a
// version present on one engine only aborts every SQLite -> PostgreSQL import
// with a schema mismatch. An engine that needs no schema change at some version
// therefore gets a no-op file rather than no file.
func TestEngineVersionSetsMatch(t *testing.T) {
	versions := func(files []migrations.File) map[string]bool {
		out := make(map[string]bool, len(files))
		for _, f := range files {
			out[f.Version] = true
		}
		return out
	}
	sqlite, pgsql := versions(migrations.SQLite()), versions(migrations.Pgsql())

	for v := range sqlite {
		if !pgsql[v] {
			t.Errorf("version %s exists for sqlite but not pgsql: add a no-op pgsql/%s.sql", v, v)
		}
	}
	for v := range pgsql {
		if !sqlite[v] {
			t.Errorf("version %s exists for pgsql but not sqlite: add a no-op sqlite/%s.sql", v, v)
		}
	}
}
