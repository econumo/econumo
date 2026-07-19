// UserBudgetAccess implements the user feature's BudgetAccess port (confirm the
// caller may use a budget before setting it as their default). It is
// intentionally tiny and independent of the full budget module: it probes the
// budgets + budgets_access tables directly rather than importing
// internal/budget. It lives here, not in internal/budget/repo, because it needs
// the user feature's BudgetAccess port type and an infra/feature package must
// not import another feature (see archtest); the CLI composition root
// (internal/cli/container.go) uses this same adapter.
package server

import (
	"context"
	"database/sql"
	"errors"

	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/user"
)

// budgetOwnerProbe returns the budget's owner id and whether the budget exists.
type budgetOwnerProbe func(ctx context.Context, db backend.DBTX, budgetID string) (ownerID string, found bool, err error)

// budgetShareProbe returns whether the user has an accepted share on the budget.
type budgetShareProbe func(ctx context.Context, db backend.DBTX, budgetID, userID string) (accepted bool, err error)

// UserBudgetAccess adapts a direct budgets-table probe to user.BudgetAccess.
type UserBudgetAccess struct {
	tx     *backend.TxManager
	owner  budgetOwnerProbe
	shared budgetShareProbe
}

var _ user.BudgetAccess = (*UserBudgetAccess)(nil)

// NewUserBudgetAccess wires the access probe for the given driver.
func NewUserBudgetAccess(driver string, tx *backend.TxManager) *UserBudgetAccess {
	switch driver {
	case "sqlite":
		return &UserBudgetAccess{
			tx: tx,
			owner: func(ctx context.Context, db backend.DBTX, budgetID string) (string, bool, error) {
				b, err := sqlitegen.New(db).GetBudgetByID(ctx, budgetID)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return "", false, nil
					}
					return "", false, err
				}
				return b.UserID, true, nil
			},
			shared: func(ctx context.Context, db backend.DBTX, budgetID, userID string) (bool, error) {
				a, err := sqlitegen.New(db).GetBudgetAccess(ctx, sqlitegen.GetBudgetAccessParams{BudgetID: budgetID, UserID: userID})
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return false, nil
					}
					return false, err
				}
				return a.IsAccepted, nil
			},
		}
	case "postgresql":
		return &UserBudgetAccess{
			tx: tx,
			owner: func(ctx context.Context, db backend.DBTX, budgetID string) (string, bool, error) {
				b, err := pgsqlgen.New(db).GetBudgetByID(ctx, budgetID)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return "", false, nil
					}
					return "", false, err
				}
				return b.UserID, true, nil
			},
			shared: func(ctx context.Context, db backend.DBTX, budgetID, userID string) (bool, error) {
				a, err := pgsqlgen.New(db).GetBudgetAccess(ctx, pgsqlgen.GetBudgetAccessParams{BudgetID: budgetID, UserID: userID})
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return false, nil
					}
					return false, err
				}
				return a.IsAccepted, nil
			},
		}
	default:
		panic("userbudget: unknown database driver " + driver)
	}
}

// HasAccess reports whether the user owns the budget or holds an accepted share
// on it. A missing budget, or one the user cannot access, maps to (false, nil).
func (l *UserBudgetAccess) HasAccess(ctx context.Context, userID vo.Id, budgetID string) (bool, error) {
	db := l.tx.Querier(ctx)
	ownerID, found, err := l.owner(ctx, db, budgetID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if ownerID == userID.String() {
		return true, nil
	}
	return l.shared(ctx, db, budgetID, userID.String())
}
