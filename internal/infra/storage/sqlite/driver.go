package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/econumo/econumo/internal/shared/datetime"
)

// wrappedDriverName is the database/sql driver name this file registers. Both
// production Open (sqlite.go) and dbtest open under this name instead of the
// bare "sqlite" modernc registers, so every connection in the process binds
// time.Time the same way.
const wrappedDriverName = "sqlite-econumo"

func init() {
	sql.Register(wrappedDriverName, &wrapDriver{inner: innerModernSQLiteDriver()})
}

// innerModernSQLiteDriver obtains the driver.Driver modernc.org/sqlite
// registered under "sqlite" in its own init(), so the wrapper delegates to
// the exact singleton instance the process uses (sharing any UDFs/collations
// registered against it), rather than constructing an independent one.
// sql.Open validates only the driver name, not connectivity, so no dial or
// Ping happens here.
func innerModernSQLiteDriver() driver.Driver {
	db, err := sql.Open(Name, ":memory:")
	if err != nil {
		panic("infra/sqlite: open throwaway db to obtain the modernc driver: " + err.Error())
	}
	defer func() { _ = db.Close() }()
	return db.Driver()
}

// checkNamedValue is the wrapper's only behavioral change: a time.Time bind
// value is formatted as the frozen persistence layout (datetime.Layout)
// BEFORE it reaches modernc/sqlite, which otherwise serializes time.Time
// with its String() method ("2006-01-02 15:04:05 +0000 UTC", sometimes with
// fractional seconds). That mismatched-format text sorts differently than
// the bare layout every fixture and legacy row uses, which silently breaks
// lexicographic datetime comparisons (including keyset pagination). Every
// other value is left to database/sql's default converter.
//
// *time.Time is handled too: sqlc emits it for nullable datetime columns
// (access_tokens.expires_at/revoked_at, users_connections_invites.expired_at,
// ...) and repos bind those pointers directly — leaving it to ErrSkip would
// let the default converter dereference the pointer and hand modernc a bare
// time.Time again. A nil pointer stays with the default converter, which
// binds it as SQL NULL.
func checkNamedValue(nv *driver.NamedValue) error {
	switch t := nv.Value.(type) {
	case time.Time:
		nv.Value = t.UTC().Format(datetime.Layout)
		return nil
	case *time.Time:
		if t == nil {
			return driver.ErrSkip
		}
		nv.Value = t.UTC().Format(datetime.Layout)
		return nil
	default:
		return driver.ErrSkip
	}
}

type wrapDriver struct{ inner driver.Driver }

func (d *wrapDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &wrapConn{Conn: c}, nil
}

// wrapConn wraps a modernc/sqlite driver.Conn, re-exposing every optional
// interface the underlying conn implements (checked with a type assertion
// per call, delegate-if-supported / ErrSkip-or-no-op otherwise) so wrapping
// doesn't silently downgrade database/sql's behavior, plus the
// NamedValueChecker that performs the time.Time normalization above.
type wrapConn struct{ driver.Conn }

var (
	_ driver.Conn               = (*wrapConn)(nil)
	_ driver.NamedValueChecker  = (*wrapConn)(nil)
	_ driver.ConnPrepareContext = (*wrapConn)(nil)
	_ driver.ExecerContext      = (*wrapConn)(nil)
	_ driver.QueryerContext     = (*wrapConn)(nil)
	_ driver.ConnBeginTx        = (*wrapConn)(nil)
	_ driver.Pinger             = (*wrapConn)(nil)
	_ driver.SessionResetter    = (*wrapConn)(nil)
	_ driver.Validator          = (*wrapConn)(nil)
)

func (c *wrapConn) CheckNamedValue(nv *driver.NamedValue) error { return checkNamedValue(nv) }

func (c *wrapConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &wrapStmt{Stmt: s}, nil
}

func (c *wrapConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		s, err := pc.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &wrapStmt{Stmt: s}, nil
	}
	return c.Prepare(query)
}

func (c *wrapConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.Conn.(driver.ExecerContext); ok {
		return ec.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *wrapConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.Conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *wrapConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bc, ok := c.Conn.(driver.ConnBeginTx); ok {
		return bc.BeginTx(ctx, opts)
	}
	return c.Conn.Begin()
}

func (c *wrapConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *wrapConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *wrapConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

// wrapStmt wraps a driver.Stmt returned by wrapConn.Prepare/PrepareContext
// with the same delegate-if-supported semantics, plus the NamedValueChecker
// (a caller going through an explicit prepared statement is checked against
// the statement first; database/sql only falls back to the conn's checker
// when the statement doesn't implement one).
type wrapStmt struct{ driver.Stmt }

var (
	_ driver.Stmt              = (*wrapStmt)(nil)
	_ driver.NamedValueChecker = (*wrapStmt)(nil)
	_ driver.StmtExecContext   = (*wrapStmt)(nil)
	_ driver.StmtQueryContext  = (*wrapStmt)(nil)
)

func (s *wrapStmt) CheckNamedValue(nv *driver.NamedValue) error { return checkNamedValue(nv) }

func (s *wrapStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := s.Stmt.(driver.StmtExecContext); ok {
		return ec.ExecContext(ctx, args)
	}
	return nil, driver.ErrSkip
}

func (s *wrapStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := s.Stmt.(driver.StmtQueryContext); ok {
		return qc.QueryContext(ctx, args)
	}
	return nil, driver.ErrSkip
}
