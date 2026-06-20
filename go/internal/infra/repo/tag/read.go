// CQRS read side for the tag module. TagListView runs a purpose-built read
// query and returns the app-layer view-row type directly (so it satisfies
// app/tag.ReadModel with no bridging adapter). It formats the timestamps into
// the API datetime form "2006-01-02 15:04:05" here, at the edge of persistence,
// so the app layer receives already-shaped strings.
//
// Reads run through TxManager.Querier(ctx), so a read issued inside a WithTx
// sees that transaction.
package tagrepo

import (
	"context"

	apptag "github.com/econumo/econumo/internal/app/tag"
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
	GetTagListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Tag, error)
}

// ReadRepo is the tag read model. Separate from the write Repo to keep the CQRS
// boundary explicit; shares the TxManager + driver-selection. It satisfies
// app/tag.ReadModel.
type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ apptag.ReadModel = (*ReadRepo)(nil)

// NewReadRepo selects the engine read querier by driver name.
func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("tagrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// TagListView returns all the user's tags ordered by position, with timestamps
// pre-formatted in the API datetime form.
func (r *ReadRepo) TagListView(ctx context.Context, userID string) ([]apptag.TagViewRow, error) {
	rows, err := r.q.GetTagListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]apptag.TagViewRow, 0, len(rows))
	for _, t := range rows {
		out = append(out, apptag.TagViewRow{
			ID:         t.ID,
			UserID:     t.UserID,
			Name:       t.Name,
			Position:   t.Position,
			IsArchived: t.IsArchived,
			CreatedAt:  t.CreatedAt.Format(apiDatetimeLayout),
			UpdatedAt:  t.UpdatedAt.Format(apiDatetimeLayout),
		})
	}
	return out, nil
}

// --- engine adapters -------------------------------------------------------

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetTagListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Tag, error) {
	return sqlitegen.New(db).GetTagListView(ctx, userID)
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetTagListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Tag, error) {
	rows, err := pgsqlgen.New(db).GetTagListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.Tag, len(rows))
	for i, t := range rows {
		out[i] = sqlitegen.Tag(t)
	}
	return out, nil
}
