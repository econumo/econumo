// Package repo implements budget.Repository over the sqlc-generated
// queries. The exception to the usual whole-struct shim is BudgetsElementsLimit,
// whose column order diverges across engines (the sqlite table was rebuilt by a
// later migration), so its rows/params are mapped field-by-field.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	dombudget "github.com/econumo/econumo/internal/budget"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	budgetRow   = sqlitegen.Budget
	accessRow   = sqlitegen.BudgetsAccess
	folderRow   = sqlitegen.BudgetsFolder
	envelopeRow = sqlitegen.BudgetsEnvelope
	elementRow  = sqlitegen.BudgetsElement
	limitRow    = sqlitegen.BudgetsElementsLimit
	upBudgetP   = sqlitegen.UpsertBudgetParams
	upAccessP   = sqlitegen.UpsertBudgetAccessParams
	upFolderP   = sqlitegen.UpsertBudgetFolderParams
	upEnvelopeP = sqlitegen.UpsertBudgetEnvelopeParams
	upElementP  = sqlitegen.UpsertBudgetElementParams
	upLimitP    = sqlitegen.UpsertBudgetLimitParams
)

type Repo struct {
	tx *backend.TxManager
	q  querier
}

var _ dombudget.Repository = (*Repo)(nil)

// NewRepo selects the engine adapter by driver name.
func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return &Repo{tx: tx, q: sqliteQuerier{}}
	case "postgresql":
		return &Repo{tx: tx, q: pgsqlQuerier{}}
	default:
		panic("budgetrepo: unknown database driver " + driver)
	}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*dombudget.Budget, error) {
	row, err := r.q.GetBudget(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, mapNotFound(err, "Budget not found")
	}
	return hydrateBudget(row)
}

