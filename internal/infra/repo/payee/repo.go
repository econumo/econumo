// Package payeerepo implements domain/payee.Repository over the sqlc-generated
// queries, working uniformly across both database engines.
//
// Engine duality, minimized — identical to the tag/category/user repos'
// approach. The whole repository (every method, plus row<->domain mapping) is
// written ONCE here against a single `querier` interface expressed in the
// canonical (sqlite-generated) types. The sqlite adapter (sqlite.go) is a
// near-native passthrough; the pgsql adapter (pgsql.go) is a thin whole-struct
// conversion shim. The engine is chosen once at construction; every query runs
// through TxManager.Querier(ctx) so it transparently joins the active
// transaction.
//
// Idempotency for create-payee is NOT here — it is the shared
// internal/infra/repo/operation.Guard, wired alongside this repo in main.go.
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

// Canonical row/param types: the sqlc-generated types are field-identical across
// engines, so the repo speaks one engine's types (sqlite's) everywhere and the
// pgsql shim copies into them.
type (
	payeeRow     = sqlitegen.Payee
	upsertParams = sqlitegen.UpsertPayeeParams
)

// querier is the engine-agnostic query surface this repo needs, expressed in the
// canonical types. The two engine adapters (sqlite.go / pgsql.go) implement it.
type querier interface {
	GetPayeeByID(ctx context.Context, db backend.DBTX, id string) (payeeRow, error)
	ListPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]payeeRow, error)
	CountPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error)
	UpsertPayee(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeletePayee(ctx context.Context, db backend.DBTX, id string) error
}

// Repo is the concrete payee repository. It holds the TxManager (source of the
// context-bound DBTX) and the engine querier. It satisfies
// domain/payee.Repository.
type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ dompayee.Repository = (*Repo)(nil)

// NewRepo selects the engine querier by driver name, panicking on an unknown
// driver. driver matches config.DatabaseDriver: "sqlite" | "postgresql".
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

// NewSQLiteRepo builds a payee repository backed by the sqlite queries.
func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}}
}

// NewPgsqlRepo builds a payee repository backed by the pgsql queries.
func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// NextIdentity allocates a fresh payee id.
func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

// GetByID loads a payee by id.
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

// ListByOwner returns the owner's payees ordered by position.
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

// CountByOwner returns the number of payees the owner has.
func (r *Repo) CountByOwner(ctx context.Context, userID vo.Id) (int, error) {
	n, err := r.q.CountPayeesByOwner(ctx, r.db(ctx), userID.String())
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Save upserts a payee. The caller runs this inside TxManager.WithTx.
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

// Delete removes a payee by id.
func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeletePayee(ctx, r.db(ctx), id.String())
}

// hydrate reconstitutes a Payee aggregate from a row.
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
