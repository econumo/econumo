// Write side for per-user custom currencies (create/update/archive/delete/hide
// a currency the caller owns). Kept apart from write.go (the CLI admin write
// path over GLOBAL currencies only) even though both hit the same table: the
// two write paths have disjoint callers (HTTP API vs CLI) and disjoint
// invariants (ownership + archival vs admin-only inserts).
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
	setCurrencyArchivedP   = sqlitegen.SetCurrencyArchivedParams
	hideP                  = sqlitegen.InsertHiddenCurrencyParams
	unhideP                = sqlitegen.DeleteHiddenCurrencyParams
	manageRateP            = sqlitegen.UpsertCurrencyRateParams
)

// manageQuerier is the engine-agnostic manage surface, in the canonical types.
// CountCurrencyUsage and OwnerCurrencyCodeExists take plain args rather than
// the generated param structs: sqlc's sqlite and pgsql codegen diverge on
// their shapes for these two queries (the sqlite CountCurrencyUsageParams
// repeats the currency id three times, once oddly as *string; the pgsql one
// collapses the repeated $1 into a single field, and returns int32 not
// int64), so each adapter builds its own param struct and normalizes its
// return type internally.
type manageQuerier interface {
	GetCurrencyRecord(ctx context.Context, db backend.DBTX, id string) (currencyRecordRow, error)
	GlobalCurrencyCodeExists(ctx context.Context, db backend.DBTX, code string) (int64, error)
	OwnerCurrencyCodeExists(ctx context.Context, db backend.DBTX, code, userID string) (int64, error)
	InsertUserCurrency(ctx context.Context, db backend.DBTX, p insertUserCurrencyP) error
	UpdateCurrencyDetails(ctx context.Context, db backend.DBTX, p updateCurrencyDetailsP) error
	SetCurrencyArchived(ctx context.Context, db backend.DBTX, p setCurrencyArchivedP) error
	DeleteCurrency(ctx context.Context, db backend.DBTX, id string) error
	GetGlobalCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error)
	CountCurrencyUsage(ctx context.Context, db backend.DBTX, id, code string) (int64, error)
	UpsertCurrencyRate(ctx context.Context, db backend.DBTX, p manageRateP) error
	InsertHiddenCurrency(ctx context.Context, db backend.DBTX, p hideP) error
	DeleteHiddenCurrency(ctx context.Context, db backend.DBTX, p unhideP) error
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
		IsArchived:     row.IsArchived,
		CreatedAt:      row.CreatedAt,
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
		IsArchived:     c.IsArchived,
		CreatedAt:      c.CreatedAt,
	})
}

func (r *ManageRepo) UpdateCurrencyDetails(ctx context.Context, id, name, symbol string, fractionDigits int) error {
	return r.q.UpdateCurrencyDetails(ctx, r.db(ctx), updateCurrencyDetailsP{
		Name:           &name,
		Symbol:         symbol,
		FractionDigits: int16(fractionDigits),
		ID:             id,
	})
}

func (r *ManageRepo) SetCurrencyArchived(ctx context.Context, id string, archived bool) error {
	return r.q.SetCurrencyArchived(ctx, r.db(ctx), setCurrencyArchivedP{
		IsArchived: archived,
		ID:         id,
	})
}

func (r *ManageRepo) DeleteCurrency(ctx context.Context, id string) error {
	return r.q.DeleteCurrency(ctx, r.db(ctx), id)
}

func (r *ManageRepo) CountCurrencyUsage(ctx context.Context, id, code string) (int64, error) {
	return r.q.CountCurrencyUsage(ctx, r.db(ctx), id, code)
}

func (r *ManageRepo) GetGlobalIDByCode(ctx context.Context, code string) (string, error) {
	id, err := r.q.GetGlobalCurrencyIDByCode(ctx, r.db(ctx), code)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.NewNotFound("Currency not found")
		}
		return "", err
	}
	return id, nil
}

