package instance_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

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
