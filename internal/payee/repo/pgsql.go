package repo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
)

type pgsqlQuerier struct{}

var _ querier = pgsqlQuerier{}

func (pgsqlQuerier) GetPayeeByID(ctx context.Context, db backend.DBTX, id string) (payeeRow, error) {
	p, err := pgsqlgen.New(db).GetPayeeByID(ctx, id)
	return payeeRow(p), err
}

func (pgsqlQuerier) ListPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) ([]payeeRow, error) {
	rows, err := pgsqlgen.New(db).ListPayeesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]payeeRow, len(rows))
	for i, p := range rows {
		out[i] = payeeRow(p)
	}
	return out, nil
}

func (pgsqlQuerier) CountPayeesByOwner(ctx context.Context, db backend.DBTX, userID string) (int64, error) {
	return pgsqlgen.New(db).CountPayeesByOwner(ctx, userID)
}

func (pgsqlQuerier) UpsertPayee(ctx context.Context, db backend.DBTX, p upsertParams) error {
	return pgsqlgen.New(db).UpsertPayee(ctx, pgsqlgen.UpsertPayeeParams(p))
}

func (pgsqlQuerier) DeletePayee(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeletePayee(ctx, id)
}

func (pgsqlQuerier) UsageCounts(ctx context.Context, db backend.DBTX, userID string, since time.Time) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT p.id, COUNT(t.id) FROM payees p
		 JOIN transactions t ON t.payee_id = p.id AND t.spent_at >= $1
		 WHERE p.user_id = $2 GROUP BY p.id`, since, userID)
	if err != nil {
		return nil, err
	}
	return scanUsageCounts(rows)
}
