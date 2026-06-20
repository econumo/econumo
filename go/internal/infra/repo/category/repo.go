// Package categoryrepo implements domain/category.Repository (and the app-layer
// OperationGuard) over the sqlc-generated queries, working uniformly across both
// database engines.
//
// Engine duality, minimized — identical to the user repo's approach. The whole
// repository (every method, plus row<->domain mapping) is written ONCE here
// against a single `querier` interface expressed in the canonical (sqlite-
// generated) types. The sqlite adapter (sqlite.go) is a near-native passthrough;
// the pgsql adapter (pgsql.go) is a thin whole-struct conversion shim. The
// engine is chosen once at construction; every query runs through
// TxManager.Querier(ctx) so it transparently joins the active transaction.
package categoryrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domcategory "github.com/econumo/econumo/internal/domain/category"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// Canonical row/param types: the sqlc-generated types are field-identical across
// engines, so the repo speaks one engine's types (sqlite's) everywhere and the
// pgsql shim copies into them.
type (
	categoryRow    = sqlitegen.Category
	upsertParams   = sqlitegen.UpsertCategoryParams
	opRow          = sqlitegen.OperationRequestsID
	insertOpParams = sqlitegen.InsertOperationIdParams
	markOpParams   = sqlitegen.MarkOperationHandledParams
	reassignParams = sqlitegen.ReassignCategoryTransactionsParams
)

// querier is the engine-agnostic query surface this repo needs, expressed in the
// canonical types. The two engine adapters (sqlite.go / pgsql.go) implement it.
type querier interface {
	GetCategoryByID(ctx context.Context, db backend.DBTX, id string) (categoryRow, error)
	ListCategoriesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]categoryRow, error)
	CountCategoriesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertCategory(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteCategory(ctx context.Context, db backend.DBTX, id string) error
	ReassignCategoryTransactions(ctx context.Context, db backend.DBTX, p reassignParams) error
	GetOperationId(ctx context.Context, db backend.DBTX, id string) (opRow, error)
	InsertOperationId(ctx context.Context, db backend.DBTX, p insertOpParams) error
	MarkOperationHandled(ctx context.Context, db backend.DBTX, p markOpParams) error
}

// Repo is the concrete category repository. It holds the TxManager (source of
// the context-bound DBTX) and the engine querier. It satisfies
// domain/category.Repository and app/category.OperationGuard.
type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domcategory.Repository = (*Repo)(nil)

// NewRepo selects the engine querier by driver name, panicking on an unknown
// driver. driver matches config.DatabaseDriver: "sqlite" | "postgresql".
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("categoryrepo: unknown database driver " + driver)
	}
}

// NewSQLiteRepo builds a category repository backed by the sqlite queries.
func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}}
}

// NewPgsqlRepo builds a category repository backed by the pgsql queries.
func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh category id.
func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads a category by id.
func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*domcategory.Category, error) {
	row, err := r.q.GetCategoryByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Category not found")
		}
		return nil, err
	}
	return hydrate(row)
}

// ListByOwner returns the owner's categories ordered by position.
func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*domcategory.Category, error) {
	rows, err := r.q.ListCategoriesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*domcategory.Category, 0, len(rows))
	for _, row := range rows {
		c, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, c)
	}
	return out, nil
}

// CountByOwner returns the number of categories the owner has.
func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountCategoriesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Save upserts a category. The caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, c *domcategory.Category) error {
	return r.q.UpsertCategory(ctx, r.db(ctx), upsertParams{
		ID:         c.Id().String(),
		UserID:     c.UserId().String(),
		Name:       c.Name(),
		Position:   c.Position(),
		Type:       c.Type().Int16(),
		Icon:       c.Icon(),
		IsArchived: c.IsArchived(),
		CreatedAt:  c.CreatedAt(),
		UpdatedAt:  c.UpdatedAt(),
	})
}

// Delete removes a category by id.
func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteCategory(ctx, r.db(ctx), id.String())
}

// ReassignTransactions points every transaction on oldID at newID.
func (r *Repo) ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error {
	newStr := newID.String()
	oldStr := oldID.String()
	return r.q.ReassignCategoryTransactions(ctx, r.db(ctx), reassignParams{
		CategoryID:   &newStr,
		CategoryID_2: &oldStr,
	})
}

// --- OperationGuard (app/category.OperationGuard) -------------------------
//
// Row-based idempotency on operation_requests_ids. Claim attempts to read an
// existing row first (a duplicate request); if absent it inserts a fresh,
// not-yet-handled row. The pre-check + insert run inside the caller's tx (or
// savepoint), so concurrent duplicates either see the existing row or collide on
// the PK insert (surfaced as an error and rolled back) — either way only one
// create wins. The row is the durable dedup, valid cross-process. See the
// package README.

// Claim records the operation id, reporting already=true if it pre-existed.
func (r *Repo) Claim(ctx context.Context, id vo.Id, now time.Time) (bool, error) {
	db := r.db(ctx)
	if _, err := r.q.GetOperationId(ctx, db, id.String()); err == nil {
		return true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err := r.q.InsertOperationId(ctx, db, insertOpParams{
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
func (r *Repo) MarkHandled(ctx context.Context, id vo.Id, now time.Time) error {
	return r.q.MarkOperationHandled(ctx, r.db(ctx), markOpParams{
		IsHandled: true,
		UpdatedAt: now,
		ID:        id.String(),
	})
}

// hydrate reconstitutes a Category aggregate from a row.
func hydrate(row categoryRow) (*domcategory.Category, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return domcategory.FromState(
		id, userID, row.Name, row.Position, domcategory.Type(row.Type),
		row.Icon, row.IsArchived, row.CreatedAt, row.UpdatedAt,
	), nil
}
