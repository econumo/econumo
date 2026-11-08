// Package repo implements debt.Repository.
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/debt"
	"github.com/econumo/econumo/internal/infra/storage/backend"
	sqlitegen "github.com/econumo/econumo/internal/infra/storage/sqlc/gen/sqlite"
	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

type (
	debtRow          = sqlitegen.Debt
	upsertParams     = sqlitegen.UpsertDebtParams
	snapshotParams   = sqlitegen.InsertDebtSnapshotParams
	splitParams      = sqlitegen.InsertDebtSplitParams
	candidateRow     = sqlitegen.CandidateDebtTransactionsRow
	reassignCatParam = sqlitegen.ReassignDebtCategoryParams
	reassignPayParam = sqlitegen.ReassignDebtPayeeParams
	reassignTagParam = sqlitegen.ReassignDebtTagParams
	reassignLblParam = sqlitegen.ReassignDebtLabelParams
	txLabelRow       = sqlitegen.TransactionsLabel
)

type querier interface {
	LockOwner(ctx context.Context, db backend.DBTX, ownerID string) error
	GetDebtByID(ctx context.Context, db backend.DBTX, id string) (debtRow, error)
	ListDebtsByCategory(ctx context.Context, db backend.DBTX, categoryID string) ([]debtRow, error)
	UpsertDebt(ctx context.Context, db backend.DBTX, p upsertParams) error
	DeleteDebt(ctx context.Context, db backend.DBTX, id string) error
	InsertDebtSnapshot(ctx context.Context, db backend.DBTX, p snapshotParams) error
	InsertDebtSplit(ctx context.Context, db backend.DBTX, p splitParams) error
	CandidateDebtTransactions(ctx context.Context, db backend.DBTX, categoryID *string) ([]candidateRow, error)
	DebtUsesCategory(ctx context.Context, db backend.DBTX, id string) (bool, error)
	DebtUsesPayee(ctx context.Context, db backend.DBTX, id *string) (bool, error)
	DebtUsesTag(ctx context.Context, db backend.DBTX, id *string) (bool, error)
	DebtUsesLabel(ctx context.Context, db backend.DBTX, id *string) (bool, error)
	ListDebtCandidateLabels(ctx context.Context, db backend.DBTX, categoryID *string) ([]txLabelRow, error)
	ReassignDebtCategory(ctx context.Context, db backend.DBTX, p reassignCatParam) error
	ReassignDebtPayee(ctx context.Context, db backend.DBTX, p reassignPayParam) error
	ReassignDebtTag(ctx context.Context, db backend.DBTX, p reassignTagParam) error
	ReassignDebtLabel(ctx context.Context, db backend.DBTX, p reassignLblParam) error
}

type Repo struct {
	tx     *backend.TxManager
	q      querier
	driver string
}

var _ debt.Repository = (*Repo)(nil)

func NewRepo(driver string, tx *backend.TxManager) *Repo {
	switch driver {
	case "sqlite":
		return NewSQLiteRepo(tx)
	case "postgresql":
		return NewPgsqlRepo(tx)
	default:
		panic("debtrepo: unknown database driver " + driver)
	}
}

func NewSQLiteRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: sqliteQuerier{}, driver: "sqlite"}
}

func NewPgsqlRepo(tx *backend.TxManager) *Repo {
	return &Repo{tx: tx, q: pgsqlQuerier{}, driver: "postgresql"}
}

func (r *Repo) db(ctx context.Context) backend.DBTX { return r.tx.Querier(ctx) }

func (r *Repo) NextIdentity() vo.Id { return vo.NewId() }

func (r *Repo) LockOwner(ctx context.Context, ownerID vo.Id) error {
	return r.q.LockOwner(ctx, r.db(ctx), ownerID.String())
}

func (r *Repo) GetByID(ctx context.Context, id vo.Id) (*model.Debt, error) {
	row, err := r.q.GetDebtByID(ctx, r.db(ctx), id.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewNotFound("Debt not found")
		}
		return nil, err
	}
	return hydrate(row)
}

func (r *Repo) ListByCategory(ctx context.Context, categoryID vo.Id) ([]*model.Debt, error) {
	rows, err := r.q.ListDebtsByCategory(ctx, r.db(ctx), categoryID.String())
	if err != nil {
		return nil, err
	}
	return hydrateAll(rows)
}