// UpsertRate mirrors WriteRepo.UpsertRate: the published date is truncated to
// midnight UTC so the per-day ON CONFLICT upsert dedupes correctly.
func (r *ManageRepo) UpsertRate(ctx context.Context, rr model.RateRow) error {
	day := time.Date(rr.Date.Year(), rr.Date.Month(), rr.Date.Day(), 0, 0, 0, 0, time.UTC)
	return r.q.UpsertCurrencyRate(ctx, r.db(ctx), manageRateP{
		ID:             rr.ID,
		CurrencyID:     rr.CurrencyID,
		BaseCurrencyID: rr.BaseCurrencyID,
		PublishedAt:    day,
		Rate:           rr.Rate,
	})
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

func (sqliteManageQuerier) SetCurrencyArchived(ctx context.Context, db backend.DBTX, p setCurrencyArchivedP) error {
	return sqlitegen.New(db).SetCurrencyArchived(ctx, p)
}

func (sqliteManageQuerier) DeleteCurrency(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteCurrency(ctx, id)
}

func (sqliteManageQuerier) GetGlobalCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return sqlitegen.New(db).GetGlobalCurrencyIDByCode(ctx, code)
}

func (sqliteManageQuerier) CountCurrencyUsage(ctx context.Context, db backend.DBTX, id, code string) (int64, error) {
	return sqlitegen.New(db).CountCurrencyUsage(ctx, sqlitegen.CountCurrencyUsageParams{
		CurrencyID:   id,
		CurrencyID_2: id,
		CurrencyID_3: &id,
		Value:        &code,
	})
}

func (sqliteManageQuerier) UpsertCurrencyRate(ctx context.Context, db backend.DBTX, p manageRateP) error {
	return sqlitegen.New(db).UpsertCurrencyRate(ctx, p)
}

func (sqliteManageQuerier) InsertHiddenCurrency(ctx context.Context, db backend.DBTX, p hideP) error {
	return sqlitegen.New(db).InsertHiddenCurrency(ctx, p)
}

func (sqliteManageQuerier) DeleteHiddenCurrency(ctx context.Context, db backend.DBTX, p unhideP) error {
	return sqlitegen.New(db).DeleteHiddenCurrency(ctx, p)
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

func (pgsqlManageQuerier) SetCurrencyArchived(ctx context.Context, db backend.DBTX, p setCurrencyArchivedP) error {
	return pgsqlgen.New(db).SetCurrencyArchived(ctx, pgsqlgen.SetCurrencyArchivedParams(p))
}

func (pgsqlManageQuerier) DeleteCurrency(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteCurrency(ctx, id)
}

func (pgsqlManageQuerier) GetGlobalCurrencyIDByCode(ctx context.Context, db backend.DBTX, code string) (string, error) {
	return pgsqlgen.New(db).GetGlobalCurrencyIDByCode(ctx, code)
}

func (pgsqlManageQuerier) CountCurrencyUsage(ctx context.Context, db backend.DBTX, id, code string) (int64, error) {
	n, err := pgsqlgen.New(db).CountCurrencyUsage(ctx, pgsqlgen.CountCurrencyUsageParams{
		CurrencyID: id,
		Value:      &code,
	})
	return int64(n), err
}

func (pgsqlManageQuerier) UpsertCurrencyRate(ctx context.Context, db backend.DBTX, p manageRateP) error {
	return pgsqlgen.New(db).UpsertCurrencyRate(ctx, pgsqlgen.UpsertCurrencyRateParams(p))
}

func (pgsqlManageQuerier) InsertHiddenCurrency(ctx context.Context, db backend.DBTX, p hideP) error {
	return pgsqlgen.New(db).InsertHiddenCurrency(ctx, pgsqlgen.InsertHiddenCurrencyParams(p))
}

func (pgsqlManageQuerier) DeleteHiddenCurrency(ctx context.Context, db backend.DBTX, p unhideP) error {
	return pgsqlgen.New(db).DeleteHiddenCurrency(ctx, pgsqlgen.DeleteHiddenCurrencyParams(p))
}
