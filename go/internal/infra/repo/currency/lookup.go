// Package currencyrepo provides the minimal currency lookup the user module
// needs to resolve the synthetic currency_id option. It is intentionally tiny:
// the full currency module (rates, CRUD) is a later phase. When that module
// lands, this lookup can be folded into it or kept as the read-only port the
// user service depends on.
package currencyrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	appuser "github.com/econumo/econumo/internal/app/user"
	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// defaultCode is the base/fallback currency, matching UserOption::DEFAULT_CURRENCY.
const defaultCode = "USD"

// getIDByCode is the engine-agnostic query closure chosen at construction.
type getIDByCode func(ctx context.Context, db backend.DBTX, code string) (string, error)

// Lookup implements app/user.CurrencyLookup over the currencies table.
type Lookup struct {
	tx     *backend.TxManager
	driver string
	q      getIDByCode
}

var _ appuser.CurrencyLookup = (*Lookup)(nil)

// New selects the engine adapter by driver name. driver matches
// config.DatabaseDriver: "sqlite" | "postgresql".
func New(driver string, tx *backend.TxManager) *Lookup {
	switch driver {
	case "sqlite":
		return &Lookup{tx: tx, driver: driver, q: func(ctx context.Context, db backend.DBTX, code string) (string, error) {
			return sqlitegen.New(db).GetCurrencyIDByCode(ctx, code)
		}}
	case "postgresql":
		return &Lookup{tx: tx, driver: driver, q: func(ctx context.Context, db backend.DBTX, code string) (string, error) {
			return pgsqlgen.New(db).GetCurrencyIDByCode(ctx, code)
		}}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
}

// GetIDByCode returns the currency uuid for a code, or a NotFoundError.
func (l *Lookup) GetIDByCode(ctx context.Context, code string) (string, error) {
	id, err := l.q(ctx, l.tx.Querier(ctx), code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.NewNotFound(fmt.Sprintf("Currency %s not found", code))
		}
		return "", err
	}
	return id, nil
}

// DefaultCode returns the fallback currency code (USD).
func (l *Lookup) DefaultCode() string { return defaultCode }

// CurrencyView is the embeddable currency shape (e.g. for the account result):
// the columns plus the display name resolved from the Intl table.
type CurrencyView struct {
	ID             string
	Code           string
	Name           string
	Symbol         string
	FractionDigits int
}

// GetByID returns the currency for an id, with the display name resolved from
// the Intl table (the stored name is NULL in practice). Missing -> NotFound.
func (l *Lookup) GetByID(ctx context.Context, id string) (CurrencyView, error) {
	db := l.tx.Querier(ctx)
	var (
		row struct {
			ID, Code, Symbol string
			Name             *string
			FractionDigits   int16
		}
		err error
	)
	switch l.driver {
	case "sqlite":
		r, e := sqlitegen.New(db).GetCurrencyByIDView(ctx, id)
		row.ID, row.Code, row.Symbol, row.Name, row.FractionDigits = r.ID, r.Code, r.Symbol, r.Name, r.FractionDigits
		err = e
	default:
		r, e := pgsqlgen.New(db).GetCurrencyByIDView(ctx, id)
		row.ID, row.Code, row.Symbol, row.Name, row.FractionDigits = r.ID, r.Code, r.Symbol, r.Name, r.FractionDigits
		err = e
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CurrencyView{}, errs.NewNotFound("Currency not found")
		}
		return CurrencyView{}, err
	}
	name := ""
	if row.Name != nil {
		name = *row.Name
	}
	if name == "" {
		name = domcurrency.DisplayName(row.Code)
	}
	return CurrencyView{
		ID:             row.ID,
		Code:           row.Code,
		Name:           name,
		Symbol:         row.Symbol,
		FractionDigits: int(row.FractionDigits),
	}, nil
}
