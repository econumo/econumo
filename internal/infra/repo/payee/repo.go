// Package payeerepo implements domain/payee.Repository.
//
// Idempotency for create-payee is NOT here — it is the shared
// internal/infra/repo/operation.Guard, wired alongside this repo.
package payeerepo

import (
	"context"
	"database/sql"
	"errors"

	dompayee "github.com/econumo/econumo/internal/domain/payee"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

type (
	payeeRow     = sqlitegen.Payee
	upsertParams = sqlitegen.UpsertPayeeParams
)

type querier interface {
	GetPayeeByID(ctx context.Context, db backend.DBTX, id string) (payeeRow, error)
	ListPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]payeeRow, error)
	CountPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertPayee(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeletePayee(ctx context.Context, db backend.DBTX, id string) error
}

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ dompayee.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("payeerepo: unknown database driver " + driver)
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

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*dompayee.Payee, error) {
	row, err := r.q.GetPayeeByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Payee not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByOwner(ctx context.Context, userID vo.Id) ([]*dompayee.Payee, error) {
	rows, err := r.q.ListPayeesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*dompayee.Payee, 0, len(rows))
	for _, row := range rows {
		p, herr := hydrate(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountPayeesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Save: the caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, p *dompayee.Payee) error {
	return r.q.UpsertPayee(ctx, r.db(ctx), upsertParams{
		ID:         p.Id().String(),
		UserID:     p.UserId().String(),
		Name:       p.Name(),
		Position:   p.Position(),
		IsArchived: p.IsArchived(),
		CreatedAt:  p.CreatedAt(),
		UpdatedAt:  p.UpdatedAt(),
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeletePayee(ctx, r.db(ctx), id.String())
}

func hydrate(row payeeRow) (*dompayee.Payee, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	return dompayee.FromState(
		id, userID, row.Name, row.Position, row.IsArchived, row.CreatedAt, row.UpdatedAt,
	), nil
}
