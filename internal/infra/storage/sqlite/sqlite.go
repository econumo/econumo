// Package sqlite is the SQLite database backend, using the pure-Go
// modernc.org/sqlite driver (CGO stays off). It registers itself under the
// driver name "sqlite" via init() and is blank-imported in cmd/econumo.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

// Name is the engine name this backend handles.
const Name = "sqlite"

func init() { backend.Register(Name, New()) }

// Backend implements backend.Backend for SQLite.
type Backend struct{ busyTimeoutMS int }

func New() *Backend { return &Backend{} }

func (b *Backend) Name() string { return Name }

// SetBusyTimeout configures the busy_timeout (in milliseconds) applied to the
// connection in Open. Zero (the default) leaves the driver default, so no PRAGMA
// is issued. Boot calls this with cfg.SQLiteBusyTimeout before Open; it satisfies
// backend.BusyTimeoutConfigurer.
func (b *Backend) SetBusyTimeout(ms int) { b.busyTimeoutMS = ms }

// Open opens the SQLite database. The DSN may be the existing
// "sqlite:///abs/path.sqlite" form or a plain file path; both are normalized to
// a path modernc.org/sqlite accepts. SQLite allows a single writer, so the pool
// is capped at one open connection to avoid "database is locked".
func (b *Backend) Open(ctx context.Context, dsn string) (*sql.DB, error) {
	path := normalizeDSN(dsn)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	// Single writer: serialize access through one connection.
	db.SetMaxOpenConns(1)
	// Enforce foreign keys: the schema relies on FK cascade semantics.
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
	}
	// Apply the configured busy_timeout so a writer waits (rather than failing
	// immediately with "database is locked") when the single connection is busy.
	// Like foreign_keys above, this rides the single pinned connection.
	if b.busyTimeoutMS > 0 {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d;", b.busyTimeoutMS)); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite pragma busy_timeout: %w", err)
		}
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	return db, nil
}

func (b *Backend) Migrations() []backend.Migration {
	files := migrations.SQLite()
	out := make([]backend.Migration, len(files))
	for i, f := range files {
		out[i] = backend.Migration{Version: f.Version, Up: f.SQL}
	}
	return out
}

// normalizeDSN converts a sqlite:// DSN to a filesystem path.
//
//	sqlite:///var/db/db.sqlite -> /var/db/db.sqlite
//	sqlite://relative.sqlite   -> relative.sqlite
//	/abs/path.sqlite           -> /abs/path.sqlite (unchanged)
func normalizeDSN(dsn string) string {
	const scheme = "sqlite://"
	if strings.HasPrefix(dsn, scheme) {
		p := strings.TrimPrefix(dsn, scheme)
		// sqlite:///abs -> "/abs"; sqlite://rel -> "rel"
		return p
	}
	return dsn
}
