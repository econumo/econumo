package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeDSN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sqlite:///abs/path/db.sqlite", "/abs/path/db.sqlite"},
		{"sqlite://relative.sqlite", "relative.sqlite"},
		{"sqlite://./relative/dir/db.sqlite", "./relative/dir/db.sqlite"},
		{"/plain/path/db.sqlite", "/plain/path/db.sqlite"},
		{"relative/plain/db.sqlite", "relative/plain/db.sqlite"},
		{"sqlite://", ""},
	}
	for _, c := range cases {
		if got := normalizeDSN(c.in); got != c.want {
			t.Errorf("normalizeDSN(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Times must be stored and bound as the frozen 'Y-m-d H:i:s' layout: that is
// what legacy rows hold, what the hand-built SQL binds, and the only form
// SQLite's date functions and text comparisons agree on. The driver default is
// time.Time.String(), and a user-supplied _time_format must not change it.
func TestOpen_StoresTimesInFrozenLayout(t *testing.T) {
	for _, suffix := range []string{"", "?_time_format=sqlite", "?_txlock=immediate"} {
		t.Run(suffix, func(t *testing.T) {
			ctx := context.Background()
			dsn := "sqlite://" + filepath.Join(t.TempDir(), "t.sqlite") + suffix
			db, err := New().Open(ctx, dsn)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer db.Close()

			if _, err := db.ExecContext(ctx, `CREATE TABLE t (at DATETIME)`); err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 9, 14, 10, 0, 0, 123456789, time.UTC)
			if _, err := db.ExecContext(ctx, `INSERT INTO t (at) VALUES (?)`, at); err != nil {
				t.Fatal(err)
			}
			var stored string
			var matched int
			if err := db.QueryRowContext(ctx, `SELECT CAST(at AS TEXT), at = ? FROM t`, at).Scan(&stored, &matched); err != nil {
				t.Fatal(err)
			}
			if stored != "2026-09-14 10:00:00" {
				t.Errorf("stored %q, want %q", stored, "2026-09-14 10:00:00")
			}
			if matched != 1 {
				t.Errorf("a bound time.Time does not compare equal to the stored value")
			}
		})
	}
}

func TestOpen_RejectsMalformedDSNQuery(t *testing.T) {
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "t.sqlite") + "?_txlock=%zz"
	if db, err := New().Open(context.Background(), dsn); err == nil {
		db.Close()
		t.Fatal("Open accepted a malformed DSN query")
	}
}

func TestName(t *testing.T) {
	b := New()
	if got := b.Name(); got != Name {
		t.Errorf("Name() = %q, want %q", got, Name)
	}
	if Name != "sqlite" {
		t.Errorf("Name const = %q, want %q", Name, "sqlite")
	}
}

func TestOpen_PragmasAndPing(t *testing.T) {
	b := New()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "t.sqlite")
	db, err := b.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys;").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// An unconfigured Backend (busyTimeoutMS == 0) issues no PRAGMA, so the value
	// stays at the driver default (0).
	var busyTimeout int
	if err := db.QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 0 {
		t.Errorf("busy_timeout = %d, want 0 (driver default when unconfigured)", busyTimeout)
	}

	if err := db.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

// TestOpen_AppliesBusyTimeout verifies SetBusyTimeout (fed from
// cfg.SQLiteBusyTimeout at boot) reaches the connection as a busy_timeout PRAGMA.
func TestOpen_AppliesBusyTimeout(t *testing.T) {
	b := New()
	b.SetBusyTimeout(5000)
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "t.sqlite")
	db, err := b.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var busyTimeout int
	if err := db.QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000 (SetBusyTimeout must apply the PRAGMA)", busyTimeout)
	}
}

func TestOpen_SingleConnectionPool(t *testing.T) {
	b := New()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "t.sqlite")
	db, err := b.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	b := New()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "no", "such", "dir", "t.sqlite")
	_, err := b.Open(context.Background(), dsn)
	if err == nil {
		t.Fatal("Open with a nonexistent parent directory: expected an error, got nil")
	}
}

func TestOpen_UsableConnection(t *testing.T) {
	b := New()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "t.sqlite")
	db, err := b.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY);"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t (id) VALUES (1);"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var id int
	if err := db.QueryRow("SELECT id FROM t;").Scan(&id); err != nil {
		t.Fatalf("select: %v", err)
	}
	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}
}

func TestMigrations_NonEmpty(t *testing.T) {
	b := New()
	migs := b.Migrations()
	if len(migs) == 0 {
		t.Fatal("Migrations() returned no migrations")
	}
	seen := make(map[string]bool, len(migs))
	for _, m := range migs {
		if m.Version == "" {
			t.Error("migration with empty Version")
		}
		// Up and Command are mutually exclusive (backend.Migration's doc comment):
		// a command step (e.g. migration:zero-deleted-accounts) carries no SQL.
		if m.Up == "" && m.Command == "" {
			t.Errorf("migration %s has neither Up SQL nor Command", m.Version)
		}
		if seen[m.Version] {
			t.Errorf("duplicate migration version %s", m.Version)
		}
		seen[m.Version] = true
	}
}
