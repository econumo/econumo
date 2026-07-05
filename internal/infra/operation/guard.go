// Package operation provides the shared row-based idempotency guard over the
// operation_requests_ids table. Every module whose create endpoint takes a
// client-supplied operation id (category, tag, ...) uses one Guard so the dedup
// logic and queries live in exactly one place.
package operation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	opRow          = sqlitegen.OperationRequestsID
	insertOpParams = sqlitegen.InsertOperationIdParams
	markOpParams   = sqlitegen.MarkOperationHandledParams
)

type querier interface {
	GetOperationId(ctx context.Context, db backend.DBTX, id string) (opRow, error)
	InsertOperationId(ctx context.Context, db backend.DBTX, p insertOpParams) error
	MarkOperationHandled(ctx context.Context, db backend.DBTX, p markOpParams) error
}

// Guard satisfies the per-module app-layer OperationGuard interface (the Claim /
// MarkHandled method set). Every query runs through TxManager.Querier(ctx) so it
// joins the caller's active transaction.
type Guard struct {
	tx *backend.TxManager
	q  querier
}

func NewGuard(driver string, tx *backend.TxManager) *Guard {
	switch driver {
	case "sqlite":
		return &Guard{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Guard{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("operation: unknown database driver " + driver)
	}
}

func (g *Guard) db(ctx context.Context) backend.DBTX { return g.tx.Querier(ctx) }

// Claim records the operation id, reporting already=true if it pre-existed.
//
// It reads an existing row first (a duplicate request); if absent it inserts a
// fresh, not-yet-handled row. The pre-check + insert run inside the caller's tx
// (or savepoint), so concurrent duplicates either see the existing row or
// collide on the PK insert (surfaced as an error and rolled back) — either way
// only one create wins.
func (g *Guard) Claim(ctx context.Context, id vo.Id, now time.Time) (bool, error) {
	db := g.db(ctx)
	if _, err := g.q.GetOperationId(ctx, db, id.String()); err == nil {
		return true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err := g.q.InsertOperationId(ctx, db, insertOpParams{
		ID:        id.String(),
		IsHandled: false,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return false, err
	}
	return false, nil
}

// MarkHandled flips is_handled to true once the operation succeeds.
func (g *Guard) MarkHandled(ctx context.Context, id vo.Id, now time.Time) error {
	return g.q.MarkOperationHandled(ctx, g.db(ctx), markOpParams{
		IsHandled: true,
		UpdatedAt: now,
		ID:        id.String(),
	})
}

type sqliteQuerier struct{}

func (sqliteQuerier) GetOperationId(ctx context.Context, db backend.DBTX, id string) (opRow, error) {
	return sqlitegen.New(db).GetOperationId(ctx, id)
}

func (sqliteQuerier) InsertOperationId(ctx context.Context, db backend.DBTX, p insertOpParams) error {
	return sqlitegen.New(db).InsertOperationId(ctx, p)
}

func (sqliteQuerier) MarkOperationHandled(ctx context.Context, db backend.DBTX, p markOpParams) error {
	return sqlitegen.New(db).MarkOperationHandled(ctx, p)
}

type pgsqlQuerier struct{}

func (pgsqlQuerier) GetOperationId(ctx context.Context, db backend.DBTX, id string) (opRow, error) {
	o, err := pgsqlgen.New(db).GetOperationId(ctx, id)
	return opRow(o), err
}

func (pgsqlQuerier) InsertOperationId(ctx context.Context, db backend.DBTX, p insertOpParams) error {
	return pgsqlgen.New(db).InsertOperationId(ctx, pgsqlgen.InsertOperationIdParams(p))
}

func (pgsqlQuerier) MarkOperationHandled(ctx context.Context, db backend.DBTX, p markOpParams) error {
	return pgsqlgen.New(db).MarkOperationHandled(ctx, pgsqlgen.MarkOperationHandledParams(p))
}
