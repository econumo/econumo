// Package tagrepo implements domain/tag.Repository.
//
// Idempotency for create-tag is NOT here — it is the shared
// internal/infra/repo/operation.Guard, wired alongside this repo.
package tagrepo

import (
	"context"
	"database/sql"
	"errors"

	domtag "github.com/econumo/econumo/internal/domain/tag"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	tagRow       = sqlitegen.Tag
	upsertParams = sqlitegen.UpsertTagParams
)

type querier interface {
	GetTagByID(ctx context.Context, db backend.DBTX, id string) (tagRow, error)
	ListTagsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]tagRow, error)
	CountTagsByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertTag(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteTag(ctx context.Context, db backend.DBTX, id string) error
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domtag.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("tagrepo: unknown database driver " + driver)
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

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*domtag.Tag, error) {
	row, err := r.q.GetTagByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Tag not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*domtag.Tag, error) {
	rows, err := r.q.ListTagsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*domtag.Tag, 0, len(rows))
	for _, row := range rows {
		t, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountTagsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Save: the caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, t *domtag.Tag) error {
	return r.q.UpsertTag(ctx, r.db(ctx), upsertParams{
		ID:         t.Id().String(),
		UserID:     t.UserId().String(),
		Name:       t.Name(),
		Position:   t.Position(),
		IsArchived: t.IsArchived(),
		CreatedAt:  t.CreatedAt(),
		UpdatedAt:  t.UpdatedAt(),
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteTag(ctx, r.db(ctx), id.String())
}

func hydrate(row tagRow) (*domtag.Tag, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return domtag.FromState(
		id, userID, row.Name, row.Position, row.IsArchived, row.CreatedAt, row.UpdatedAt,
	), nil
}
