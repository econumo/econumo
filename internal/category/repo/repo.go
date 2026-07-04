// Package repo implements category.Repository and the app-layer
// OperationGuard.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domcategory "github.com/econumo/econumo/internal/category"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	categoryRow    = sqlitegen.Category
	upsertParams   = sqlitegen.UpsertCategoryParams
	opRow          = sqlitegen.OperationRequestsID
	insertOpParams = sqlitegen.InsertOperationIdParams
	markOpParams   = sqlitegen.MarkOperationHandledParams
	reassignParams = sqlitegen.ReassignCategoryTransactionsParams
)

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

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domcategory.Repository = (*Repo)(nil)

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

func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}}
}

func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Category, error) {
	row, err := r.q.GetCategoryByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Category not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Category, error) {
	rows, err := r.q.ListCategoriesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*model.Category, 0, len(rows))
	for _, row := range rows {
		c, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountCategoriesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Save: the caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, c *model.Category) error {
	return r.q.UpsertCategory(ctx, r.db(ctx), upsertParams{
		ID:         c.ID.String(),
		UserID:     c.UserID.String(),
		Name:       c.Name,
		Position:   c.Position,
		Type:       c.Type.Int16(),
		Icon:       c.Icon,
		IsArchived: c.IsArchived,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	})
}

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

// OperationGuard (category.OperationGuard): row-based idempotency on
// operation_requests_ids. Claim reads an existing row first (a duplicate
// request); if absent it inserts a fresh, not-yet-handled row. The pre-check +
// insert run inside the caller's tx (or savepoint), so concurrent duplicates
// either see the existing row or collide on the PK insert (surfaced as an error
// and rolled back) — either way only one create wins. The row is the durable
// dedup, valid cross-process.
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

func hydrate(row categoryRow) (*model.Category, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Category{ID: id, UserID: userID, Name: row.Name, Position: row.Position,
		Type: model.CategoryType(row.Type), Icon: row.Icon, IsArchived: row.IsArchived,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
