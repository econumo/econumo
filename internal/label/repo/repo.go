// Package repo implements label.Repository.
//
// Idempotency for create-label is NOT here — it is the shared
// internal/infra/operation.Guard, wired alongside this repo.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	domlabel "github.com/econumo/econumo/internal/label"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// labelRow aliases the sqlc-generated Label model directly: unlike tags, a
// label's icon was part of the table from the start, so every label query
// (write and read side) reuses the same physical column order and there is no
// per-query row type.
type (
	labelRow     = sqlitegen.Label
	upsertParams = sqlitegen.UpsertLabelParams
)

type querier interface {
	GetLabelByID(ctx context.Context, db backend.DBTX, id string) (labelRow, error)
	ListLabelsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]labelRow, error)
	UpsertLabel(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteLabel(ctx context.Context, db backend.DBTX, id string) error
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ domlabel.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("labelrepo: unknown database driver " + driver)
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

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Label, error) {
	row, err := r.q.GetLabelByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Label not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Label, error) {
	rows, err := r.q.ListLabelsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*model.Label, 0, len(rows))
	for _, row := range rows {
		l, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, l)
	}
	return out, nil
}

// Save: the caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, l *model.Label) error {
	return r.q.UpsertLabel(ctx, r.db(ctx), upsertParams{
		ID:         l.ID.String(),
		UserID:     l.UserID.String(),
		Name:       l.Name,
		Icon:       l.Icon,
		SortKey:    string(l.SortKey),
		IsArchived: l.IsArchived,
		CreatedAt:  l.CreatedAt,
		UpdatedAt:  l.UpdatedAt,
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteLabel(ctx, r.db(ctx), id.String())
}

func hydrate(row labelRow) (*model.Label, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Label{ID: id, UserID: userID, Name: row.Name, Icon: row.Icon, SortKey: sortkey.Key(row.SortKey),
		IsArchived: row.IsArchived, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}
