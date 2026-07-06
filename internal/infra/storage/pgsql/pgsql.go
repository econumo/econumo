// Package pgsql is the PostgreSQL database backend. It uses the pure-Go pgx
// driver (jackc/pgx) via its database/sql adapter (CGO stays off), configured for
// the SIMPLE query protocol — no server-side prepared statements — so it works
// through PgBouncer transaction/statement pooling. It registers itself under the
// driver name "postgresql" via init() and is blank-imported in cmd/econumo.
package pgsql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	"github.com/econumo/econumo/internal/infra/storage/migrations"
)

// Name is the engine name this backend handles.
const Name = "postgresql"

func init() { backend.Register(Name, New()) }

// Backend implements backend.Backend for PostgreSQL.
type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Name() string { return Name }

// Open opens the PostgreSQL database and verifies connectivity. The DSN is the
// standard postgres:// URL from config (DATABASE_URL).
func (b *Backend) Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := OpenDB(dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pgsql ping: %w", err)
	}
	return db, nil
}

// OpenDB builds a *sql.DB over pgx for the given DSN WITHOUT pinging it (so test
// harnesses can configure the connection before first use). It is shared by the
// production backend and the engine-comparison test harness so both exercise the
// identical driver + protocol mode.
//
// pgx is put in QueryExecModeSimpleProtocol: parameters are sent inline (text)
// and NO server-side prepared statements are used. This is what lets the backend
// run through PgBouncer transaction/statement pooling — extended-protocol prepared
// statements desynchronize across pooled server connections ("bind message has N
// result formats but query has M columns"), so an existing PgBouncer-fronted
// deployment works only with the simple protocol.
func OpenDB(dsn string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(sanitizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("pgsql parse dsn: %w", err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	return stdlib.OpenDB(*cfg), nil
}

// doctrineOnlyParams are DATABASE_URL query parameters that some legacy .env files
// carry but the PostgreSQL wire protocol does not understand. Otherwise they are
// forwarded as startup parameters; a direct PostgreSQL tolerates some, but
// PgBouncer rejects unknown startup parameters outright ("unsupported startup
// parameter"). Stripping them lets such an existing DATABASE_URL
// (e.g. ...?serverVersion=17&charset=utf8) work unchanged here.
var doctrineOnlyParams = []string{"serverVersion", "charset", "default_dbname"}

// sanitizeDSN removes those unsupported query parameters from a postgres:// URL,
// leaving genuine connection parameters (sslmode, application_name, …) intact. A
// DSN that does not parse as a URL is returned unchanged.
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

func (b *Backend) Migrations() []backend.Migration {
	files := migrations.Pgsql()
	out := make([]backend.Migration, len(files))
	for i, f := range files {
		out[i] = backend.Migration{Version: f.Version, Up: f.SQL}
	}
	return out
}
