package budgetrepo

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	pgsqlgen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/pgsql"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
)

// querier is the engine-agnostic budget query surface, in the canonical
// (sqlite) types. The pgsql adapter converts whole structs where the generated
// models are field-identical, and field-by-field for the limit row/params
// (whose column order diverges across engines).
type querier interface {
	GetBudget(ctx context.Context, db backend.DBTX, id string) (budgetRow, error)
	ListBudgetsForUser(ctx context.Context, db backend.DBTX, userID string) ([]budgetRow, error)
	UpsertBudget(ctx context.Context, db backend.DBTX, p upBudgetP) error
	DeleteBudget(ctx context.Context, db backend.DBTX, id string) error

	ListBudgetExcludedAccountIDs(ctx context.Context, db backend.DBTX, budgetID string) ([]string, error)
	AddBudgetExcludedAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error
	RemoveBudgetExcludedAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error

	ListBudgetAccess(ctx context.Context, db backend.DBTX, budgetID string) ([]accessRow, error)
	GetBudgetAccess(ctx context.Context, db backend.DBTX, budgetID, userID string) (accessRow, error)
	UpsertBudgetAccess(ctx context.Context, db backend.DBTX, p upAccessP) error
	DeleteBudgetAccess(ctx context.Context, db backend.DBTX, budgetID, userID string) error

	ListBudgetFolders(ctx context.Context, db backend.DBTX, budgetID string) ([]folderRow, error)
	GetBudgetFolder(ctx context.Context, db backend.DBTX, id string) (folderRow, error)
	UpsertBudgetFolder(ctx context.Context, db backend.DBTX, p upFolderP) error
	DeleteBudgetFolder(ctx context.Context, db backend.DBTX, id string) error

	ListBudgetEnvelopes(ctx context.Context, db backend.DBTX, budgetID string) ([]envelopeRow, error)
	GetBudgetEnvelope(ctx context.Context, db backend.DBTX, id string) (envelopeRow, error)
	UpsertBudgetEnvelope(ctx context.Context, db backend.DBTX, p upEnvelopeP) error
	DeleteBudgetEnvelope(ctx context.Context, db backend.DBTX, id string) error
	ListEnvelopeCategoryIDs(ctx context.Context, db backend.DBTX, envelopeID string) ([]string, error)
	AddEnvelopeCategory(ctx context.Context, db backend.DBTX, envelopeID, categoryID string) error
	RemoveEnvelopeCategory(ctx context.Context, db backend.DBTX, envelopeID, categoryID string) error

	ListBudgetElements(ctx context.Context, db backend.DBTX, budgetID string) ([]elementRow, error)
	GetBudgetElement(ctx context.Context, db backend.DBTX, id string) (elementRow, error)
	GetBudgetElementByExternal(ctx context.Context, db backend.DBTX, budgetID, externalID string) (elementRow, error)
	UpsertBudgetElement(ctx context.Context, db backend.DBTX, p upElementP) error
	DeleteBudgetElement(ctx context.Context, db backend.DBTX, id string) error

	ListBudgetLimitsForPeriod(ctx context.Context, db backend.DBTX, budgetID string, period time.Time) ([]limitRow, error)
	GetBudgetLimit(ctx context.Context, db backend.DBTX, elementID string, period time.Time) (limitRow, error)
	UpsertBudgetLimit(ctx context.Context, db backend.DBTX, p upLimitP) error
	DeleteBudgetLimit(ctx context.Context, db backend.DBTX, id string) error
	DeleteBudgetLimitsByBudget(ctx context.Context, db backend.DBTX, budgetID string) error
}

type sqliteQuerier struct{}

