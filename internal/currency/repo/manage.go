// Write side for per-user custom currencies (create/update/delete/hide
// a currency the caller owns). Kept apart from write.go (the CLI admin write
// path over GLOBAL currencies only) even though both hit the same table: the
// two write paths have disjoint callers (HTTP API vs CLI) and disjoint
// invariants (ownership vs admin-only inserts).
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	appcurrency "github.com/econumo/econumo/internal/currency"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
)

// Canonical manage row/param types (the sqlite-generated ones).
type (
	currencyRecordRow      = sqlitegen.GetCurrencyRecordRow
	insertUserCurrencyP    = sqlitegen.InsertUserCurrencyParams
	updateCurrencyDetailsP = sqlitegen.UpdateCurrencyDetailsParams
	hideP                  = sqlitegen.InsertHiddenCurrencyParams
	unhideP                = sqlitegen.DeleteHiddenCurrencyParams
	hideAllP               = sqlitegen.HideGlobalCurrenciesParams
)

// manageQuerier is the engine-agnostic manage surface, in the canonical types.
// CountCurrencyUsage and OwnerCurrencyCodeExists take plain args rather than
// the generated param structs: sqlc's sqlite and pgsql codegen diverge on
// their shapes for these two queries (the sqlite CountCurrencyUsageParams
// repeats the currency id, once oddly as *string; the pgsql one collapses
// the repeated $1 into a single arg and returns int32 not int64), so each
// adapter builds its own params and normalizes its return type internally.
type manageQuerier interface {
	GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error)
	GlobalCurrencyCodeExists(ctx context.Context, db backend.DBTX, code string) (int64, error)
	OwnerCurrencyCodeExists(ctx context.Context, db backend.DBTX, code, userID string) (int64, error)
	InsertUserCurrency(ctx context.Context, db backend.DBTX, p insertUserCurrencyP) error
	UpdateCurrencyDetails(ctx context.Context, db backend.DBTX, p updateCurrencyDetailsP) error
	SoftDeleteCurrency(ctx context.Context, db backend.DBTX, id string) error
	CountCurrencyUsage(ctx context.Context, db backend.DBTX, id string) (int64, error)
	InsertHiddenCurrency(ctx context.Context, db backend.DBTX, p hideP) error
	DeleteHiddenCurrency(ctx context.Context, db backend.DBTX, p unhideP) error
	HideGlobalCurrencies(ctx context.Context, db backend.DBTX, p hideAllP) error
	ShowGlobalCurrencies(ctx context.Context, db backend.DBTX, userID string) error
}

// ManageRepo implements the currency feature's write-side port for per-user
// custom currencies.
type ManageRepo struct {
	tx *backend.TxManager
	q  manageQuerier
}

var _ appcurrency.ManageModel = (*ManageRepo)(nil)

// NewManageRepo selects the engine adapter by driver name. driver matches
// config.DatabaseDriver: "sqlite" | "postgresql".
func NewManageRepo(driver string, tx *backend.TxManager) *ManageRepo {
	switch driver {
	case "sqlite":
		return &ManageRepo{tx: tx, q: sqliteManageQuerier{}}
	case "postgresql":
		return &ManageRepo{tx: tx, q: pgsqlManageQuerier{}}
	default:
		panic("currencyrepo: unknown database driver " + driver)
	}
}

func (r *ManageRepo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *ManageRepo) GetCurrencyRecord(ctx context.Context, id string) (model.CurrencyRecord, error) {
	row, err := r.q.GetCurrencyRecord(ctx, r.db(ctx), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.CurrencyRecord{}, errs.NewNotFound("Currency not found")
		}
		return model.CurrencyRecord{}, err
	}
	return model.CurrencyRecord{
		ID:             row.ID,
		Code:           row.Code,
		Symbol:         row.Symbol,
		Name:           row.Name,
		FractionDigits: int(row.FractionDigits),
		UserID:         row.UserID,
		Rate:           row.Rate,
		CreatedAt:      row.CreatedAt,
		IsDeleted:      row.IsDeleted,
	}, nil
}

