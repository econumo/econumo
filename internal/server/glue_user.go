// UserBudgetExistence implements the user feature's BudgetExistence port
// (confirm a budget id exists before setting it as the user's default). It is
// intentionally tiny and independent of the full budget module: update-budget
// does an existence-only check (no ownership/access check), so this queries
// the budgets table directly rather than importing internal/budget. It lives
// here, not in internal/budget/repo, because it needs the user feature's
// BudgetExistence port type and an infra/feature package must not import
// another feature (see archtest); the CLI composition root
// (internal/cli/container.go) uses this same adapter.
package server

import (
	"context"
	"database/sql"
	"errors"

	currencyrepo "github.com/econumo/econumo/internal/currency/repo"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/user"
)

// UserCurrencyLookup adapts currencyrepo.Lookup to user.CurrencyLookup:
// GetIDByCode is user-aware (own-first-then-global) here, unlike the
// concrete Lookup's global-only GetIDByCode used directly by the CLI and the
// rate provider. Shared by server.BuildAPI and the CLI composition root
// (internal/cli/container.go).
type UserCurrencyLookup struct {
	inner *currencyrepo.Lookup
}

var _ user.CurrencyLookup = (*UserCurrencyLookup)(nil)

// NewUserCurrencyLookup wraps a currencyrepo.Lookup.
func NewUserCurrencyLookup(inner *currencyrepo.Lookup) *UserCurrencyLookup {
	return &UserCurrencyLookup{inner: inner}
}

// GetIDByCode resolves a code preferring the user's own custom currency, then
// a global one.
func (l *UserCurrencyLookup) GetIDByCode(ctx context.Context, userID, code string) (string, error) {
	return l.inner.GetIDByCodeForUser(ctx, userID, code)
}

// DefaultCode returns the fallback currency code (USD).
func (l *UserCurrencyLookup) DefaultCode() string { return l.inner.DefaultCode() }

// getBudgetByID is the existence-probe closure chosen at construction. It
// returns sql.ErrNoRows when the budget does not exist.
type getBudgetByID func(ctx context.Context, db backend.DBTX, id string) error

// UserBudgetExistence adapts a direct budgets-table probe to user.BudgetExistence.
type UserBudgetExistence struct {
	tx *backend.TxManager
	q  getBudgetByID
}

var _ user.BudgetExistence = (*UserBudgetExistence)(nil)

// NewUserBudgetExistence wires the probe for the given driver.
func NewUserBudgetExistence(driver string, tx *backend.TxManager) *UserBudgetExistence {
	switch driver {
	case "sqlite":
		return &UserBudgetExistence{tx: tx, q: func(ctx context.Context, db backend.DBTX, id string) error {
			_, err := sqlitegen.New(db).GetBudgetByID(ctx, id)
			return err
		}}
	case "postgresql":
		return &UserBudgetExistence{tx: tx, q: func(ctx context.Context, db backend.DBTX, id string) error {
			_, err := pgsqlgen.New(db).GetBudgetByID(ctx, id)
			return err
		}}
	default:
		panic("userbudget: unknown database driver " + driver)
	}
}

// Exists reports whether a budget with the given id exists. A sql.ErrNoRows from
// the underlying query maps to (false, nil); any other error propagates.
func (l *UserBudgetExistence) Exists(ctx context.Context, budgetID string) (bool, error) {
	err := l.q(ctx, l.tx.Querier(ctx), budgetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