func (sqliteQuerier) GetBudget(ctx context.Context, db backend.DBTX, id string) (budgetRow, error) {
	return sqlitegen.New(db).GetBudgetByID(ctx, id)
}
func (sqliteQuerier) ListBudgetsForUser(ctx context.Context, db backend.DBTX, userID string) ([]budgetRow, error) {
	return sqlitegen.New(db).ListBudgetsForUser(ctx, sqlitegen.ListBudgetsForUserParams{UserID: userID, UserID_2: userID})
}
func (sqliteQuerier) UpsertBudget(ctx context.Context, db backend.DBTX, p upBudgetP) error {
	return sqlitegen.New(db).UpsertBudget(ctx, p)
}
func (sqliteQuerier) DeleteBudget(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteBudget(ctx, id)
}
func (sqliteQuerier) ListBudgetExcludedAccountIDs(ctx context.Context, db backend.DBTX, budgetID string) ([]string, error) {
	return sqlitegen.New(db).ListBudgetExcludedAccountIDs(ctx, budgetID)
}
func (sqliteQuerier) AddBudgetExcludedAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error {
	return sqlitegen.New(db).AddBudgetExcludedAccount(ctx, sqlitegen.AddBudgetExcludedAccountParams{BudgetID: budgetID, AccountID: accountID})
}
func (sqliteQuerier) RemoveBudgetExcludedAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error {
	return sqlitegen.New(db).RemoveBudgetExcludedAccount(ctx, sqlitegen.RemoveBudgetExcludedAccountParams{BudgetID: budgetID, AccountID: accountID})
}
func (sqliteQuerier) ListBudgetAccess(ctx context.Context, db backend.DBTX, budgetID string) ([]accessRow, error) {
	return sqlitegen.New(db).ListBudgetAccess(ctx, budgetID)
}
func (sqliteQuerier) GetBudgetAccess(ctx context.Context, db backend.DBTX, budgetID, userID string) (accessRow, error) {
	return sqlitegen.New(db).GetBudgetAccess(ctx, sqlitegen.GetBudgetAccessParams{BudgetID: budgetID, UserID: userID})
}
func (sqliteQuerier) UpsertBudgetAccess(ctx context.Context, db backend.DBTX, p upAccessP) error {
	return sqlitegen.New(db).UpsertBudgetAccess(ctx, p)
}
func (sqliteQuerier) DeleteBudgetAccess(ctx context.Context, db backend.DBTX, budgetID, userID string) error {
	return sqlitegen.New(db).DeleteBudgetAccess(ctx, sqlitegen.DeleteBudgetAccessParams{BudgetID: budgetID, UserID: userID})
}
func (sqliteQuerier) ListBudgetFolders(ctx context.Context, db backend.DBTX, budgetID string) ([]folderRow, error) {
	return sqlitegen.New(db).ListBudgetFolders(ctx, budgetID)
}
func (sqliteQuerier) GetBudgetFolder(ctx context.Context, db backend.DBTX, id string) (folderRow, error) {
	return sqlitegen.New(db).GetBudgetFolder(ctx, id)
}
func (sqliteQuerier) UpsertBudgetFolder(ctx context.Context, db backend.DBTX, p upFolderP) error {
	return sqlitegen.New(db).UpsertBudgetFolder(ctx, p)
}
func (sqliteQuerier) DeleteBudgetFolder(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteBudgetFolder(ctx, id)
}
func (sqliteQuerier) ListBudgetEnvelopes(ctx context.Context, db backend.DBTX, budgetID string) ([]envelopeRow, error) {
	return sqlitegen.New(db).ListBudgetEnvelopes(ctx, budgetID)
}
func (sqliteQuerier) GetBudgetEnvelope(ctx context.Context, db backend.DBTX, id string) (envelopeRow, error) {
	return sqlitegen.New(db).GetBudgetEnvelope(ctx, id)
}
func (sqliteQuerier) UpsertBudgetEnvelope(ctx context.Context, db backend.DBTX, p upEnvelopeP) error {
	return sqlitegen.New(db).UpsertBudgetEnvelope(ctx, p)
}
func (sqliteQuerier) DeleteBudgetEnvelope(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteBudgetEnvelope(ctx, id)
}
func (sqliteQuerier) ListEnvelopeCategoryIDs(ctx context.Context, db backend.DBTX, envelopeID string) ([]string, error) {
	return sqlitegen.New(db).ListEnvelopeCategoryIDs(ctx, envelopeID)
}
func (sqliteQuerier) AddEnvelopeCategory(ctx context.Context, db backend.DBTX, envelopeID, categoryID string) error {
	return sqlitegen.New(db).AddEnvelopeCategory(ctx, sqlitegen.AddEnvelopeCategoryParams{BudgetEnvelopeID: envelopeID, CategoryID: categoryID})
}
func (sqliteQuerier) RemoveEnvelopeCategory(ctx context.Context, db backend.DBTX, envelopeID, categoryID string) error {
	return sqlitegen.New(db).RemoveEnvelopeCategory(ctx, sqlitegen.RemoveEnvelopeCategoryParams{BudgetEnvelopeID: envelopeID, CategoryID: categoryID})
}
func (sqliteQuerier) ListBudgetElements(ctx context.Context, db backend.DBTX, budgetID string) ([]elementRow, error) {
	return sqlitegen.New(db).ListBudgetElements(ctx, budgetID)
}
func (sqliteQuerier) GetBudgetElement(ctx context.Context, db backend.DBTX, id string) (elementRow, error) {
	return sqlitegen.New(db).GetBudgetElement(ctx, id)
}
func (sqliteQuerier) GetBudgetElementByExternal(ctx context.Context, db backend.DBTX, budgetID, externalID string) (elementRow, error) {
	return sqlitegen.New(db).GetBudgetElementByExternal(ctx, sqlitegen.GetBudgetElementByExternalParams{BudgetID: budgetID, ExternalID: externalID})
}
func (sqliteQuerier) UpsertBudgetElement(ctx context.Context, db backend.DBTX, p upElementP) error {
	return sqlitegen.New(db).UpsertBudgetElement(ctx, p)
}
func (sqliteQuerier) DeleteBudgetElement(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteBudgetElement(ctx, id)
}

