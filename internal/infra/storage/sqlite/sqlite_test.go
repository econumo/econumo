package sqlite

import (
	"context"
	"path/filepath"
	"testing"
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

	// The busy-timeout config value (SQLITE_BUSY_TIMEOUT) is not currently wired
	// from cfg into the Backend (the busyTimeoutMS field is never set nor read
	// by Open), so the pragma stays at the driver default regardless of config.
	var busyTimeout int
	if err := db.QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 0 {
		t.Errorf("busy_timeout = %d, want 0 (driver default; Backend does not apply cfg.SQLiteBusyTimeout)", busyTimeout)
	}

	if err := db.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
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
		if m.Up == "" {
			t.Errorf("migration %s has empty Up SQL", m.Version)
		}
		if seen[m.Version] {
			t.Errorf("duplicate migration version %s", m.Version)
		}
		seen[m.Version] = true
	}
}