func (r *Repo) ListForUser(ctx context.Context, userID vo.Id) ([]*dombudget.Budget, error) {
	rows, err := r.q.ListBudgetsForUser(ctx, r.db(ctx), userID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*dombudget.Budget, 0, len(rows))
	for _, row := range rows {
		b, herr := hydrateBudget(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repo) Save(ctx context.Context, b *dombudget.Budget) error {
	return r.q.UpsertBudget(ctx, r.db(ctx), upBudgetP{
		ID: b.Id().String(), CurrencyID: b.CurrencyId().String(), UserID: b.UserId().String(),
		Name: b.Name(), StartedAt: b.StartedAt(), CreatedAt: b.CreatedAt(), UpdatedAt: b.UpdatedAt(),
	})
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteBudget(ctx, r.db(ctx), id.String())
}

func (r *Repo) ExcludedAccountIDs(ctx context.Context, budgetID vo.Id) ([]vo.Id, error) {
	rows, err := r.q.ListBudgetExcludedAccountIDs(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	return parseIDs(rows)
}

func (r *Repo) ExcludeAccount(ctx context.Context, budgetID, accountID vo.Id) error {
	return r.q.AddBudgetExcludedAccount(ctx, r.db(ctx), budgetID.String(), accountID.String())
}

func (r *Repo) IncludeAccount(ctx context.Context, budgetID, accountID vo.Id) error {
	return r.q.RemoveBudgetExcludedAccount(ctx, r.db(ctx), budgetID.String(), accountID.String())
}

func (r *Repo) ListAccess(ctx context.Context, budgetID vo.Id) ([]*dombudget.BudgetAccess, error) {
	rows, err := r.q.ListBudgetAccess(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*dombudget.BudgetAccess, 0, len(rows))
	for _, row := range rows {
		a, herr := hydrateAccess(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *Repo) GetAccess(ctx context.Context, budgetID, userID vo.Id) (*dombudget.BudgetAccess, error) {
	row, err := r.q.GetBudgetAccess(ctx, r.db(ctx), budgetID.String(), userID.String())
	if err != nil {
		return nil, mapNotFound(err, "BudgetAccess not found")
	}
	return hydrateAccess(row)
}

func (r *Repo) SaveAccess(ctx context.Context, a *dombudget.BudgetAccess) error {
	return r.q.UpsertBudgetAccess(ctx, r.db(ctx), upAccessP{
		BudgetID: a.BudgetId().String(), UserID: a.UserId().String(), Role: a.Role().Int16(),
		IsAccepted: a.IsAccepted(), CreatedAt: a.CreatedAt(), UpdatedAt: a.UpdatedAt(),
	})
}

func (r *Repo) DeleteAccess(ctx context.Context, budgetID, userID vo.Id) error {
	return r.q.DeleteBudgetAccess(ctx, r.db(ctx), budgetID.String(), userID.String())
}

func (r *Repo) ListFolders(ctx context.Context, budgetID vo.Id) ([]*dombudget.BudgetFolder, error) {
	rows, err := r.q.ListBudgetFolders(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*dombudget.BudgetFolder, 0, len(rows))
	for _, row := range rows {
		f, herr := hydrateFolder(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *Repo) GetFolder(ctx context.Context, id vo.Id) (*dombudget.BudgetFolder, error) {
	row, err := r.q.GetBudgetFolder(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, mapNotFound(err, "BudgetFolder not found")
	}
	return hydrateFolder(row)
}

func (r *Repo) SaveFolder(ctx context.Context, f *dombudget.BudgetFolder) error {
	return r.q.UpsertBudgetFolder(ctx, r.db(ctx), upFolderP{
		ID: f.Id().String(), BudgetID: f.BudgetId().String(), Name: f.Name(),
		Position: f.Position(), CreatedAt: f.CreatedAt(), UpdatedAt: f.UpdatedAt(),
	})
}

func (r *Repo) DeleteFolder(ctx context.Context, id vo.Id) error {
	return r.q.DeleteBudgetFolder(ctx, r.db(ctx), id.String())
}

func (r *Repo) ListEnvelopes(ctx context.Context, budgetID vo.Id) ([]*dombudget.BudgetEnvelope, error) {
	rows, err := r.q.ListBudgetEnvelopes(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*dombudget.BudgetEnvelope, 0, len(rows))
	for _, row := range rows {
		e, herr := hydrateEnvelope(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *Repo) GetEnvelope(ctx context.Context, id vo.Id) (*dombudget.BudgetEnvelope, error) {
	row, err := r.q.GetBudgetEnvelope(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, mapNotFound(err, "BudgetEnvelope not found")
	}
	return hydrateEnvelope(row)
}

func (r *Repo) SaveEnvelope(ctx context.Context, e *dombudget.BudgetEnvelope) error {
	return r.q.UpsertBudgetEnvelope(ctx, r.db(ctx), upEnvelopeP{
		ID: e.Id().String(), BudgetID: e.BudgetId().String(), Name: strPtr(e.Name()),
		Icon: strPtr(e.Icon()), IsArchived: e.IsArchived(), CreatedAt: e.CreatedAt(), UpdatedAt: e.UpdatedAt(),
	})
}

func (r *Repo) DeleteEnvelope(ctx context.Context, id vo.Id) error {
	return r.q.DeleteBudgetEnvelope(ctx, r.db(ctx), id.String())
}

func (r *Repo) EnvelopeCategoryIDs(ctx context.Context, envelopeID vo.Id) ([]vo.Id, error) {
	rows, err := r.q.ListEnvelopeCategoryIDs(ctx, r.db(ctx), envelopeID.String())
	if err != nil {
		return nil, err
	}
	return parseIDs(rows)
}

func (r *Repo) AddEnvelopeCategory(ctx context.Context, envelopeID, categoryID vo.Id) error {
	return r.q.AddEnvelopeCategory(ctx, r.db(ctx), envelopeID.String(), categoryID.String())
}

func (r *Repo) RemoveEnvelopeCategory(ctx context.Context, envelopeID, categoryID vo.Id) error {
	return r.q.RemoveEnvelopeCategory(ctx, r.db(ctx), envelopeID.String(), categoryID.String())
}

func (r *Repo) ListElements(ctx context.Context, budgetID vo.Id) ([]*dombudget.BudgetElement, error) {
	rows, err := r.q.ListBudgetElements(ctx, r.db(ctx), budgetID.String())
	if err != nil {
		return nil, err
	}
	out := make([]*dombudget.BudgetElement, 0, len(rows))
	for _, row := range rows {
		e, herr := hydrateElement(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *Repo) GetElement(ctx context.Context, id vo.Id) (*dombudget.BudgetElement, error) {
	row, err := r.q.GetBudgetElement(ctx, r.db(ctx), id.String())
	if err != nil {
		return nil, mapNotFound(err, "BudgetElement not found")
	}
	return hydrateElement(row)
}

func (r *Repo) GetElementByExternal(ctx context.Context, budgetID, externalID vo.Id) (*dombudget.BudgetElement, error) {
	row, err := r.q.GetBudgetElementByExternal(ctx, r.db(ctx), budgetID.String(), externalID.String())
	if err != nil {
		return nil, mapNotFound(err, "BudgetElement not found")
	}
	return hydrateElement(row)
}

func (r *Repo) SaveElement(ctx context.Context, e *dombudget.BudgetElement) error {
	return r.q.UpsertBudgetElement(ctx, r.db(ctx), upElementP{
		ID: e.Id().String(), BudgetID: e.BudgetId().String(), CurrencyID: idPtr(e.CurrencyId()),
		FolderID: idPtr(e.FolderId()), ExternalID: e.ExternalId().String(), Type: e.Type().Int16(),
		CreatedAt: e.CreatedAt(), UpdatedAt: e.UpdatedAt(), Position: e.Position(),
	})
}

func (r *Repo) DeleteElement(ctx context.Context, id vo.Id) error {
	return r.q.DeleteBudgetElement(ctx, r.db(ctx), id.String())
}

func (r *Repo) ListLimitsForPeriod(ctx context.Context, budgetID vo.Id, period time.Time) ([]*dombudget.BudgetElementLimit, error) {
	rows, err := r.q.ListBudgetLimitsForPeriod(ctx, r.db(ctx), budgetID.String(), period)
	if err != nil {
		return nil, err
	}
	out := make([]*dombudget.BudgetElementLimit, 0, len(rows))
	for _, row := range rows {
		l, herr := hydrateLimit(row)
		if herr != nil {
			return nil, herr
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *Repo) GetLimit(ctx context.Context, elementID vo.Id, period time.Time) (*dombudget.BudgetElementLimit, error) {
	row, err := r.q.GetBudgetLimit(ctx, r.db(ctx), elementID.String(), period)
	if err != nil {
		return nil, mapNotFound(err, "BudgetElementLimit not found")
	}
	return hydrateLimit(row)
}

func (r *Repo) SaveLimit(ctx context.Context, l *dombudget.BudgetElementLimit) error {
	return r.q.UpsertBudgetLimit(ctx, r.db(ctx), upLimitP{
		ID: l.Id().String(), ElementID: l.ElementId().String(), Period: l.Period(),
		CreatedAt: l.CreatedAt(), UpdatedAt: l.UpdatedAt(), Amount: l.Amount().String(),
	})
}

func (r *Repo) DeleteLimit(ctx context.Context, id vo.Id) error {
	return r.q.DeleteBudgetLimit(ctx, r.db(ctx), id.String())
}

func (r *Repo) DeleteLimitsByBudget(ctx context.Context, budgetID vo.Id) error {
	return r.q.DeleteBudgetLimitsByBudget(ctx, r.db(ctx), budgetID.String())
}

func hydrateBudget(row budgetRow) (*dombudget.Budget, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	currencyID, err := vo.ParseId(row.CurrencyID)
	if err != nil {
		return nil, err
	}
	return dombudget.FromState(id, userID, row.Name, currencyID, row.StartedAt, row.CreatedAt, row.UpdatedAt), nil
}

func hydrateAccess(row accessRow) (*dombudget.BudgetAccess, error) {
	budgetID, err := vo.ParseId(row.BudgetID)
	if err != nil {
		return nil, err
	}
	userID, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	// AccessFromState does not carry a separate id (PK is budget+user); pass the
	// budget id as a stand-in (the access entity's Id() is unused on the wire).
	return dombudget.AccessFromState(budgetID, budgetID, userID, dombudget.UserRole(row.Role), row.IsAccepted, row.CreatedAt, row.UpdatedAt), nil
}

func hydrateFolder(row folderRow) (*dombudget.BudgetFolder, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	budgetID, err := vo.ParseId(row.BudgetID)
	if err != nil {
		return nil, err
	}
	return dombudget.FolderFromState(id, budgetID, row.Name, row.Position, row.CreatedAt, row.UpdatedAt), nil
}

func hydrateEnvelope(row envelopeRow) (*dombudget.BudgetEnvelope, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	budgetID, err := vo.ParseId(row.BudgetID)
	if err != nil {
		return nil, err
	}
	return dombudget.EnvelopeFromState(id, budgetID, derefStr(row.Name), derefStr(row.Icon), row.IsArchived, row.CreatedAt, row.UpdatedAt), nil
}

func hydrateElement(row elementRow) (*dombudget.BudgetElement, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	budgetID, err := vo.ParseId(row.BudgetID)
	if err != nil {
		return nil, err
	}
	externalID, err := vo.ParseId(row.ExternalID)
	if err != nil {
		return nil, err
	}
	currencyID, err := parseOpt(row.CurrencyID)
	if err != nil {
		return nil, err
	}
	folderID, err := parseOpt(row.FolderID)
	if err != nil {
		return nil, err
	}
	return dombudget.ElementFromState(id, budgetID, externalID, dombudget.ElementType(row.Type), currencyID, folderID, row.Position, row.CreatedAt, row.UpdatedAt), nil
}

func hydrateLimit(row limitRow) (*dombudget.BudgetElementLimit, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	elementID, err := vo.ParseId(row.ElementID)
	if err != nil {
		return nil, err
	}
	return dombudget.LimitFromState(id, elementID, vo.NewDecimal(row.Amount), row.Period, row.CreatedAt, row.UpdatedAt), nil
}

func mapNotFound(err error, msg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.NewNotFound(msg)
	}
	return err
}

func parseIDs(ss []string) ([]vo.Id, error) {
	out := make([]vo.Id, 0, len(ss))
	for _, s := range ss {
		id, err := vo.ParseId(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func parseOpt(s *string) (*vo.Id, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func idPtr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func strPtr(s string) *string { return &s }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
