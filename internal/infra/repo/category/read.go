// CQRS read side for the category module. CategoryListView runs a purpose-built
// read query and returns the app-layer view-row type directly (so it satisfies
// app/category.ReadModel with no bridging adapter). It formats the timestamps
// into the API datetime form "2006-01-02 15:04:05" here, at the edge of
// persistence, so the app layer receives already-shaped strings.
//
// Reads run through TxManager.Querier(ctx), so a read issued inside a WithTx
// sees that transaction.
package categoryrepo

import (
	"context"

	appcategory "github.com/econumo/econumo/internal/app/category"
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
	GetCategoryListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Category, error)
}

// ReadRepo is the category read model. Separate from the write Repo to keep the
// CQRS boundary explicit; shares the TxManager + driver-selection. It satisfies
// app/category.ReadModel.
type ReadRepo struct {
	tx *backend.TxManager
	q  readQuerier
}

var _ appcategory.ReadModel = (*ReadRepo)(nil)

// NewReadRepo selects the engine read querier by driver name.
func NewReadRepo(driver string, tx *backend.TxManager) *ReadRepo {
	switch driver {
	case "sqlite":
		return &ReadRepo{tx: tx, q: sqliteReadQuerier{}}
	case "postgresql":
		return &ReadRepo{tx: tx, q: pgsqlReadQuerier{}}
	default:
		panic("categoryrepo: unknown database driver " + driver)
	}
}

func (r *ReadRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

// CategoryListView returns all the user's categories ordered by position, with
// timestamps pre-formatted in the API datetime form.
func (r *ReadRepo) CategoryListView(ctx context.Context, userID string) ([]appcategory.CategoryViewRow, error) {
	rows, err := r.q.GetCategoryListView(ctx, r.db(ctx), userID)
	if err != nil {
		return nil, err
	}
	out := make([]appcategory.CategoryViewRow, 0, len(rows))
	for _, c := range rows {
		out = append(out, appcategory.CategoryViewRow{
			ID:         c.ID,
			UserID:     c.UserID,
			Name:       c.Name,
			Position:   c.Position,
			Type:       c.Type,
			Icon:       c.Icon,
			IsArchived: c.IsArchived,
			CreatedAt:  c.CreatedAt.Format(apiDatetimeLayout),
			UpdatedAt:  c.UpdatedAt.Format(apiDatetimeLayout),
		})
	}
	return out, nil
}

// --- engine adapters -------------------------------------------------------

type sqliteReadQuerier struct{}

func (sqliteReadQuerier) GetCategoryListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Category, error) {
	// own + shared: the query repeats the user id positionally (own user_id, and
	// again in the shared-owners subquery), so sqlc generates a two-field param.
	return sqlitegen.New(db).GetCategoryListView(ctx, sqlitegen.GetCategoryListViewParams{UserID: userID, UserID_2: userID})
}

type pgsqlReadQuerier struct{}

func (pgsqlReadQuerier) GetCategoryListView(ctx context.Context, db backend.DBTX, userID string) ([]sqlitegen.Category, error) {
	rows, err := pgsqlgen.New(db).GetCategoryListView(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]sqlitegen.Category, len(rows))
	for i, c := range rows {
		out[i] = sqlitegen.Category(c)
	}
	return out, nil
}