// Save: the caller runs this inside TxManager.WithTx.
func (r *Repo) Save(ctx context.Context, d *model.Debt) error {
	p := upsertParams{
		ID:         d.ID.String(),
		UserID:     d.OwnerID.String(),
		Name:       d.Name,
		CurrencyID: d.CurrencyID.String(),
		CategoryID: d.CategoryID.String(),
		PayeeID:    idPtr(d.PayeeID),
		TagID:      idPtr(d.TagID),
		LabelID:    idPtr(d.LabelID),
		Payment:    d.Payment.String(),
		Schedule:   string(d.Schedule),
		CreatedDay: dateText(d.CreatedDay),
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
	}
	if c := d.Current; c != nil {
		asOf, balance, last := dateText(c.AsOf), c.Balance.String(), dateText(c.LastPaymentDate)
		p.AsOf, p.Balance, p.LastPaymentDate = &asOf, &balance, &last
	}
	if d.StartDate != nil {
		s := dateText(*d.StartDate)
		p.StartDate = &s
	}
	return r.q.UpsertDebt(ctx, r.db(ctx), p)
}

func (r *Repo) Delete(ctx context.Context, id vo.Id) error {
	return r.q.DeleteDebt(ctx, r.db(ctx), id.String())
}

func (r *Repo) AddSnapshot(ctx context.Context, s *model.DebtSnapshot) error {
	return r.q.InsertDebtSnapshot(ctx, r.db(ctx), snapshotParams{
		ID:              s.ID.String(),
		DebtID:          s.DebtID.String(),
		AsOf:            dateText(s.AsOf),
		Balance:         s.Balance.String(),
		Payment:         s.Payment.String(),
		Schedule:        string(s.Schedule),
		LastPaymentDate: dateText(s.LastPaymentDate),
		CreatedAt:       s.CreatedAt,
	})
}

func (r *Repo) InsertSplit(ctx context.Context, s model.DebtSplit) error {
	return r.q.InsertDebtSplit(ctx, r.db(ctx), splitParams{
		DebtID:        s.DebtID.String(),
		TransactionID: s.TransactionID.String(),
		Interest:      s.Interest.String(),
	})
}

func (r *Repo) CandidateTransactions(ctx context.Context, categoryID vo.Id) ([]debt.CandidateTx, error) {
	c := categoryID.String()
	rows, err := r.q.CandidateDebtTransactions(ctx, r.db(ctx), &c)
	if err != nil {
		return nil, err
	}
	labelRows, err := r.q.ListDebtCandidateLabels(ctx, r.db(ctx), &c)
	if err != nil {
		return nil, err
	}
	labels := make(map[string][]vo.Id, len(labelRows))
	for _, l := range labelRows {
		lid, err := vo.ParseId(l.LabelID)
		if err != nil {
			return nil, err
		}
		labels[l.TransactionID] = append(labels[l.TransactionID], lid)
	}
	out := make([]debt.CandidateTx, 0, len(rows))
	for _, row := range rows {
		id, err := vo.ParseId(row.ID)
		if err != nil {
			return nil, err
		}
		cur, err := vo.ParseId(row.CurrencyID)
		if err != nil {
			return nil, err
		}
		payee, err := parseIdPtr(row.PayeeID)
		if err != nil {
			return nil, err
		}
		tag, err := parseIdPtr(row.TagID)
		if err != nil {
			return nil, err
		}
		out = append(out, debt.CandidateTx{ID: id, SpentAt: row.SpentAt, Amount: vo.NewDecimal(row.Amount),
			CurrencyID: cur, PayeeID: payee, TagID: tag, LabelIDs: labels[row.ID]})
	}
	return out, nil
}

func (r *Repo) UsesCategory(ctx context.Context, id vo.Id) (bool, error) {
	return r.q.DebtUsesCategory(ctx, r.db(ctx), id.String())
}

func (r *Repo) UsesPayee(ctx context.Context, id vo.Id) (bool, error) {
	s := id.String()
	return r.q.DebtUsesPayee(ctx, r.db(ctx), &s)
}

func (r *Repo) UsesTag(ctx context.Context, id vo.Id) (bool, error) {
	s := id.String()
	return r.q.DebtUsesTag(ctx, r.db(ctx), &s)
}

func (r *Repo) UsesLabel(ctx context.Context, id vo.Id) (bool, error) {
	s := id.String()
	return r.q.DebtUsesLabel(ctx, r.db(ctx), &s)
}

