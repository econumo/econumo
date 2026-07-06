// Package backend defines the database backend contract that lets a single
// Econumo binary talk to either SQLite or PostgreSQL, selected at runtime by
// the engine derived from the DATABASE_URL scheme.
//
// Both concrete backends (the sqlite and pgsql packages) register themselves
// via init() and are blank-imported in cmd/econumo, e.g.:
//
//	import (
//		_ "github.com/econumo/econumo/internal/infra/storage/pgsql"
//		_ "github.com/econumo/econumo/internal/infra/storage/sqlite"
//	)
//
// At startup the program calls Get(cfg.DatabaseDriver) to obtain the matching
// Backend, opens the *sql.DB, runs the backend's Migrations through the custom
// migration runner, and hands the *sql.DB to the savepoint-aware tx manager
// (see tx.go) and the per-engine sqlc Querier.
//
// There is intentionally no Dialect/Rebind layer: per-engine placeholder and
// type differences are resolved by sqlc at codegen time (one *Queries per engine,
// both satisfying a shared Querier interface), so the Backend only supplies the
// right *sql.DB and migration set.
package backend

import (
	"context"
	"database/sql"
	"sort"
	"sync"
)

// DBTX is the execution surface that sqlc-generated code depends on. It is
// satisfied by both *sql.DB and *sql.Tx from database/sql, which is what makes
// the savepoint-aware WithTx (see tx.go) able to swap a transaction in for the
// pooled DB transparently. Its method set mirrors sqlc's generated DBTX exactly.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Migration is a single forward-only schema change. Version is an ordered,
// comparable identifier (e.g. "0001_baseline"); Up is the SQL applied inside a
// transaction by the migration runner. Migrations are dialect-specific and live
// embedded in their owning Backend.
type Migration struct {
	Version string
	Up      string
}

// Backend abstracts a database engine. A Backend knows its driver name, how to
// open a *sql.DB for a DSN, and which embedded migrations to apply. The sqlc
// Querier is engine-specific and obtained separately by the repo layer (one
// *Queries per engine, both satisfying the shared Querier interface), so it is
// deliberately not part of this interface.
type Backend interface {
	// Name returns the engine name (e.g. "sqlite" or "postgresql").
	Name() string
	// Open establishes a *sql.DB for the given DSN. Implementations configure
	// the underlying pure-Go driver (modernc.org/sqlite or lib/pq), apply any
	// connection-pool settings, and verify connectivity. CGO stays off.
	Open(ctx context.Context, dsn string) (*sql.DB, error)
	// Migrations returns this engine's ordered embedded migrations. The custom
	// runner applies any whose Version is not yet recorded in schema_migrations.
	Migrations() []Migration
}

// BusyTimeoutConfigurer is an optional Backend capability for engines with a
// busy-timeout knob (SQLite). Boot applies cfg.SQLiteBusyTimeout via this
// interface before Open when the selected backend implements it; engines
// without the knob (PostgreSQL) simply do not implement it.
type BusyTimeoutConfigurer interface {
	SetBusyTimeout(ms int)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Backend)
)

// Register adds a Backend under its driver name. It is intended to be called
// from a backend package's init(); registering the same name twice panics so
// that wiring mistakes surface at startup rather than silently.
func Register(name string, b Backend) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic("backend: Register called twice for driver " + name)
	}
	if b == nil {
		panic("backend: Register called with nil Backend for driver " + name)
	}
	registry[name] = b
}

// Get returns the Backend registered for the given driver name. The second
// result reports whether a backend was found, mirroring the comma-ok idiom.
func Get(name string) (Backend, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	b, ok := registry[name]
	return b, ok
}

// Registered returns the sorted list of driver names currently registered.
// Useful for error messages when an unknown engine is requested.
func Registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