func (r *ManageRepo) GlobalCodeExists(ctx context.Context, code string) (bool, error) {
	n, err := r.q.GlobalCurrencyCodeExists(ctx, r.db(ctx), code)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *ManageRepo) OwnerCodeExists(ctx context.Context, userID, code string) (bool, error) {
	n, err := r.q.OwnerCurrencyCodeExists(ctx, r.db(ctx), code, userID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *ManageRepo) InsertUserCurrency(ctx context.Context, c model.CurrencyRecord) error {
	return r.q.InsertUserCurrency(ctx, r.db(ctx), insertUserCurrencyP{
		ID:             c.ID,
		Code:           c.Code,
		Symbol:         c.Symbol,
		Name:           c.Name,
		FractionDigits: int16(c.FractionDigits),
		UserID:         c.UserID,
		Rate:           c.Rate,
		CreatedAt:      c.CreatedAt,
	})
}

func (r *ManageRepo) UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int, rate *string) error {
	return r.q.UpdateCurrencyDetails(ctx, r.db(ctx), updateCurrencyDetailsP{
		Name:           &name,
		Symbol:         symbol,
		FractionDigits: int16(fractionDigits),
		Rate:           rate,
		ID:             id,
	})
}

func (r *ManageRepo) SoftDeleteCurrency(ctx context.Context, id string) error {
	return r.q.SoftDeleteCurrency(ctx, r.db(ctx), id)
}

func (r *ManageRepo) CountCurrencyUsage(ctx context.Context, id string) (int64, error) {
	return r.q.CountCurrencyUsage(ctx, r.db(ctx), id)
}

// HideCurrency marks a global currency hidden for a user. Idempotent: a
// repeat call ON CONFLICTs into a no-op.
func (r *ManageRepo) HideCurrency(ctx context.Context, userID, currencyID string, now time.Time) error {
	return r.q.InsertHiddenCurrency(ctx, r.db(ctx), hideP{
		UserID:     userID,
		CurrencyID: currencyID,
		CreatedAt:  now,
	})
}

// ShowCurrency clears a hidden-currency mark. Idempotent: deleting an absent
// row affects zero rows without erroring.
// HideGlobalCurrencies hides every global for userID except the two exclusion
// slots (base + profile default; callers pad with the base id when the profile
// default IS the base).
func (r *ManageRepo) HideGlobalCurrencies(ctx context.Context, userID string, excludeIDs []string, now time.Time) error {
	first := ""
	second := ""
	if len(excludeIDs) > 0 {
		first = excludeIDs[0]
		second = excludeIDs[0]
	}
	if len(excludeIDs) > 1 {
		second = excludeIDs[1]
	}
	return r.q.HideGlobalCurrencies(ctx, r.db(ctx), hideAllP{
		UserID:    userID,
		CreatedAt: now,
		ID:        first,
		ID_2:      second,
	})
}

func (r *ManageRepo) ShowGlobalCurrencies(ctx context.Context, userID string) error {
	return r.q.ShowGlobalCurrencies(ctx, r.db(ctx), userID)
}

func (r *ManageRepo) ShowCurrency(ctx context.Context, userID, currencyID string) error {
	return r.q.DeleteHiddenCurrency(ctx, r.db(ctx), unhideP{
		UserID:     userID,
		CurrencyID: currencyID,
	})
}

// sqliteManageQuerier is the native passthrough (canonical types ARE sqlite's).
type sqliteManageQuerier struct{}

var _ manageQuerier = sqliteManageQuerier{}

func (sqliteManageQuerier) GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error) {
	return sqlitegen.New(db).GetCurrencyRecord(ctx, id)
}

func (sqliteManageQuerier) GlobalCurrencyCodeExists(ctx context.Context, db backend.DBTX, code string) (int64, error) {
	return sqlitegen.New(db).GlobalCurrencyCodeExists(ctx, code)
}

func (sqliteManageQuerier) OwnerCurrencyCodeExists(ctx context.Context, db backend.DBTX, code, userID string) (int64, error) {
	return sqlitegen.New(db).OwnerCurrencyCodeExists(ctx, sqlitegen.OwnerCurrencyCodeExistsParams{
		Code:   code,
		UserID: &userID,
	})
}

func (sqliteManageQuerier) InsertUserCurrency(ctx context.Context, db backend.DBTX, p insertUserCurrencyP) error {
	return sqlitegen.New(db).InsertUserCurrency(ctx, p)
}

func (sqliteManageQuerier) UpdateCurrencyDetails(ctx context.Context, db backend.DBTX, p updateCurrencyDetailsP) error {
	return sqlitegen.New(db).UpdateCurrencyDetails(ctx, p)
}