// limitPeriodArg renders a limit period as the 'Y-m-d H:i:s' string the
// datetime() comparison normalizes against (SQLite only).
func limitPeriodArg(period time.Time) string { return period.Format(datetime.Layout) }

func (sqliteQuerier) ListBudgetLimitsForPeriod(ctx context.Context, db backend.DBTX, budgetID string, period time.Time) ([]limitRow, error) {
	// The query is datetime(l.period) = datetime(?); bind the period as a
	// 'Y-m-d H:i:s' string so it normalizes equal regardless of the stored form
	// (a bound time.Time does not compare equal to the stored datetime text).
	return sqlitegen.New(db).ListBudgetLimitsForPeriod(ctx, sqlitegen.ListBudgetLimitsForPeriodParams{BudgetID: budgetID, Datetime: limitPeriodArg(period)})
}
func (sqliteQuerier) GetBudgetLimit(ctx context.Context, db backend.DBTX, elementID string, period time.Time) (limitRow, error) {
	return sqlitegen.New(db).GetBudgetLimit(ctx, sqlitegen.GetBudgetLimitParams{ElementID: elementID, Datetime: limitPeriodArg(period)})
}
func (sqliteQuerier) UpsertBudgetLimit(ctx context.Context, db backend.DBTX, p upLimitP) error {
	// Store `period` as a 'Y-m-d H:i:s' string, NOT a time.Time: the modernc
	// driver serializes time.Time as RFC3339 ("...T...Z"), which SQLite's
	// datetime() cannot parse — so the read side's `datetime(period)=datetime(?)`
	// (see ListBudgetLimitsForPeriod / GetBudgetLimit) would never match and a
	// set limit would silently read back as budgeted=0. Bind period the same way
	// the read side does (limitPeriodArg). Done via raw exec because the
	// generated param type is time.Time. created_at/updated_at are never compared
	// with datetime(), so their RFC3339 form is harmless.
	_, err := db.ExecContext(ctx,
		`INSERT INTO budgets_elements_limits (id, element_id, period, created_at, updated_at, amount)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET amount = excluded.amount, updated_at = excluded.updated_at`,
		p.ID, p.ElementID, p.Period.Format(datetime.Layout), p.CreatedAt, p.UpdatedAt, p.Amount)
	return err
}
func (sqliteQuerier) DeleteBudgetLimit(ctx context.Context, db backend.DBTX, id string) error {
	return sqlitegen.New(db).DeleteBudgetLimit(ctx, id)
}
func (sqliteQuerier) DeleteBudgetLimitsByBudget(ctx context.Context, db backend.DBTX, budgetID string) error {
	return sqlitegen.New(db).DeleteBudgetLimitsByBudget(ctx, budgetID)
}

// pgsqlQuerier is the whole-struct shim (field-by-field for the limit row/params).
type pgsqlQuerier struct{}

