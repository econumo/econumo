// Package userbudget provides the minimal, read-only budget-existence lookup the
// user module's update-budget use case needs. It is intentionally tiny and
// independent of the full budget module: PHP's BudgetService.updateBudget only
// does an existence-only get (no ownership/access check) before setting the
// user's default budget, so this port exposes exactly that — Exists(id).
//
// When/if the budget module's own repository is the canonical owner of the
// budgets table, this can be folded into it; for now it is the read-only port
// the user service depends on (mirrors currencyrepo.Lookup).
package userbudget

import (
	"context"
	"database/sql"
	"errors"

	appuser "github.com/econumo/econumo/internal/app/user"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// getByID is the engine-agnostic existence-probe closure chosen at construction.
// It returns sql.ErrNoRows when the budget does not exist.
type getByID func(ctx context.Context, db backend.DBTX, id string) error

// Lookup implements app/user.BudgetExistence over the budgets table.
type Lookup struct {
	tx *backend.TxManager
	q  getByID
}

var _ appuser.BudgetExistence = (*Lookup)(nil)

// New selects the engine adapter by driver name. driver matches
// config.DatabaseDriver: "sqlite" | "postgresql".
func New(driver string, tx *backend.TxManager) *Lookup {
	switch driver {
	case "sqlite":
		return &Lookup{tx: tx, q: func(ctx context.Context, db backend.DBTX, id string) error {
			_, err := sqlitegen.New(db).GetBudgetByID(ctx, id)
			return err
		}}
	case "postgresql":
		return &Lookup{tx: tx, q: func(ctx context.Context, db backend.DBTX, id string) error {
			_, err := pgsqlgen.New(db).GetBudgetByID(ctx, id)
			return err
		}}
	default:
		panic("userbudget: unknown database driver " + driver)
	}
}

// Exists reports whether a budget with the given id exists. A sql.ErrNoRows from
// the underlying query maps to (false, nil); any other error propagates.
func (l *Lookup) Exists(ctx context.Context, budgetID string) (bool, error) {
	err := l.q(ctx, l.tx.Querier(ctx), budgetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