func (r *Repo) ReassignCategory(ctx context.Context, oldID, newID vo.Id) error {
	return r.q.ReassignDebtCategory(ctx, r.db(ctx), reassignCatParam{CategoryID: newID.String(), CategoryID_2: oldID.String()})
}

func (r *Repo) ReassignPayee(ctx context.Context, oldID, newID vo.Id) error {
	n, o := newID.String(), oldID.String()
	return r.q.ReassignDebtPayee(ctx, r.db(ctx), reassignPayParam{PayeeID: &n, PayeeID_2: &o})
}

func (r *Repo) ReassignTag(ctx context.Context, oldID, newID vo.Id) error {
	n, o := newID.String(), oldID.String()
	return r.q.ReassignDebtTag(ctx, r.db(ctx), reassignTagParam{TagID: &n, TagID_2: &o})
}

func (r *Repo) ReassignLabel(ctx context.Context, oldID, newID vo.Id) error {
	n, o := newID.String(), oldID.String()
	return r.q.ReassignDebtLabel(ctx, r.db(ctx), reassignLblParam{LabelID: &n, LabelID_2: &o})
}

// ListByClassification returns the debts keyed on a payee, a tag or a label.
func (r *Repo) ListByClassification(ctx context.Context, kind string, id vo.Id) ([]*model.Debt, error) {
	var column string
	switch kind {
	case "payee":
		column = "payee_id"
	case "tag":
		column = "tag_id"
	case "label":
		column = "label_id"
	default:
		return nil, fmt.Errorf("debtrepo: unknown classification kind %q", kind)
	}
	query := "SELECT " + debtCols + " FROM debts WHERE " + column + " = " + placeholders(r.driver, 1) + " ORDER BY created_at, id"
	rows, err := r.db(ctx).QueryContext(ctx, query, id.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scanned []debtRow
	for rows.Next() {
		var row debtRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.CurrencyID, &row.CategoryID, &row.PayeeID, &row.TagID, &row.LabelID,
			&row.StartDate, &row.AsOf, &row.Balance, &row.Payment, &row.Schedule, &row.LastPaymentDate,
			&row.CreatedDay, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hydrateAll(scanned)
}

const debtCols = "id, user_id, name, currency_id, category_id, payee_id, tag_id, label_id, start_date, as_of, balance, payment, schedule, last_payment_date, created_day, created_at, updated_at"

// The id-list reads below are hand-built (variadic IN); only the placeholders
// differ per engine.
func (r *Repo) ListByOwners(ctx context.Context, ownerIDs []vo.Id) ([]*model.Debt, error) {
	if len(ownerIDs) == 0 {
		return nil, nil
	}
	query := "SELECT " + debtCols + " FROM debts WHERE user_id IN (" + placeholders(r.driver, len(ownerIDs)) + ") ORDER BY created_at, id"
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(ownerIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scanned []debtRow
	for rows.Next() {
		var row debtRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.CurrencyID, &row.CategoryID, &row.PayeeID, &row.TagID, &row.LabelID,
			&row.StartDate, &row.AsOf, &row.Balance, &row.Payment, &row.Schedule, &row.LastPaymentDate,
			&row.CreatedDay, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hydrateAll(scanned)
}

func (r *Repo) Snapshots(ctx context.Context, debtIDs []vo.Id) ([]*model.DebtSnapshot, error) {
	if len(debtIDs) == 0 {
		return nil, nil
	}
	query := "SELECT id, debt_id, as_of, balance, payment, schedule, last_payment_date, created_at FROM debts_snapshots WHERE debt_id IN (" +
		placeholders(r.driver, len(debtIDs)) + ") ORDER BY debt_id, as_of, created_at, id"
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(debtIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.DebtSnapshot
	for rows.Next() {
		var id, debtID, balance, payment, schedule string
		var asOf, last, createdAt time.Time
		if err := rows.Scan(&id, &debtID, &asOf, &balance, &payment, &schedule, &last, &createdAt); err != nil {
			return nil, err
		}
		sid, err := vo.ParseId(id)
		if err != nil {
			return nil, err
		}
		did, err := vo.ParseId(debtID)
		if err != nil {
			return nil, err
		}
		out = append(out, &model.DebtSnapshot{ID: sid, DebtID: did, AsOf: day(asOf), Balance: vo.NewDecimal(balance),
			Payment: vo.NewDecimal(payment), Schedule: model.RecurringSchedule(schedule), LastPaymentDate: day(last), CreatedAt: createdAt})
	}
	return out, rows.Err()
}

func (r *Repo) Splits(ctx context.Context, debtIDs []vo.Id) ([]model.DebtSplit, error) {
	if len(debtIDs) == 0 {
		return nil, nil
	}
	query := "SELECT debt_id, transaction_id, interest FROM debts_payments WHERE debt_id IN (" +
		placeholders(r.driver, len(debtIDs)) + ") ORDER BY debt_id, transaction_id"
	rows, err := r.db(ctx).QueryContext(ctx, query, idArgs(debtIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DebtSplit
	for rows.Next() {
		var debtID, txID, interest string
		if err := rows.Scan(&debtID, &txID, &interest); err != nil {
			return nil, err
		}
		did, err := vo.ParseId(debtID)
		if err != nil {
			return nil, err
		}
		tid, err := vo.ParseId(txID)
		if err != nil {
			return nil, err
		}
		out = append(out, model.DebtSplit{DebtID: did, TransactionID: tid, Interest: vo.NewDecimal(interest)})
	}
	return out, rows.Err()
}

func (r *Repo) DeleteSplits(ctx context.Context, debtID vo.Id, transactionIDs []vo.Id) error {
	if len(transactionIDs) == 0 {
		return nil
	}
	query := "DELETE FROM debts_payments WHERE debt_id = " + placeholder(r.driver, 1) +
		" AND transaction_id IN (" + placeholdersFrom(r.driver, 2, len(transactionIDs)) + ")"
	args := append([]any{debtID.String()}, idArgs(transactionIDs)...)
	_, err := r.db(ctx).ExecContext(ctx, query, args...)
	return err
}

func hydrateAll(rows []debtRow) ([]*model.Debt, error) {
	out := make([]*model.Debt, 0, len(rows))
	for _, row := range rows {
		d, err := hydrate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func hydrate(row debtRow) (*model.Debt, error) {
	id, err := vo.ParseId(row.ID)
	if err != nil {
		return nil, err
	}
	owner, err := vo.ParseId(row.UserID)
	if err != nil {
		return nil, err
	}
	currency, err := vo.ParseId(row.CurrencyID)
	if err != nil {
		return nil, err
	}
	category, err := vo.ParseId(row.CategoryID)
	if err != nil {
		return nil, err
	}
	payee, err := parseIdPtr(row.PayeeID)
	if err != nil {
		return nil, err
	}
	tag, err := parseIdPtr(row.TagID)
	if err != nil {
		return nil, err
	}
	label, err := parseIdPtr(row.LabelID)
	if err != nil {
		return nil, err
	}
	var start *time.Time
	if row.StartDate != nil {
		s := day(*row.StartDate)
		start = &s
	}
	d := &model.Debt{
		ID: id, OwnerID: owner, CurrencyID: currency, CategoryID: category, Name: row.Name,
		PayeeID: payee, TagID: tag, LabelID: label, StartDate: start,
		Payment: vo.NewDecimal(row.Payment), Schedule: model.RecurringSchedule(row.Schedule),
		CreatedDay: day(row.CreatedDay), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.AsOf != nil && row.Balance != nil && row.LastPaymentDate != nil {
		d.Current = &model.DebtTerms{
			AsOf:            day(*row.AsOf),
			Balance:         vo.NewDecimal(*row.Balance),
			Payment:         d.Payment,
			Schedule:        d.Schedule,
			LastPaymentDate: day(*row.LastPaymentDate),
		}
	}
	return d, nil
}

func dateText(t time.Time) string { return t.Format(datetime.DateLayout) }

func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func idPtr(id *vo.Id) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func parseIdPtr(s *string) (*vo.Id, error) {
	if s == nil {
		return nil, nil
	}
	id, err := vo.ParseId(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func idArgs(ids []vo.Id) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id.String()
	}
	return args
}

func placeholder(driver string, pos int) string {
	if driver == "postgresql" {
		return "$" + strconv.Itoa(pos)
	}
	return "?"
}

func placeholders(driver string, n int) string { return placeholdersFrom(driver, 1, n) }

func placeholdersFrom(driver string, start, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = placeholder(driver, start+i)
	}
	return strings.Join(parts, ",")
}
