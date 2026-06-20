// CQRS read side for the payee module. PayeeListView runs a purpose-built read
// query and returns the app-layer view-row type directly (so it satisfies
// app/payee.ReadModel with no bridging adapter). It formats the timestamps into
// the API datetime form "2006-01-02 15:04:05" here, at the edge of persistence,
// so the app layer receives already-shaped strings.
//
// Reads run through TxManager.Querier(ctx), so a read issued inside a WithTx
// sees that transaction.
package payeerepo

import (
	"context"

	apppayee "github.com/econumo/econumo/internal/app/payee"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// apiDatetimeLayout is the wire datetime format for createdAt/updatedAt
// (space separator, no timezone).
const apiDatetimeLayout = "2006-01-02 15:04:05"

// readQuerier is the engine-agnostic read surface, expressed in the canonical
// (sqlite-generated) shape. Implemented per engine below.
type readQuerier interface {
	GetPayeeListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Payee, error)
}

// ReadRepo is the payee read model. Separate from the write Repo to keep the
// CQRS boundary explicit; shares the TxManager + driver-selection. It satisfies
// app/payee.ReadModel.
type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ apppayee.ReadModel = (*ReadRepo)(nil)

// NewReadRepo selects the engine read querier by driver name.
func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("payeerepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// PayeeListView returns all the user's payees ordered by position, with
// timestamps pre-formatted in the API datetime form.
func (r *ReadRepo) PayeeListView(ctx context.Context, userID string) ([]apppayee.PayeeViewRow, error) {
	rows, err := r.q.GetPayeeListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]apppayee.PayeeViewRow, 0, len(rows))
	for _, p := range rows {
		out = append(out, apppayee.PayeeViewRow{
			ID:         p.ID,
			UserID:     p.UserID,
			Name:       p.Name,
			Position:   p.Position,
			IsArchived: p.IsArchived,
			CreatedAt:  p.CreatedAt.Format(apiDatetimeLayout),
			UpdatedAt:  p.UpdatedAt.Format(apiDatetimeLayout),
		})
	}
	return out, nil
}

// --- engine adapters -------------------------------------------------------

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetPayeeListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Payee, error) {
	return sqlitegen.New(db).GetPayeeListView(ctx, userID)
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetPayeeListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Payee, error) {
	rows, err := pgsqlgen.New(db).GetPayeeListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.Payee, len(rows))
	for i, p := range rows {
		out[i] = sqlitegen.Payee(p)
	}
	return out, nil
}
