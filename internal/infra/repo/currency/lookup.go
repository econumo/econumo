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

	domcurrency "github.com/econumo/econumo/internal/domain/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
)

// defaultCode is the base/fallback currency (matches the user module's default).
const defaultCode = "USD"

// currencyViewRow is the canonical (sqlite-generated) GetCurrencyByIDView row.
type currencyViewRow = sqlitegen.GetCurrencyByIDViewRow

// lookupQuerier is the engine-agnostic lookup surface, in the canonical types.
type lookupQuerier interface {
	GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error)
	GetCurrencyByIDView(ctx context.Context, db backend.DBTX, id string) (currencyViewRow, error)
}

// Lookup implements the user feature's CurrencyLookup port over the
// currencies table (structurally — this package cannot import the user
// feature without an infra→feature dependency, so satisfaction is checked at
// the wiring call site in server.BuildAPI, not with a local assertion here).
type Lookup struct {
	tx *backend.TxManager
	q  lookupQuerier
}

// New selects the engine adapter by driver name. driver matches
// config.DatabaseDriver: "sqlite" | "postgresql".
func New(driver string, tx *backend.TxManager) *Lookup {
	switch driver {
	case "sqlite":
		return &Lookup{tx: tx, q: sqliteLookupQuerier{}}
	case "postgresql":
		return &Lookup{tx: tx, q: pgsqlLookupQuerier{}}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
}

// GetIDByCode returns the currency uuid for a code, or a NotFoundError.
func (l *Lookup) GetIDByCode(ctx context.Context, code string) (string, error) {
	id, err := l.q.GetCurrencyIDByCode(ctx, l.tx.Querier(ctx), code)
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
	row, err := l.q.GetCurrencyByIDView(ctx, l.tx.Querier(ctx), id)
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

// sqliteLookupQuerier is the native passthrough.
type sqliteLookupQuerier struct{}

var _ lookupQuerier = sqliteLookupQuerier{}

func (sqliteLookupQuerier) GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return sqlitegen.New(db).GetCurrencyIDByCode(ctx, code)
}

func (sqliteLookupQuerier) GetCurrencyByIDView(ctx context.Context, db backend.DBTX, id string) (currencyViewRow, error) {
	return sqlitegen.New(db).GetCurrencyByIDView(ctx, id)
}

// pgsqlLookupQuerier is the thin whole-struct conversion shim.
type pgsqlLookupQuerier struct{}

var _ lookupQuerier = pgsqlLookupQuerier{}

func (pgsqlLookupQuerier) GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return pgsqlgen.New(db).GetCurrencyIDByCode(ctx, code)
}

func (pgsqlLookupQuerier) GetCurrencyByIDView(ctx context.Context, db backend.DBTX, id string) (currencyViewRow, error) {
	row, err := pgsqlgen.New(db).GetCurrencyByIDView(ctx, id)
	return currencyViewRow(row), err
}
