// Package repo implements tag.Repository.
//
// Idempotency for create-tag is NOT here — it is the shared
// internal/infra/operation.Guard, wired alongside this repo.
package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
	domtag "github.com/econumo/econumo/internal/tag"
)

type (
	tagRow            = sqlitegen.Tag
	upsertParams      = sqlitegen.UpsertTagParams
	reassignTxParams  = sqlitegen.ReassignTagTransactionsParams
	reassignRecParams = sqlitegen.ReassignTagRecurringParams
)

type querier interface {
	GetTagByID(ctx context.Context, db backend.DBTX, id string) (tagRow, error)
	ListTagsByOwner(ctx context.Context, db backend.DBTX, userID string) ([]tagRow, error)
	UpsertTag(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteTag(ctx context.Context, db backend.DBTX, id string) error
	ReassignTagTransactions(ctx context.Context, db backend.DBTX, p reassignTxParams) error
	ReassignTagRecurring(ctx context.Context, db backend.DBTX, p reassignRecParams) error
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

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Tag, error) {
	row, err := r.q.GetTagByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Tag not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*model.Tag, error) {
	rows, err := r.q.ListTagsByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*model.Tag, 0, len(rows))
	for _, row := range rows {
		t, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, t)
	}
	return out, nil
}

// Save: the caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, t *model.Tag) error {
	return r.q.UpsertTag(ctx, r.db(ctx), upsertParams{
		ID:         t.ID.String(),
		UserID:     t.UserID.String(),
		Name:       t.Name,
		Icon:       t.Icon,
		SortKey:    string(t.SortKey),
		IsArchived: t.IsArchived,
		CreatedAt:  t.CreatedAt,
		UpdatedAt:  t.UpdatedAt,
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteTag(ctx, r.db(ctx), id.String())
}

func hydrate(row tagRow) (*model.Tag, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Tag{ID: id, UserID: userID, Name: row.Name, Icon: row.Icon, SortKey: sortkey.Key(row.SortKey),
		IsArchived: row.IsArchived, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

// ReassignTransactions points every transaction on oldID at newID.
func (r *Repo) ReassignTransactions(ctx context.Context, oldID, newID vo.Id) error {
	newStr, oldStr := newID.String(), oldID.String()
	return r.q.ReassignTagTransactions(ctx, r.db(ctx), reassignTxParams{TagID: &newStr, TagID_2: &oldStr})
}

// ReassignRecurring points every recurring template on oldID at newID.
func (r *Repo) ReassignRecurring(ctx context.Context, oldID, newID vo.Id) error {
	newStr, oldStr := newID.String(), oldID.String()
	return r.q.ReassignTagRecurring(ctx, r.db(ctx), reassignRecParams{TagID: &newStr, TagID_2: &oldStr})
}
