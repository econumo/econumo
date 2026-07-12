package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"
)

// TestWrappedDriver_TimeBindIsBareLayout pins the driver.go fix: a time.Time
// bind value (including one carrying fractional seconds, as clock.Now()
// produces) must land in SQLite as the bare 19-char persistence layout, not
// modernc's default String() form.
func TestWrappedDriver_TimeBindIsBareLayout(t *testing.T) {
	db, err := sql.Open(wrappedDriverName, "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, ts DATETIME NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	when := time.Date(2026, 6, 15, 12, 0, 0, 123456789, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO t (id, ts) VALUES (1, ?)", when); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got string
	if err := db.QueryRowContext(ctx, "SELECT CAST(ts AS TEXT) FROM t WHERE id = 1").Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if want := "2026-06-15 12:00:00"; got != want {
		t.Fatalf("stored ts = %q (len %d), want %q (len %d)", got, len(got), want, len(want))
	}
}

// TestWrappedDriver_TimeBindThroughPreparedStatement exercises wrapStmt's
// NamedValueChecker (the path a manually prepared statement takes, distinct
// from the direct ExecerContext path the first test covers).
func TestWrappedDriver_TimeBindThroughPreparedStatement(t *testing.T) {
	db, err := sql.Open(wrappedDriverName, "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, ts DATETIME NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	stmt, err := db.PrepareContext(ctx, "INSERT INTO t (id, ts) VALUES (?, ?)")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Cleanup(func() { _ = stmt.Close() })

	when := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if _, err := stmt.ExecContext(ctx, 1, when); err != nil {
		t.Fatalf("stmt exec: %v", err)
	}

	var got string
	if err := db.QueryRowContext(ctx, "SELECT CAST(ts AS TEXT) FROM t WHERE id = 1").Scan(&got); err != nil {
		t.Fatalf("select: %v", err)
	}
	if want := "2026-06-15 12:00:00"; got != want {
		t.Fatalf("stored ts = %q, want %q", got, want)
	}
}

// TestWrappedDriver_NonTimeValuesUnaffected proves checkNamedValue's ErrSkip
// path leaves ordinary values to database/sql's default converter.
func TestWrappedDriver_NonTimeValuesUnaffected(t *testing.T) {
	db, err := sql.Open(wrappedDriverName, "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT NOT NULL, n INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO t (id, name, n) VALUES (?, ?, ?)", 1, "hello", 42); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var name string
	var n int
	if err := db.QueryRowContext(ctx, "SELECT name, n FROM t WHERE id = 1").Scan(&name, &n); err != nil {
		t.Fatalf("select: %v", err)
	}
	if name != "hello" || n != 42 {
		t.Fatalf("got (%q, %d), want (\"hello\", 42)", name, n)
	}
}

// TestWrappedDriver_ConnOptionalInterfaces pins, at runtime, that the *conn
// actually produced by the registered "sqlite-econumo" driver satisfies
// every optional driver interface the wrapper is meant to preserve. This
// complements the compile-time `var _ driver.X = (*wrapConn)(nil)` /
// (*wrapStmt)(nil) assertions in driver.go: those prove wrapConn/wrapStmt
// implement the interfaces; this proves a live connection from the pool is
// actually one of them (i.e. Conn/Raw isn't handing back something else).
func TestWrappedDriver_ConnOptionalInterfaces(t *testing.T) {
	db, err := sql.Open(wrappedDriverName, "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer conn.Close()

	err = conn.Raw(func(dc any) error {
		checks := map[string]bool{
			"driver.Conn":               isDriverConn(dc),
			"driver.NamedValueChecker":  isNamedValueChecker(dc),
			"driver.ConnPrepareContext": isConnPrepareContext(dc),
			"driver.ExecerContext":      isExecerContext(dc),
			"driver.QueryerContext":     isQueryerContext(dc),
			"driver.ConnBeginTx":        isConnBeginTx(dc),
			"driver.Pinger":             isPinger(dc),
			"driver.SessionResetter":    isSessionResetter(dc),
			"driver.Validator":          isValidator(dc),
		}
		for name, ok := range checks {
			if !ok {
				t.Errorf("live conn from %q does not implement %s", wrappedDriverName, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("conn.Raw: %v", err)
	}
}

func isDriverConn(v any) bool         { _, ok := v.(driver.Conn); return ok }
func isNamedValueChecker(v any) bool  { _, ok := v.(driver.NamedValueChecker); return ok }
func isConnPrepareContext(v any) bool { _, ok := v.(driver.ConnPrepareContext); return ok }
func isExecerContext(v any) bool      { _, ok := v.(driver.ExecerContext); return ok }
func isQueryerContext(v any) bool     { _, ok := v.(driver.QueryerContext); return ok }
func isConnBeginTx(v any) bool        { _, ok := v.(driver.ConnBeginTx); return ok }
func isPinger(v any) bool             { _, ok := v.(driver.Pinger); return ok }
func isSessionResetter(v any) bool    { _, ok := v.(driver.SessionResetter); return ok }
func isValidator(v any) bool          { _, ok := v.(driver.Validator); return ok }