func (sqliteManageQuerier) SoftDeleteCurrency(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).SoftDeleteCurrency(ctx, id)
}

func (sqliteManageQuerier) CountCurrencyUsage(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	return sqlitegen.New(db).CountCurrencyUsage(ctx, sqlitegen.CountCurrencyUsageParams{
		CurrencyID:   id,
		CurrencyID_2: id,
		CurrencyID_3: &id,
		Value:        &id,
	})
}

func (sqliteManageQuerier) InsertHiddenCurrency(ctx context.Context, db backend.DBTX, p hideP) error {
	return sqlitegen.New(db).InsertHiddenCurrency(ctx, p)
}

func (sqliteManageQuerier) DeleteHiddenCurrency(ctx context.Context, db backend.DBTX, p unhideP) error {
	return sqlitegen.New(db).DeleteHiddenCurrency(ctx, p)
}

func (sqliteManageQuerier) HideGlobalCurrencies(ctx context.Context, db backend.DBTX, p hideAllP) error {
	return sqlitegen.New(db).HideGlobalCurrencies(ctx, p)
}

func (sqliteManageQuerier) ShowGlobalCurrencies(ctx context.Context, db backend.DBTX, userID string) error {
	return sqlitegen.New(db).ShowGlobalCurrencies(ctx, userID)
}

// pgsqlManageQuerier is the thin conversion shim: whole-struct casts where the
// generated types are field-identical, field-copy where they diverge.
type pgsqlManageQuerier struct{}

var _ manageQuerier = pgsqlManageQuerier{}

func (pgsqlManageQuerier) GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error) {
	row, err := pgsqlgen.New(db).GetCurrencyRecord(ctx, id)
	return currencyRecordRow(row), err
}

func (pgsqlManageQuerier) GlobalCurrencyCodeExists(ctx context.Context, db backend.DBTX, code string) (int64, error) {
	return pgsqlgen.New(db).GlobalCurrencyCodeExists(ctx, code)
}

func (pgsqlManageQuerier) OwnerCurrencyCodeExists(ctx context.Context, db backend.DBTX, code, userID string) (int64, error) {
	return pgsqlgen.New(db).OwnerCurrencyCodeExists(ctx, pgsqlgen.OwnerCurrencyCodeExistsParams{
		Code:   code,
		UserID: &userID,
	})
}

func (pgsqlManageQuerier) InsertUserCurrency(ctx context.Context, db backend.DBTX, p insertUserCurrencyP) error {
	return pgsqlgen.New(db).InsertUserCurrency(ctx, pgsqlgen.InsertUserCurrencyParams(p))
}

func (pgsqlManageQuerier) UpdateCurrencyDetails(ctx context.Context, db backend.DBTX, p updateCurrencyDetailsP) error {
	return pgsqlgen.New(db).UpdateCurrencyDetails(ctx, pgsqlgen.UpdateCurrencyDetailsParams(p))
}

func (pgsqlManageQuerier) SoftDeleteCurrency(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).SoftDeleteCurrency(ctx, id)
}

func (pgsqlManageQuerier) CountCurrencyUsage(ctx context.Context, db backend.DBTX, id string) (int64, error) {
	n, err := pgsqlgen.New(db).CountCurrencyUsage(ctx, id)
	return int64(n), err
}

func (pgsqlManageQuerier) InsertHiddenCurrency(ctx context.Context, db backend.DBTX, p hideP) error {
	return pgsqlgen.New(db).InsertHiddenCurrency(ctx, pgsqlgen.InsertHiddenCurrencyParams(p))
}

func (pgsqlManageQuerier) DeleteHiddenCurrency(ctx context.Context, db backend.DBTX, p unhideP) error {
	return pgsqlgen.New(db).DeleteHiddenCurrency(ctx, pgsqlgen.DeleteHiddenCurrencyParams(p))
}

func (pgsqlManageQuerier) HideGlobalCurrencies(ctx context.Context, db backend.DBTX, p hideAllP) error {
	return pgsqlgen.New(db).HideGlobalCurrencies(ctx, pgsqlgen.HideGlobalCurrenciesParams(p))
}

func (pgsqlManageQuerier) ShowGlobalCurrencies(ctx context.Context, db backend.DBTX, userID string) error {
	return pgsqlgen.New(db).ShowGlobalCurrencies(ctx, userID)
}
