package instance_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/infra/instance"
	"github.com/econumo/econumo/internal/test/dbtest"
)

func TestIDIsStableAndDerivedFromTheEarliestMigration(t *testing.T) {
	db := dbtest.New(t) // migrations already applied

	got, err := instance.ID(context.Background(), db.Raw)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	if len(got) != 12 {
		t.Fatalf("id = %q, want 12 hex chars", got)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(got) {
		t.Fatalf("id = %q, want lowercase hex", got)
	}

	again, err := instance.ID(context.Background(), db.Raw)
	if err != nil {
		t.Fatalf("ID rerun: %v", err)
	}
	if again != got {
		t.Fatalf("id changed between calls: %q then %q", got, again)
	}
}

// TestIDMatchesKnownDigest pins the exact algorithm (prefix, separator,
// truncation window), not just its shape. The wantID below is an external
// oracle, computed independently of this package:
//
//	printf 'econumo:instance:v1:20210812210548|2021-08-12 21:05:48' | sha256sum
//	-> 1d820739d37023d3d197673a775261afc025a8ab59ff53ae1b967788894b42dc
//	-> first 12 hex chars: 1d820739d370
//
// It must NEVER be "fixed" by pasting in whatever ID() currently returns:
// TestIDIsStableAndDerivedFromTheEarliestMigration already tolerates any
// algorithm change (it only checks shape and call-to-call stability), so this
// is the only test that would catch an accidental change to the digest
// inputs — which would silently reassign every deployed instance's identity
// on next boot.
func TestIDMatchesKnownDigest(t *testing.T) {
	const wantID = "1d820739d370"

	for _, tc := range []struct {
		name string
		loc  *time.Location
	}{
		{"UTC", time.UTC},
		// Same underlying instant, non-UTC location: pins that normalize()
		// converts to UTC before formatting rather than hashing the wall clock.
		{"non-UTC location, same instant", time.FixedZone("TEST", 5*60*60+30*60)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`CREATE TABLE schema_migrations (version VARCHAR(191) NOT NULL PRIMARY KEY, applied_at TIMESTAMP NOT NULL)`); err != nil {
				t.Fatalf("create: %v", err)
			}
			appliedAt := time.Date(2021, 8, 12, 21, 5, 48, 0, time.UTC).In(tc.loc)
			if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, "20210812210548", appliedAt); err != nil {
				t.Fatalf("insert: %v", err)
			}

			got, err := instance.ID(context.Background(), db)
			if err != nil {
				t.Fatalf("ID: %v", err)
			}
			if got != wantID {
				t.Fatalf("id = %q, want %q", got, wantID)
			}
		})
	}
}

func TestIDIsEmptyWithoutMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version VARCHAR(191) NOT NULL PRIMARY KEY, applied_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := instance.ID(context.Background(), db)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	if got != "" {
		t.Fatalf("id = %q, want empty", got)
	}
}