func (pgsqlQuerier) GetBudget(ctx context.Context, db backend.DBTX, id string) (budgetRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetByID(ctx, id)
	return budgetRow(v), err
}
func (pgsqlQuerier) ListBudgetsForUser(ctx context.Context, db backend.DBTX, userID string) ([]budgetRow, error) {
	rows, err := pgsqlgen.New(db).ListBudgetsForUser(ctx, pgsqlgen.ListBudgetsForUserParams{UserID: userID, UserID_2: userID})
	if err != nil {
		return nil, err
	}
	out := make([]budgetRow, len(rows))
	for i, v := range rows {
		out[i] = budgetRow(v)
	}
	return out, nil
}
func (pgsqlQuerier) UpsertBudget(ctx context.Context, db backend.DBTX, p upBudgetP) error {
	return pgsqlgen.New(db).UpsertBudget(ctx, pgsqlgen.UpsertBudgetParams(p))
}
func (pgsqlQuerier) DeleteBudget(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteBudget(ctx, id)
}
func (pgsqlQuerier) ListBudgetExcludedAccountIDs(ctx context.Context, db backend.DBTX, budgetID string) ([]string, error) {
	return pgsqlgen.New(db).ListBudgetExcludedAccountIDs(ctx, budgetID)
}
func (pgsqlQuerier) AddBudgetExcludedAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error {
	return pgsqlgen.New(db).AddBudgetExcludedAccount(ctx, pgsqlgen.AddBudgetExcludedAccountParams{BudgetID: budgetID, AccountID: accountID})
}
func (pgsqlQuerier) RemoveBudgetExcludedAccount(ctx context.Context, db backend.DBTX, budgetID, accountID string) error {
	return pgsqlgen.New(db).RemoveBudgetExcludedAccount(ctx, pgsqlgen.RemoveBudgetExcludedAccountParams{BudgetID: budgetID, AccountID: accountID})
}
func (pgsqlQuerier) ListBudgetAccess(ctx context.Context, db backend.DBTX, budgetID string) ([]accessRow, error) {
	rows, err := pgsqlgen.New(db).ListBudgetAccess(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := make([]accessRow, len(rows))
	for i, v := range rows {
		out[i] = accessRow(v)
	}
	return out, nil
}
func (pgsqlQuerier) GetBudgetAccess(ctx context.Context, db backend.DBTX, budgetID, userID string) (accessRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetAccess(ctx, pgsqlgen.GetBudgetAccessParams{BudgetID: budgetID, UserID: userID})
	return accessRow(v), err
}
func (pgsqlQuerier) UpsertBudgetAccess(ctx context.Context, db backend.DBTX, p upAccessP) error {
	return pgsqlgen.New(db).UpsertBudgetAccess(ctx, pgsqlgen.UpsertBudgetAccessParams(p))
}
func (pgsqlQuerier) DeleteBudgetAccess(ctx context.Context, db backend.DBTX, budgetID, userID string) error {
	return pgsqlgen.New(db).DeleteBudgetAccess(ctx, pgsqlgen.DeleteBudgetAccessParams{BudgetID: budgetID, UserID: userID})
}
func (pgsqlQuerier) ListBudgetFolders(ctx context.Context, db backend.DBTX, budgetID string) ([]folderRow, error) {
	rows, err := pgsqlgen.New(db).ListBudgetFolders(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := make([]folderRow, len(rows))
	for i, v := range rows {
		out[i] = folderRow(v)
	}
	return out, nil
}
func (pgsqlQuerier) GetBudgetFolder(ctx context.Context, db backend.DBTX, id string) (folderRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetFolder(ctx, id)
	return folderRow(v), err
}
func (pgsqlQuerier) UpsertBudgetFolder(ctx context.Context, db backend.DBTX, p upFolderP) error {
	return pgsqlgen.New(db).UpsertBudgetFolder(ctx, pgsqlgen.UpsertBudgetFolderParams(p))
}
func (pgsqlQuerier) DeleteBudgetFolder(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteBudgetFolder(ctx, id)
}
func (pgsqlQuerier) ListBudgetEnvelopes(ctx context.Context, db backend.DBTX, budgetID string) ([]envelopeRow, error) {
	rows, err := pgsqlgen.New(db).ListBudgetEnvelopes(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := make([]envelopeRow, len(rows))
	for i, v := range rows {
		out[i] = envelopeRow(v)
	}
	return out, nil
}
func (pgsqlQuerier) GetBudgetEnvelope(ctx context.Context, db backend.DBTX, id string) (envelopeRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetEnvelope(ctx, id)
	return envelopeRow(v), err
}
func (pgsqlQuerier) UpsertBudgetEnvelope(ctx context.Context, db backend.DBTX, p upEnvelopeP) error {
	return pgsqlgen.New(db).UpsertBudgetEnvelope(ctx, pgsqlgen.UpsertBudgetEnvelopeParams(p))
}
func (pgsqlQuerier) DeleteBudgetEnvelope(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteBudgetEnvelope(ctx, id)
}
func (pgsqlQuerier) ListEnvelopeCategoryIDs(ctx context.Context, db backend.DBTX, envelopeID string) ([]string, error) {
	return pgsqlgen.New(db).ListEnvelopeCategoryIDs(ctx, envelopeID)
}
func (pgsqlQuerier) AddEnvelopeCategory(ctx context.Context, db backend.DBTX, envelopeID, categoryID string) error {
	return pgsqlgen.New(db).AddEnvelopeCategory(ctx, pgsqlgen.AddEnvelopeCategoryParams{BudgetEnvelopeID: envelopeID, CategoryID: categoryID})
}
func (pgsqlQuerier) RemoveEnvelopeCategory(ctx context.Context, db backend.DBTX, envelopeID, categoryID string) error {
	return pgsqlgen.New(db).RemoveEnvelopeCategory(ctx, pgsqlgen.RemoveEnvelopeCategoryParams{BudgetEnvelopeID: envelopeID, CategoryID: categoryID})
}
func (pgsqlQuerier) ListBudgetElements(ctx context.Context, db backend.DBTX, budgetID string) ([]elementRow, error) {
	rows, err := pgsqlgen.New(db).ListBudgetElements(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	out := make([]elementRow, len(rows))
	for i, v := range rows {
		out[i] = elementRow(v)
	}
	return out, nil
}
func (pgsqlQuerier) GetBudgetElement(ctx context.Context, db backend.DBTX, id string) (elementRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetElement(ctx, id)
	return elementRow(v), err
}
func (pgsqlQuerier) GetBudgetElementByExternal(ctx context.Context, db backend.DBTX, budgetID, externalID string) (elementRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetElementByExternal(ctx, pgsqlgen.GetBudgetElementByExternalParams{BudgetID: budgetID, ExternalID: externalID})
	return elementRow(v), err
}
func (pgsqlQuerier) UpsertBudgetElement(ctx context.Context, db backend.DBTX, p upElementP) error {
	return pgsqlgen.New(db).UpsertBudgetElement(ctx, pgsqlgen.UpsertBudgetElementParams(p))
}
func (pgsqlQuerier) DeleteBudgetElement(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteBudgetElement(ctx, id)
}
func (pgsqlQuerier) ListBudgetLimitsForPeriod(ctx context.Context, db backend.DBTX, budgetID string, period time.Time) ([]limitRow, error) {
	rows, err := pgsqlgen.New(db).ListBudgetLimitsForPeriod(ctx, pgsqlgen.ListBudgetLimitsForPeriodParams{BudgetID: budgetID, Period: period})
	if err != nil {
		return nil, err
	}
	out := make([]limitRow, len(rows))
	for i, v := range rows {
		out[i] = limitRow{ID: v.ID, ElementID: v.ElementID, Period: v.Period, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, Amount: v.Amount}
	}
	return out, nil
}
func (pgsqlQuerier) GetBudgetLimit(ctx context.Context, db backend.DBTX, elementID string, period time.Time) (limitRow, error) {
	v, err := pgsqlgen.New(db).GetBudgetLimit(ctx, pgsqlgen.GetBudgetLimitParams{ElementID: elementID, Period: period})
	if err != nil {
		return limitRow{}, err
	}
	return limitRow{ID: v.ID, ElementID: v.ElementID, Period: v.Period, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, Amount: v.Amount}, nil
}
func (pgsqlQuerier) UpsertBudgetLimit(ctx context.Context, db backend.DBTX, p upLimitP) error {
	return pgsqlgen.New(db).UpsertBudgetLimit(ctx, pgsqlgen.UpsertBudgetLimitParams{
		ID: p.ID, ElementID: p.ElementID, Period: p.Period, Amount: p.Amount, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	})
}
func (pgsqlQuerier) DeleteBudgetLimit(ctx context.Context, db backend.DBTX, id string) error {
	return pgsqlgen.New(db).DeleteBudgetLimit(ctx, id)
}
func (pgsqlQuerier) DeleteBudgetLimitsByBudget(ctx context.Context, db backend.DBTX, budgetID string) error {
	return pgsqlgen.New(db).DeleteBudgetLimitsByBudget(ctx, budgetID)
}
