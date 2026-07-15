// Package repo is the currency module's persistence layer: the minimal
// read-only Lookup the user module needs to resolve the synthetic
// currency_id option, the CQRS read side (read.go), the CLI write side
// (write.go), the Convertor rate provider (convertor_provider.go), and the
// Open Exchange Rates client (openexchangerates.go).
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	domcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
)

// defaultCode is the base/fallback currency (matches the user module's default).
const defaultCode = "USD"

// currencyViewRow is the canonical (sqlite-generated) GetCurrencyByIDView row.
type currencyViewRow = sqlitegen.GetCurrencyByIDViewRow

// currencyRecordRow (the canonical GetCurrencyRecord row) is declared in
// manage.go, shared package-wide.

// lookupQuerier is the engine-agnostic lookup surface, in the canonical types.
type lookupQuerier interface {
	GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error)
	GetCurrencyIDByCodeForUser(ctx context.Context, db backend.DBTX, code, userID string) (string, error)
	GetCurrencyByIDView(ctx context.Context, db backend.DBTX, id string) (currencyViewRow, error)
	GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error)
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

// GetIDByCodeForUser resolves a code preferring the user's own custom
// currency, then a global one. Foreign customs never resolve (the generated
// query's ORDER BY places the user's own row first when both exist).
func (l *Lookup) GetIDByCodeForUser(ctx context.Context, userID, code string) (string, error) {
	id, err := l.q.GetCurrencyIDByCodeForUser(ctx, l.tx.Querier(ctx), code, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.NewNotFound(fmt.Sprintf("Currency %s not found", code))
		}
		return "", err
	}
	return id, nil
}

// EnsureUsable reports whether the user may denominate new entities in the
// currency: global, or their own non-archived custom. A foreign custom or an
// own archived custom is rejected with a field-level validation error; a
// missing currency id is NotFound.
func (l *Lookup) EnsureUsable(ctx context.Context, userID, currencyID string) error {
	row, err := l.q.GetCurrencyRecord(ctx, l.tx.Querier(ctx), currencyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewNotFound("Currency not found")
		}
		return err
	}
	if row.UserID == nil {
		return nil
	}
	if *row.UserID == userID && !row.IsArchived {
		return nil
	}
	return errs.NewValidation("Validation failed",
		errs.FieldError{Key: "currencyId", Message: "Currency is not available"})
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

func (sqliteLookupQuerier) GetCurrencyIDByCodeForUser(ctx context.Context, db backend.DBTX, code, userID string) (string, error) {
	return sqlitegen.New(db).GetCurrencyIDByCodeForUser(ctx, sqlitegen.GetCurrencyIDByCodeForUserParams{Code: code, UserID: &userID})
}

func (sqliteLookupQuerier) GetCurrencyByIDView(ctx context.Context, db backend.DBTX, id string) (currencyViewRow, error) {
	return sqlitegen.New(db).GetCurrencyByIDView(ctx, id)
}

func (sqliteLookupQuerier) GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error) {
	return sqlitegen.New(db).GetCurrencyRecord(ctx, id)
}

// pgsqlLookupQuerier is the thin whole-struct conversion shim.
type pgsqlLookupQuerier struct{}

var _ lookupQuerier = pgsqlLookupQuerier{}

func (pgsqlLookupQuerier) GetCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return pgsqlgen.New(db).GetCurrencyIDByCode(ctx, code)
}

func (pgsqlLookupQuerier) GetCurrencyIDByCodeForUser(ctx context.Context, db backend.DBTX, code, userID string) (string, error) {
	return pgsqlgen.New(db).GetCurrencyIDByCodeForUser(ctx, pgsqlgen.GetCurrencyIDByCodeForUserParams{Code: code, UserID: &userID})
}

func (pgsqlLookupQuerier) GetCurrencyByIDView(ctx context.Context, db backend.DBTX, id string) (currencyViewRow, error) {
	row, err := pgsqlgen.New(db).GetCurrencyByIDView(ctx, id)
	return currencyViewRow(row), err
}

func (pgsqlLookupQuerier) GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error) {
	row, err := pgsqlgen.New(db).GetCurrencyRecord(ctx, id)
	return currencyRecordRow(row), err
}
