// Package pgsql is the PostgreSQL database backend, using the pure-Go lib/pq
// driver (CGO stays off). It registers itself under the driver name
// "postgresql" via init() and is blank-imported in cmd/econumo.
package pgsql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

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
	db, err := sql.Open("postgres", sanitizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("pgsql open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pgsql ping: %w", err)
	}
	return db, nil
}

// doctrineOnlyParams are DATABASE_URL query parameters that Symfony/Doctrine
// accept but libpq does not understand. lib/pq forwards any unrecognized query
// parameter to the server as a startup parameter; a direct PostgreSQL tolerates
// some, but PgBouncer rejects unknown startup parameters outright
// ("unsupported startup parameter"). Stripping them lets an existing PHP-style
// DATABASE_URL (e.g. ...?serverVersion=17&charset=utf8) work unchanged here.
var doctrineOnlyParams = []string{"serverVersion", "charset", "default_dbname"}

// sanitizeDSN removes the Doctrine-only query parameters from a postgres:// URL,
// leaving genuine libpq parameters (sslmode, application_name, connect_timeout,
// …) intact. A DSN that does not parse as a URL (e.g. libpq key=value form) is
// returned unchanged for lib/pq to handle.
func sanitizeDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Query() == nil {
		return dsn
	}
	q := u.Query()
	stripped := false
	for key := range q {
		for _, bad := range doctrineOnlyParams {
			if strings.EqualFold(key, bad) {
				q.Del(key)
				stripped = true
			}
		}
	}
	if !stripped {
		return dsn
	}
	u.RawQuery = q.Encode()
	return u.String()
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
