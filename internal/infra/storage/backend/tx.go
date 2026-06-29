package backend

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
)

// TxManager owns a *sql.DB and provides a reentrant, savepoint-aware unit of
// work. It is the single place the app layer goes through to run work inside a
// transaction, so the rest of the code never imports database/sql directly.
//
// The same TxManager type backs both production and tests: in production the
// outermost WithTx does BEGIN/COMMIT/ROLLBACK; in tests the harness opens an
// outer transaction, stuffs it into the context (see ContextWithTx), and the
// service's own WithTx then nests as a SAVEPOINT/RELEASE so the outer rollback
// cleanly undoes everything between tests with no reseed.
type TxManager struct {
	db *sql.DB
}

func NewTxManager(db *sql.DB) *TxManager {
	return &TxManager{db: db}
}

// DB returns the underlying *sql.DB. Intended for wiring (migrations, health
// checks); the repo/app layers should use Querier/WithTx instead.
func (m *TxManager) DB() *sql.DB {
	return m.db
}

// txState holds the active *sql.Tx and a per-transaction monotonic counter used
// to generate unique savepoint names while nested.
type txState struct {
	tx      *sql.Tx
	savepct atomic.Int64
}

type ctxKey struct{}

// ctxWithDBTX returns a child context carrying the given txState as the active
// transaction for this unit of work.
func ctxWithDBTX(ctx context.Context, st *txState) context.Context {
	return context.WithValue(ctx, ctxKey{}, st)
}

// dbtxFromCtx pulls the active txState from ctx, reporting whether one is set.
func dbtxFromCtx(ctx context.Context) (*txState, bool) {
	st, ok := ctx.Value(ctxKey{}).(*txState)
	return st, ok
}

// ContextWithTx injects an externally-managed *sql.Tx as the active transaction
// for ctx. This is the seam the test harness uses: it opens an outer tx, calls
// ContextWithTx, runs the request through the router, and rolls the outer tx
// back at the end. Any WithTx encountered downstream will see the active tx and
// nest via SAVEPOINT instead of opening its own BEGIN.
func ContextWithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return ctxWithDBTX(ctx, &txState{tx: tx})
}

// Querier returns the DBTX bound to the current context: the active *sql.Tx if
// the call is inside a unit of work, otherwise the pooled *sql.DB. The repo
// layer calls this and hands the result to the engine's sqlc constructor
// (e.g. sqlitegen.New(qx) / pgsqlgen.New(qx)) so every query runs on the right
// executor without the repo knowing whether a transaction is in flight.
func (m *TxManager) Querier(ctx context.Context) DBTX {
	if st, ok := dbtxFromCtx(ctx); ok {
		return st.tx
	}
	return m.db
}

// WithTx runs fn inside a transaction, reentrantly.
//
//   - No active tx in ctx: open a new transaction with BeginTx, store it in a
//     child context, run fn, then COMMIT on a nil error or ROLLBACK otherwise.
//   - Active tx already in ctx: issue a uniquely-named SAVEPOINT, run fn, then
//     RELEASE the savepoint on success or ROLLBACK TO it on error. The enclosing
//     transaction stays open and its eventual outcome is decided by the
//     outermost WithTx (or, in tests, by the harness rolling back the outer tx).
//
// SAVEPOINT / RELEASE SAVEPOINT / ROLLBACK TO SAVEPOINT is identical syntax on
// both SQLite and PostgreSQL, so this is fully driver-agnostic.
func (m *TxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if st, ok := dbtxFromCtx(ctx); ok {
		return m.nested(ctx, st, fn)
	}
	return m.top(ctx, fn)
}

func (m *TxManager) top(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	st := &txState{tx: tx}
	txCtx := ctxWithDBTX(ctx, st)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("%w (rollback failed: %v)", err, rbErr)
			}
			return
		}
		if cErr := tx.Commit(); cErr != nil {
			err = fmt.Errorf("commit transaction: %w", cErr)
		}
	}()

	err = fn(txCtx)
	return err
}

func (m *TxManager) nested(ctx context.Context, st *txState, fn func(ctx context.Context) error) (err error) {
	name := fmt.Sprintf("sp_%d", st.savepct.Add(1))
	tx := st.tx

	if _, err = tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return fmt.Errorf("create savepoint %s: %w", name, err)
	}

	defer func() {
		if p := recover(); p != nil {
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name)
			panic(p)
		}
		if err != nil {
			if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name); rbErr != nil {
				err = fmt.Errorf("%w (rollback to savepoint %s failed: %v)", err, name, rbErr)
			}
			return
		}
		if _, rErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name); rErr != nil {
			err = fmt.Errorf("release savepoint %s: %w", name, rErr)
		}
	}()

	// The savepoint shares the same active txState, so further nesting bumps the
	// same counter and produces fresh savepoint names.
	err = fn(ctx)
	return err
}
