// Package pgsql is the PostgreSQL database backend, using the pure-Go lib/pq
// driver (CGO stays off). It registers itself under the driver name
// "postgresql" via init() and is blank-imported in cmd/econumo.
package pgsql

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

// Name is the engine name this backend handles.
const Name = "postgresql"

func init() { backend.Register(Name, New()) }

// Backend implements backend.Backend for PostgreSQL.
type Backend struct{}

// New returns a PostgreSQL backend.
func New() *Backend { return &Backend{} }

// Name returns "postgresql".
func (b *Backend) Name() string { return Name }

// Open opens the PostgreSQL database via lib/pq. The DSN is the standard
// postgres:// URL from config (DATABASE_URL).
func (b *Backend) Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("pgsql open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pgsql ping: %w", err)
	}
	return db, nil
}

// Migrations returns the embedded PostgreSQL migrations.
func (b *Backend) Migrations() []backend.Migration {
	files := migrations.Pgsql()
	out := make([]backend.Migration, len(files))
	for i, f := range files {
		out[i] = backend.Migration{Version: f.Version, Up: f.SQL}
	}
	return out
}
